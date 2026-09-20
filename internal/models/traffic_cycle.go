package models

import (
	"time"

	"gorm.io/gorm"
)

// 计费周期与流量明细分桶的同轴不变量。
//
// 背景（2026-09-17，即 v3 账期口径之前的规则）：traffic_logs 按小时分桶落库（services 的
// trafficPeriodBucket 把上报时刻归一到整点），而当时的计费/配额口径按
// `period_start >= user.traffic_cycle_start` 过滤。两套时间轴必须同轴：
// 周期起点若落在小时中间，该小时的整桶明细会因桶起点早于周期起点而被整段排除。最坏情况是
// 新用户注册后立刻买套餐——整个首个计费小时落在同一个桶里，排除后计费用量恒为 0（仪表盘仍
// 按原始口径显示流量），且整小时不扣配额，超额判定与节点摘除一并失效。
//
// 2026-09-20（审计 F3）起，归属口径改为**账期 ID**（`traffic_logs.cycle_id = 用户的
// traffic_cycle_id`，见 models.CycleUsageSQL）：节点在采集时刻打标，切换周期后新账期天然
// 从 0 起算，旧账期的行（含切换后迟到的增量）留在旧 ID 上，不再依赖时间轴比较。本条不变量
// 仍保留，用于两处：
//  1. cycle_id = 0 的存量行与旧 agent 行——它们没有账期标记，只能回退按时间轴归属；
//  2. traffic_cycle_start 仍参与保留策略安全线（TrafficSafeDeleteBefore）与展示。
//
// 不变量（写入侧共同维持）：
//  1. traffic_cycle_start 恒为 UTC 整点（TrafficCycleAlign）；
//  2. 切换周期时在同一事务内：递增 traffic_cycle_id（新账期归属的唯一依据）+ 清零该整点桶内
//     **cycle_id = 0** 的计费字节（只保护上面第 1 条的回退路径；带账期标记的行一律不动，
//     否则会把旧账期的账本抹掉——审计 §5「重置后的可追溯性」）。
//
// 第 2 条的清零范围收窄后，带账期标记的行不再被改写：新账期用量自 0 起算由 cycle_id 天然保证，
// 旧账期的字节完整留痕。原始字节列（up_bytes/down_bytes）始终不动。

// CycleMatchSQL 某条流量明细是否属于该用户「当前账期」的**判定谓词**（布尔表达式）。
//
// 计费口径的语义决策只在这一个常量里出现：需要分列求和的消费方（UserBilled 要分别取
// billed_up / billed_down）从它派生，而不是就地重写一遍谓词——口径漂移会让
// 「展示用量 / 配额判定 / 自动续费触发」三处互相矛盾，且改动时不会有人发现。
//
// 主路径按账期 ID 精确归属（审计 F3：节点在采集时刻打标，切换周期后旧账期的迟到增量
// 不会算进新套餐）；cycle_id = 0 的存量行与旧 agent 行没有账期标记，回退按时间轴归属
// （period_start >= traffic_cycle_start，即 2026-09-17 引入的整点对齐口径）。
//
// 消费方（同源）：services 的 UserBilled / FindViolators / filterValidUsers，
// billing 的 exhaustCandidates。
// 别名固定为 l（traffic_logs）、u（users）。
const CycleMatchSQL = `(l.cycle_id = u.traffic_cycle_id
			OR (l.cycle_id = 0 AND l.period_start >= u.traffic_cycle_start))`

// CycleUsageSQL 某条流量明细**计入当前账期的字节数**，由 CycleMatchSQL 派生。
// 用于 SUM(...) 或 CASE 表达式位置。
const CycleUsageSQL = `CASE WHEN ` + CycleMatchSQL + ` THEN l.billed_up + l.billed_down ELSE 0 END`

// TrafficCycleAlign 把计费周期起点对齐到 UTC 整点。
func TrafficCycleAlign(t time.Time) time.Time { return t.UTC().Truncate(time.Hour) }

// TrafficCycleBucket 返回某周期起点所对齐的明细小时桶起点文本，用于按 period_start 精确匹配。
// 用库内存储格式（FormatDBTime）而非直接绑定 time.Time：SQLite 按文本字面量比较，
// 格式不一致会恒真/恒假。
func TrafficCycleBucket(cycleStart time.Time) string {
	return FormatDBTime(TrafficCycleAlign(cycleStart))
}

// ZeroBilledInCycleBucket 清零某用户在其周期起点所在小时桶内 **cycle_id = 0** 行的计费字节。
// 只动 billed 两列，且只动没有账期标记的行（存量行/旧 agent 行）——它们按 period_start 回退
// 归属，不清零会把「整点到切换时刻」的旧消费算进新周期；带账期标记的行由 cycle_id 精确归属，
// 清零反而会抹掉旧账期账本。调用方须与 traffic_cycle_id 递增处于同一事务。
func ZeroBilledInCycleBucket(db *gorm.DB, userID uint64, cycleStart time.Time) error {
	return db.Model(&TrafficLog{}).
		Where("user_id = ? AND period_start = ? AND cycle_id = 0", userID, TrafficCycleBucket(cycleStart)).
		Updates(map[string]any{"billed_up": 0, "billed_down": 0}).Error
}
