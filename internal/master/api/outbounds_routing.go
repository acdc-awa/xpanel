package api

import (
	"log"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/xray"
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
	InboundRef         *uint64 `json:"inbound_ref"` // Phase T：引用落地入站（vnext 自动构造）
}

// routingRuleForm 路由规则创建/修改表单
type routingRuleForm struct {
	OutboundTag string `json:"outbound_tag" binding:"required,max=64"`
	RuleJSON    string `json:"rule_json"`
	Domain      string `json:"domain"`
	IP          string `json:"ip"`
	Port        string `json:"port"`
	Network     string `json:"network"`
	Protocol    string `json:"protocol"`
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
	// 出站 JSON 有效性 + REALITY 密钥格式预检（01 号文档 §4 第 6 项）
	if err := xray.ValidateOutbound(req.SettingsJSON, req.StreamSettingsJSON); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	// Phase T：InboundRef 校验（引用存在 + 无环 + 目标自动标 relay）
	if req.InboundRef != nil {
		if msg := d.checkInboundRef(id, *req.InboundRef, 0); msg != "" {
			util.BadRequest(c, msg)
			return
		}
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
		InboundRef:         req.InboundRef,
	}

	if err := d.DB.Create(&ob).Error; err != nil {
		util.ServerError(c, "创建出站规则失败")
		return
	}

	if err := d.enqueueConfig(id); err != nil {
		log.Printf("outbounds_routing: 自动推送配置失败 (server=%d): %v", id, err)
	}
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
		InboundRef         *uint64 `json:"inbound_ref"` // nil=不变；0=解除引用；>0=设置引用
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	// 出站 JSON 有效性 + REALITY 密钥格式预检（01 号文档 §4 第 6 项）
	sj := ob.SettingsJSON
	if req.SettingsJSON != nil {
		sj = *req.SettingsJSON
	}
	ssj := ob.StreamSettingsJSON
	if req.StreamSettingsJSON != nil {
		ssj = *req.StreamSettingsJSON
	}
	if err := xray.ValidateOutbound(sj, ssj); err != nil {
		util.BadRequest(c, err.Error())
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

	// Phase T：InboundRef 变更（>0 设置 / 0 解除），校验无环 + 维护目标入站标记
	if req.InboundRef != nil {
		if *req.InboundRef > 0 {
			if msg := d.checkInboundRef(ob.ServerID, *req.InboundRef, outboundID); msg != "" {
				util.BadRequest(c, msg)
				return
			}
			updates["inbound_ref"] = *req.InboundRef
		} else {
			updates["inbound_ref"] = nil
		}
	}

	// 在 Updates 前捕获旧引用（GORM Updates 会回写 struct 字段，否则解绑后 oldRef 已被置 nil）
	oldRef := ob.InboundRef
	if len(updates) > 0 {
		if err := d.DB.Model(&ob).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新出站规则失败")
			return
		}
	}
	// 引用目标维护：新目标标 relay；旧目标（被改走/解除）无其他引用则回 idle
	if req.InboundRef != nil {
		if *req.InboundRef > 0 {
			d.ensureRelayMark(*req.InboundRef)
		}
		if oldRef != nil && (*req.InboundRef == 0 || *oldRef != *req.InboundRef) {
			d.demoteIfUnreferenced(*oldRef)
		}
	}

	d.DB.First(&ob, outboundID)
	if err := d.enqueueConfig(id); err != nil {
		log.Printf("outbounds_routing: 自动推送配置失败 (server=%d): %v", id, err)
	}
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

	// Phase T：删除前记录引用目标，删除后无其他引用则回 idle
	var ob models.ServerOutbound
	d.DB.Where("id = ? AND server_id = ?", outboundID, id).First(&ob)
	if err := d.DB.Where("id = ? AND server_id = ?", outboundID, id).Delete(&models.ServerOutbound{}).Error; err != nil {
		util.ServerError(c, "删除出站规则失败")
		return
	}
	if ob.InboundRef != nil {
		d.demoteIfUnreferenced(*ob.InboundRef)
	}

	if err := d.enqueueConfig(id); err != nil {
		log.Printf("outbounds_routing: 自动推送配置失败 (server=%d): %v", id, err)
	}
	util.OK(c, gin.H{"deleted": outboundID})
}

