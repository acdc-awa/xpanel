package models

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// 审计目标类型（AuditLog.TargetType）单源映射。
//
// 目标类型来自路由的第一段（管理端 action 由路径推导，形如 servers.:id.outbounds），
// 业务手打点的场景由调用方显式传（见 services.AuditService.LogEvent）。
// 未登记的模块返回空串 = 该操作没有明确目标对象（settings / data / topology-layout 等）。
var auditTargetTypeBySegment = map[string]string{
	"users":             "user",
	"servers":           "server",
	"inbounds":          "inbound",
	"plans":             "plan",
	"orders":            "order",
	"certs":             "cert",
	"ca-certs":          "ca_cert",
	"access-points":     "access_point",
	"permission-groups": "permission_group",
	"sub-templates":     "sub_template",
	"notices":           "notice",
	"gift-cards":        "gift_card",
	"invitations":       "invitation",
}

// AuditTargetType 由 action（路由推导或手打点前缀）判定目标类型；空串表示无目标对象。
func AuditTargetType(action string) string {
	a := strings.ToLower(strings.TrimSpace(action))
	if a == "" {
		return ""
	}
	if i := strings.IndexAny(a, ".:"); i >= 0 {
		a = a[:i]
	}
	return auditTargetTypeBySegment[a]
}

// AuditTargetTypeFromPath 由请求路径判定目标类型（/api/v1/admin/servers/12 → server）。
func AuditTargetTypeFromPath(path string) string {
	p := strings.TrimSpace(path)
	if p == "" {
		return ""
	}
	segs := strings.Split(strings.Trim(p, "/"), "/")
	// 跳过 api v1 admin 前缀；只认第一段业务模块。
	start := 0
	for i, s := range segs {
		if s == "admin" {
			start = i + 1
			break
		}
	}
	if start >= len(segs) {
		return ""
	}
	return auditTargetTypeBySegment[strings.ToLower(segs[start])]
}

// auditBackfillDoneKey 目标快照回填的完成标记（settings 键）。
const auditBackfillDoneKey = "audit_target_backfill_version"

// BackfillAuditTargets 回填存量审计行的目标与操作人快照（v5，仅 legacy 行）。
//
// 数据来源全是既有 detail 信封里的字段（v2 信封：path / params / target / status），
// 不新增信息，只是把「埋在 JSON 里、无法索引」的东西提到列上：
//   - target_type ← detail.path 的第一段
//   - target_id   ← detail.params.id
//   - target_name ← detail.target（写入时注册表预读的显示名）
//   - status      ← detail.status
//   - operator_name ← users.email（走 Unscoped：软删用户的名称仍要能显示）
//
// 只在首次 v5 启动跑一次（settings 标记），失败不阻断启动——检索降级不影响数据正确性。
// 已硬删用户的历史行拿不到名字（留空），这是设计内的边界。
func BackfillAuditTargets(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&AuditLog{}) || !db.Migrator().HasTable(&Setting{}) {
		return nil
	}
	if readSettingValue(db, auditBackfillDoneKey) == "1" {
		return nil
	}
	const batch = 1000
	var cursor uint64
	var filled int
	for {
		var rows []AuditLog
		if err := db.Select("id", "operator_id", "detail", "operator_name", "target_type", "target_id", "target_name", "status").
			Where("id > ?", cursor).Order("id ASC").Limit(batch).Find(&rows).Error; err != nil {
			return fmt.Errorf("读取审计日志失败: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		names, err := userEmailsByID(db, rows)
		if err != nil {
			return err
		}
		for _, row := range rows {
			cursor = row.ID
			updates := auditBackfillUpdates(row, names[row.OperatorID])
			if len(updates) == 0 {
				continue
			}
			if err := db.Model(&AuditLog{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("回填审计日志 %d 失败: %w", row.ID, err)
			}
			filled++
		}
	}
	if err := upsertSetting(db, auditBackfillDoneKey, "1"); err != nil {
		return err
	}
	if filled > 0 {
		log.Printf("schema v5: 已回填 %d 条审计日志的目标/操作人快照", filled)
	}
	return nil
}

// auditBackfillUpdates 计算单行的回填列（只补空值，不覆盖已有快照）。
func auditBackfillUpdates(row AuditLog, operatorName string) map[string]any {
	updates := map[string]any{}
	if row.OperatorName == "" && operatorName != "" {
		updates["operator_name"] = operatorName
	}
	var env struct {
		Path   string            `json:"path"`
		Params map[string]string `json:"params"`
		Target string            `json:"target"`
		Status int               `json:"status"`
	}
	if strings.TrimSpace(row.Detail) != "" && json.Unmarshal([]byte(row.Detail), &env) == nil {
		if row.TargetType == "" {
			if tt := AuditTargetTypeFromPath(env.Path); tt != "" {
				updates["target_type"] = tt
			}
		}
		if row.TargetID == 0 && env.Params != nil {
			if id, err := strconv.ParseUint(strings.TrimSpace(env.Params["id"]), 10, 64); err == nil && id > 0 {
				updates["target_id"] = id
			}
		}
		if row.TargetName == "" && strings.TrimSpace(env.Target) != "" {
			updates["target_name"] = strings.TrimSpace(env.Target)
		}
		if row.Status == 0 && env.Status > 0 {
			updates["status"] = env.Status
		}
	}
	return updates
}

// userEmailsByID 批量取操作人邮箱（含软删行；硬删的用户查不到，留空）。
func userEmailsByID(db *gorm.DB, rows []AuditLog) (map[uint64]string, error) {
	ids := make([]uint64, 0, len(rows))
	seen := make(map[uint64]bool, len(rows))
	for _, r := range rows {
		if r.OperatorID == 0 || seen[r.OperatorID] {
			continue
		}
		seen[r.OperatorID] = true
		ids = append(ids, r.OperatorID)
	}
	out := make(map[uint64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var users []User
	if err := db.Unscoped().Select("id", "email").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, fmt.Errorf("读取操作人邮箱失败: %w", err)
	}
	for _, u := range users {
		out[u.ID] = u.Email
	}
	return out, nil
}
