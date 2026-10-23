package api

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/protocols"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// validOutboundProtocol 出站协议白名单（与生成器支持范围保持一致）。
func validOutboundProtocol(p string) bool {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "freedom", "blackhole", "vless", "socks", "http", "hysteria", "custom":
		return true
	}
	return false
}

// outboundShape 一次创建/更新后的最终出站载荷。
// 新模型只允许三种落库形态（外加载荷为空的引用草稿）：
//   - 基础出口：protocol + settings_json（仅 freedom/blackhole）；
//   - 引用入站：仅 inbound_ref（载荷列必须为空）；
//   - 自定义 JSON：仅 outbound_json（protocol 固定 custom）；
//   - 草稿：非基础协议 + 无任何载荷，等待连线。
type outboundShape struct {
	Protocol     string
	SettingsJSON string
	StreamJSON   string
	OutboundJSON string
	InboundRef   *uint64
}

// normalizeOutboundShape 校验三形态互斥并归一协议与载荷；msg 非空表示拒绝原因。
func normalizeOutboundShape(in outboundShape) (outboundShape, string) {
	proto := strings.ToLower(strings.TrimSpace(in.Protocol))
	settings := strings.TrimSpace(in.SettingsJSON)
	stream := strings.TrimSpace(in.StreamJSON)
	outJSON := strings.TrimSpace(in.OutboundJSON)
	hasRef := in.InboundRef != nil && *in.InboundRef > 0

	in.Protocol, in.SettingsJSON, in.StreamJSON, in.OutboundJSON = proto, settings, stream, outJSON

	switch {
	case outJSON != "":
		var m map[string]any
		if err := json.Unmarshal([]byte(outJSON), &m); err != nil {
			return in, "Outbound JSON 格式非法: " + err.Error()
		}
		if p, _ := m["protocol"].(string); strings.TrimSpace(p) == "" {
			return in, "自定义 Outbound JSON 必须包含非空 protocol 字段"
		}
		if hasRef {
			return in, "自定义 JSON 与引用入站不能同时设置"
		}
		if settings != "" || stream != "" {
			return in, "自定义 JSON 模式下无需填写 settings/streamSettings"
		}
		if proto == "freedom" || proto == "blackhole" {
			return in, "基础出口（freedom/blackhole）不支持自定义 JSON"
		}
		in.Protocol, in.SettingsJSON, in.StreamJSON = "custom", "", ""
	case hasRef:
		if settings != "" || stream != "" {
			return in, "引用入站模式下无需填写 settings/streamSettings"
		}
		if proto == "freedom" || proto == "blackhole" {
			return in, "基础出口（freedom/blackhole）不支持引用落地入站"
		}
		in.SettingsJSON, in.StreamJSON, in.OutboundJSON = "", "", ""
	case proto == "custom":
		return in, "自定义 JSON 模式必须填写 outbound_json"
	case proto == "freedom" || proto == "blackhole":
		if stream != "" {
			return in, "基础出口（freedom/blackhole）不支持 streamSettings"
		}
		in.OutboundJSON = ""
	default:
		// 非基础协议：只允许无载荷草稿；旧版外部代理节点表单已下线。
		if settings != "" || stream != "" {
			return in, "外部代理节点表单已下线，请改用自定义 JSON 填写完整 Outbound"
		}
		in.OutboundJSON = ""
	}
	return in, ""
}

// serverOutboundTagExists 校验出站 tag 存在（含模板内置 direct/blocked）。
func serverOutboundTagExists(db *gorm.DB, serverID uint64, tag string) bool {
	if tag == "" {
		return false
	}
	var cnt int64
	db.Model(&models.ServerOutbound{}).Where("server_id = ? AND tag = ?", serverID, tag).Count(&cnt)
	return cnt > 0
}

func validRuleJSON(raw string) bool {
	return raw == "" || json.Valid([]byte(raw))
}

