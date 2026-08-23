package api

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// l4RuleView L4 规则对外视图结构。
// 纯传输管道定义，不含权限语义——访问控制由用户接入点（UserAccessPoint）白名单单点收口。
type l4RuleView struct {
	ID                uint64    `json:"id"`
	ServerID          uint64    `json:"server_id"`
	ListenPort        int       `json:"listen_port"`
	TargetServerID    uint64    `json:"target_server_id"`
	TargetServerName  string    `json:"target_server_name,omitempty"`
	TargetInboundID   uint64    `json:"target_inbound_id"`
	TargetInboundTag  string    `json:"target_inbound_tag,omitempty"`
	TargetInboundPort int       `json:"target_inbound_port,omitempty"`
	Remark            string    `json:"remark"`
	Enabled           bool      `json:"enabled"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// l4RuleForm L4 规则创建/更新表单。
// Target* 允许为空（0）：先保存「待连线」缺省规则，后在拓扑画布拖线完成目标映射。
type l4RuleForm struct {
	ListenPort      int    `json:"listen_port" binding:"required,min=1,max=65535"`
	TargetServerID  uint64 `json:"target_server_id"`
	TargetInboundID uint64 `json:"target_inbound_id"`
	Remark          string `json:"remark"`
	Enabled         *bool  `json:"enabled"`
}

// AdminGetL4Rules GET /api/v1/admin/servers/:id/l4-rules
func (d *Deps) AdminGetL4Rules(c *gin.Context) {
	serverID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的服务器 ID")
		return
	}

	var srv models.Server
	if err := d.DB.First(&srv, serverID).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}

	var rules []models.L4PortRule
	if err := d.DB.Where("server_id = ?", serverID).Order("listen_port ASC").Find(&rules).Error; err != nil {
		util.ServerError(c, "查询 L4 规则失败: "+err.Error())
		return
	}

	targetSrvIDs := make([]uint64, 0, len(rules))
	targetInbIDs := make([]uint64, 0, len(rules))
	for _, r := range rules {
		targetSrvIDs = append(targetSrvIDs, r.TargetServerID)
		targetInbIDs = append(targetInbIDs, r.TargetInboundID)
	}

	srvMap := make(map[uint64]models.Server)
	if len(targetSrvIDs) > 0 {
		var srvs []models.Server
		d.DB.Where("id IN ?", targetSrvIDs).Find(&srvs)
		for _, s := range srvs {
			srvMap[s.ID] = s
		}
	}

	inbMap := make(map[uint64]models.Inbound)
	if len(targetInbIDs) > 0 {
		var inbs []models.Inbound
		d.DB.Where("id IN ?", targetInbIDs).Find(&inbs)
		for _, inb := range inbs {
			inbMap[inb.ID] = inb
		}
	}

	views := make([]l4RuleView, 0, len(rules))
	for _, r := range rules {
		v := l4RuleView{
			ID:              r.ID,
			ServerID:        r.ServerID,
			ListenPort:      r.ListenPort,
			TargetServerID:  r.TargetServerID,
			TargetInboundID: r.TargetInboundID,
			Remark:          r.Remark,
			Enabled:         r.Enabled,
			CreatedAt:       r.CreatedAt,
			UpdatedAt:       r.UpdatedAt,
		}
		if ts, ok := srvMap[r.TargetServerID]; ok {
			v.TargetServerName = ts.Name
		}
		if ti, ok := inbMap[r.TargetInboundID]; ok {
			v.TargetInboundTag = ti.Tag
			v.TargetInboundPort = ti.Port
		}
		views = append(views, v)
	}

	util.OK(c, views)
}

// AdminCreateL4Rule POST /api/v1/admin/servers/:id/l4-rules
func (d *Deps) AdminCreateL4Rule(c *gin.Context) {
	serverID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的服务器 ID")
		return
	}

	var srv models.Server
	if err := d.DB.First(&srv, serverID).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}

	var f l4RuleForm
	if err := c.ShouldBindJSON(&f); err != nil {
		util.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	// 校验目标入站与服务器是否存在（缺省规则允许 target 为空，待拓扑拖线完成映射）
	if f.TargetInboundID != 0 {
		var targetInb models.Inbound
		if err := d.DB.First(&targetInb, f.TargetInboundID).Error; err != nil {
			util.BadRequest(c, "目标入站不存在")
			return
		}
		if targetInb.ServerID != f.TargetServerID {
			util.BadRequest(c, "目标入站与目标服务器不匹配")
			return
		}
	}

	// 校验中转端口唯一性
	var count int64
	d.DB.Model(&models.L4PortRule{}).Where("server_id = ? AND listen_port = ?", serverID, f.ListenPort).Count(&count)
	if count > 0 {
		util.BadRequest(c, "该中转服务器已存在相同监听端口的转发规则")
		return
	}

	enabled := true
	if f.Enabled != nil {
		enabled = *f.Enabled
	}

	rule := models.L4PortRule{
		ServerID:        serverID,
		ListenPort:      f.ListenPort,
		TargetServerID:  f.TargetServerID,
		TargetInboundID: f.TargetInboundID,
		Remark:          f.Remark,
		Enabled:         enabled,
	}

	if err := d.DB.Create(&rule).Error; err != nil {
		util.ServerError(c, "创建 L4 规则失败: "+err.Error())
		return
	}
	d.TriggerUserChange()

	util.OK(c, gin.H{
		"id":                rule.ID,
		"server_id":         rule.ServerID,
		"listen_port":       rule.ListenPort,
		"target_server_id":  rule.TargetServerID,
		"target_inbound_id": rule.TargetInboundID,
		"remark":            rule.Remark,
		"enabled":           rule.Enabled,
	})
}

// AdminUpdateL4Rule PUT /api/v1/admin/servers/:id/l4-rules/:rule_id
func (d *Deps) AdminUpdateL4Rule(c *gin.Context) {
	serverID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的服务器 ID")
		return
	}
	ruleID, err := strconv.ParseUint(c.Param("rule_id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的规则 ID")
		return
	}

	var rule models.L4PortRule
	if err := d.DB.Where("id = ? AND server_id = ?", ruleID, serverID).First(&rule).Error; err != nil {
		util.Fail(c, 404, "L4 转发规则不存在")
		return
	}

	var f l4RuleForm
	if err := c.ShouldBindJSON(&f); err != nil {
		util.BadRequest(c, "参数校验失败: "+err.Error())
		return
	}

	// 校验目标入站（缺省规则允许 target 为空，待拓扑拖线完成映射）
	if f.TargetInboundID != 0 {
		var targetInb models.Inbound
		if err := d.DB.First(&targetInb, f.TargetInboundID).Error; err != nil {
			util.BadRequest(c, "目标入站不存在")
			return
		}
		if targetInb.ServerID != f.TargetServerID {
			util.BadRequest(c, "目标入站与目标服务器不匹配")
			return
		}
	}

	// 校验中转端口唯一性（排除自身）
	var count int64
	d.DB.Model(&models.L4PortRule{}).Where("server_id = ? AND listen_port = ? AND id != ?", serverID, f.ListenPort, ruleID).Count(&count)
	if count > 0 {
		util.BadRequest(c, "该中转服务器已存在相同监听端口的转发规则")
		return
	}

	rule.ListenPort = f.ListenPort
	rule.TargetServerID = f.TargetServerID
	rule.TargetInboundID = f.TargetInboundID
	rule.Remark = f.Remark
	if f.Enabled != nil {
		rule.Enabled = *f.Enabled
	}

	if err := d.DB.Save(&rule).Error; err != nil {
		util.ServerError(c, "更新 L4 规则失败: "+err.Error())
		return
	}
	d.TriggerUserChange()

	util.OK(c, gin.H{
		"id":                rule.ID,
		"server_id":         rule.ServerID,
		"listen_port":       rule.ListenPort,
		"target_server_id":  rule.TargetServerID,
		"target_inbound_id": rule.TargetInboundID,
		"remark":            rule.Remark,
		"enabled":           rule.Enabled,
	})
}

// AdminDeleteL4Rule DELETE /api/v1/admin/servers/:id/l4-rules/:rule_id
func (d *Deps) AdminDeleteL4Rule(c *gin.Context) {
	serverID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的服务器 ID")
		return
	}
	ruleID, err := strconv.ParseUint(c.Param("rule_id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的规则 ID")
		return
	}

	var rule models.L4PortRule
	if err := d.DB.Where("id = ? AND server_id = ?", ruleID, serverID).First(&rule).Error; err != nil {
		util.Fail(c, 404, "L4 转发规则不存在")
		return
	}

	// 接入点引用保护：被用户接入点连线的 L4 规则禁止删除（订阅管道断裂）
	var apCnt int64
	d.DB.Model(&models.UserAccessPoint{}).Where("target_type = 'l4_rule' AND target_l4_rule_id = ?", ruleID).Count(&apCnt)
	if apCnt > 0 {
		util.BadRequest(c, "该转发规则被 "+strconv.FormatInt(apCnt, 10)+" 个用户接入点引用，无法删除，请先解除接入点连线")
		return
	}

	if err := d.DB.Delete(&rule).Error; err != nil {
		util.ServerError(c, "删除 L4 规则失败: "+err.Error())
		return
	}
	d.TriggerUserChange()

	util.OK(c, gin.H{"deleted": true})
}
