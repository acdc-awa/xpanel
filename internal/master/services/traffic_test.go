package services

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/models"
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
