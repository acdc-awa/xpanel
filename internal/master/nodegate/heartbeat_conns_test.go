package nodegate

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// TestHubHeartbeatPersistsConnStats 连接数上报链路（2026-11-01）：
//
//	① 上报后落 servers 三列，并在内存快照里保留同一份数据；
//	② 字段缺省（旧 agent / 非 Linux 节点）不覆盖已有值，不能把真实计数抹成 0；
//	③ 从未上报过的服务器保持 -1（区别于「确实 0 连接」）。
func TestHubHeartbeatPersistsConnStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_conns.db")
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

	if err := db.Create(&models.Server{ID: 4, Name: "n4", Host: "10.0.0.4", NodeID: "node-4", Secret: "s"}).Error; err != nil {
		t.Fatal(err)
	}

	// 从未上报：默认值必须是 -1，前端据此显示「—」而不是「0 个连接」
	var fresh models.Server
	if err := db.First(&fresh, 4).Error; err != nil {
		t.Fatal(err)
	}
	if fresh.ConnTCPEstablished != -1 || fresh.ConnTCPTotal != -1 || fresh.ConnUDP != -1 {
		t.Fatalf("未上报时应为 -1，实际 %d/%d/%d",
			fresh.ConnTCPEstablished, fresh.ConnTCPTotal, fresh.ConnUDP)
	}

	h := &Hub{
		DB:            db,
		conns:         make(map[uint64]*Conn),
		latestMetrics: make(map[uint64]*NodeMetricsSnapshot),
	}
	conn := newTestConn(4)
	h.conns[4] = conn

	raw, _ := json.Marshal(protocol.HeartbeatPayload{
		XrayRunning: true,
		Conns:       &protocol.ConnStatsPayload{TCPEstablished: 42, TCPTotal: 57, UDP: 3},
	})
	h.handleHeartbeat(conn, &protocol.Message{Type: protocol.MsgHeartbeat, Payload: raw})

	var got models.Server
	if err := db.First(&got, 4).Error; err != nil {
		t.Fatal(err)
	}
	if got.ConnTCPEstablished != 42 || got.ConnTCPTotal != 57 || got.ConnUDP != 3 {
		t.Fatalf("落库 = %d/%d/%d，want 42/57/3",
			got.ConnTCPEstablished, got.ConnTCPTotal, got.ConnUDP)
	}
	if m, _ := h.GetLatestMetrics(4); m == nil || m.Conns == nil || m.Conns.TCPEstablished != 42 {
		t.Fatalf("内存快照未带连接数: %+v", m)
	}

	// 旧 agent 心跳（不带 conns）不得覆盖已有计数；节流窗口内也要立即写库，故先清掉节流时间
	conn.lastDBUpdate = conn.lastDBUpdate.Add(-60e9)
	rawOld, _ := json.Marshal(protocol.HeartbeatPayload{XrayRunning: true, CPU: 1})
	h.handleHeartbeat(conn, &protocol.Message{Type: protocol.MsgHeartbeat, Payload: rawOld})

	got = models.Server{}
	if err := db.First(&got, 4).Error; err != nil {
		t.Fatal(err)
	}
	if got.ConnTCPEstablished != 42 || got.ConnTCPTotal != 57 || got.ConnUDP != 3 {
		t.Fatalf("缺省 conns 不应覆盖已有值，实际 %d/%d/%d",
			got.ConnTCPEstablished, got.ConnTCPTotal, got.ConnUDP)
	}
}

// TestHubHeartbeatConnStatsZeroIsReal 0 是合法值：节点确实没有连接时必须照写 0，
// 不能被「不覆盖」逻辑吞掉（这正是用 -1 而不是 0 表示未上报的原因）。
func TestHubHeartbeatConnStatsZeroIsReal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test_conns_zero.db")
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

	if err := db.Create(&models.Server{ID: 6, Name: "n6", Host: "10.0.0.6", NodeID: "node-6", Secret: "s"}).Error; err != nil {
		t.Fatal(err)
	}
	h := &Hub{
		DB:            db,
		conns:         make(map[uint64]*Conn),
		latestMetrics: make(map[uint64]*NodeMetricsSnapshot),
	}
	conn := newTestConn(6)
	h.conns[6] = conn

	raw, _ := json.Marshal(protocol.HeartbeatPayload{
		XrayRunning: false,
		Conns:       &protocol.ConnStatsPayload{TCPEstablished: 0, TCPTotal: 0, UDP: 0},
	})
	h.handleHeartbeat(conn, &protocol.Message{Type: protocol.MsgHeartbeat, Payload: raw})

	var got models.Server
	if err := db.First(&got, 6).Error; err != nil {
		t.Fatal(err)
	}
	if got.ConnTCPEstablished != 0 || got.ConnTCPTotal != 0 || got.ConnUDP != 0 {
		t.Fatalf("零值应照写，实际 %d/%d/%d",
			got.ConnTCPEstablished, got.ConnTCPTotal, got.ConnUDP)
	}
}
