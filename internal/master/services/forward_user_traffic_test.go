package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testForwardUserDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.Server{}, &models.Inbound{}, &models.PermissionGroup{}, &models.ForwardUser{},
		&models.TrafficLog{}, &models.TrafficBatch{}, &models.Setting{},
		&models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func TestForwardUserTrafficTrackingAndLifecycle(t *testing.T) {
	db := testForwardUserDB(t)
	srv := models.Server{ID: 1, Name: "S1", Host: "1.2.3.4", NodeID: "n1", Secret: "sec"}
	db.Create(&srv)

	pg := models.PermissionGroup{ID: 1, Name: "转发专组"}
	db.Create(&pg)

	inb := models.Inbound{
		ID:       10,
		ServerID: 1,
		Tag:      "fwd-socks-1080",
		Protocol: "socks",
		Port:     1080,
		Type:     models.InboundTypeForward,
		Enabled:  true,
	}
	db.Create(&inb)

	targetInbID := uint64(10)
	ap := models.UserAccessPoint{
		ID:              1,
		Name:            "Socks AP",
		TargetType:      "inbound",
		TargetInboundID: &targetInbID,
		Enabled:         true,
	}
	db.Create(&ap)
	db.Create(&models.PermissionGroupAccessPoint{
		PermissionGroupID: 1,
		AccessPointID:     1,
	})

	fu := models.ForwardUser{
		ID:                100,
		Username:          "fwd_user_1",
		Password:          "secretpass",
		PermissionGroupID: 1,
		TrafficLimitGB:    1, // 1 GB 配额
		TrafficReset:      "monthly",
		Enabled:           true,
	}
	db.Create(&fu)

	var notifiedServers []uint64
	mockNow := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	svc := &TrafficService{
		DB: db,
		now: func() time.Time {
			return mockNow
		},
		OnInboundLifecycleChanged: func(sid uint64) {
			notifiedServers = append(notifiedServers, sid)
		},
	}

	// 1. 上报 500 MB 流量
	halfGB := int64(500 * 1024 * 1024)
	report := protocol.TrafficReportPayload{
		Period:  "2026-09-15T12:00:00Z",
		BatchID: "batch-1",
		Entries: []protocol.TrafficEntry{
			{
				Email:     fmt.Sprintf("fu%d.i%d@panel.local", fu.ID, inb.ID),
				Inbound:   inb.Tag,
				UpBytes:   halfGB / 2,
				DownBytes: halfGB / 2,
			},
		},
	}
	if _, err := svc.Save(report, 1); err != nil {
		t.Fatalf("Save traffic failed: %v", err)
	}

	// 检查 forward_users 累加
	var dbFU models.ForwardUser
	db.First(&dbFU, fu.ID)
	if dbFU.TrafficUsedBytes != halfGB {
		t.Errorf("expected traffic_used_bytes = %d, got %d", halfGB, dbFU.TrafficUsedBytes)
	}
	if !dbFU.Enabled {
		t.Errorf("forward user should still be enabled")
	}

	// 2. 检查生命周期，500MB < 1GB，不应熔断
	svc.checkInboundLifecycle()
	db.First(&dbFU, fu.ID)
	if !dbFU.Enabled {
		t.Errorf("forward user should not be disabled before quota exceeded")
	}

	// 3. 再上报 600 MB，累计 1.1 GB > 1GB
	report2 := protocol.TrafficReportPayload{
		Period:  "2026-09-15T12:05:00Z",
		BatchID: "batch-2",
		Entries: []protocol.TrafficEntry{
			{
				Email:     fmt.Sprintf("fu%d.i%d@panel.local", fu.ID, inb.ID),
				Inbound:   inb.Tag,
				UpBytes:   300 * 1024 * 1024,
				DownBytes: 300 * 1024 * 1024,
			},
		},
	}
	if _, err := svc.Save(report2, 1); err != nil {
		t.Fatalf("Save traffic 2 failed: %v", err)
	}

	// 巡检生命周期，超额应熔断
	notifiedServers = nil
	svc.checkInboundLifecycle()
	db.First(&dbFU, fu.ID)
	if dbFU.Enabled {
		t.Errorf("forward user should be disabled after exceeding quota")
	}
	if len(notifiedServers) != 1 || notifiedServers[0] != 1 {
		t.Errorf("expected server 1 to be notified on quota exceed, got: %v", notifiedServers)
	}

	// 4. 时间跨入下个月 10 月 1 日，周期重置
	notifiedServers = nil
	mockNow = time.Date(2026, 10, 1, 0, 5, 0, 0, time.UTC)
	svc.resetInboundTraffic()

	db.First(&dbFU, fu.ID)
	if dbFU.TrafficUsedBytes != 0 {
		t.Errorf("expected traffic_used_bytes = 0 after monthly reset, got %d", dbFU.TrafficUsedBytes)
	}
	if !dbFU.Enabled {
		t.Errorf("forward user should be re-enabled after reset")
	}
	if len(notifiedServers) != 1 || notifiedServers[0] != 1 {
		t.Errorf("expected server 1 to be notified on reset re-enable, got: %v", notifiedServers)
	}
}

