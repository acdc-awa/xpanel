package models

import (
	"time"

	"gorm.io/gorm"
)

// 计费周期与流量明细分桶的同轴不变量。
//
// 背景：traffic_logs 按小时分桶落库（services 的 trafficPeriodBucket 把上报时刻归一到整点），
// 而计费/配额口径按 `period_start >= user.traffic_cycle_start` 过滤。两套时间轴必须同轴：
// 周期起点若落在小时中间，该小时的整桶明细会因桶起点早于周期起点而被整段排除。最坏情况是
// 新用户注册后立刻买套餐——整个首个计费小时落在同一个桶里，排除后计费用量恒为 0（仪表盘仍
// 按原始口径显示流量），且整小时不扣配额，超额判定与节点摘除一并失效。
//
// 不变量（写入侧共同维持）：
//  1. traffic_cycle_start 恒为 UTC 整点（TrafficCycleAlign）；
//  2. 切换周期（购买/续费/重置）时，同一事务内清零该整点桶内已累计的计费字节
//     （ZeroBilledInCycleBucket），使「周期开始 ⇒ 已用量为 0」。
//
// 第 2 条不可省：对齐只把桶纳入统计范围，而该桶内「整点到切换时刻」的字节属于切换前
// （旧周期/未购套餐期），不清零会被算进新周期。切换之后的上报继续 upsert 累加到同一行，
// 因此新周期用量自切换时刻起精确起算。原始字节列（up_bytes/down_bytes）不动，仪表盘、
// 每日汇总与节点流量口径均不受影响。

// TrafficCycleAlign 把计费周期起点对齐到 UTC 整点。
func TrafficCycleAlign(t time.Time) time.Time { return t.UTC().Truncate(time.Hour) }

// TrafficCycleBucket 返回某周期起点所对齐的明细小时桶起点文本，用于按 period_start 精确匹配。
// 用库内存储格式（FormatDBTime）而非直接绑定 time.Time：SQLite 按文本字面量比较，
// 格式不一致会恒真/恒假。
func TrafficCycleBucket(cycleStart time.Time) string {
	return FormatDBTime(TrafficCycleAlign(cycleStart))
}

// ZeroBilledInCycleBucket 清零某用户在其周期起点所在小时桶内已累计的计费字节。
// 只动 billed 两列；调用方须与 traffic_cycle_start 的写入处于同一事务。
func ZeroBilledInCycleBucket(db *gorm.DB, userID uint64, cycleStart time.Time) error {
	return db.Model(&TrafficLog{}).
		Where("user_id = ? AND period_start = ?", userID, TrafficCycleBucket(cycleStart)).
		Updates(map[string]any{"billed_up": 0, "billed_down": 0}).Error
}
