package api

import (
	"log"

	"github.com/acdc-awa/xpanel/internal/models"
)

// dependentServerIDs 返回「消费这些入站」的其他服务器（按 server_id 去重）。
//
// 两类消费关系都只在**生成期**现场解析目标，引用方库里没有任何字段跟着变：
//  1. 出站 InboundRef 指向该入站（机间链式代理）：地址/端口/凭据/传输层/证书 pin
//     全部来自目标入站与其所在服务器；
//  2. 四层直通 TargetInboundID 指向该入站（转发落地）：redirect 目标是落地入站的
//     「服务器地址 + 端口」，派生出口由 EnsureTunnelDerivedObjects 在 Generate 内重算。
//
// 只统计启用行：生成侧本就跳过停用出站/直通，重推只会白让对端重启一次 xray。
func (d *Deps) dependentServerIDs(inboundIDs []uint64) []uint64 {
	if len(inboundIDs) == 0 {
		return nil
	}
	seen := make(map[uint64]bool, len(inboundIDs))
	var out []uint64
	add := func(serverID uint64) {
		if serverID == 0 || seen[serverID] {
			return
		}
		seen[serverID] = true
		out = append(out, serverID)
	}
	var refs []models.ServerOutbound
	if err := d.DB.Select("server_id").Distinct("server_id").
		Where("inbound_ref IN ? AND enabled = ?", inboundIDs, true).Find(&refs).Error; err == nil {
		for _, ob := range refs {
			add(ob.ServerID)
		}
	}
	var tunnels []models.Inbound
	if err := d.DB.Select("server_id").Distinct("server_id").
		Where("target_inbound_id IN ? AND enabled = ?", inboundIDs, true).Find(&tunnels).Error; err == nil {
		for _, inb := range tunnels {
			add(inb.ServerID)
		}
	}
	return out
}

// enqueueDependentServers 重新生成并推送「消费这些入站」的服务器配置。
// excludeServerID 非 0 时跳过它（调用方已经重推过该服务器，重复推送会多一次 xray 重启）。
// 依赖方失败只记日志：源入站已保存成功，不能让某台对端生成失败把整次编辑变成 500。
func (d *Deps) enqueueDependentServers(inboundIDs []uint64, excludeServerID uint64) {
	for _, serverID := range d.dependentServerIDs(inboundIDs) {
		if serverID == excludeServerID {
			continue
		}
		if err := d.enqueueConfig(serverID); err != nil {
			log.Printf("config fanout: 依赖入站 %v 的服务器 %d 生成/推送失败: %v", inboundIDs, serverID, err)
		}
	}
}