// checkInboundRef 校验 InboundRef 设置：目标存在 + 引用不成环。
// excludeOutboundID 用于更新时排除自身（创建传 0）。返回错误消息，空串 = 通过。
func (d *Deps) checkInboundRef(outboundServerID, targetInboundID, excludeOutboundID uint64) string {
	var target models.Inbound
	if err := d.DB.First(&target, targetInboundID).Error; err != nil {
		return "引用的入站不存在"
	}
	if !target.Enabled {
		return "引用的入站已停用"
	}
	if target.Type == models.InboundTypeIdle {
		return "引用的入站为 idle，请先启用为 user/relay"
	}
	if d.wouldCreateRefCycle(outboundServerID, targetInboundID, excludeOutboundID) {
		return "引用将形成转发环路（A→B→A），已拒绝"
	}
	return ""
}

// wouldCreateRefCycle 环判定：出站 X（在 outboundServerID 服务器上）引用 targetInboundID。
// 从 target 出发沿「目标所在服务器的出站引用」扩展，若可达 outboundServerID 的任一入站
// （或回到 target 自身），则 X 会闭合转发环（A→B→A）。
func (d *Deps) wouldCreateRefCycle(outboundServerID, targetInboundID, excludeOutboundID uint64) bool {
	reach := map[uint64]bool{targetInboundID: true}
	queue := []uint64{targetInboundID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		var inb models.Inbound
		if err := d.DB.First(&inb, cur).Error; err != nil {
			continue
		}
		if inb.ServerID == outboundServerID {
			return true // 可达出站所在服务器的入站 → 闭合
		}
		var outs []models.ServerOutbound
		d.DB.Where("server_id = ? AND inbound_ref IS NOT NULL AND id != ?", inb.ServerID, excludeOutboundID).Find(&outs)
		for _, ob := range outs {
			if !ob.Enabled || ob.InboundRef == nil {
				continue
			}
			ref := *ob.InboundRef
			if ref == targetInboundID {
				return true
			}
			if !reach[ref] {
				reach[ref] = true
				queue = append(queue, ref)
			}
		}
	}
	return false
}

// ensureRelayMark 引用目标入站自动标记 relay。
func (d *Deps) ensureRelayMark(targetInboundID uint64) {
	d.DB.Model(&models.Inbound{}).Where("id = ?", targetInboundID).Update("type", models.InboundTypeRelay)
}

// demoteIfUnreferenced 目标入站不再被任何出站引用时回 idle（管理员可再改回）。
func (d *Deps) demoteIfUnreferenced(targetInboundID uint64) {
	var cnt int64
	d.DB.Model(&models.ServerOutbound{}).Where("inbound_ref = ?", targetInboundID).Count(&cnt)
	if cnt == 0 {
		d.DB.Model(&models.Inbound{}).Where("id = ?", targetInboundID).Update("type", models.InboundTypeIdle)
	}
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
		Protocol:    req.Protocol,
		InboundTag:  req.InboundTag,
		Enabled:     enabled,
		Priority:    priority,
		Remark:      req.Remark,
	}

	if err := d.DB.Create(&rule).Error; err != nil {
		util.ServerError(c, "创建路由规则失败")
		return
	}

	if err := d.enqueueConfig(id); err != nil {
		log.Printf("outbounds_routing: 自动推送配置失败 (server=%d): %v", id, err)
	}
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
		Protocol    *string `json:"protocol"`
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
	if req.Protocol != nil {
		updates["protocol"] = *req.Protocol
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
	if err := d.enqueueConfig(id); err != nil {
		log.Printf("outbounds_routing: 自动推送配置失败 (server=%d): %v", id, err)
	}
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

	if err := d.enqueueConfig(id); err != nil {
		log.Printf("outbounds_routing: 自动推送配置失败 (server=%d): %v", id, err)
	}
	util.OK(c, gin.H{"deleted": ruleID})
}
