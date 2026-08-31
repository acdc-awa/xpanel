package api

import (
	"encoding/json"
	"net"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/nodegate"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/master/subscribe"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// ---- 入站内部账户指令（relay UUID 节点自治） ----

// adminInternalAccount 执行 setup/rotate 指令：Ask 节点 → 回执写 internal_uuid → 重新生成配置。
func (d *Deps) adminInternalAccount(c *gin.Context, typ string) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var inb models.Inbound
	if err := d.DB.First(&inb, id).Error; err != nil {
		util.Fail(c, 404, "入站不存在")
		return
	}
	if inb.Type != models.InboundTypeRelay {
		util.BadRequest(c, "仅 relay 入站支持内部账户指令")
		return
	}
	if d.Hub == nil {
		util.ServerError(c, "节点网关未初始化")
		return
	}
	res, err := d.Hub.Ask(inb.ServerID, typ,
		protocol.SetupInternalAccountPayload{Tag: inb.Tag}, nodegate.AskTimeout)
	if err != nil {
		util.BadRequest(c, "指令失败（节点离线或超时）："+err.Error())
		return
	}
	if !res.OK {
		util.BadRequest(c, "节点处理失败: "+res.Error)
		return
	}
	var out protocol.SetupInternalResult
	if data, ok := res.Data.(map[string]any); ok {
		out.Tag, _ = data["tag"].(string)
		out.UUID, _ = data["uuid"].(string)
	} else {
		util.ServerError(c, "回执格式异常")
		return
	}
	if out.UUID == "" {
		util.ServerError(c, "回执缺少 UUID")
		return
	}
	if err := d.DB.Model(&inb).Update("internal_uuid", out.UUID).Error; err != nil {
		util.ServerError(c, "保存内部 UUID 失败")
		return
	}
	if err := d.enqueueConfig(inb.ServerID); err != nil {
		pushFail(c, inb.ServerID, err)
		return
	}
	util.OK(c, gin.H{"inbound_id": inb.ID, "internal_uuid": out.UUID})
}

// AdminSetupInternal POST /api/v1/admin/inbounds/:id/setup-internal —— 节点生成并上报内部 UUID（幂等）。
func (d *Deps) AdminSetupInternal(c *gin.Context) {
	d.adminInternalAccount(c, protocol.MsgSetupInternalAccount)
}

// AdminRotateInternal POST /api/v1/admin/inbounds/:id/rotate-internal —— 强制重新生成内部 UUID。
func (d *Deps) AdminRotateInternal(c *gin.Context) {
	d.adminInternalAccount(c, protocol.MsgRotateInternalAccount)
}

type permissionGroupView struct {
	models.PermissionGroup
	AccessPointCount int      `json:"access_point_count"`
	AccessPointNames []string `json:"access_point_names"`
}

