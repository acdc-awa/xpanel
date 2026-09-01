package services

import (
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/models"
)

func TestResetPeriodKey(t *testing.T) {
	thursday := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC) // Thursday
	monday := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		now    time.Time
		policy string
		want   string
	}{
		{thursday, "daily", "2026-08-13"},
		{thursday, "weekly", "2026-08-10"},
		{monday, "weekly", "2026-08-10"},
		{sunday, "weekly", "2026-08-10"},
		{thursday, "monthly", "2026-08-01"},
		{thursday, "never", ""},
	}
	for _, c := range cases {
		if got := resetPeriodKey(c.now, c.policy); got != c.want {
			t.Errorf("resetPeriodKey(%v, %q) = %q, want %q", c.now, c.policy, got, c.want)
		}
	}
}

func TestRetentionPolicyDeletesOnlyExpired(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.TrafficLog{}, &models.NodeReport{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Date(2026, 8, 16, 5, 0, 0, 0, time.UTC)
	s := &TrafficService{DB: db, now: func() time.Time { return now }}

	old := now.AddDate(0, 0, -200)
	new := now.AddDate(0, 0, -1)
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 1, UpBytes: 1, PeriodStart: old}).Error; err != nil {
		t.Fatalf("old log: %v", err)
	}
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 1, UpBytes: 1, PeriodStart: new}).Error; err != nil {
		t.Fatalf("new log: %v", err)
	}
	if err := db.Create(&models.NodeReport{ServerID: 1, ReportedAt: old}).Error; err != nil {
		t.Fatalf("old report: %v", err)
	}
	if err := db.Create(&models.NodeReport{ServerID: 1, ReportedAt: new}).Error; err != nil {
		t.Fatalf("new report: %v", err)
	}
	if err := db.Create(&models.AuditLog{OperatorType: "user", OperatorID: 1, Action: "x", CreatedAt: old}).Error; err != nil {
		t.Fatalf("old audit: %v", err)
	}
	if err := db.Create(&models.AuditLog{OperatorType: "user", OperatorID: 1, Action: "x", CreatedAt: new}).Error; err != nil {
		t.Fatalf("new audit: %v", err)
	}

	s.runRetention()

	var logs, reports, audits int64
	db.Model(&models.TrafficLog{}).Count(&logs)
	db.Model(&models.NodeReport{}).Count(&reports)
	db.Model(&models.AuditLog{}).Count(&audits)
	if logs != 1 || reports != 1 || audits != 1 {
		t.Fatalf("retention counts = logs:%d reports:%d audits:%d, want all 1", logs, reports, audits)
	}
}

func TestAggDailyOnlyRecentWindow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.TrafficLog{}, &models.TrafficDaily{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Date(2026, 8, 16, 5, 0, 0, 0, time.UTC)
	s := &TrafficService{DB: db, now: func() time.Time { return now }}

	old := now.AddDate(0, 0, -30)
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 1, UpBytes: 100, PeriodStart: old}).Error; err != nil {
		t.Fatalf("old log: %v", err)
	}
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 1, UpBytes: 200, PeriodStart: now.Add(-time.Hour)}).Error; err != nil {
		t.Fatalf("recent log: %v", err)
	}

	s.AggDaily()

	var dailies []models.TrafficDaily
	db.Find(&dailies)
	if len(dailies) != 1 {
		t.Fatalf("dailies = %d, want 1（窗口外旧数据不扫描）", len(dailies))
	}
	if dailies[0].UpBytes != 200 {
		t.Fatalf("recent daily up = %d, want 200", dailies[0].UpBytes)
	}
}

func TestResetInboundTrafficOncePerPeriod(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	inb := models.Inbound{ServerID: 1, Tag: "in", Protocol: "vless", Port: 443, Up: 100, Down: 200, TrafficReset: "daily"}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	s := &TrafficService{DB: db}
	s.resetInboundTraffic()
	db.First(&inb, inb.ID)
	if inb.Up != 0 || inb.Down != 0 {
		t.Fatalf("first tick should reset, got up=%d down=%d", inb.Up, inb.Down)
	}
	if inb.LastResetDate == "" {
		t.Fatal("first tick should record last_reset_date")
	}

	// 周期内新产生的流量不应被第二个 tick 清零（ISSUE-05 回归）
	db.Model(&inb).Updates(map[string]any{"up": 50, "down": 60})
	s.resetInboundTraffic()
	db.First(&inb, inb.ID)
	if inb.Up != 50 || inb.Down != 60 {
		t.Fatalf("second tick in same period should not reset, got up=%d down=%d", inb.Up, inb.Down)
	}
}