// outboundForm 出站创建/修改表单
type outboundForm struct {
	Tag                string  `json:"tag" binding:"required,max=64"`
	Protocol           string  `json:"protocol" binding:"required,max=32"`
	SettingsJSON       string  `json:"settings_json"`
	StreamSettingsJSON string  `json:"stream_settings_json"`
	OutboundJSON       string  `json:"outbound_json"` // 原生完整 Xray Outbound JSON
	SendThrough        string  `json:"send_through"`
	Enabled            *bool   `json:"enabled"`
	Priority           *int    `json:"priority"`
	Remark             string  `json:"remark"`
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

// isReservedOutboundTag 判断是否为系统预留内置出站 Tag（direct 与 blocked）。
func isReservedOutboundTag(tag string) bool {
	t := strings.ToLower(strings.TrimSpace(tag))
	return t == "direct" || t == "blocked"
}

// tunnelOwnerDesc 描述派生行的归属直通（用于拒绝文案）。找不到入站时退化为基础文案。
func (d *Deps) tunnelOwnerDesc(ownerInboundID uint64) string {
	if ownerInboundID == 0 {
		return ""
	}
	var inb models.Inbound
	if err := d.DB.First(&inb, ownerInboundID).Error; err != nil || inb.Tag == "" {
		return "四层直通"
	}
	return "四层直通「" + inb.Tag + "」"
}

// tunnelOutboundTagConflict 检查新建/改名的四层直通要用的派生出口 tag 是否已被别人的出站占用。
// 返回错误消息，空串 = 通过。excludeOwner > 0 时忽略该直通自己名下的行（更新场景）。
// 之所以在写入前查：派生出口由对账函数自动维护，撞名时对账只能报错跳过，不如在这里直接拦住，
// 让管理员看到"这个 tag 被占了"，而不是事后发现直通连不上。
func (d *Deps) tunnelOutboundTagConflict(serverID uint64, inboundTag string, excludeOwner uint64) string {
	if inboundTag == "" {
		return ""
	}
	want := xray.TunnelOutboundTag(inboundTag)
	q := d.DB.Model(&models.ServerOutbound{}).Where("server_id = ? AND tag = ?", serverID, want)
	if excludeOwner > 0 {
		q = q.Where("owner_inbound_id = 0 OR owner_inbound_id <> ?", excludeOwner)
	}
	var cnt int64
	if err := q.Count(&cnt).Error; err != nil {
		return ""
	}
	if cnt > 0 {
		return "出站 tag " + want + " 已被现有出站占用，无法为该直通创建派生出口（请改名或先处理该出站）"
	}
	return ""
}

// ensureTunnelDerived 写入型端点使用：对账直通派生对象，失败时按 fail-closed 报错。
// 用于创建/更新直通（此时失败多半是 tag 撞名，应当拒绝这次写入而不是留个连不上的直通）。
func (d *Deps) ensureTunnelDerived(serverID uint64) string {
	if err := services.EnsureTunnelDerivedObjects(d.DB, serverID); err != nil {
		log.Printf("[tunnel-derived] 对账失败（server=%d）: %v", serverID, err)
		return err.Error()
	}
	return ""
}

// EnsureDefaultServerOutbounds 保证该服务器拥有且仅拥有一份默认的 direct (freedom) 与 blocked (blackhole) 内置出站，并保证 default_outbound_tag 为 direct。
func EnsureDefaultServerOutbounds(db *gorm.DB, serverID uint64) {
	if db == nil || serverID == 0 {
		return
	}

	// 1. 确保并清理重复的 direct 出站
	// settings 须与模板 direct 段 / 迁移回填值一致（canonical 形状，见 models.DefaultFreedomDirectSettingsJSON）：
	// 同 tag 出站在生成时是整体覆盖模板而非深合并，只写 domainStrategy 会让模板的私网拦截
	// finalRules 凭空消失，且出站编辑器从 finalRules 反推「屏蔽内网私有 IP」开关，会把它显示成关闭。
	var directs []models.ServerOutbound
	db.Where("server_id = ? AND tag = ?", serverID, "direct").Order("id ASC").Find(&directs)
	if len(directs) == 0 {
		directOb := models.ServerOutbound{
			ServerID:     serverID,
			Tag:          "direct",
			Protocol:     "freedom",
			SettingsJSON: models.DefaultFreedomDirectSettingsJSON,
			Remark:       "默认直连出站",
			Priority:     0,
			Enabled:      true,
		}
		_ = db.Create(&directOb).Error
	} else if len(directs) > 1 {
		// 保留第一个，清理多余的重复项
		for i := 1; i < len(directs); i++ {
			_ = db.Delete(&models.ServerOutbound{}, directs[i].ID).Error
		}
	}

	// 2. 确保并清理重复的 blocked 出站
	var blockeds []models.ServerOutbound
	db.Where("server_id = ? AND (tag = ? OR tag = ?)", serverID, "blocked", "blackhole").Order("id ASC").Find(&blockeds)
	if len(blockeds) == 0 {
		blockedOb := models.ServerOutbound{
			ServerID:     serverID,
			Tag:          "blocked",
			Protocol:     "blackhole",
			SettingsJSON: `{"response":{"type":"none"}}`,
			Remark:       "默认黑洞阻断",
			Priority:     99,
			Enabled:      true,
		}
		_ = db.Create(&blockedOb).Error
	} else if len(blockeds) > 1 {
		for i := 1; i < len(blockeds); i++ {
			_ = db.Delete(&models.ServerOutbound{}, blockeds[i].ID).Error
		}
	}

	// 3. 确保 default_outbound_tag 为 direct（若为空）
	var srv models.Server
	if err := db.First(&srv, serverID).Error; err == nil {
		if srv.DefaultOutboundTag == "" {
			db.Model(&srv).Update("default_outbound_tag", "direct")
		}
	}
}

// AdminGetServerOutbounds GET /api/v1/admin/servers/:id/outbounds
func (d *Deps) AdminGetServerOutbounds(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的服务器 ID")
		return
	}
	EnsureDefaultServerOutbounds(d.DB, id)
	// C2：打开路由页时顺带让直通的派生行与直通对齐（老库首次加载即完成回填）
	d.ensureTunnelDerived(id)
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
		util.BadRequest(c, "无效的服务器 ID")
		return
	}
	var srv models.Server
	if err := d.DB.First(&srv, id).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}

	var req outboundForm
	if !util.BindJSON(c, &req) {
		return
	}
	shape, msg := normalizeOutboundShape(outboundShape{
		Protocol:     req.Protocol,
		SettingsJSON: req.SettingsJSON,
		StreamJSON:   req.StreamSettingsJSON,
		OutboundJSON: req.OutboundJSON,
		InboundRef:   req.InboundRef,
	})
	if msg != "" {
		util.BadRequest(c, msg)
		return
	}
	req.Protocol = shape.Protocol
	req.SettingsJSON = shape.SettingsJSON
	req.StreamSettingsJSON = shape.StreamJSON
	req.OutboundJSON = shape.OutboundJSON
	if req.OutboundJSON == "" {
		// 出站 JSON 有效性 + REALITY 密钥格式预检（01 号文档 §4 第 6 项）
		if err := xray.ValidateOutbound(req.SettingsJSON, req.StreamSettingsJSON); err != nil {
			util.BadRequest(c, err.Error())
			return
		}
	}
	// ISSUE-13：tag 非空/同服务器唯一。
	req.Tag = strings.TrimSpace(req.Tag)
	if req.Tag == "" {
		util.BadRequest(c, "出站 Tag 不能为空")
		return
	}
	if isReservedOutboundTag(req.Tag) {
		util.BadRequest(c, "direct 与 blocked 为系统内置预留出站，不可重复手动创建")
		return
	}
	var tagCnt int64
	d.DB.Model(&models.ServerOutbound{}).Where("server_id = ? AND tag = ?", id, req.Tag).Count(&tagCnt)
	if tagCnt > 0 {
		util.BadRequest(c, "该服务器上已存在同名出站 Tag")
		return
	}
	// InboundRef 校验（引用存在 + 协议可投影 + 无环）与出站凭据生成
	var targetInboundID uint64
	if req.InboundRef != nil && *req.InboundRef > 0 {
		if msg := d.checkInboundRef(id, *req.InboundRef, 0); msg != "" {
			util.BadRequest(c, msg)
			return
		}
		var target models.Inbound
		if err := d.DB.First(&target, *req.InboundRef).Error; err == nil {
			// 行协议跟随目标入站：列表/画布徽章与草稿判定都以它为准。
			req.Protocol = target.Protocol
		}
		targetInboundID = *req.InboundRef
	} else {
		req.InboundRef = nil
	}
	if !validOutboundProtocol(req.Protocol) {
		util.BadRequest(c, "不支持的出站协议: "+req.Protocol+"（支持 freedom/blackhole/vless/socks/http/hysteria/custom）")
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
		OutboundJSON:       req.OutboundJSON,
		SendThrough:        req.SendThrough,
		Enabled:            enabled,
		Priority:           priority,
		Remark:             req.Remark,
		InboundRef:         req.InboundRef,
	}
	if ob.InboundRef != nil && *ob.InboundRef > 0 {
		ob.InternalUUID, _ = util.NewUUID()
	}

	if err := d.DB.Create(&ob).Error; err != nil {
		util.ServerError(c, "创建出站规则失败")
		return
	}

	if err := d.enqueueConfig(id); err != nil {
		pushFail(c, id, err)
		return
	}
	// 跨服联动推送落地机
	if targetInboundID > 0 {
		var targetInb models.Inbound
		if err := d.DB.First(&targetInb, targetInboundID).Error; err == nil {
			_ = d.enqueueConfig(targetInb.ServerID)
		}
	}
	util.OK(c, gin.H{"outbound": ob})
}

