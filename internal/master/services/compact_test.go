package services

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func newCompactTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.TrafficLog{}, &models.NodeReport{}, &models.Setting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// 同一小时内的多行应归并为一行，合计与计费口径不变，当前小时不动，且可重复执行。
func TestCompactTrafficLogsMergesHourBuckets(t *testing.T) {
	db := newCompactTestDB(t)
	now := time.Date(2026, 8, 16, 5, 0, 0, 0, time.UTC)
	h2 := time.Date(2026, 8, 16, 2, 0, 0, 0, time.UTC)
	h3 := time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC)

	rows := []models.TrafficLog{
		{UserID: 1, InboundID: 7, UpBytes: 100, DownBytes: 200, BilledUp: 10, BilledDown: 20, PeriodStart: h3.Add(1 * time.Minute)},
		{UserID: 1, InboundID: 7, UpBytes: 300, DownBytes: 400, BilledUp: 30, BilledDown: 40, PeriodStart: h3.Add(20 * time.Minute)},
		{UserID: 1, InboundID: 7, UpBytes: 500, DownBytes: 600, BilledUp: 50, BilledDown: 60, PeriodStart: h3.Add(59 * time.Minute)},
		{UserID: 2, InboundID: 7, UpBytes: 1, DownBytes: 2, BilledUp: 1, BilledDown: 2, PeriodStart: h3.Add(30 * time.Minute)},
		{UserID: 1, InboundID: 8, UpBytes: 11, DownBytes: 12, BilledUp: 11, BilledDown: 12, PeriodStart: h2.Add(15 * time.Minute)},
		{UserID: 1, InboundID: 8, UpBytes: 13, DownBytes: 14, BilledUp: 13, BilledDown: 14, PeriodStart: h2.Add(45 * time.Minute)},
		// 当前小时（未结束）不应被触碰
		{UserID: 1, InboundID: 9, UpBytes: 5, DownBytes: 5, BilledUp: 5, BilledDown: 5, PeriodStart: now.Add(10 * time.Minute)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 合计基线（用于校验口径不变）
	type trafficSums struct {
		Up   int64 `gorm:"column:up"`
		Down int64 `gorm:"column:down"`
		BU   int64 `gorm:"column:bu"`
		BD   int64 `gorm:"column:bd"`
	}
	baseline := trafficSums{100 + 300 + 500 + 1 + 11 + 13 + 5, 200 + 400 + 600 + 2 + 12 + 14 + 5, 10 + 30 + 50 + 1 + 11 + 13 + 5, 20 + 40 + 60 + 2 + 12 + 14 + 5}

	before, after, err := CompactTrafficLogs(db, now)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if before != int64(len(rows)) {
		t.Fatalf("before = %d, want %d", before, len(rows))
	}
	// h3: 4 行 → 2 行（user1 与 user2 各一行）；h2: 2 行 → 1 行；当前小时 1 行不动 → 4
	if after != 4 || before-after != 3 {
		t.Fatalf("after = %d (removed %d), want 4 (removed 3)", after, before-after)
	}

	var h3Rows []models.TrafficLog
	if err := db.Where("user_id = ? AND inbound_id = ? AND period_start = ?", 1, 7, h3).Find(&h3Rows).Error; err != nil {
		t.Fatalf("query h3: %v", err)
	}
	if len(h3Rows) != 1 {
		t.Fatalf("h3 user1 rows = %d, want 1", len(h3Rows))
	}
	got := h3Rows[0]
	if got.UpBytes != 900 || got.DownBytes != 1200 || got.BilledUp != 90 || got.BilledDown != 120 {
		t.Fatalf("h3 merged = %+v, want up=900 down=1200 bu=90 bd=120", got)
	}

	var current models.TrafficLog
	if err := db.Where("inbound_id = ?", 9).First(&current).Error; err != nil {
		t.Fatalf("current hour row: %v", err)
	}
	if !current.PeriodStart.Equal(now.Add(10 * time.Minute)) {
		t.Fatalf("current hour row mutated: %v", current.PeriodStart)
	}

	// 口径校验：合计不变
	var agg trafficSums
	if err := db.Model(&models.TrafficLog{}).
		Select("COALESCE(SUM(up_bytes),0) AS up, COALESCE(SUM(down_bytes),0) AS down, COALESCE(SUM(billed_up),0) AS bu, COALESCE(SUM(billed_down),0) AS bd").
		Scan(&agg).Error; err != nil {
		t.Fatalf("sum: %v", err)
	}
	if agg != baseline {
		t.Fatalf("totals changed: got %+v want %+v", agg, baseline)
	}

	// 幂等：再跑一次应无删减
	_, after2, err := CompactTrafficLogs(db, now)
	if err != nil {
		t.Fatalf("compact #2: %v", err)
	}
	if after2 != after {
		t.Fatalf("second compact changed rows: %d → %d", after, after2)
	}
}

// 心跳：保留期内按分钟抽稀（每分钟留一行），超期行删除，当前分钟不动。
func TestCompactNodeReportsDownsamplesAndTrims(t *testing.T) {
	db := newCompactTestDB(t)
	now := time.Date(2026, 8, 16, 12, 0, 30, 0, time.UTC)
	day := time.Date(2026, 8, 16, 8, 0, 0, 0, time.UTC)

	rows := []models.NodeReport{
		{ServerID: 1, ReportedAt: day.Add(5 * time.Second)},
		{ServerID: 1, ReportedAt: day.Add(35 * time.Second)},
		{ServerID: 1, ReportedAt: day.Add(59 * time.Second)},
		{ServerID: 1, ReportedAt: day.Add(1*time.Minute + 10*time.Second)},
		{ServerID: 2, ReportedAt: day.Add(5 * time.Second)},
		{ServerID: 2, ReportedAt: day.Add(40 * time.Second)},
		// 超出默认 30 天保留期 → 应删除
		{ServerID: 1, ReportedAt: now.AddDate(0, 0, -40)},
		// 当前分钟 → 不抽稀
		{ServerID: 1, ReportedAt: now.Add(-5 * time.Second)},
		{ServerID: 1, ReportedAt: now.Add(-1 * time.Second)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	before, after, err := CompactNodeReports(db, now)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if before != int64(len(rows)) {
		t.Fatalf("before = %d, want %d", before, len(rows))
	}
	// server1: 分钟0 三行→1，分钟1 一行，超期删1，当前分钟两行保留 → 1+1+2 = 4
	// server2: 分钟0 两行→1 → 1
	if after != 5 {
		t.Fatalf("after = %d, want 5", after)
	}

	var kept []models.NodeReport
	if err := db.Where("server_id = ? AND reported_at >= ? AND reported_at < ?", 1, day, day.Add(time.Minute)).Find(&kept).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(kept) != 1 || !kept[0].ReportedAt.Equal(rows[0].ReportedAt) {
		t.Fatalf("minute bucket not reduced to earliest row: %+v", kept)
	}

	var outdated int64
	if err := db.Model(&models.NodeReport{}).Where("reported_at < ?", now.AddDate(0, 0, -30)).Count(&outdated).Error; err != nil {
		t.Fatalf("count outdated: %v", err)
	}
	if outdated != 0 {
		t.Fatalf("outdated rows remain: %d", outdated)
	}

	// 幂等
	_, after2, err := CompactNodeReports(db, now)
	if err != nil {
		t.Fatalf("compact #2: %v", err)
	}
	if after2 != after {
		t.Fatalf("second compact changed rows: %d → %d", after, after2)
	}
}

// 分级降采样：近 6h 按分钟、6–24h 按 10 分钟、24h 以上按小时各留最早一行。
func TestCompactNodeReportsTieredDownsample(t *testing.T) {
	db := newCompactTestDB(t)
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)

	tier1 := now.Add(-2 * time.Hour)  // 龄 2h → 1 分钟桶
	tier2 := now.Add(-8 * time.Hour)  // 龄 8h → 10 分钟桶
	tier3 := now.Add(-72 * time.Hour) // 龄 72h → 1 小时桶

	rows := []models.NodeReport{
		// tier1：同分钟两行 → 1
		{ServerID: 3, ReportedAt: tier1},
		{ServerID: 3, ReportedAt: tier1.Add(30 * time.Second)},
		// tier2：同 10 分钟桶三行 → 1；下一桶一行 → 保留
		{ServerID: 3, ReportedAt: tier2.Add(1 * time.Minute)},
		{ServerID: 3, ReportedAt: tier2.Add(2 * time.Minute)},
		{ServerID: 3, ReportedAt: tier2.Add(9 * time.Minute)},
		{ServerID: 3, ReportedAt: tier2.Add(11 * time.Minute)},
		// tier3：同小时桶两行 → 1；下一小时一行 → 保留
		{ServerID: 3, ReportedAt: tier3.Add(1 * time.Minute)},
		{ServerID: 3, ReportedAt: tier3.Add(59 * time.Minute)},
		{ServerID: 3, ReportedAt: tier3.Add(65 * time.Minute)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	_, after, err := CompactNodeReports(db, now)
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	// 2(tier1→1) + 4(tier2→2) + 3(tier3→2) = 5
	if after != 5 {
		t.Fatalf("after = %d, want 5", after)
	}

	countIn := func(lo, hi time.Time) int64 {
		t.Helper()
		var n int64
		if err := db.Model(&models.NodeReport{}).
			Where("server_id = ? AND reported_at >= ? AND reported_at < ?", 3, lo, hi).
			Count(&n).Error; err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}
	if n := countIn(tier1.Truncate(time.Minute), tier1.Truncate(time.Minute).Add(time.Minute)); n != 1 {
		t.Fatalf("tier1 minute bucket = %d, want 1", n)
	}
	if n := countIn(tier2.Truncate(10*time.Minute), tier2.Truncate(10*time.Minute).Add(10*time.Minute)); n != 1 {
		t.Fatalf("tier2 10min bucket = %d, want 1", n)
	}
	if n := countIn(tier3.Truncate(time.Hour), tier3.Truncate(time.Hour).Add(time.Hour)); n != 1 {
		t.Fatalf("tier3 hour bucket = %d, want 1", n)
	}
}

// 周期起点落在小时桶内的用户：该小时不合并，计费口径（period_start >= cycle_start）逐行不变。
func TestCompactTrafficHourProtectsCycleStartHour(t *testing.T) {
	db := newCompactTestDB(t)
	h := time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC)
	cycle := h.Add(30 * time.Minute) // 用户 1 的计费周期起点落在 03:30

	rows := []models.TrafficLog{
		// 受保护用户 1：三行，周期起点 03:30 在其中
		{UserID: 1, InboundID: 7, UpBytes: 100, DownBytes: 200, BilledUp: 100, BilledDown: 200, PeriodStart: h.Add(10 * time.Minute)},
		{UserID: 1, InboundID: 7, UpBytes: 300, DownBytes: 400, BilledUp: 300, BilledDown: 400, PeriodStart: h.Add(35 * time.Minute)},
		{UserID: 1, InboundID: 7, UpBytes: 500, DownBytes: 600, BilledUp: 500, BilledDown: 600, PeriodStart: h.Add(50 * time.Minute)},
		// 普通用户 2：三行 → 合并为一行
		{UserID: 2, InboundID: 7, UpBytes: 1, DownBytes: 2, BilledUp: 1, BilledDown: 2, PeriodStart: h.Add(5 * time.Minute)},
		{UserID: 2, InboundID: 7, UpBytes: 3, DownBytes: 4, BilledUp: 3, BilledDown: 4, PeriodStart: h.Add(25 * time.Minute)},
		{UserID: 2, InboundID: 7, UpBytes: 5, DownBytes: 6, BilledUp: 5, BilledDown: 6, PeriodStart: h.Add(55 * time.Minute)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 计费口径基线：用户 1 从 cycle 起的合计（仅 35/50 分钟两行）
	billedSince := func() (int64, int64) {
		var r struct{ Up, Down int64 }
		if err := db.Model(&models.TrafficLog{}).
			Where("user_id = ? AND period_start >= ?", 1, cycle).
			Select("COALESCE(SUM(billed_up),0) AS up, COALESCE(SUM(billed_down),0) AS down").
			Scan(&r).Error; err != nil {
			t.Fatalf("billed sum: %v", err)
		}
		return r.Up, r.Down
	}
	up0, down0 := billedSince()

	if err := compactTrafficHour(db, h, map[uint64]bool{1: true}); err != nil {
		t.Fatalf("compact hour: %v", err)
	}

	var u1 []models.TrafficLog
	if err := db.Where("user_id = ?", 1).Order("period_start ASC").Find(&u1).Error; err != nil {
		t.Fatalf("query u1: %v", err)
	}
	if len(u1) != 3 {
		t.Fatalf("protected user rows = %d, want 3", len(u1))
	}
	var u2 []models.TrafficLog
	if err := db.Where("user_id = ?", 2).Find(&u2).Error; err != nil {
		t.Fatalf("query u2: %v", err)
	}
	if len(u2) != 1 || u2[0].UpBytes != 9 || u2[0].DownBytes != 12 || !u2[0].PeriodStart.Equal(h) {
		t.Fatalf("unprotected merge wrong: %+v", u2)
	}
	if up1, down1 := billedSince(); up1 != up0 || down1 != down0 {
		t.Fatalf("billing sum changed: (%d,%d) → (%d,%d)", up0, down0, up1, down1)
	}

	// 全表合计不变（逐字节守恒）
	var agg struct{ Up, Down, BU, BD int64 }
	if err := db.Model(&models.TrafficLog{}).
		Select("COALESCE(SUM(up_bytes),0) up, COALESCE(SUM(down_bytes),0) down, COALESCE(SUM(billed_up),0) bu, COALESCE(SUM(billed_down),0) bd").
		Scan(&agg).Error; err != nil {
		t.Fatalf("agg: %v", err)
	}
	wantUp := int64(100 + 300 + 500 + 1 + 3 + 5)
	if agg.Up != wantUp {
		t.Fatalf("total up = %d, want %d", agg.Up, wantUp)
	}

	// 幂等：再跑一次行数不变
	if err := compactTrafficHour(db, h, map[uint64]bool{1: true}); err != nil {
		t.Fatalf("compact #2: %v", err)
	}
	var n int64
	if err := db.Model(&models.TrafficLog{}).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 4 {
		t.Fatalf("rows after second run = %d, want 4", n)
	}
}

// 非整点探测：空库/整点桶 → false；存在非整点明细 → true。
func TestNeedsTrafficCompaction(t *testing.T) {
	db := newCompactTestDB(t)
	if need, err := NeedsTrafficCompaction(db); err != nil || need {
		t.Fatalf("empty: need=%v err=%v", need, err)
	}
	h := time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC)
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 7, UpBytes: 1, PeriodStart: h}).Error; err != nil {
		t.Fatalf("seed bucketed: %v", err)
	}
	if need, err := NeedsTrafficCompaction(db); err != nil || need {
		t.Fatalf("bucketed: need=%v err=%v", need, err)
	}
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 7, UpBytes: 1, PeriodStart: h.Add(5 * time.Minute)}).Error; err != nil {
		t.Fatalf("seed raw: %v", err)
	}
	if need, err := NeedsTrafficCompaction(db); err != nil || !need {
		t.Fatalf("raw: need=%v err=%v", need, err)
	}
}