// TestSaveDuplicateDeliveryMergesAndBumpsInbound 同 (user, inbound, period) 重复投递：
// TrafficLog 合并为一行且字节累加，inbounds 每次投递都补计（P1-1）。
func TestSaveDuplicateDeliveryMergesAndBumpsInbound(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	inb := models.Inbound{ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TrafficService{DB: db}
	payload := protocol.TrafficReportPayload{
		Period: "2026-08-24T00:00:00Z",
		Entries: []protocol.TrafficEntry{
			{UserID: 42, Inbound: "vless-in", UpBytes: 100, DownBytes: 200},
		},
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.Save(payload, 1); err != nil {
			t.Fatalf("Save #%d: %v", i+1, err)
		}
	}

	var logs []models.TrafficLog
	if err := db.Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("TrafficLog 行数 = %d, want 1（重复投递应合并）", len(logs))
	}
	if logs[0].UpBytes != 200 || logs[0].DownBytes != 400 {
		t.Fatalf("合并后 up=%d down=%d, want 200/400", logs[0].UpBytes, logs[0].DownBytes)
	}
	if err := db.First(&inb, inb.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inb.Up != 200 || inb.Down != 400 {
		t.Fatalf("inbounds 补计 up=%d down=%d, want 200/400", inb.Up, inb.Down)
	}
}

// TestSaveInboundDimensionEntryOnlyBumpsInbound 入站维度条目（Email 恒空、Inbound=tag，
// agent 从 inbound>>> 计数器派生）：仅累计 inbounds.up/down，不落 traffic_logs
// （流水严格用户维度，防今日流量 KPI 双计）；未知 tag 与无从归属条目安全跳过。
func TestSaveInboundDimensionEntryOnlyBumpsInbound(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	inb := models.Inbound{ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TrafficService{DB: db}
	payload := protocol.TrafficReportPayload{
		Period: "2026-09-01T00:00:00Z",
		Entries: []protocol.TrafficEntry{
			{Inbound: "vless-in", UpBytes: 500, DownBytes: 700},  // 已知 tag：入账
			{Inbound: "ghost-tag", UpBytes: 100, DownBytes: 100}, // 未知 tag：跳过
			{UpBytes: 50, DownBytes: 50},                         // 无从归属：跳过
		},
	}
	if _, err := svc.Save(payload, 1); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var logs int64
	db.Model(&models.TrafficLog{}).Count(&logs)
	if logs != 0 {
		t.Fatalf("入站维度条目不得落 traffic_logs，行数 = %d", logs)
	}
	if err := db.First(&inb, inb.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inb.Up != 500 || inb.Down != 700 {
		t.Fatalf("inbounds 计数 up=%d down=%d, want 500/700", inb.Up, inb.Down)
	}
}

// TestSaveConcurrentDuplicateDeliveryMerges 并发双投同 (user, inbound, period)：
// upsert 合并为一行、全部字节累加、inbounds 补计齐全（P1-1 并发路径）。
func TestSaveConcurrentDuplicateDeliveryMerges(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared&_pragma=busy_timeout(5000)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	inb := models.Inbound{ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TrafficService{DB: db}
	payload := protocol.TrafficReportPayload{
		Period: "2026-08-24T00:00:00Z",
		Entries: []protocol.TrafficEntry{
			{UserID: 42, Inbound: "vless-in", UpBytes: 100, DownBytes: 200},
		},
	}
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.Save(payload, 1)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("并发 Save #%d: %v", i, err)
		}
	}

	var logs []models.TrafficLog
	if err := db.Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("TrafficLog 行数 = %d, want 1（并发双投应合并为一行）", len(logs))
	}
	if logs[0].UpBytes != 100*n || logs[0].DownBytes != 200*n {
		t.Fatalf("并发合并后 up=%d down=%d, want %d/%d", logs[0].UpBytes, logs[0].DownBytes, 100*n, 200*n)
	}
	if err := db.First(&inb, inb.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inb.Up != 100*n || inb.Down != 200*n {
		t.Fatalf("inbounds 并发补计 up=%d down=%d, want %d/%d", inb.Up, inb.Down, 100*n, 200*n)
	}
}

// TestFindViolators 事件驱动处置的判定口径（与 filterValidUsers 快照语义严格一致）：
// 跨阈值/未跨/已过期/无额度（不限）/非活跃 五态。
func TestFindViolators(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.TrafficLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	gb := int64(1024 * 1024 * 1024)
	expired := time.Now().Add(-24 * time.Hour)
	users := []models.User{
		{Username: "over", Email: "over@t.com", UUID: "11111111-1111-1111-1111-111111111111", SubscribeToken: "t1", Status: models.StatusActive, PlanTrafficBytes: 1 * gb},
		{Username: "under", Email: "under@t.com", UUID: "22222222-2222-2222-2222-222222222222", SubscribeToken: "t2", Status: models.StatusActive, PlanTrafficBytes: 10 * gb},
		{Username: "expired", Email: "expired@t.com", UUID: "33333333-3333-3333-3333-333333333333", SubscribeToken: "t3", Status: models.StatusActive, ExpireAt: &expired},
		{Username: "unlimited", Email: "unlimited@t.com", UUID: "44444444-4444-4444-4444-444444444444", SubscribeToken: "t4", Status: models.StatusActive},
		{Username: "disabled", Email: "disabled@t.com", UUID: "55555555-5555-5555-5555-555555555555", SubscribeToken: "t5", Status: models.StatusDisabled, PlanTrafficBytes: gb},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	// over 用 2GB（跨阈值）；under 用 0.5GB（未跨）；unlimited 用 100GB 但无额度快照=不限
	logs := []models.TrafficLog{
		{UserID: users[0].ID, UpBytes: 2 * gb, DownBytes: 0, PeriodStart: time.Now()},
		{UserID: users[1].ID, UpBytes: gb / 2, DownBytes: 0, PeriodStart: time.Now()},
		{UserID: users[3].ID, UpBytes: 100 * gb, DownBytes: 0, PeriodStart: time.Now()},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TrafficService{DB: db}
	got, err := svc.FindViolators([]uint64{users[0].ID, users[1].ID, users[2].ID, users[3].ID, users[4].ID})
	if err != nil {
		t.Fatalf("FindViolators: %v", err)
	}
	want := map[uint64]bool{users[0].ID: true, users[2].ID: true}
	if len(got) != len(want) {
		t.Fatalf("violators = %v, want 仅 over+expired (%v)", got, want)
	}
	for _, id := range got {
		if !want[id] {
			t.Errorf("不应判违规: user=%d（got=%v）", id, got)
		}
	}
}

// TestSaveReturnsReportedUserIDs Save 返回本帧实际计入的用户 ID 去重集合
// （入站维度条目/零字节条目/未知用户不计入）。
func TestSaveReturnsReportedUserIDs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	inb := models.Inbound{ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TrafficService{DB: db}
	payload := protocol.TrafficReportPayload{
		Period: "2026-09-01T00:00:00Z",
		Entries: []protocol.TrafficEntry{
			{UserID: 7, Inbound: "vless-in", UpBytes: 100, DownBytes: 0},
			{UserID: 7, Inbound: "vless-in", UpBytes: 0, DownBytes: 50},
			{UserID: 8, Inbound: "vless-in", UpBytes: 10, DownBytes: 10},
			{UserID: 9, Inbound: "vless-in", UpBytes: 0, DownBytes: 0}, // 零字节：不计入
			{Inbound: "vless-in", UpBytes: 1, DownBytes: 1},            // 入站维度：不计入
		},
	}
	ids, err := svc.Save(payload, 1)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("reported ids = %v, want [7 8] 两个用户", ids)
	}
	seen := map[uint64]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if !seen[7] || !seen[8] {
		t.Fatalf("reported ids = %v, want 含 7 与 8", ids)
	}
}