// AdminUpdateServerOutbound PUT /api/v1/admin/servers/:id/outbounds/:outbound_id
func (d *Deps) AdminUpdateServerOutbound(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的服务器 ID")
		return
	}
	outboundID, err := strconv.ParseUint(c.Param("outbound_id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的出站 ID")
		return
	}

	var ob models.ServerOutbound
	if err := d.DB.Where("id = ? AND server_id = ?", outboundID, id).First(&ob).Error; err != nil {
		util.Fail(c, 404, "出站规则不存在")
		return
	}
	// C2：四层直通的派生出口由直通自动维护（目标只有它一个真源），不允许在这里手改——
	// 否则会出现"两个地方都能改目标、以谁为准说不清"的局面（唯一入口原则）。
	if ob.OwnerInboundID > 0 {
		util.BadRequest(c, d.tunnelOwnerDesc(ob.OwnerInboundID)+"的派生出口由直通自动维护，请在通道页或入站页修改转发目标")
		return
	}

	var req struct {
		Tag                *string `json:"tag"`
		Protocol           *string `json:"protocol"`
		SettingsJSON       *string `json:"settings_json"`
		StreamSettingsJSON *string `json:"stream_settings_json"`
		OutboundJSON       *string `json:"outbound_json"`
		SendThrough        *string `json:"send_through"`
		Enabled            *bool   `json:"enabled"`
		Priority           *int    `json:"priority"`
		Remark             *string `json:"remark"`
		InboundRef         *uint64 `json:"inbound_ref"` // nil=不变；0=解除引用；>0=设置引用
	}
	if !util.BindJSON(c, &req) {
		return
	}

	// 形态字段（协议/载荷/引用）任一被触碰时，按最终形态做互斥校验并归一。
	touchesShape := req.Protocol != nil || req.SettingsJSON != nil || req.StreamSettingsJSON != nil ||
		req.OutboundJSON != nil || req.InboundRef != nil
	updates := map[string]any{}
	if touchesShape {
		eff := outboundShape{
			Protocol:     ob.Protocol,
			SettingsJSON: ob.SettingsJSON,
			StreamJSON:   ob.StreamSettingsJSON,
			OutboundJSON: ob.OutboundJSON,
			InboundRef:   ob.InboundRef,
		}
		if req.Protocol != nil {
			eff.Protocol = *req.Protocol
		}
		if req.SettingsJSON != nil {
			eff.SettingsJSON = *req.SettingsJSON
		}
		if req.StreamSettingsJSON != nil {
			eff.StreamJSON = *req.StreamSettingsJSON
		}
		if req.OutboundJSON != nil {
			eff.OutboundJSON = *req.OutboundJSON
		}
		if req.InboundRef != nil {
			if *req.InboundRef > 0 {
				eff.InboundRef = req.InboundRef
			} else {
				eff.InboundRef = nil
			}
		}
		normalized, msg := normalizeOutboundShape(eff)
		if msg != "" {
			util.BadRequest(c, msg)
			return
		}
		eff = normalized
		if req.Protocol != nil && isReservedOutboundTag(ob.Tag) &&
			strings.ToLower(strings.TrimSpace(*req.Protocol)) != strings.ToLower(ob.Protocol) {
			util.BadRequest(c, "系统内置出站不可修改协议")
			return
		}
		// 引用目标：协议可投影 + 无环；行协议跟随目标入站。
		if eff.InboundRef != nil && *eff.InboundRef > 0 {
			if msg := d.checkInboundRef(ob.ServerID, *eff.InboundRef, outboundID); msg != "" {
				util.BadRequest(c, msg)
				return
			}
			var target models.Inbound
			if err := d.DB.First(&target, *eff.InboundRef).Error; err == nil {
				eff.Protocol = target.Protocol
			}
			if ob.InternalUUID == "" {
				u, _ := util.NewUUID()
				updates["internal_uuid"] = u
			}
		}
		if eff.OutboundJSON == "" {
			// 出站 JSON 有效性 + REALITY 密钥格式预检（01 号文档 §4 第 6 项）
			if err := xray.ValidateOutbound(eff.SettingsJSON, eff.StreamJSON); err != nil {
				util.BadRequest(c, err.Error())
				return
			}
		}
		if !validOutboundProtocol(eff.Protocol) {
			util.BadRequest(c, "不支持的出站协议: "+eff.Protocol+"（支持 freedom/blackhole/vless/socks/http/hysteria/custom）")
			return
		}
		updates["protocol"] = eff.Protocol
		updates["outbound_json"] = eff.OutboundJSON
		updates["settings_json"] = eff.SettingsJSON
		updates["stream_settings_json"] = eff.StreamJSON
	}
	if req.InboundRef != nil {
		if *req.InboundRef > 0 {
			updates["inbound_ref"] = *req.InboundRef
		} else {
			updates["inbound_ref"] = nil
		}
	}

	if req.Tag != nil {
		tag := strings.TrimSpace(*req.Tag)
		if tag == "" {
			util.BadRequest(c, "出站 Tag 不能为空")
			return
		}
		// 若当前记录是系统内置出站，禁止修改 Tag
		if isReservedOutboundTag(ob.Tag) && strings.ToLower(tag) != strings.ToLower(ob.Tag) {
			util.BadRequest(c, "系统内置出站（direct/blocked）不可修改 Tag")
			return
		}
		// 若当前是非内置出站，禁止重命名为预留 Tag
		if !isReservedOutboundTag(ob.Tag) && isReservedOutboundTag(tag) {
			util.BadRequest(c, "不可将出站 Tag 修改为系统预留名称（direct/blocked）")
			return
		}
		var tagCnt int64
		d.DB.Model(&models.ServerOutbound{}).Where("server_id = ? AND tag = ? AND id != ?", id, tag, outboundID).Count(&tagCnt)
		if tagCnt > 0 {
			util.BadRequest(c, "该服务器上已存在同名出站 Tag")
			return
		}
		updates["tag"] = tag
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

	oldTag := ob.Tag
	// 在 Updates 前捕获旧引用（GORM Updates 会回写 struct 字段，否则解绑后 oldRef 已被置 nil）
	oldRef := ob.InboundRef
	if len(updates) > 0 {
		if err := d.DB.Model(&ob).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新出站规则失败")
			return
		}
		// 级联改名：若出站 Tag 发生改名，同步更新本服务器上引用该 Tag 的路由规则与默认出口，杜绝拓扑隐藏或规则悬空
		if req.Tag != nil {
			newTag := strings.TrimSpace(*req.Tag)
			if oldTag != "" && oldTag != newTag {
				d.DB.Model(&models.ServerRoutingRule{}).
					Where("server_id = ? AND outbound_tag = ?", id, oldTag).
					Update("outbound_tag", newTag)
				d.DB.Model(&models.Server{}).
					Where("id = ? AND default_outbound_tag = ?", id, oldTag).
					Update("default_outbound_tag", newTag)
			}
		}
	}

	// 联动推送：若引用发生变化，重推旧目标服务器与新目标服务器配置
	if req.InboundRef != nil {
		pushedServers := map[uint64]bool{id: true}
		if oldRef != nil && *oldRef > 0 && (*req.InboundRef == 0 || *oldRef != *req.InboundRef) {
			var oldTarget models.Inbound
			if err := d.DB.First(&oldTarget, *oldRef).Error; err == nil && !pushedServers[oldTarget.ServerID] {
				_ = d.enqueueConfig(oldTarget.ServerID)
				pushedServers[oldTarget.ServerID] = true
			}
		}
		if *req.InboundRef > 0 && (oldRef == nil || *oldRef != *req.InboundRef) {
			var newTarget models.Inbound
			if err := d.DB.First(&newTarget, *req.InboundRef).Error; err == nil && !pushedServers[newTarget.ServerID] {
				_ = d.enqueueConfig(newTarget.ServerID)
				pushedServers[newTarget.ServerID] = true
			}
		}
	}

	d.DB.First(&ob, outboundID)
	if err := d.enqueueConfig(id); err != nil {
		pushFail(c, id, err)
		return
	}
	util.OK(c, gin.H{"outbound": ob})
}