// AdminPermissionGroups GET /api/v1/admin/permission-groups
func (d *Deps) AdminPermissionGroups(c *gin.Context) {
	var list []models.PermissionGroup
	if err := d.DB.Order("id ASC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}

	var apLinks []models.PermissionGroupAccessPoint
	_ = d.DB.Find(&apLinks)
	groupAPMap := make(map[uint64][]uint64)
	allAPIDs := make([]uint64, 0, len(apLinks))
	for _, l := range apLinks {
		groupAPMap[l.PermissionGroupID] = append(groupAPMap[l.PermissionGroupID], l.AccessPointID)
		allAPIDs = append(allAPIDs, l.AccessPointID)
	}
	var aps []models.UserAccessPoint
	if len(allAPIDs) > 0 {
		_ = d.DB.Select("id, name").Where("id IN ?", allAPIDs).Find(&aps)
	}
	apNameMap := make(map[uint64]string, len(aps))
	for _, ap := range aps {
		apNameMap[ap.ID] = ap.Name
	}

	items := make([]permissionGroupView, 0, len(list))
	for _, g := range list {
		apNames := make([]string, 0)
		for _, apID := range groupAPMap[g.ID] {
			if name, ok := apNameMap[apID]; ok && name != "" {
				apNames = append(apNames, name)
			}
		}
		items = append(items, permissionGroupView{
			PermissionGroup:  g,
			AccessPointCount: len(groupAPMap[g.ID]),
			AccessPointNames: apNames,
		})
	}
	util.OK(c, gin.H{"items": items})
}

// AdminCreatePermissionGroup POST /api/v1/admin/permission-groups
func (d *Deps) AdminCreatePermissionGroup(c *gin.Context) {
	var req struct {
		Name   string `json:"name" binding:"required,max=64"`
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	g := models.PermissionGroup{Name: req.Name, Remark: req.Remark}
	if err := d.DB.Create(&g).Error; err != nil {
		util.BadRequest(c, "创建失败（名称可能重复）")
		return
	}
	d.TriggerUserChange()
	util.OK(c, gin.H{"group": g})
}

// AdminUpdatePermissionGroup PUT /api/v1/admin/permission-groups/:id
func (d *Deps) AdminUpdatePermissionGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var g models.PermissionGroup
	if err := d.DB.First(&g, id).Error; err != nil {
		util.Fail(c, 404, "权限组不存在")
		return
	}
	var req struct {
		Name          *string `json:"name"`
		Remark        *string `json:"remark"`
		ClashTemplate *string `json:"clash_template"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}
	if req.ClashTemplate != nil {
		updates["clash_template"] = *req.ClashTemplate
	}
	if len(updates) > 0 {
		if err := d.DB.Model(&g).Updates(updates).Error; err != nil {
			util.BadRequest(c, "更新失败（名称可能重复）")
			return
		}
	}
	util.OK(c, gin.H{"group": g})
}

// AdminPreviewPermissionGroupTemplate POST /api/v1/admin/permission-groups/:id/preview-template
// 实时编译预览某个权限组的订阅模板渲染效果。
func (d *Deps) AdminPreviewPermissionGroupTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var g models.PermissionGroup
	if err := d.DB.First(&g, id).Error; err != nil {
		util.Fail(c, 404, "权限组不存在")
		return
	}
	var req struct {
		Template string `json:"template"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Template = g.ClashTemplate
	}

	// AP 单点授权：查询该权限组可见的启用接入点，沿管道解析生成节点（与订阅同规则）
	dtos := d.previewAccessPointNodes(id, "00000000-0000-0000-0000-000000000001")

	// 若该组暂无节点，尝试获取任意可用接入点作为样例展示
	isSampleNodes := false
	if len(dtos) == 0 {
		dtos = d.previewAccessPointNodes(0, "00000000-0000-0000-0000-000000000001")
		isSampleNodes = len(dtos) > 0
	}

	panelHost := c.Request.Host
	if h, _, err := net.SplitHostPort(panelHost); err == nil {
		panelHost = h
	}
	rendered := subscribe.BuildClashWithTemplate(dtos, req.Template, panelHost)
	_, names := subscribe.FormatNodesYAML(dtos)

	util.OK(c, gin.H{
		"rendered":        rendered,
		"proxy_count":     len(names),
		"proxy_names":     names,
		"is_sample_nodes": isSampleNodes,
	})
}

// AdminDeletePermissionGroup DELETE /api/v1/admin/permission-groups/:id
// （级联删除入站集合；绑定套餐/显式绑定用户的组拒绝删除）
func (d *Deps) AdminDeletePermissionGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var cnt int64
	d.DB.Model(&models.Plan{}).Where("permission_group_id = ?", id).Count(&cnt)
	if cnt > 0 {
		util.BadRequest(c, "该权限组正被套餐绑定，请先解绑")
		return
	}
	// U18：用户显式绑定检查——删除会让用户静默失去全部节点授权
	var userCnt int64
	d.DB.Model(&models.User{}).Where("permission_group_id = ?", id).Count(&userCnt)
	if userCnt > 0 {
		util.BadRequest(c, "该权限组正被用户绑定，请先在用户管理中解除")
		return
	}
	err = d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("permission_group_id = ?", id).Delete(&models.PermissionGroupAccessPoint{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.PermissionGroup{}, id).Error
	})
	if err != nil {
		util.ServerError(c, "删除失败")
		return
	}
	d.TriggerUserChange()
	util.OK(c, gin.H{"ok": true})
}

// ---- 拓扑画布（T8）：一次拉全量 ----

// topoOutbound 画布出站轻量视图。
type topoOutbound struct {
	ID         uint64  `json:"id"`
	ServerID   uint64  `json:"server_id"`
	Tag        string  `json:"tag"`
	Protocol   string  `json:"protocol"`
	InboundRef *uint64 `json:"inbound_ref"` // Phase T：引用落地入站
	Enabled    bool    `json:"enabled"`
	Priority   int     `json:"priority"`
}

