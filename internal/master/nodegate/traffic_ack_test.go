package nodegate

import (
	"testing"
	"time"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

// TestHandleTrafficReportSendsAck 审计 F1 回归：落库成功必须回 traffic_ack(ok=true)。
// 节点只有收到 ok 才删本地批次——不回执就等于「主控已记账但节点反复重发」或
// 「主控没记账而节点已丢数据」，两种都不可接受。
func TestHandleTrafficReportSendsAck(t *testing.T) {
	h := newTestHub(t)
	if err := h.DB.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}, &models.User{},
		&models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{}, &models.TrafficBatch{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	u := models.User{Username: "ack", Email: "ack@t.com", UUID: "uuid-ack",
		SubscribeToken: "tok-ack", Status: models.StatusActive}
	if err := h.DB.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	if err := h.DB.Model(&models.User{}).Where("id = ?", u.ID).
		Update("traffic_cycle_start", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).Error; err != nil {
		t.Fatal(err)
	}
	h.Traffic = &services.TrafficService{DB: h.DB}

	conn := newTestConn(7)
	msg := mustMessage(t, protocol.MsgTrafficReport, protocol.TrafficReportPayload{
		Period:  "2026-09-20T00:00:00Z",
		BatchID: "ack-batch-1",
		Seq:     11,
		BootID:  "boot-x",
		Entries: []protocol.TrafficEntry{{UserID: u.ID, UpBytes: 100}},
	})
	h.handleTrafficReport(conn, msg)

	ack := readAck(t, conn)
	if ack.BatchID != "ack-batch-1" || !ack.OK {
		t.Fatalf("回执 = %+v, want batch=ack-batch-1 ok=true", ack)
	}

	// 重复投递同一批次：仍回 ok（幂等成功），节点据此删批、不再重发
	h.handleTrafficReport(conn, msg)
	ack = readAck(t, conn)
	if ack.BatchID != "ack-batch-1" || !ack.OK {
		t.Fatalf("重复批次回执 = %+v, want ok=true（幂等）", ack)
	}
	var n int64
	if err := h.DB.Model(&models.TrafficLog{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("重复投递不应新增流水行，实际 %d", n)
	}
}

// TestHandleTrafficReportNoAckForLegacy 旧 agent 不带 BatchID：不回执（否则会往旧节点发
// 它不认识的消息类型），落库行为保持不变。
func TestHandleTrafficReportNoAckForLegacy(t *testing.T) {
	h := newTestHub(t)
	if err := h.DB.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}, &models.User{},
		&models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{}, &models.TrafficBatch{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	u := models.User{Username: "legacy", Email: "legacy@t.com", UUID: "uuid-legacy",
		SubscribeToken: "tok-legacy", Status: models.StatusActive}
	if err := h.DB.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	h.Traffic = &services.TrafficService{DB: h.DB}

	conn := newTestConn(7)
	msg := mustMessage(t, protocol.MsgTrafficReport, protocol.TrafficReportPayload{
		Period:  "2026-09-20T00:00:00Z",
		Entries: []protocol.TrafficEntry{{UserID: u.ID, UpBytes: 100}},
	})
	h.handleTrafficReport(conn, msg)

	select {
	case data := <-conn.Send:
		t.Fatalf("旧 agent（无批次号）不应收到回执，实际收到 %s", string(data))
	case <-time.After(50 * time.Millisecond):
	}
}

// TestAckTrafficIgnoresClosedConn 连接已关闭时回执不阻塞（readPump 上不得空等）。
func TestAckTrafficIgnoresClosedConn(t *testing.T) {
	h := newTestHub(t)
	conn := newTestConn(7)
	conn.closeSafe()
	done := make(chan struct{})
	go func() {
		h.ackTraffic(conn, "b1", true, "")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("连接关闭后回执应立刻返回（走 done 分支），不得空等超时")
	}
}

// mustMessage 构造协议帧。
func mustMessage(t *testing.T, typ string, payload any) *protocol.Message {
	t.Helper()
	raw, err := protocol.Encode(typ, "", payload)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	m, err := protocol.Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return m
}

// readAck 从连接发送队列取一条回执（超时失败）。
func readAck(t *testing.T, conn *Conn) protocol.TrafficAckPayload {
	t.Helper()
	select {
	case data := <-conn.Send:
		m, err := protocol.Decode(data)
		if err != nil {
			t.Fatalf("decode ack: %v", err)
		}
		if m.Type != protocol.MsgTrafficAck {
			t.Fatalf("消息类型 = %s, want %s", m.Type, protocol.MsgTrafficAck)
		}
		var ack protocol.TrafficAckPayload
		if err := m.PayloadTo(&ack); err != nil {
			t.Fatalf("ack payload: %v", err)
		}
		return ack
	case <-time.After(2 * time.Second):
		t.Fatal("未收到流量回执")
		return protocol.TrafficAckPayload{}
	}
}
