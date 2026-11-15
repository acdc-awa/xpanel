package services

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// 服务器网卡周期流量（2026-11-15）。
//
// 数据源是 agent 上报的「自开机累计」物理网卡 Rx/Tx 字节（/proc/net/dev，已过滤 lo/docker/veth/tun/tap）。
// 本文件负责两件事：
//  1. 把「周期」解析成左闭右开的日期区间（业务时区，默认 Asia/Shanghai）；
//  2. 首次升级时从 node_reports 保留期一次性回填日聚合，让面板立刻能显示本周期用量。
//
// 日聚合本身由 nodegate 在指标帧上做差分维护（见 Hub.accumulateNIC / flushNIC）。

// SettingNICDailyBackfilled 网卡日聚合回填完成标记（settings 键，一次性）。
const SettingNICDailyBackfilled = "nic_traffic_daily_backfilled"

// NICCycle 服务器网卡流量的当前统计周期（左闭右开，边界为业务时区当天 00:00）。
type NICCycle struct {
	Start time.Time // 本周期起点
	End   time.Time // 下周期起点（不含）
	Label string    // 面向面板的说明：自然月 / 计费周期（月付，按到期日） / 自定义 30 天
	Mode  string    // natural_month / billing / custom_months / custom_days
}

// ResolveNICCycle 解析服务器的网卡流量统计周期。
//
// 口径优先级：
//  1. 自定义天数：nic_cycle_start 非空且 nic_cycle_days>0 → 从起点起每 N 天一个周期；
//  2. 自定义月周期：nic_cycle_start 非空、天数为 0 → 从起点起按 billing_cycle 的月数滚动
//     （计费周期识别不出/一次性时按 1 个月）；
//  3. 自动：billing_cycle 可识别且 expire_at 有效 → 以到期日（下一次续费日）为锚点滚动，
//     到期日已过也按同一节奏向前推；识别不出/一次性/无到期日 → 自然月。
//
// 允许锚点在未来：周期网格天然向过去延伸，当前窗口仍是「包含今天的那一格」。
// 所有日期边界按业务时区（services.BusinessLocation）取 00:00，和仪表盘「今日/本月」同轴。
func ResolveNICCycle(s *models.Server, now time.Time, loc *time.Location) NICCycle {
	if loc == nil {
		loc = time.UTC
	}
	now = now.In(loc)

	if anchor, ok := parseCalendarDate(s.NICCycleStart, loc); ok {
		if s.NICCycleDays > 0 {
			start, end := cycleByDays(anchor, s.NICCycleDays, now)
			return NICCycle{
				Start: start,
				End:   end,
				Label: fmt.Sprintf("自定义 %d 天", s.NICCycleDays),
				Mode:  "custom_days",
			}
		}
		months, monthsOK := BillingCycleMonths(s.BillingCycle)
		if !monthsOK {
			months = 1
		}
		start, end := cycleByMonths(anchor, months, now)
		label := "自定义月周期"
		if monthsOK {
			label = fmt.Sprintf("计费周期（%s）", strings.TrimSpace(s.BillingCycle))
		}
		return NICCycle{Start: start, End: end, Label: label, Mode: "custom_months"}
	}

	if months, ok := BillingCycleMonths(s.BillingCycle); ok {
		if anchor, ok := parseCalendarDate(s.ExpireAt, loc); ok {
			start, end := cycleByMonths(anchor, months, now)
			return NICCycle{
				Start: start,
				End:   end,
				Label: fmt.Sprintf("计费周期（%s，按到期日）", strings.TrimSpace(s.BillingCycle)),
				Mode:  "billing",
			}
		}
	}

	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	end := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, loc)
	return NICCycle{Start: start, End: end, Label: "自然月", Mode: "natural_month"}
}

// BillingCycleMonths 把 servers.billing_cycle 的中文/英文标签归一为月数。
//
// 只认明确写了周期长度的标签；「一次性/永久/买断」返回 ok=false（表示没有周期，调用方回退自然月）。
// 顺序敏感：「两年」「三年」「半年」必须先于「年」，「季」必须先于「月」。
func BillingCycleMonths(label string) (int, bool) {
	s := strings.ToLower(strings.TrimSpace(label))
	if s == "" {
		return 0, false
	}
	s = strings.NewReplacer(" ", "", "　", "", "-", "", "_", "", "个", "").Replace(s)

	switch {
	case strings.Contains(s, "一次"), strings.Contains(s, "永久"), strings.Contains(s, "终身"),
		strings.Contains(s, "买断"), strings.Contains(s, "lifetime"), strings.Contains(s, "onetime"):
		return 0, false
	case strings.Contains(s, "两年"), strings.Contains(s, "2年"), strings.Contains(s, "24月"),
		strings.Contains(s, "biennial"), strings.Contains(s, "2year"):
		return 24, true
	case strings.Contains(s, "三年"), strings.Contains(s, "3年"), strings.Contains(s, "36月"),
		strings.Contains(s, "triennial"), strings.Contains(s, "3year"):
		return 36, true
	case strings.Contains(s, "半年"), strings.Contains(s, "6月"), strings.Contains(s, "semiannual"),
		strings.Contains(s, "halfyear"), strings.Contains(s, "half-year"):
		return 6, true
	case strings.Contains(s, "季"), strings.Contains(s, "3月"), strings.Contains(s, "quarter"):
		return 3, true
	case strings.Contains(s, "年"), strings.Contains(s, "12月"), strings.Contains(s, "annual"),
		strings.Contains(s, "yearly"), strings.Contains(s, "year"):
		return 12, true
	case strings.Contains(s, "月"), strings.Contains(s, "month"):
		return 1, true
	}
	return 0, false
}

