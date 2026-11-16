package models

import (
	"encoding/json"
	"log"
	"time"

	"gorm.io/gorm"
)

// 一次性清理「审计噪音」历史行（2026-11-16）。
//
// 背景：审计列表长期被三类条目淹没——逐次认证失败、只读预览、位次/布局这类碎操作。
// 代码侧已收口（middleware/audit.go 的 auditSkipRoutes + 退役 4 个手动打点，见 services/audit.go），
// 但库里已有的行不会自己消失。这里在启动迁移里做一次删除。
//
// 为什么不提升 schema 版本：只删行、不改结构也不改语义，旧面板读同一个库完全不受影响，
// 因此用 settings 标记（与本文件同目录 BackfillAuditTargets 的 audit_target_backfill_version 同套路）。
// 生产库停在 v5 也不受影响——本步骤只依赖 audit_logs / settings 两张早就存在的表。
const auditNoisePurgeKey = "audit_noise_purge_version"

// auditNoiseActions 历史噪音 action 清单。
//
// 与 middleware/audit.go 的 auditSkipRoutes 对应（那边是路由口径，这边是落库后的 action 口径），
// 另含 4 个已退役的手动打点：auth.login_failed / auth.2fa_failed / auth.logout / audit-logs.export。
// 新增跳过项时两处要一起改。
var auditNoiseActions = []string{
	"auth.login_failed",
	"auth.2fa_failed",
	"auth.logout",
	"audit-logs.export",
	"topology-layout",
	"notices.order",
	"plans.order",
	"servers.order",
	"permission-groups.order",
	"notifications.:key",
	"permission-groups.:id.preview-template",
	"xray.preview-config",
}

// PurgeAuditNoise 删除历史审计噪音行，跑一次（settings 标记，幂等）。
//
// 设计取舍：
//   - **失败不阻断启动**：清理只是可见性优化，不该让面板起不来；失败只记日志，
//     标记不写，下次启动重试（重复执行只会再删 0 行）。
//   - **只删列出的 action**：真实配置变更（inbounds/servers/users/settings…）一行不动。
//   - **不动 id 序列**：SQLite AUTOINCREMENT 继续递增，页面上的 #id 不会与历史混淆。
//   - **留一条自述审计**：删完写一条 `audit.noise_purge`（system），解释「记录为什么少了」。
func PurgeAuditNoise(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&AuditLog{}) || !db.Migrator().HasTable(&Setting{}) {
		return nil
	}
	if readSettingValue(db, auditNoisePurgeKey) == "1" {
		return nil
	}
	res := db.Where("action IN ?", auditNoiseActions).Delete(&AuditLog{})
	if res.Error != nil {
		log.Printf("[audit] 历史噪音清理失败（下次启动重试）：%v", res.Error)
		return nil // 非致命：检索质量优化，不影响正确性
	}
	deleted := res.RowsAffected

	// 只有真删了行才留自述行：全新库 / 已清过的库不该在审计列表里凭空多一条「清理了 0 条」。
	if deleted > 0 {
		detail, _ := json.Marshal(map[string]any{
			"v":       2,
			"message": "清理历史审计噪音",
			"rows":    deleted,
			"actions": auditNoiseActions,
		})
		if err := db.Create(&AuditLog{
			OperatorType: "system",
			Action:       "audit.noise_purge",
			Category:     AuditCategory("audit.noise_purge"),
			Status:       200,
			Detail:       string(detail),
			CreatedAt:    time.Now(),
		}).Error; err != nil {
			// 自述行写失败不影响清理结果，标记照写（否则每次启动都重跑一次删除）。
			log.Printf("[audit] 写入清理记录失败：%v", err)
		}
	}
	if err := upsertSetting(db, auditNoisePurgeKey, "1"); err != nil {
		log.Printf("[audit] 写清理标记失败（下次启动会重试，幂等）：%v", err)
		return nil
	}
	if deleted > 0 {
		log.Printf("[audit] 已清理 %d 条历史审计噪音", deleted)
	}
	return nil
}