// AdminDeleteServerOutbound DELETE /api/v1/admin/servers/:id/outbounds/:outbound_id
func (d *Deps) AdminDeleteServerOutbound(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的服务器 ID")
		return
	}
	outboundID, err := strconv.ParseUint(c.Param("outbound_id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的出站 ID")
		return
	}

	var ob models.ServerOutbound
	if err := d.DB.Where("id = ? AND server_id = ?", outboundID, id).First(&ob).Error; err != nil {
		util.Fail(c, 404, "出站规则不存在")
		return
	}
	// 保护系统预留出站不可被删除
	if isReservedOutboundTag(ob.Tag) {
		util.BadRequest(c, "系统内置出站（direct/blocked）不可删除")
		return
	}
	// C2：派生出口随直通的生命周期，删除入口只有一个——停用或删除那条直通
	if ob.OwnerInboundID > 0 {
		util.BadRequest(c, d.tunnelOwnerDesc(ob.OwnerInboundID)+"的派生出口不可单独删除，请停用或删除该直通")
		return
	}

	// 删除当前默认出口时必须回退：否则 servers.default_outbound_tag 悬空，
	// 生成配置找不到该 tag，xray 退回 outbounds[0]（顺序不确定）。
	// direct 是系统保留出站且不可删除，回退目标始终有效。
	var srv models.Server
	if err := d.DB.First(&srv, id).Error; err == nil && srv.DefaultOutboundTag == ob.Tag {
		if err := d.DB.Model(&models.Server{}).Where("id = ?", id).Update("default_outbound_tag", "direct").Error; err != nil {
			util.ServerError(c, "重置默认出口失败")
			return
		}
	}

	if err := d.DB.Where("id = ? AND server_id = ?", outboundID, id).Delete(&models.ServerOutbound{}).Error; err != nil {
		util.ServerError(c, "删除出站规则失败")
		return
	}
	// 级联删除引用该出站 tag 的路由规则（避免生成配置引用不存在的出站）
	if ob.Tag != "" {
		if err := d.DB.Where("server_id = ? AND outbound_tag = ?", id, ob.Tag).Delete(&models.ServerRoutingRule{}).Error; err != nil {
			log.Printf("outbounds_routing: 级联删除路由规则失败 (server=%d tag=%s): %v", id, ob.Tag, err)
		}
	}
	if ob.InboundRef != nil && *ob.InboundRef > 0 {
		var targetInb models.Inbound
		if err := d.DB.First(&targetInb, *ob.InboundRef).Error; err == nil {
			_ = d.enqueueConfig(targetInb.ServerID)
		}
	}

	if err := d.enqueueConfig(id); err != nil {
		pushFail(c, id, err)
		return
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
	// 引用出站由目标协议插件投影（vnext / 凭据 / 传输 / 安全）。
	// socks/http/dokodemo-door 没有客户端投影实现，写进来只会在生成配置时 500。
	if protocols.Find(target.Protocol) == nil {
		return "引用的入站协议 " + target.Protocol + " 不支持作为引用目标（当前仅 VLESS / Hysteria 2）"
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

// AdminGetServerRoutingRules GET /api/v1/admin/servers/:id/routing
func (d *Deps) AdminGetServerRoutingRules(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的服务器 ID")
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
		util.BadRequest(c, "无效的服务器 ID")
		return
	}
	var srv models.Server
	if err := d.DB.First(&srv, id).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}

	var req routingRuleForm
	if !util.BindJSON(c, &req) {
		return
	}

	// ISSUE-13：outboundTag 必须真实存在（含内置 direct/blocked）；RuleJSON 必须是合法 JSON。
	if !serverOutboundTagExists(d.DB, id, req.OutboundTag) {
		util.BadRequest(c, "路由目标出站不存在: "+req.OutboundTag)
		return
	}
	if !validRuleJSON(req.RuleJSON) {
		util.BadRequest(c, "RuleJSON 不是合法 JSON")
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
		pushFail(c, id, err)
		return
	}
	util.OK(c, gin.H{"rule": rule})
}

