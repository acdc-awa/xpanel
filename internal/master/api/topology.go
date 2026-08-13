package api

import (
	"log"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/master/nodegate"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/protocol"
	"github.com/zhx/xray-panel/internal/pkg/util"
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
		log.Printf("topology: 内部账户变更后推送配置失败 (server=%d): %v", inb.ServerID, err)
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

// ---- 权限组 CRUD + 入站集合 ----

// AdminPermissionGroups GET /api/v1/admin/permission-groups
func (d *Deps) AdminPermissionGroups(c *gin.Context) {
	var list []models.PermissionGroup
	if err := d.DB.Order("id ASC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	util.OK(c, gin.H{"items": list})
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
		Name   *string `json:"name"`
		Remark *string `json:"remark"`
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
	if len(updates) > 0 {
		if err := d.DB.Model(&g).Updates(updates).Error; err != nil {
			util.BadRequest(c, "更新失败（名称可能重复）")
			return
		}
	}
	util.OK(c, gin.H{"group": g})
}

// AdminDeletePermissionGroup DELETE /api/v1/admin/permission-groups/:id
// （级联删除入站集合；绑定套餐的组拒绝删除）
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
	err = d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("permission_group_id = ?", id).Delete(&models.PermissionGroupInbound{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.PermissionGroup{}, id).Error
	})
	if err != nil {
		util.ServerError(c, "删除失败")
		return
	}
	util.OK(c, gin.H{"ok": true})
}

// AdminGroupInbounds GET /api/v1/admin/permission-groups/:id/inbounds —— 组内入站 ID 集合。
func (d *Deps) AdminGroupInbounds(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var links []models.PermissionGroupInbound
	if err := d.DB.Where("permission_group_id = ?", id).Find(&links).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	ids := make([]uint64, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.InboundID)
	}
	util.OK(c, gin.H{"inbound_ids": ids})
}

// AdminSetGroupInbounds POST /api/v1/admin/permission-groups/:id/inbounds
// body: {"inbound_ids": [1,2]} —— 全量替换（仅允许 type=user 入站）。
func (d *Deps) AdminSetGroupInbounds(c *gin.Context) {
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
		InboundIDs []uint64 `json:"inbound_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
		return
	}
	// 校验只包含 type=user 入站（relay/idle 不进用户授权体系）
	if len(req.InboundIDs) > 0 {
		var cnt int64
		d.DB.Model(&models.Inbound{}).
			Where("id IN ? AND type != ?", req.InboundIDs, models.InboundTypeUser).Count(&cnt)
		if cnt > 0 {
			util.BadRequest(c, "权限组只能包含 type=user 入站")
			return
		}
	}
	err = d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("permission_group_id = ?", id).Delete(&models.PermissionGroupInbound{}).Error; err != nil {
			return err
		}
		if len(req.InboundIDs) == 0 {
			return nil
		}
		rows := make([]models.PermissionGroupInbound, 0, len(req.InboundIDs))
		for _, iid := range req.InboundIDs {
			rows = append(rows, models.PermissionGroupInbound{PermissionGroupID: id, InboundID: iid})
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		util.ServerError(c, "保存失败")
		return
	}
	util.OK(c, gin.H{"group_id": id, "count": len(req.InboundIDs)})
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
	ID         uint64 `json:"id"`
	ServerID   uint64 `json:"server_id"`
	InboundTag string `json:"inbound_tag"`
	OutboundTag string `json:"outbound_tag"`
	Enabled    bool   `json:"enabled"`
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
		if d.Hub != nil && d.Hub.IsOnline(v.ID) {
			v.Status = 1
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

	util.OK(c, gin.H{
		"servers":       srvViews,
		"inbounds":      inbViews,
		"outbounds":     outViews,
		"routing_rules": ruleViews,
	})
}
