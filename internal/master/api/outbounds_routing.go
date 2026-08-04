package api

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// outboundForm 出站创建/修改表单
type outboundForm struct {
	Tag                string `json:"tag" binding:"required,max=64"`
	Protocol           string `json:"protocol" binding:"required,max=32"`
	SettingsJSON       string `json:"settings_json"`
	StreamSettingsJSON string `json:"stream_settings_json"`
	SendThrough        string `json:"send_through"`
	Enabled            *bool  `json:"enabled"`
	Priority           *int   `json:"priority"`
	Remark             string `json:"remark"`
}

// routingRuleForm 路由规则创建/修改表单
type routingRuleForm struct {
	OutboundTag string `json:"outbound_tag" binding:"required,max=64"`
	RuleJSON    string `json:"rule_json"`
	Domain      string `json:"domain"`
	IP          string `json:"ip"`
	Port        string `json:"port"`
	Network     string `json:"network"`
	InboundTag  string `json:"inbound_tag"`
	Enabled     *bool  `json:"enabled"`
	Priority    *int   `json:"priority"`
	Remark      string `json:"remark"`
}

// AdminGetServerOutbounds GET /api/v1/admin/servers/:id/outbounds
func (d *Deps) AdminGetServerOutbounds(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法服务器 ID")
		return
	}
	var list []models.ServerOutbound
	if err := d.DB.Where("server_id = ?", id).Order("priority ASC, id ASC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询出站规则失败")
		return
	}
	util.OK(c, gin.H{"items": list})
}

// AdminCreateServerOutbound POST /api/v1/admin/servers/:id/outbounds
func (d *Deps) AdminCreateServerOutbound(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法服务器 ID")
		return
	}
	var srv models.Server
	if err := d.DB.First(&srv, id).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}

	var req outboundForm
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}

	ob := models.ServerOutbound{
		ServerID:           id,
		Tag:                req.Tag,
		Protocol:           req.Protocol,
		SettingsJSON:       req.SettingsJSON,
		StreamSettingsJSON: req.StreamSettingsJSON,
		SendThrough:        req.SendThrough,
		Enabled:            enabled,
		Priority:           priority,
		Remark:             req.Remark,
	}

	if err := d.DB.Create(&ob).Error; err != nil {
		util.ServerError(c, "创建出站规则失败")
		return
	}

	d.enqueueConfig(id)
	util.OK(c, gin.H{"outbound": ob})
}

// AdminUpdateServerOutbound PUT /api/v1/admin/servers/:id/outbounds/:outbound_id
func (d *Deps) AdminUpdateServerOutbound(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法服务器 ID")
		return
	}
	outboundID, err := strconv.ParseUint(c.Param("outbound_id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法出站 ID")
		return
	}

	var ob models.ServerOutbound
	if err := d.DB.Where("id = ? AND server_id = ?", outboundID, id).First(&ob).Error; err != nil {
		util.Fail(c, 404, "出站规则不存在")
		return
	}

	var req struct {
		Tag                *string `json:"tag"`
		Protocol           *string `json:"protocol"`
		SettingsJSON       *string `json:"settings_json"`
		StreamSettingsJSON *string `json:"stream_settings_json"`
		SendThrough        *string `json:"send_through"`
		Enabled            *bool   `json:"enabled"`
		Priority           *int    `json:"priority"`
		Remark             *string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	updates := map[string]any{}
	if req.Tag != nil {
		if *req.Tag == "" {
			util.BadRequest(c, "Tag 不能为空")
			return
		}
		updates["tag"] = *req.Tag
	}
	if req.Protocol != nil {
		if *req.Protocol == "" {
			util.BadRequest(c, "Protocol 不能为空")
			return
		}
		updates["protocol"] = *req.Protocol
	}
	if req.SettingsJSON != nil {
		updates["settings_json"] = *req.SettingsJSON
	}
	if req.StreamSettingsJSON != nil {
		updates["stream_settings_json"] = *req.StreamSettingsJSON
	}
	if req.SendThrough != nil {
		updates["send_through"] = *req.SendThrough
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}

	if len(updates) > 0 {
		if err := d.DB.Model(&ob).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新出站规则失败")
			return
		}
	}

	d.DB.First(&ob, outboundID)
	d.enqueueConfig(id)
	util.OK(c, gin.H{"outbound": ob})
}