// topoRule 画布路由规则轻量视图。
type topoRule struct {
	ID          uint64 `json:"id"`
	ServerID    uint64 `json:"server_id"`
	InboundTag  string `json:"inbound_tag"`
	OutboundTag string `json:"outbound_tag"`
	Enabled     bool   `json:"enabled"`
}

// AdminTopology GET /api/v1/admin/topology —— 可视化画布数据源（服务器盒子 + 入站/出站项 + 引用/规则线）。
func (d *Deps) AdminTopology(c *gin.Context) {
	// 服务器（实时在线状态）
	var servers []models.Server
	if err := d.DB.Order("id ASC").Find(&servers).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	srvViews := make([]serverView, 0, len(servers))
	srvName := map[uint64]string{}
	for i := range servers {
		v := toServerView(&servers[i])
		// 在线状态以网关注册表为准双向覆盖（同 AdminServers：防 DB 残留 status=1 假在线）
		if d.Hub != nil {
			v.Status = 0
			if d.Hub.IsOnline(v.ID) {
				v.Status = 1
			}
		}
		srvViews = append(srvViews, v)
		srvName[servers[i].ID] = servers[i].Name
	}

	// 入站（含服务器名）
	var inbounds []models.Inbound
	if err := d.DB.Order("server_id ASC, id ASC").Find(&inbounds).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	inbViews := make([]inboundView, 0, len(inbounds))
	for i := range inbounds {
		inbViews = append(inbViews, toInboundView(&inbounds[i], srvName[inbounds[i].ServerID]))
	}

	// 出站（轻量）
	var outbounds []models.ServerOutbound
	if err := d.DB.Order("server_id ASC, id ASC").Find(&outbounds).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	outViews := make([]topoOutbound, 0, len(outbounds))
	for i := range outbounds {
		outViews = append(outViews, topoOutbound{
			ID: outbounds[i].ID, ServerID: outbounds[i].ServerID,
			Tag: outbounds[i].Tag, Protocol: outbounds[i].Protocol,
			InboundRef: outbounds[i].InboundRef, Enabled: outbounds[i].Enabled,
			Priority: outbounds[i].Priority,
		})
	}

	// 路由规则（轻量）
	var rules []models.ServerRoutingRule
	if err := d.DB.Order("server_id ASC, id ASC").Find(&rules).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	ruleViews := make([]topoRule, 0, len(rules))
	for i := range rules {
		ruleViews = append(ruleViews, topoRule{
			ID: rules[i].ID, ServerID: rules[i].ServerID,
			InboundTag: rules[i].InboundTag, OutboundTag: rules[i].OutboundTag,
			Enabled: rules[i].Enabled,
		})
	}

	srvMap, inbMap := d.fetchTopologyContext()

	// 用户接入点（面向客户端与订阅的入口端点 + 开放权限组）
	var accessPoints []models.UserAccessPoint
	if err := d.DB.Order("id ASC").Find(&accessPoints).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	apIDs := make([]uint64, 0, len(accessPoints))
	for i := range accessPoints {
		apIDs = append(apIDs, accessPoints[i].ID)
	}
	apGroupMap := services.BatchAccessPointPermissionGroupIDs(d.DB, apIDs)
	apViews := make([]AccessPointView, 0, len(accessPoints))

	for i := range accessPoints {
		ap := accessPoints[i]
		apViews = append(apViews, buildAccessPointView(ap, apGroupMap[ap.ID], srvMap, inbMap))
	}

	// 对外接入层（显式端点分组：layers + 各层挂载入站数）
	var layers []models.AccessLayer
	if err := d.DB.Order("server_id ASC, id ASC").Find(&layers).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	layerInbCount := make(map[uint64]int, len(layers))
	for i := range inbounds {
		if inbounds[i].LayerID != nil {
			layerInbCount[*inbounds[i].LayerID]++
		}
	}
	layerViews := make([]accessLayerView, 0, len(layers))
	for i := range layers {
		layerViews = append(layerViews, toAccessLayerView(layers[i], layerInbCount[layers[i].ID]))
	}

	util.OK(c, gin.H{
		"servers":       srvViews,
		"inbounds":      inbViews,
		"access_points": apViews,
		"outbounds":     outViews,
		"routing_rules": ruleViews,
		"layers":        layerViews,
	})
}