// AdminUpdateServerRoutingRule PUT /api/v1/admin/servers/:id/routing/:rule_id
func (d *Deps) AdminUpdateServerRoutingRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的服务器 ID")
		return
	}
	ruleID, err := strconv.ParseUint(c.Param("rule_id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的规则 ID")
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
	if !util.BindJSON(c, &req) {
		return
	}

	if rule.OwnerInboundID > 0 {
		if req.InboundTag != nil && *req.InboundTag != rule.InboundTag {
			util.BadRequest(c, "直通派生规则的来源入站由直通入站本身维护，禁止修改")
			return
		}
		if req.Enabled != nil && *req.Enabled != rule.Enabled {
			util.BadRequest(c, "直通派生规则的启用状态跟随直通入站，请直接在入站管理中启用或停用直通")
			return
		}
	}

	updates := map[string]any{}
	if req.OutboundTag != nil {
		if *req.OutboundTag == "" {
			util.BadRequest(c, "OutboundTag 不能为空")
			return
		}
		if !serverOutboundTagExists(d.DB, id, *req.OutboundTag) {
			util.BadRequest(c, "路由目标出站不存在: "+*req.OutboundTag)
			return
		}
		updates["outbound_tag"] = *req.OutboundTag
	}
	if req.RuleJSON != nil && !validRuleJSON(*req.RuleJSON) {
		util.BadRequest(c, "RuleJSON 不是合法 JSON")
		return
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
		pushFail(c, id, err)
		return
	}
	util.OK(c, gin.H{"rule": rule})
}