// parseCalendarDate 解析 YYYY-MM-DD（也容忍完整时间戳）为业务时区当天 00:00。
func parseCalendarDate(raw string, loc *time.Location) (time.Time, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, false
	}
	if i := strings.IndexAny(s, "T "); i > 0 {
		s = s[:i]
	}
	for _, layout := range []string{"2006-01-02", "2006-1-2"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc), true
		}
	}
	return time.Time{}, false
}

// cycleByDays 求包含 now 的固定天数周期（以 anchor 为网格起点）。
func cycleByDays(anchor time.Time, days int, now time.Time) (time.Time, time.Time) {
	if days <= 0 {
		days = 1
	}
	// 用「日历日序号」做整除，避免夏令时导致某天只有 23/25 小时而算错一格。
	diff := int(dayOrdinal(now) - dayOrdinal(anchor))
	k := floorDiv(diff, days)
	start := anchor.AddDate(0, 0, k*days)
	return start, start.AddDate(0, 0, days)
}

// cycleByMonths 求包含 now 的月周期（以 anchor 为网格起点，按 months 个月滚动）。
func cycleByMonths(anchor time.Time, months int, now time.Time) (time.Time, time.Time) {
	if months <= 0 {
		months = 1
	}
	k := monthsBetween(anchor, now) / months
	// 修正整数除法的截断与日期夹取（1/31 + 1 月 = 2/28）造成的边界偏差。
	for !cycleShift(anchor, (k+1)*months).After(now) {
		k++
	}
	for cycleShift(anchor, k*months).After(now) {
		k--
	}
	return cycleShift(anchor, k*months), cycleShift(anchor, (k+1)*months)
}

// cycleShift 从 anchor 起平移 months 个月，日号超出目标月天数时夹到月末。
func cycleShift(anchor time.Time, months int) time.Time {
	y, m, d := anchor.Date()
	first := time.Date(y, m+time.Month(months), 1, 0, 0, 0, 0, anchor.Location())
	lastDay := first.AddDate(0, 1, -1).Day()
	if d > lastDay {
		d = lastDay
	}
	return time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, anchor.Location())
}

func monthsBetween(a, b time.Time) int {
	return (b.Year()-a.Year())*12 + int(b.Month()-a.Month())
}

func dayOrdinal(t time.Time) int64 {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// NICDelta 计算累计计数器的一次增量；新值小于旧值视为计数器归零（节点重启/网卡重置），
// 把新值当作从 0 起算。方向独立判断：只重置了一个方向时另一方向不受影响。
func NICDelta(cur, prev uint64) int64 {
	if cur >= prev {
		return int64(cur - prev)
	}
	return int64(cur)
}

// BackfillNICDaily 首次升级时把 node_reports 保留期内的累计网卡字节反推成日聚合。
//
// 为什么需要：日表是从本版本起才开始累计的；不回填的话，升级当天所有服务器都只能显示
// 「从今天起」的流量，本周期剩余时间都看不到接近真实的值。node_reports 里有 1 分钟粒度的
// 累计字节，足够把当前周期补到「准实时」。
//
// 语义：
//   - 首次调用且日表为空时执行；写完落 settings 标记，之后重复调用为 no-op；
//   - 整个回填 + 标记在**一个事务**里完成：失败回滚，下次启动重试，不会留下半份数据；
//   - 日表非空但标记缺失（标记被手工删过/从旧备份恢复）时不覆盖，只补标记；
//   - 0/0 的行视为「该帧没上报网卡数据」（旧 agent/非 Linux 节点/确实空闲），跳过且不建基线；
//     否则下一次差分会把「自开机累计」整段当成当天流量。
//
// 调用方应在 AutoMigrate 之后、nodegate.NewHub 之前执行（此时没有并发写者）。
func BackfillNICDaily(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if strings.TrimSpace(GetSetting(db, SettingNICDailyBackfilled)) != "" {
		return nil
	}
	if !db.Migrator().HasTable(&models.ServerNICTrafficDaily{}) {
		return nil
	}
	var existing int64
	if err := db.Model(&models.ServerNICTrafficDaily{}).Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return SetSetting(db, SettingNICDailyBackfilled, "1")
	}

	loc := BusinessLocation(db)
	var servers []models.Server
	if err := db.Unscoped().Order("id ASC").Find(&servers).Error; err != nil {
		return err
	}
	rows := make([]models.ServerNICTrafficDaily, 0, len(servers)*32)
	for i := range servers {
		var reports []models.NodeReport
		if err := db.Select("reported_at", "rx_bytes", "tx_bytes").
			Where("server_id = ?", servers[i].ID).
			Order("reported_at ASC").
			Find(&reports).Error; err != nil {
			return err
		}
		for _, row := range aggregateNICReports(reports, loc) {
			row.ServerID = servers[i].ID
			rows = append(rows, row)
		}
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&models.ServerNICTrafficDaily{}).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			if err := tx.CreateInBatches(rows, 500).Error; err != nil {
				return err
			}
		}
		return SetSetting(tx, SettingNICDailyBackfilled, "1")
	})
}

