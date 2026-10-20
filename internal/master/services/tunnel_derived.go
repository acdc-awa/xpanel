package services

import (
	"fmt"
	"log"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 四层直通派生对象的维护（批次 C2，2026-09-28）。
//
// 背景：直通的 freedom 出站（out-<tag>，目标由 settings.redirect 承担）与转发规则
// （inboundTag=<tag> → out-<tag>）在 C1 之前是**生成期现场派生**、库里没有行，路由页只能显示
// 只读虚拟行，管理员既不能把规则排到它之前（给无认证的直通做来源限制），也不能把直通流量
// 指到别的出站。C2 起两者落库为真行，由本文件负责与直通本身保持一致。
//
// 归属标记：owner_inbound_id（0 = 普通行，>0 = 该入站自动维护）。靠标记而不是 tag 前缀识别，
// 避免与管理员手建的 out-xxx 撞车、也能区分"管理员改过的规则"。
//
// 不变量（每次 reconcile 后成立）：
//  1. 非草稿且存在的直通入站，有且只有一条 owner=它的出站行（tag = out-<入站tag>，
//     freedom，settings 含 redirect 目标）与一条 owner=它的规则行；
//  2. 草稿（无目标/未连线）或已不存在的直通，不留下任何 owner 行；
//  3. 出站行的 enabled 始终跟随入站（历史上"入站不启用就不生成上游对象"）；
//  4. 规则的 enabled 跟随入站；规则的 inbound_tag 恒等于入站 tag；规则的 outbound_tag
//     在"指向缺失/空"时归位到派生出口，但**管理员改成别的存活出站时不覆盖**；
//     规则的匹配条件（domain/ip/port/protocol/source 等）与优先级一律不覆盖。

const (
	tunnelDerivedOutboundRemark = "四层直通派生出口（随直通自动维护）"
	tunnelDerivedRuleRemark     = "四层直通转发规则（随直通自动维护）"
)

// IsTunnelInbound 判定入站是否为四层直通：type=tunnel，或通道托管的 dokodemo-door 入站。
// 与生成器（xray.Generate）的判定同源，勿单独改。
func IsTunnelInbound(inb *models.Inbound) bool {
	if inb == nil {
		return false
	}
	return inb.Type == models.InboundTypeTunnel ||
		(inb.Type == models.InboundTypeChannel && inb.Protocol == models.ProtocolDokodemo) ||
		(inb.Type == models.InboundTypeForward && inb.Protocol == models.ProtocolDokodemo)
}

// EnsureTunnelDerivedObjects 让该服务器上全部四层直通的派生对象与直通本身对齐（幂等）。
// 生成配置前、直通相关写入后都会调用它；重复调用无副作用。
//
// 返回 error 的唯一情形是"派生出口的 tag 被非派生出站占用"（管理员先手建了 out-<tag>）——
// 此时不静默抢占，交由调用方（创建/更新入站的 API）报错，避免把管理员的出站改成直通出口。
func EnsureTunnelDerivedObjects(db *gorm.DB, serverID uint64) error {
	if db == nil || serverID == 0 {
		return nil
	}

	var inbounds []models.Inbound
	if err := db.Where("server_id = ?", serverID).Find(&inbounds).Error; err != nil {
		return err
	}
	// 这里只消费 CertDomains（直通入站），与 agent 能力无关，source 传 nil。
	topo, err := buildGenerateContext(db, nil, inbounds, nil)
	if err != nil {
		return err
	}

	var ownedObs []models.ServerOutbound
	if err := db.Where("server_id = ? AND owner_inbound_id > 0", serverID).Find(&ownedObs).Error; err != nil {
		return err
	}
	var ownedRules []models.ServerRoutingRule
	if err := db.Where("server_id = ? AND owner_inbound_id > 0", serverID).Find(&ownedRules).Error; err != nil {
		return err
	}
	obByOwner := make(map[uint64]*models.ServerOutbound, len(ownedObs))
	for i := range ownedObs {
		obByOwner[ownedObs[i].OwnerInboundID] = &ownedObs[i]
	}
	ruleByOwner := make(map[uint64]*models.ServerRoutingRule, len(ownedRules))
	for i := range ownedRules {
		ruleByOwner[ownedRules[i].OwnerInboundID] = &ownedRules[i]
	}

	// 第一遍：出站（先改名，后面的规则悬空判定才能看到最新 tag 集合）
	live := make(map[uint64]bool) // 仍是直通且非草稿的入站
	for i := range inbounds {
		inb := &inbounds[i]
		if !IsTunnelInbound(inb) {
			continue
		}
		host, port, hasTarget := xray.TunnelTarget(inb, topo)
		if !hasTarget {
			continue // 草稿：目标缺失/未连线，派生对象在第三遍清掉
		}
		live[inb.ID] = true
		wantTag := xray.TunnelOutboundTag(inb.Tag)
		wantSettings, err := xray.TunnelOutboundSettings(host, port, xray.TunnelSendProxyProtocol(inb))
		if err != nil {
			return err
		}

		var wantRef *uint64
		if inb.TargetInboundID != nil && *inb.TargetInboundID > 0 {
			wantRef = inb.TargetInboundID
		}

		ob := obByOwner[inb.ID]
		if ob == nil {
			var taken int64
			if err := db.Model(&models.ServerOutbound{}).
				Where("server_id = ? AND tag = ?", serverID, wantTag).Count(&taken).Error; err != nil {
				return err
			}
			if taken > 0 {
				return fmt.Errorf("出站 tag %s 已被现有出站占用，无法为四层直通「%s」创建派生出口", wantTag, inb.Tag)
			}
			row := models.ServerOutbound{
				ServerID:       serverID,
				Tag:            wantTag,
				Protocol:       "freedom",
				SettingsJSON:   wantSettings,
				InboundRef:     wantRef,
				OwnerInboundID: inb.ID,
				Enabled:        inb.Enabled,
				Priority:       models.TunnelOutboundPriority,
				Remark:         tunnelDerivedOutboundRemark,
			}
			if err := db.Create(&row).Error; err != nil {
				return err
			}
			obByOwner[inb.ID] = &row
			continue
		}
		updates := map[string]any{}
		if ob.Tag != wantTag {
			updates["tag"] = wantTag
		}
		if ob.Protocol != "freedom" {
			updates["protocol"] = "freedom"
		}
		if ob.SettingsJSON != wantSettings {
			updates["settings_json"] = wantSettings
		}
		if ob.Enabled != inb.Enabled {
			updates["enabled"] = inb.Enabled
		}
		if ob.Priority != models.TunnelOutboundPriority {
			updates["priority"] = models.TunnelOutboundPriority
		}
		if (ob.InboundRef == nil && wantRef != nil) ||
			(ob.InboundRef != nil && wantRef == nil) ||
			(ob.InboundRef != nil && wantRef != nil && *ob.InboundRef != *wantRef) {
			updates["inbound_ref"] = wantRef
		}
		if len(updates) > 0 {
			if err := db.Model(&models.ServerOutbound{}).Where("id = ?", ob.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
	}

	// 第二遍：规则（需要第一遍之后的存活出站 tag 集合来判断"指向是否悬空"）
	tagSet := map[string]bool{}
	var allObs []models.ServerOutbound
	if err := db.Where("server_id = ?", serverID).Find(&allObs).Error; err != nil {
		return err
	}
	for i := range allObs {
		tagSet[allObs[i].Tag] = true
	}
	// 模板内置出站不在库内，但其 tag 是合法的规则指向
	for _, t := range []string{"api", "direct", "blocked", "blackhole"} {
		tagSet[t] = true
	}

	for i := range inbounds {
		inb := &inbounds[i]
		if !IsTunnelInbound(inb) || !live[inb.ID] {
			continue
		}
		wantTag := xray.TunnelOutboundTag(inb.Tag)
		rl := ruleByOwner[inb.ID]
		if rl == nil {
			row := models.ServerRoutingRule{
				ServerID:       serverID,
				OutboundTag:    wantTag,
				InboundTag:     inb.Tag,
				OwnerInboundID: inb.ID,
				Enabled:        inb.Enabled,
				Priority:       models.TunnelRulePriority,
				Remark:         tunnelDerivedRuleRemark,
			}
			if err := db.Create(&row).Error; err != nil {
				return err
			}
			ruleByOwner[inb.ID] = &row
			continue
		}
		updates := map[string]any{}
		if rl.InboundTag != inb.Tag {
			updates["inbound_tag"] = inb.Tag
		}
		if rl.Enabled != inb.Enabled {
			updates["enabled"] = inb.Enabled
		}
		// 指向归位：空或已悬空（改名后旧 out-<tag> 不复存在）才改；管理员改成别的存活出站则尊重
		if rl.OutboundTag != wantTag && (rl.OutboundTag == "" || !tagSet[rl.OutboundTag]) {
			updates["outbound_tag"] = wantTag
		}
		if len(updates) > 0 {
			if err := db.Model(&models.ServerRoutingRule{}).Where("id = ?", rl.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
	}

	// 第三遍：清理孤儿（直通已删除、已改类型、或变成草稿）
	for owner, ob := range obByOwner {
		if live[owner] {
			continue
		}
		if err := db.Delete(&models.ServerOutbound{}, ob.ID).Error; err != nil {
			return err
		}
	}
	for owner, rl := range ruleByOwner {
		if live[owner] {
			continue
		}
		if err := db.Delete(&models.ServerRoutingRule{}, rl.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

// LogTunnelDerivedReconcile 在配置生成等"不该因对账失败而拒绝服务"的路径上记录对账错误。
func LogTunnelDerivedReconcile(serverID uint64, err error) {
	if err != nil {
		log.Printf("[tunnel-derived] 直通派生对象对账失败（server=%d）: %v", serverID, err)
	}
}

// danglingTunnelRuleOutbound 检查"派生的直通规则"是否引用了缺失的出站，返回非空即说明有问题。
// 入参是即将下发的那两份集合（只含启用行），所以禁用出站/禁用规则不会误报。
func danglingTunnelRuleOutbound(outbounds []models.ServerOutbound, rules []models.ServerRoutingRule) string {
	tags := make(map[string]bool, len(outbounds))
	for i := range outbounds {
		tags[outbounds[i].Tag] = true
	}
	// 模板内置出站不在 server_outbounds 里，但规则指向它们是合法的
	for _, t := range []string{"api", "direct", "blocked", "blackhole"} {
		tags[t] = true
	}
	for i := range rules {
		r := &rules[i]
		if r.OwnerInboundID == 0 || r.OutboundTag == "" {
			continue
		}
		if !tags[r.OutboundTag] {
			return fmt.Sprintf("四层直通「%s」的转发规则引用的出站 %s 不存在（该直通会连不上，请检查直通与出站）", r.InboundTag, r.OutboundTag)
		}
	}
	return ""
}
