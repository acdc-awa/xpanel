package nodegate

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 心跳保活每帧都更新，但 node_reports 指标行按 nodeReportSampleInterval 抽稀：
// 间隔内的连续心跳不落库，间隔过后再落一行。这是抑制 node_reports 膨胀的核心行为。
func TestHeartbeatReportSampling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Server{}, &models.NodeReport{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.Create(&models.Server{ID: 1, Name: "n1", Host: "10.0.0.1", NodeID: "node-1", Secret: "s"}).Error; err != nil {
		t.Fatal(err)
	}

	h := &Hub{DB: db}
	conn := newTestConn(1)

	raw, _ := json.Marshal(protocol.HeartbeatPayload{CPU: 12.5, OnlineUsers: 3})
	msg := &protocol.Message{Type: protocol.MsgHeartbeat, Payload: raw}

	count := func() int64 {
		var n int64
		db.Model(&models.NodeReport{}).Count(&n)
		return n
	}

	// 首帧落库，紧随其后的第二帧在采样间隔内 → 不落库
	h.handleHeartbeat(conn, msg)
	h.handleHeartbeat(conn, msg)
	if got := count(); got != 1 {
		t.Fatalf("间隔内心跳落库行数 = %d, want 1（应抽稀）", got)
	}

	// 保活时间仍每帧更新
	var srv models.Server
	if err := db.First(&srv, 1).Error; err != nil {
		t.Fatal(err)
	}
	if srv.Status != 1 || srv.LastSeenAt == nil {
		t.Fatalf("心跳应每帧更新保活状态: status=%d last_seen=%v", srv.Status, srv.LastSeenAt)
	}

	// 越过采样间隔后再落一行
	conn.lastReportAt = time.Now().Add(-2 * nodeReportSampleInterval)
	h.handleHeartbeat(conn, msg)
	if got := count(); got != 2 {
		t.Fatalf("越过采样间隔后行数 = %d, want 2", got)
	}
}

// TestHubLatestMetricsAndPeakWindow 测试内存快照即时可用性与 1 分钟窗口峰值保留：
func TestHubLatestMetricsAndPeakWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_peak.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Server{}, &models.NodeReport{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.Create(&models.Server{ID: 2, Name: "n2", Host: "10.0.0.2", NodeID: "node-2", Secret: "s"}).Error; err != nil {
		t.Fatal(err)
	}

	h := &Hub{
		DB:            db,
		conns:         make(map[uint64]*Conn),
		latestMetrics: make(map[uint64]*NodeMetricsSnapshot),
	}
	conn := newTestConn(2)
	h.conns[2] = conn

	// 1. 首帧心跳：10 MB/s (80 Mbps)
	raw1, _ := json.Marshal(protocol.HeartbeatPayload{CPU: 20, RxRate: 10 * 1024 * 1024, TxRate: 2 * 1024 * 1024})
	msg1 := &protocol.Message{Type: protocol.MsgHeartbeat, Payload: raw1}
	h.handleHeartbeat(conn, msg1)

	// 内存快照应立即反映首帧速率
	m1, ok := h.GetLatestMetrics(2)
	if !ok || m1 == nil {
		t.Fatal("首帧心跳后应有内存快照")
	}
	if m1.RxRate != 10*1024*1024 {
		t.Fatalf("内存快照 RxRate = %v, want %v", m1.RxRate, 10*1024*1024)
	}

	// 2. 间隔内第二帧（突发尖峰 100 MB/s）：
	raw2, _ := json.Marshal(protocol.HeartbeatPayload{CPU: 80, RxRate: 100 * 1024 * 1024, TxRate: 20 * 1024 * 1024})
	msg2 := &protocol.Message{Type: protocol.MsgHeartbeat, Payload: raw2}
	h.handleHeartbeat(conn, msg2)

	// node_reports 仍应为 1 行（被 1 分钟抽稀拦截）
	var count int64
	db.Model(&models.NodeReport{}).Count(&count)
	if count != 1 {
		t.Fatalf("抽稀期间 node_reports 行数 = %d, want 1", count)
	}

	// 但内存快照应立刻反映突发 100 MB/s（零延迟）
	m2, ok := h.GetLatestMetrics(2)
	if !ok || m2.RxRate != 100*1024*1024 {
		t.Fatalf("第二帧内存快照 RxRate = %v, want 100MB/s (即时更新)", m2.RxRate)
	}

	// 3. 间隔内第三帧（下载结束回落为 0）并跨越采样窗口落盘：
	conn.lastReportAt = time.Now().Add(-2 * nodeReportSampleInterval)
	raw3, _ := json.Marshal(protocol.HeartbeatPayload{CPU: 10, RxRate: 0, TxRate: 0})
	msg3 := &protocol.Message{Type: protocol.MsgHeartbeat, Payload: raw3}
	h.handleHeartbeat(conn, msg3)

	// 第二行应落库
	var reports []models.NodeReport
	db.Where("server_id = 2").Order("id ASC").Find(&reports)
	if len(reports) != 2 {
		t.Fatalf("落盘行数 = %d, want 2", len(reports))
	}
	// 关键验证：第二行落库的 RxRate 必须保留刚才窗口内的尖峰 100 MB/s，而不是单点采样的 0！
	if reports[1].RxRate != 100*1024*1024 {
		t.Fatalf("抽稀落库未保留窗口内峰值: reports[1].RxRate = %v, want 100MB/s", reports[1].RxRate)
	}
}