// aggregateNICReports 把一组按时间升序的 node_reports 反推成按业务日的增量。
// 返回行按日期升序；last_raw_* 取该日最后一帧的累计值（供主控重启后继续差分）。
func aggregateNICReports(reports []models.NodeReport, loc *time.Location) []models.ServerNICTrafficDaily {
	type dayAcc struct {
		rx, tx         int64
		lastRx, lastTx uint64
	}
	days := make(map[string]*dayAcc, 32)
	order := make([]string, 0, 32)

	var havePrev bool
	var prevRx, prevTx uint64
	for _, r := range reports {
		if r.RxBytes == 0 && r.TxBytes == 0 {
			continue
		}
		date := r.ReportedAt.In(loc).Format("2006-01-02")
		a := days[date]
		if a == nil {
			a = &dayAcc{}
			days[date] = a
			order = append(order, date)
		}
		if !havePrev {
			prevRx, prevTx = r.RxBytes, r.TxBytes
			havePrev = true
			a.lastRx, a.lastTx = r.RxBytes, r.TxBytes
			continue
		}
		a.rx += NICDelta(r.RxBytes, prevRx)
		a.tx += NICDelta(r.TxBytes, prevTx)
		prevRx, prevTx = r.RxBytes, r.TxBytes
		a.lastRx, a.lastTx = r.RxBytes, r.TxBytes
	}

	rows := make([]models.ServerNICTrafficDaily, 0, len(order))
	for _, date := range order {
		a := days[date]
		rows = append(rows, models.ServerNICTrafficDaily{
			Date:      date,
			RxBytes:   a.rx,
			TxBytes:   a.tx,
			LastRawRx: a.lastRx,
			LastRawTx: a.lastTx,
		})
	}
	return rows
}

// NICQuotaDirection 归一额度口径：total（双向合计）/ rx（仅下行）/ tx（仅上行）/ max（双向取较大）。
func NICQuotaDirection(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "rx":
		return "rx"
	case "tx":
		return "tx"
	case "max":
		return "max"
	default:
		return "total"
	}
}

// NICQuotaUsed 按额度口径取「计费用量」。
func NICQuotaUsed(direction string, rx, tx int64) int64 {
	switch NICQuotaDirection(direction) {
	case "rx":
		return rx
	case "tx":
		return tx
	case "max":
		if tx > rx {
			return tx
		}
		return rx
	default:
		return rx + tx
	}
}

// NICQuotaState 额度状态：quota<=0 表示不限；<80% ok，>=80% warning，>=100% exceeded。
func NICQuotaState(quotaBytes, usedBytes int64) string {
	if quotaBytes <= 0 {
		return ""
	}
	switch {
	case usedBytes >= quotaBytes:
		return "exceeded"
	case float64(usedBytes) >= float64(quotaBytes)*0.8:
		return "warning"
	default:
		return "ok"
	}
}

// NICQuotaPercent 已用百分比（保留 1 位小数；超额度可以 >100）。
func NICQuotaPercent(quotaBytes, usedBytes int64) float64 {
	if quotaBytes <= 0 {
		return 0
	}
	p := float64(usedBytes) * 100 / float64(quotaBytes)
	return float64(int64(p*10+0.5)) / 10
}

// NICGBToBytes 把 GB 额度换算为字节（仅用于展示判定，配额上限在 API 校验）。
func NICGBToBytes(gb int64) int64 {
	if gb <= 0 {
		return 0
	}
	return gb * 1024 * 1024 * 1024
}

// FormatNICCycleRange 面板/日志用的周期区间（YYYY-MM-DD 至 YYYY-MM-DD，结束日不含）。
func FormatNICCycleRange(c NICCycle) string {
	return c.Start.Format("2006-01-02") + " 至 " + c.End.Format("2006-01-02")
}