// AdminDeleteServerRoutingRule DELETE /api/v1/admin/servers/:id/routing/:rule_id
func (d *Deps) AdminDeleteServerRoutingRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的服务器 ID")
		return
	}
	ruleID, err := strconv.ParseUint(c.Param("rule_id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的规则 ID")
		return
	}

	var rl models.ServerRoutingRule
	if err := d.DB.Where("id = ? AND server_id = ?", ruleID, id).First(&rl).Error; err != nil {
		util.Fail(c, 404, "路由规则不存在")
		return
	}
	// C2：派生规则可编辑（改指向、加匹配条件、调优先级都是本批次要开的口子），但不可删除——
	// 删掉它直通就没有转发规则了（流量落到默认出口，表现为"这条直通连不上"）。要停就停用/删除直通本身。
	if rl.OwnerInboundID > 0 {
		util.BadRequest(c, d.tunnelOwnerDesc(rl.OwnerInboundID)+"的转发规则不可删除，请停用或删除该直通（也可以把规则指向 blocked 来停止转发）")
		return
	}

	if err := d.DB.Where("id = ? AND server_id = ?", ruleID, id).Delete(&models.ServerRoutingRule{}).Error; err != nil {
		util.ServerError(c, "删除路由规则失败")
		return
	}

	if err := d.enqueueConfig(id); err != nil {
		pushFail(c, id, err)
		return
	}
	util.OK(c, gin.H{"deleted": ruleID})
}
