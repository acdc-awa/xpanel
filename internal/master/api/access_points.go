package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/master/services"
	"github.com/acdc/xray-panel/internal/models"
	"github.com/acdc/xray-panel/internal/pkg/util"
)

// accessPointForm 用户接入点请求体（消费者模型：核心为 Name + 权限组，Host/Port 可选覆写）
type accessPointForm struct {
	Name               string   `json:"name" binding:"required,max=128"`
	CustomHost         string   `json:"custom_host" binding:"max=255"`
	CustomPort         int      `json:"custom_port"`
	TargetType         string   `json:"target_type"` // "inbound" | "l4_rule" | ""
	TargetInboundID    *uint64  `json:"target_inbound_id"`
	TargetL4RuleID     *uint64  `json:"target_l4_rule_id"`
	Enabled            *bool    `json:"enabled"`
	Remark             string   `json:"remark" binding:"max=255"`
	PermissionGroupIDs []uint64 `json:"permission_group_ids"`
}

// AccessPointView 用户接入点对外结构（带动态计算的已消费端点数据）
type AccessPointView struct {
	models.UserAccessPoint
	PermissionGroupIDs []uint64 `json:"permission_group_ids"`
	TargetServerName   string   `json:"target_server_name,omitempty"`
	TargetInboundTag   string   `json:"target_inbound_tag,omitempty"`
	TargetL4Port       int      `json:"target_l4_port,omitempty"`
	ResolvedHost       string   `json:"resolved_host,omitempty"`
	ResolvedPort       int      `json:"resolved_port,omitempty"`
	ResolvedProtocol   string   `json:"resolved_protocol,omitempty"`
	ResolvedTargetDesc string   `json:"resolved_target_desc,omitempty"`
}

func buildAccessPointView(
	ap models.UserAccessPoint,
	gids []uint64,
	srvMap map[uint64]models.Server,
	inbMap map[uint64]models.Inbound,
	l4Map map[uint64]models.L4PortRule,
) AccessPointView {
	if gids == nil {
		gids = []uint64{}
	}
	v := AccessPointView{
		UserAccessPoint:    ap,
		PermissionGroupIDs: gids,
	}

	if ap.TargetType == "inbound" && ap.TargetInboundID != nil && *ap.TargetInboundID > 0 {
		if inb, ok := inbMap[*ap.TargetInboundID]; ok {
			v.TargetInboundTag = inb.Tag
			srv := srvMap[inb.ServerID]
			v.TargetServerName = srv.Name
			v.ResolvedProtocol = inb.Protocol
			if ap.CustomHost != "" {
				v.ResolvedHost = ap.CustomHost
			} else {
				v.ResolvedHost = srv.Host
			}
			if ap.CustomPort > 0 {
				v.ResolvedPort = ap.CustomPort
			} else {
				v.ResolvedPort = inb.Port
			}
			v.ResolvedTargetDesc = fmt.Sprintf("%s · %s", srv.Name, inb.Tag)
		}
	} else if ap.TargetType == "l4_rule" && ap.TargetL4RuleID != nil && *ap.TargetL4RuleID > 0 {
		if rule, ok := l4Map[*ap.TargetL4RuleID]; ok {
			v.TargetL4Port = rule.ListenPort
			l4Srv := srvMap[rule.ServerID]
			v.TargetServerName = l4Srv.Name
			if ap.CustomHost != "" {
				v.ResolvedHost = ap.CustomHost
			} else {
				v.ResolvedHost = l4Srv.Host
			}
			if ap.CustomPort > 0 {
				v.ResolvedPort = ap.CustomPort
			} else {
				v.ResolvedPort = rule.ListenPort
			}
			if inb, ok := inbMap[rule.TargetInboundID]; ok {
				v.TargetInboundTag = inb.Tag
				v.ResolvedProtocol = inb.Protocol
				targetSrv := srvMap[inb.ServerID]
				v.ResolvedTargetDesc = fmt.Sprintf("%s :%d ➜ %s · %s", l4Srv.Name, rule.ListenPort, targetSrv.Name, inb.Tag)
			} else {
				v.ResolvedTargetDesc = fmt.Sprintf("%s :%d", l4Srv.Name, rule.ListenPort)
			}
		}
	}

	return v
}

