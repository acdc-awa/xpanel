package services

import (
	"strings"
	"testing"
	"time"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func testChannelDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Server{}, &models.Inbound{}, &models.ProxyChannel{},
		&models.TrafficLog{}, &models.TrafficBatch{}, &models.Setting{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func TestChannelTrafficTrackingAndLifecycle(t *testing.T) {
	db := testChannelDB(t)
	srv := models.Server{ID: 1, Name: "S1", Host: "1.2.3.4", NodeID: "n1", Secret: "sec"}
	db.Create(&srv)

	inb := models.Inbound{
		ID:       10,
		ServerID: 1,
		Tag:      "chan-socks5-1080",
		Protocol: "socks",
		Port:     1080,
		Type:     models.InboundTypeChannel,
		Enabled:  true,
	}
	db.Create(&inb)

	ch := models.ProxyChannel{
		ID:             1,
		Name:           "专用Socks5",
		ServerID:       1,
		InboundID:      10,
		Port:           1080,
		Protocol:       "socks5",
		TrafficLimitGB: 1, // 1 GB 配额
		TrafficReset:   "monthly",
		AutoDisable:    true,
		Status:         models.ChannelStatusActive,
		Enabled:        true,
	}
	db.Create(&ch)

	svc := &TrafficService{DB: db}

	now := time.Now().Truncate(time.Hour)
	nowStr := now.Format(time.RFC3339)

	// 1. 上报 500 MB 流量（小于 1 GB 配额）
	halfGB := int64(500 * 1024 * 1024)
	payload1 := protocol.TrafficReportPayload{
		BatchID: "batch-1",
		Period:  nowStr,
		Entries: []protocol.TrafficEntry{
			{
				Inbound:   "chan-socks5-1080",
				UpBytes:   halfGB / 2,
				DownBytes: halfGB / 2,
			},
		},
	}
	if _, err := svc.Save(payload1, 1); err != nil {
		t.Fatalf("Save traffic batch 1 failed: %v", err)
	}

	// 校验 ProxyChannel 累加用量
	var updatedCh models.ProxyChannel
	db.First(&updatedCh, 1)
	if updatedCh.TrafficUsedBytes != halfGB {
		t.Fatalf("Expected used bytes %d, got %d", halfGB, updatedCh.TrafficUsedBytes)
	}

	// 检查 lifecycle：未超额，状态保持 active
	svc.checkInboundLifecycle()
	db.First(&updatedCh, 1)
	if updatedCh.Status != models.ChannelStatusActive {
		t.Fatalf("Expected status active, got %s", updatedCh.Status)
	}

	// 2. 再次上报 600 MB 流量（总共 1100 MB > 1 GB 配额）
	overGB := int64(600 * 1024 * 1024)
	payload2 := protocol.TrafficReportPayload{
		BatchID: "batch-2",
		Period:  nowStr,
		Entries: []protocol.TrafficEntry{
			{
				Inbound:   "chan-socks5-1080",
				UpBytes:   overGB / 2,
				DownBytes: overGB / 2,
			},
		},
	}
	if _, err := svc.Save(payload2, 1); err != nil {
		t.Fatalf("Save traffic batch 2 failed: %v", err)
	}

	db.First(&updatedCh, 1)
	if updatedCh.TrafficUsedBytes != halfGB+overGB {
		t.Fatalf("Expected used bytes %d, got %d", halfGB+overGB, updatedCh.TrafficUsedBytes)
	}

	// 运行 lifecycle 检查：应触发 quota_exceeded 并停用底层 Inbound
	svc.checkInboundLifecycle()
	db.First(&updatedCh, 1)
	if updatedCh.Status != models.ChannelStatusExceeded {
		t.Fatalf("Expected status quota_exceeded, got %s", updatedCh.Status)
	}

	var updatedInb models.Inbound
	db.First(&updatedInb, 10)
	if updatedInb.Enabled {
		t.Fatalf("Expected underlying inbound disabled after exceeding quota")
	}

	// 3. 测试自动周期重置复原
	// 模拟已过去一个月，设置旧的 reset key
	db.Model(&updatedCh).Update("last_reset_date", "2026-08-01")
	svc.resetInboundTraffic()

	db.First(&updatedCh, 1)
	if updatedCh.TrafficUsedBytes != 0 || updatedCh.Status != models.ChannelStatusActive {
		t.Fatalf("Expected channel reset to 0 and active, got used=%d, status=%s", updatedCh.TrafficUsedBytes, updatedCh.Status)
	}

	db.First(&updatedInb, 10)
	if !updatedInb.Enabled {
		t.Fatalf("Expected underlying inbound re-enabled after cycle reset")
	}
}
