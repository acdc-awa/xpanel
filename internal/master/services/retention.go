// 数据保留天数设置（数据管理页）：traffic_logs / node_reports / audit_logs
// 三张日志表的自动清理周期，替代 ISSUE-09 的硬编码常量（常量保留为缺省值）。
package services

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// 设置键（settings 表 key）。
const (
	SettingRetentionTrafficDays = "retention_traffic_days"      // traffic_logs 保留天数（默认 90）
	SettingRetentionNodeDays    = "retention_node_reports_days" // node_reports 保留天数（默认 30）
	SettingRetentionAuditDays   = "retention_audit_days"        // audit_logs 保留天数（默认 180）
)

// clampRetentionDays 解析天数设置：空/非法回退默认值，收敛到 1–3650 天。
func clampRetentionDays(v string, def int) int {
	n := def
	if v != "" {
		if p, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && p > 0 {
			n = p
		}
	}
	if n < 1 {
		n = 1
	}
	if n > 3650 {
		n = 3650
	}
	return n
}

// RetentionTrafficDays traffic_logs 保留天数。
func RetentionTrafficDays(db *gorm.DB) int {
	return clampRetentionDays(GetSetting(db, SettingRetentionTrafficDays), trafficLogRetentionDays)
}

// RetentionNodeReportDays node_reports 保留天数。
func RetentionNodeReportDays(db *gorm.DB) int {
	return clampRetentionDays(GetSetting(db, SettingRetentionNodeDays), nodeReportRetentionDays)
}

// RetentionAuditDays audit_logs 保留天数。
func RetentionAuditDays(db *gorm.DB) int {
	return clampRetentionDays(GetSetting(db, SettingRetentionAuditDays), auditLogRetentionDays)
}

// RetentionSettingsGroup 读取「数据保留」分组（DB 直读；返回归一化后的生效值）。
func RetentionSettingsGroup(db *gorm.DB) map[string]string {
	return map[string]string{
		SettingRetentionTrafficDays: strconv.Itoa(RetentionTrafficDays(db)),
		SettingRetentionNodeDays:    strconv.Itoa(RetentionNodeReportDays(db)),
		SettingRetentionAuditDays:   strconv.Itoa(RetentionAuditDays(db)),
	}
}

// SaveRetentionSettingsGroup 保存「数据保留」分组（仅接受白名单键，值域 1–3650 天）。
func SaveRetentionSettingsGroup(db *gorm.DB, vals map[string]string) error {
	allowed := map[string]bool{
		SettingRetentionTrafficDays: true,
		SettingRetentionNodeDays:    true,
		SettingRetentionAuditDays:   true,
	}
	parsed := map[string]string{}
	for k, v := range vals {
		if !allowed[k] {
			return errors.New("未知设置键: " + k)
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 1 || n > 3650 {
			return errors.New("保留天数需为 1–3650 的整数")
		}
		parsed[k] = strconv.Itoa(n)
	}
	for k, v := range parsed {
		if err := SetSetting(db, k, v); err != nil {
			return err
		}
	}
	return nil
}

// TrafficSafeDeleteBefore 返回 traffic_logs 明细可安全清理的最晚「before」日期
// （YYYY-MM-DD，删除动作删 period_start < before 00:00 的行，before 当日及以后保留）。
// 两条安全线取较早者：
//  1. 计费安全线——明细是配额判定/订阅已用展示/仪表盘 Top10 的唯一数据源，但三者均只统计
//     period_start ≥ 各自用户 traffic_cycle_start 的行，删除早于全员最早周期起点的行不影响任何口径；
//  2. 聚合安全线——AggDaily 每 5 分钟重算最近 7 天（滚动窗口）：窗口内某日被部分删除时，
//     该日汇总会被缩小值覆盖；整日删光则汇总行脱离 GROUP BY、残留旧值不再更新。
//     两种情况都会造成 daily 与明细口径不一致，故删除上界还须早于 now-7d 的日界。
//
// 查询失败返回空串（调用方应拒绝清理）。
func TrafficSafeDeleteBefore(db *gorm.DB, now time.Time) string {
	var starts []sql.NullTime
	if err := db.Model(&models.User{}).Pluck("traffic_cycle_start", &starts).Error; err != nil {
		return ""
	}
	var minCycle time.Time
	for _, s := range starts {
		if s.Valid && !s.Time.IsZero() && (minCycle.IsZero() || s.Time.Before(minCycle)) {
			minCycle = s.Time
		}
	}
	loc := now.Location()
	toDate := func(t time.Time) time.Time {
		y, m, d := t.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	agg := toDate(now.AddDate(0, 0, -aggWindowDays))
	if !minCycle.IsZero() {
		if c := toDate(minCycle); c.Before(agg) {
			agg = c
		}
	}
	return agg.Format("2006-01-02")
}