// ---- 画布布局云端同步（盒子位置/宽度 + 内容哈希去重，settings 表存 JSON） ----

const settingTopologyLayout = "topology_layout"

// topoLayout 云端布局载荷（hash = 客户端对内容算的内容哈希，用于跨浏览器版本去重）
type topoLayout struct {
	Hash      string `json:"hash"`
	Positions map[string]struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"positions"`
	Widths map[string]float64 `json:"widths"`
}

// AdminGetTopologyLayout GET /api/v1/admin/topology-layout —— 拉取画布布局（跨浏览器/设备统一）
func (d *Deps) AdminGetTopologyLayout(c *gin.Context) {
	resp := gin.H{"hash": "", "positions": gin.H{}, "widths": gin.H{}}
	var set models.Setting
	if err := d.DB.Where("key = ?", settingTopologyLayout).First(&set).Error; err == nil && set.Value != "" {
		var raw topoLayout
		if err := json.Unmarshal([]byte(set.Value), &raw); err == nil {
			resp["hash"] = raw.Hash
			if raw.Positions != nil {
				pos := gin.H{}
				for k, v := range raw.Positions {
					pos[k] = gin.H{"x": v.X, "y": v.Y}
				}
				resp["positions"] = pos
			}
			if raw.Widths != nil {
				w := gin.H{}
				for k, v := range raw.Widths {
					w[k] = v
				}
				resp["widths"] = w
			}
		}
	}
	util.OK(c, resp)
}

// AdminSaveTopologyLayout PUT /api/v1/admin/topology-layout —— 保存画布布局（upsert settings，原样透传 hash）
func (d *Deps) AdminSaveTopologyLayout(c *gin.Context) {
	var req topoLayout
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	val, err := json.Marshal(req)
	if err != nil {
		util.BadRequest(c, "序列化失败")
		return
	}
	var set models.Setting
	if err := d.DB.Where("key = ?", settingTopologyLayout).First(&set).Error; err != nil {
		if err := d.DB.Create(&models.Setting{Key: settingTopologyLayout, Value: string(val)}).Error; err != nil {
			util.ServerError(c, "保存失败")
			return
		}
	} else {
		if err := d.DB.Model(&set).Update("value", string(val)).Error; err != nil {
			util.ServerError(c, "保存失败")
			return
		}
	}
	util.OK(c, nil)
}

// previewAccessPointNodes 按权限组可见的启用接入点生成预览节点（groupID = 0 时取全部启用接入点作样例）。
// 管道解析与订阅同源：直连入站继承节点地址/端口（挂层走接入层决议）；AP CustomHost/Port 覆写优先。
func (d *Deps) previewAccessPointNodes(groupID uint64, mockUUID string) []contracts.ProxyNodeDTO {
	var aps []models.UserAccessPoint
	_ = d.DB.Where("enabled = ?", true).Order("id ASC").Find(&aps).Error
	if len(aps) == 0 {
		return nil
	}
	apIDs := make([]uint64, 0, len(aps))
	for _, ap := range aps {
		apIDs = append(apIDs, ap.ID)
	}
	apGroupMap := services.BatchAccessPointPermissionGroupIDs(d.DB, apIDs)

	var servers []models.Server
	_ = d.DB.Find(&servers).Error
	srvMap := make(map[uint64]models.Server, len(servers))
	for _, s := range servers {
		srvMap[s.ID] = s
	}
	var inbs []models.Inbound
	_ = d.DB.Where("enabled = ? AND type = ?", true, models.InboundTypeUser).Find(&inbs).Error
	inbs = services.FilterAvailableInbounds(inbs)
	inbMap := make(map[uint64]models.Inbound, len(inbs))
	for _, inb := range inbs {
		inbMap[inb.ID] = inb
	}
	var layers []models.AccessLayer
	_ = d.DB.Find(&layers).Error
	layerMap := make(map[uint64]models.AccessLayer, len(layers))
	for _, l := range layers {
		layerMap[l.ID] = l
	}

	dtos := make([]contracts.ProxyNodeDTO, 0, len(aps))
	for _, ap := range aps {
		if groupID > 0 {
			matched := false
			for _, gid := range apGroupMap[ap.ID] {
				if gid == groupID {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		dto := subscribe.ResolveAPSubscription(&ap, srvMap, inbMap, layerMap, mockUUID)
		if dto == nil {
			continue
		}
		dtos = append(dtos, *dto)
	}
	return dtos
}