// 受保护（周期起点落在桶内）的非整点行不算「待压缩」，避免每日探测与全量扫描空转。
func TestNeedsTrafficCompactionIgnoresProtectedRows(t *testing.T) {
	db := newCompactTestDB(t)
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}
	h := time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC)
	cycle := h.Add(30 * time.Minute)
	if err := db.Create(&models.User{Username: "u1", PasswordHash: "x", TrafficCycleStart: cycle}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// 非整点行且其小时 == 用户周期起点小时 → 受保护，不算待压缩
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 7, UpBytes: 1, PeriodStart: h.Add(40 * time.Minute)}).Error; err != nil {
		t.Fatalf("seed protected: %v", err)
	}
	if need, err := NeedsTrafficCompaction(db); err != nil || need {
		t.Fatalf("protected row should not need compaction: need=%v err=%v", need, err)
	}
	// 另一小时的原始行 → 算待压缩
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 7, UpBytes: 1, PeriodStart: h.Add(2 * time.Hour).Add(5 * time.Minute)}).Error; err != nil {
		t.Fatalf("seed raw: %v", err)
	}
	if need, err := NeedsTrafficCompaction(db); err != nil || !need {
		t.Fatalf("raw row should need compaction: need=%v err=%v", need, err)
	}
}

// 一次性迁移：压缩存量明细并落标记；标记存在后重复调用为 no-op（不再压缩新写入的原始行）。
func TestMigrateLegacyLogsOneTimeAndIdempotent(t *testing.T) {
	db := newCompactTestDB(t)
	now := time.Now().UTC()
	h := now.Add(-2 * time.Hour).Truncate(time.Hour)

	raw := []models.TrafficLog{
		{UserID: 1, InboundID: 7, UpBytes: 10, PeriodStart: h.Add(1 * time.Minute)},
		{UserID: 1, InboundID: 7, UpBytes: 20, PeriodStart: h.Add(30 * time.Minute)},
	}
	if err := db.Create(&raw).Error; err != nil {
		t.Fatalf("seed traffic: %v", err)
	}
	var reports []models.NodeReport
	base := now.Truncate(time.Minute).Add(-time.Hour) // 锚定整分钟，避免跨分钟桶
	for i := 0; i < 6; i++ {
		reports = append(reports, models.NodeReport{ServerID: 1, ReportedAt: base.Add(time.Duration(i) * 10 * time.Second)})
	}
	if err := db.Create(&reports).Error; err != nil {
		t.Fatalf("seed node: %v", err)
	}

	if err := MigrateLegacyLogs(db, "test"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if got := GetSetting(db, SettingLogsCompacted); got != "1" {
		t.Fatalf("marker = %q, want 1", got)
	}
	var tl, nrc int64
	db.Model(&models.TrafficLog{}).Count(&tl)
	db.Model(&models.NodeReport{}).Count(&nrc)
	if tl != 1 {
		t.Fatalf("traffic rows = %d, want 1", tl)
	}
	if nrc != 1 {
		t.Fatalf("node rows = %d, want 1 (同一分钟抽稀)", nrc)
	}

	// 标记存在后，第二次调用应整体跳过：新写入的原始行不被压缩。
	if err := db.Create(&models.TrafficLog{UserID: 2, InboundID: 7, UpBytes: 5, PeriodStart: h.Add(2 * time.Minute)}).Error; err != nil {
		t.Fatalf("seed after: %v", err)
	}
	if err := MigrateLegacyLogs(db, "test"); err != nil {
		t.Fatalf("migrate #2: %v", err)
	}
	var tl2 int64
	db.Model(&models.TrafficLog{}).Count(&tl2)
	if tl2 != 2 {
		t.Fatalf("second migrate should be no-op, rows = %d want 2", tl2)
	}
}
