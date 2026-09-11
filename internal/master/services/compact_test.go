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
