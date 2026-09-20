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
//  1. 计费安全线——明细是配额判定/订阅已用展示/仪表盘 Top10 的唯一数据源，删除早于全员最早
//     周期起点（min traffic_cycle_start）的行不影响任何口径；
//  2. 聚合安全线——AggDaily 每 5 分钟重算最近 7 天（滚动窗口）：窗口内某日被部分删除时，
//     该日汇总会被缩小值覆盖；整日删光则汇总行脱离 GROUP BY、残留旧值不再更新。
//     两种情况都会造成 daily 与明细口径不一致，故删除上界还须早于 now-7d 的日界。
//
// ⚠ 第 1 条的成立依据在 v3 账期口径（2026-09-20）之后**不再是**「period_start ≥ traffic_cycle_start」，
// 而是下面这条隐式不变量。改动本函数或任何删除 traffic_logs 的逻辑之前，必须先确认它仍然成立：
//
//	归属判据是 l.cycle_id = u.traffic_cycle_id（models.CycleMatchSQL），与 period_start 无关；
//	而带「当前账期」标记的行只可能在主控递增该用户账期**之后**产生——节点经 sync_users 得知新
//	账期，推送发生在事务提交之后；period_start 取节点的发送时刻（对齐整点），故必然 ≥ cycle_start，
//	于是必然晚于 min(cycle_start)，落在安全线之内。
//
// 这条链依赖节点与主控的时钟一致：节点时钟慢于主控超过 1 小时时，带当前账期的行会落进更旧的
// 小时桶、跌到安全线之下而被删除——表现为静默少计，且该行连同账期归属一起丢失。要彻底摆脱这个
// 依赖，安全线须改按账期计算（而非按时间轴），那是一次口径变更，不在本函数范围内。
//
// 查询失败返回空串（调用方应拒绝清理）。
func TrafficSafeDeleteBefore(db *gorm.DB, now time.Time) string {
	// 按业务时区切天，与仪表盘/每日汇总口径一致（返回值是业务日期字符串）。
	loc := BusinessLocation(db)
	toDate := func(t time.Time) time.Time {
		y, m, d := t.In(loc).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	agg := toDate(now.AddDate(0, 0, -aggWindowDays))

	// users 表不存在（全新库/仅迁移了部分表）时不存在任何计费周期，仅受每日聚合窗口约束。
	if !db.Migrator().HasTable(&models.User{}) {
		return agg.Format("2006-01-02")
	}
	var starts []sql.NullTime
	if err := db.Model(&models.User{}).Pluck("traffic_cycle_start", &starts).Error; err != nil {
		return "" // 查询失败：无法校验，调用方应拒绝清理
	}
	var minCycle time.Time
	for _, s := range starts {
		if s.Valid && !s.Time.IsZero() && (minCycle.IsZero() || s.Time.Before(minCycle)) {
			minCycle = s.Time
		}
	}
	if !minCycle.IsZero() {
		if c := toDate(minCycle); c.Before(agg) {
			agg = c
		}
	}
	return agg.Format("2006-01-02")
}