func (d *Deps) fetchTopologyContext() (map[uint64]models.Server, map[uint64]models.Inbound, map[uint64]models.L4PortRule) {
	var servers []models.Server
	_ = d.DB.Find(&servers).Error
	srvMap := make(map[uint64]models.Server)
	for _, s := range servers {
		srvMap[s.ID] = s
	}
	var inbounds []models.Inbound
	_ = d.DB.Find(&inbounds).Error
	inbMap := make(map[uint64]models.Inbound)
	for _, inb := range inbounds {
		inbMap[inb.ID] = inb
	}
	var l4Rules []models.L4PortRule
	_ = d.DB.Find(&l4Rules).Error
	l4Map := make(map[uint64]models.L4PortRule)
	for _, r := range l4Rules {
		l4Map[r.ID] = r
	}
	return srvMap, inbMap, l4Map
}

// validateAccessPointTarget 校验接入点目标绑定合法性（目标类型白名单 + 引用实体存在且类型正确）。
func (d *Deps) validateAccessPointTarget(c *gin.Context, targetType string, inboundID, l4RuleID *uint64) bool {
	switch targetType {
	case "":
		return true
	case "inbound":
		if inboundID == nil || *inboundID == 0 {
			util.BadRequest(c, "直连模式需指定目标入站")
			return false
		}
		var inb models.Inbound
		if err := d.DB.First(&inb, *inboundID).Error; err != nil {
			util.BadRequest(c, "目标入站不存在")
			return false
		}
		if inb.Type != models.InboundTypeUser {
			util.BadRequest(c, "接入点只能直连 type=user 的用户入站（relay 为内部落地，不参与订阅）")
			return false
		}
		return true
	case "l4_rule":
		if l4RuleID == nil || *l4RuleID == 0 {
			util.BadRequest(c, "中转模式需指定 L4 转发规则")
			return false
		}
		var rule models.L4PortRule
		if err := d.DB.First(&rule, *l4RuleID).Error; err != nil {
			util.BadRequest(c, "目标 L4 转发规则不存在")
			return false
		}
		return true
	default:
		util.BadRequest(c, "目标类型仅支持 inbound / l4_rule")
		return false
	}
}

// AdminGetAccessPoints GET /api/v1/admin/access-points —— 查询所有用户接入点
func (d *Deps) AdminGetAccessPoints(c *gin.Context) {
	var list []models.UserAccessPoint
	if err := d.DB.Order("id ASC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询接入点列表失败")
		return
	}
	ids := make([]uint64, 0, len(list))
	for i := range list {
		ids = append(ids, list[i].ID)
	}
	groupMap := services.BatchAccessPointPermissionGroupIDs(d.DB, ids)
	srvMap, inbMap, l4Map := d.fetchTopologyContext()

	views := make([]AccessPointView, 0, len(list))
	for i := range list {
		ap := list[i]
		views = append(views, buildAccessPointView(ap, groupMap[ap.ID], srvMap, inbMap, l4Map))
	}
	util.OK(c, gin.H{"items": views})
}

// AdminCreateAccessPoint POST /api/v1/admin/access-points —— 创建用户接入点
func (d *Deps) AdminCreateAccessPoint(c *gin.Context) {
	var req accessPointForm
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		util.BadRequest(c, "接入点名称不能为空")
		return
	}
	if !d.validateAccessPointTarget(c, req.TargetType, req.TargetInboundID, req.TargetL4RuleID) {
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	ap := models.UserAccessPoint{
		Name:            name,
		CustomHost:      strings.TrimSpace(req.CustomHost),
		CustomPort:      req.CustomPort,
		TargetType:      req.TargetType,
		TargetInboundID: req.TargetInboundID,
		TargetL4RuleID:  req.TargetL4RuleID,
		Enabled:         enabled,
		Remark:          req.Remark,
	}

	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&ap).Error; err != nil {
			return err
		}
		if req.PermissionGroupIDs != nil {
			return services.SyncAccessPointPermissionGroups(tx, ap.ID, req.PermissionGroupIDs)
		}
		return nil
	}); err != nil {
		util.ServerError(c, "创建接入点失败")
		return
	}

	d.TriggerUserChange()

	srvMap, inbMap, l4Map := d.fetchTopologyContext()
	util.OK(c, gin.H{
		"access_point": buildAccessPointView(ap, req.PermissionGroupIDs, srvMap, inbMap, l4Map),
	})
}

