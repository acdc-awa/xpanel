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