// AdminDeleteServerOutbound DELETE /api/v1/admin/servers/:id/outbounds/:outbound_id
func (d *Deps) AdminDeleteServerOutbound(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法服务器 ID")
		return
	}
	outboundID, err := strconv.ParseUint(c.Param("outbound_id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法出站 ID")
		return
	}

	if err := d.DB.Where("id = ? AND server_id = ?", outboundID, id).Delete(&models.ServerOutbound{}).Error; err != nil {
		util.ServerError(c, "删除出站规则失败")
		return
	}

	d.enqueueConfig(id)
	util.OK(c, gin.H{"deleted": outboundID})
}

// AdminGetServerRoutingRules GET /api/v1/admin/servers/:id/routing
func (d *Deps) AdminGetServerRoutingRules(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法服务器 ID")
		return
	}
	var list []models.ServerRoutingRule
	if err := d.DB.Where("server_id = ?", id).Order("priority ASC, id ASC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询路由规则失败")
		return
	}
	util.OK(c, gin.H{"items": list})
}

// AdminCreateServerRoutingRule POST /api/v1/admin/servers/:id/routing
func (d *Deps) AdminCreateServerRoutingRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法服务器 ID")
		return
	}
	var srv models.Server
	if err := d.DB.First(&srv, id).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}

	var req routingRuleForm
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}

	rule := models.ServerRoutingRule{
		ServerID:    id,
		OutboundTag: req.OutboundTag,
		RuleJSON:    req.RuleJSON,
		Domain:      req.Domain,
		IP:          req.IP,
		Port:        req.Port,
		Network:     req.Network,
		InboundTag:  req.InboundTag,
		Enabled:     enabled,
		Priority:    priority,
		Remark:      req.Remark,
	}

	if err := d.DB.Create(&rule).Error; err != nil {
		util.ServerError(c, "创建路由规则失败")
		return
	}

	d.enqueueConfig(id)
	util.OK(c, gin.H{"rule": rule})
}

// AdminUpdateServerRoutingRule PUT /api/v1/admin/servers/:id/routing/:rule_id
func (d *Deps) AdminUpdateServerRoutingRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法服务器 ID")
		return
	}
	ruleID, err := strconv.ParseUint(c.Param("rule_id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法规则 ID")
		return
	}

	var rule models.ServerRoutingRule
	if err := d.DB.Where("id = ? AND server_id = ?", ruleID, id).First(&rule).Error; err != nil {
		util.Fail(c, 404, "路由规则不存在")
		return
	}

	var req struct {
		OutboundTag *string `json:"outbound_tag"`
		RuleJSON    *string `json:"rule_json"`
		Domain      *string `json:"domain"`
		IP          *string `json:"ip"`
		Port        *string `json:"port"`
		Network     *string `json:"network"`
		InboundTag  *string `json:"inbound_tag"`
		Enabled     *bool   `json:"enabled"`
		Priority    *int    `json:"priority"`
		Remark      *string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	updates := map[string]any{}
	if req.OutboundTag != nil {
		if *req.OutboundTag == "" {
			util.BadRequest(c, "OutboundTag 不能为空")
			return
		}
		updates["outbound_tag"] = *req.OutboundTag
	}
	if req.RuleJSON != nil {
		updates["rule_json"] = *req.RuleJSON
	}
	if req.Domain != nil {
		updates["domain"] = *req.Domain
	}
	if req.IP != nil {
		updates["ip"] = *req.IP
	}
	if req.Port != nil {
		updates["port"] = *req.Port
	}
	if req.Network != nil {
		updates["network"] = *req.Network
	}
	if req.InboundTag != nil {
		updates["inbound_tag"] = *req.InboundTag
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Priority != nil {
		updates["priority"] = *req.Priority
	}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}

	if len(updates) > 0 {
		if err := d.DB.Model(&rule).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新路由规则失败")
			return
		}
	}

	d.DB.First(&rule, ruleID)
	d.enqueueConfig(id)
	util.OK(c, gin.H{"rule": rule})
}

// AdminDeleteServerRoutingRule DELETE /api/v1/admin/servers/:id/routing/:rule_id
func (d *Deps) AdminDeleteServerRoutingRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法服务器 ID")
		return
	}
	ruleID, err := strconv.ParseUint(c.Param("rule_id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法规则 ID")
		return
	}

	if err := d.DB.Where("id = ? AND server_id = ?", ruleID, id).Delete(&models.ServerRoutingRule{}).Error; err != nil {
		util.ServerError(c, "删除路由规则失败")
		return
	}

	d.enqueueConfig(id)
	util.OK(c, gin.H{"deleted": ruleID})
}