func TestForwardUserTraffic_WritesTrafficLogs(t *testing.T) {
	db := testForwardUserDB(t)
	// 创建商业用户（ID: 100，与 ForwardUser 相同 ID，验证绝不撞车）
	commUser := models.User{ID: 100, UUID: "comm-uuid-100", Username: "commercial_100", Status: 1, PlanTrafficBytes: 10 * 1024 * 1024 * 1024}
	if err := db.Create(&commUser).Error; err != nil {
		t.Fatalf("Create commercial user failed: %v", err)
	}

	srv := models.Server{ID: 1, Name: "S1", Host: "1.2.3.4", NodeID: "n1", Secret: "sec"}
	db.Create(&srv)

	pg := models.PermissionGroup{ID: 1, Name: "转发专组"}
	db.Create(&pg)

	inb := models.Inbound{
		ID:       10,
		ServerID: 1,
		Tag:      "fwd-socks-1080",
		Protocol: "socks",
		Port:     1080,
		Type:     models.InboundTypeForward,
		Ratio:    1.5,
		Enabled:  true,
	}
	db.Create(&inb)

	targetInbID := uint64(10)
	ap := models.UserAccessPoint{
		ID:              2,
		Name:            "Socks AP 2",
		TargetType:      "inbound",
		TargetInboundID: &targetInbID,
		Enabled:         true,
	}
	db.Create(&ap)
	db.Create(&models.PermissionGroupAccessPoint{
		PermissionGroupID: 1,
		AccessPointID:     2,
	})

	fu := models.ForwardUser{
		ID:                100,
		Username:          "fwd_user_100",
		Password:          "secretpass",
		PermissionGroupID: 1,
		TrafficLimitGB:    5,
		Enabled:           true,
	}
	db.Create(&fu)

	mockNow := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	svc := &TrafficService{
		DB: db,
		now: func() time.Time {
			return mockNow
		},
	}

	upBytes := int64(100 * 1024 * 1024)   // 100 MB
	downBytes := int64(200 * 1024 * 1024) // 200 MB
	report := protocol.TrafficReportPayload{
		Period:  "2026-09-15T12:00:00Z",
		BatchID: "batch-log-1",
		Entries: []protocol.TrafficEntry{
			{
				Email:     fmt.Sprintf("fu%d.i%d@panel.local", fu.ID, inb.ID),
				Inbound:   inb.Tag,
				UpBytes:   upBytes,
				DownBytes: downBytes,
			},
		},
	}
	if _, err := svc.Save(report, 1); err != nil {
		t.Fatalf("Save traffic failed: %v", err)
	}

	// 1. 验证 traffic_logs 写入了对应行，user_id 为 ForwardUserTrafficUserID(100)
	expectedUID := models.ForwardUserTrafficUserID(100)
	var tLog models.TrafficLog
	if err := db.Where("user_id = ? AND inbound_id = ?", expectedUID, inb.ID).First(&tLog).Error; err != nil {
		t.Fatalf("traffic_logs not found for ForwardUser: %v", err)
	}
	if tLog.UpBytes != upBytes || tLog.DownBytes != downBytes {
		t.Errorf("traffic_logs bytes mismatch: up=%d down=%d", tLog.UpBytes, tLog.DownBytes)
	}
	expectedBilledUp := int64(float64(upBytes) * 1.5)
	expectedBilledDown := int64(float64(downBytes) * 1.5)
	if tLog.BilledUp != expectedBilledUp || tLog.BilledDown != expectedBilledDown {
		t.Errorf("traffic_logs billed mismatch: billedUp=%d (want %d), billedDown=%d (want %d)", tLog.BilledUp, expectedBilledUp, tLog.BilledDown, expectedBilledDown)
	}

	// 2. 验证商业用户（ID: 100）的已用流量完全未受影响，依然为 0
	bUp, bDown, err := svc.UserBilled(100)
	if err != nil {
		t.Fatalf("UserBilled(100) failed: %v", err)
	}
	if bUp != 0 || bDown != 0 {
		t.Fatalf("commercial user 100 used bytes corrupted by ForwardUser! got bUp=%d, bDown=%d, want 0", bUp, bDown)
	}

	// 3. 验证 FindViolators 未误触商业用户
	violators, err := svc.FindViolators([]uint64{100})
	if err != nil {
		t.Fatalf("FindViolators failed: %v", err)
	}
	if len(violators) != 0 {
		t.Fatalf("FindViolators returned unexpected violators: %+v", violators)
	}
}