// AdminUpdateAccessPoint PUT /api/v1/admin/access-points/:id —— 更新用户接入点
func (d *Deps) AdminUpdateAccessPoint(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法接入点 ID")
		return
	}
	var ap models.UserAccessPoint
	if err := d.DB.First(&ap, id).Error; err != nil {
		util.Fail(c, 404, "接入点不存在")
		return
	}

	var req accessPointForm
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if !d.validateAccessPointTarget(c, req.TargetType, req.TargetInboundID, req.TargetL4RuleID) {
		return
	}

	updates := map[string]any{}
	if req.Name != "" {
		updates["name"] = strings.TrimSpace(req.Name)
	}
	updates["custom_host"] = strings.TrimSpace(req.CustomHost)
	updates["custom_port"] = req.CustomPort
	updates["target_type"] = req.TargetType
	updates["target_inbound_id"] = req.TargetInboundID
	updates["target_l4_rule_id"] = req.TargetL4RuleID
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	updates["remark"] = req.Remark

	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		if len(updates) > 0 {
			if err := tx.Model(&ap).Updates(updates).Error; err != nil {
				return err
			}
		}
		if req.PermissionGroupIDs != nil {
			return services.SyncAccessPointPermissionGroups(tx, ap.ID, req.PermissionGroupIDs)
		}
		return nil
	}); err != nil {
		util.ServerError(c, "更新接入点失败")
		return
	}

	d.TriggerUserChange()

	d.DB.First(&ap, id)
	gids := services.AccessPointPermissionGroupIDs(d.DB, id)
	srvMap, inbMap, l4Map := d.fetchTopologyContext()
	util.OK(c, gin.H{
		"access_point": buildAccessPointView(ap, gids, srvMap, inbMap, l4Map),
	})
}

// AdminSetAccessPointTarget PUT /api/v1/admin/access-points/:id/target —— 快捷连线/解绑目标
func (d *Deps) AdminSetAccessPointTarget(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法接入点 ID")
		return
	}
	var ap models.UserAccessPoint
	if err := d.DB.First(&ap, id).Error; err != nil {
		util.Fail(c, 404, "接入点不存在")
		return
	}

	var req struct {
		TargetType      string  `json:"target_type"` // "inbound" | "l4_rule" | ""
		TargetInboundID *uint64 `json:"target_inbound_id"`
		TargetL4RuleID  *uint64 `json:"target_l4_rule_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
		return
	}
	if !d.validateAccessPointTarget(c, req.TargetType, req.TargetInboundID, req.TargetL4RuleID) {
		return
	}

	updates := map[string]any{
		"target_type":       req.TargetType,
		"target_inbound_id": req.TargetInboundID,
		"target_l4_rule_id": req.TargetL4RuleID,
	}
	if err := d.DB.Model(&ap).Updates(updates).Error; err != nil {
		util.ServerError(c, "更新目标失败")
		return
	}

	d.TriggerUserChange()

	d.DB.First(&ap, id)
	gids := services.AccessPointPermissionGroupIDs(d.DB, id)
	srvMap, inbMap, l4Map := d.fetchTopologyContext()
	util.OK(c, gin.H{
		"access_point": buildAccessPointView(ap, gids, srvMap, inbMap, l4Map),
	})
}

// AdminDeleteAccessPoint DELETE /api/v1/admin/access-points/:id —— 删除用户接入点
func (d *Deps) AdminDeleteAccessPoint(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法接入点 ID")
		return
	}
	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		_ = tx.Where("access_point_id = ?", id).Delete(&models.PermissionGroupAccessPoint{}).Error
		return tx.Delete(&models.UserAccessPoint{}, id).Error
	}); err != nil {
		util.ServerError(c, "删除接入点失败")
		return
	}
	d.TriggerUserChange()
	util.OK(c, gin.H{"deleted": id})
}
