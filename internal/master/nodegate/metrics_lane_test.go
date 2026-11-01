package nodegate

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
)

func feed(t *testing.T, h *Hub, conn *Conn, typ string, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	switch typ {
	case protocol.MsgHeartbeat:
		h.handleHeartbeat(conn, &protocol.Message{Type: typ, Payload: raw})
	case protocol.MsgMetrics:
		h.handleMetrics(conn, &protocol.Message{Type: typ, Payload: raw})
	default:
		t.Fatalf("未知类型 %s", typ)
	}
}

// TestHubMetricsLaneDoesNotClobber 快慢车道就地合并且互不清空（2026-11-01）：
//
//	① 旧 agent（只有心跳）：实时字段照旧由心跳写入，行为与拆分前一致；
//	② 收到首帧 metrics 后连接标记为快车道，随后的心跳（不带 CPU/网速）不能再把它们抹成 0；
//	③ 用户速率与快车道时间戳只由快车道维护；
//	④ 慢字段（内存/磁盘/在线名单）仍由心跳更新，不被快车道覆盖。
func TestHubMetricsLaneDoesNotClobber(t *testing.T) {
	h := &Hub{
		conns:         make(map[uint64]*Conn),
		latestMetrics: make(map[uint64]*NodeMetricsSnapshot),
	}
	conn := newTestConn(7)
	h.conns[7] = conn

	// ① 旧 agent：心跳带全集
	feed(t, h, conn, protocol.MsgHeartbeat, protocol.HeartbeatPayload{
		CPU: 11, Mem: 222, Disk: 333, RxRate: 44, TxRate: 55, XrayRunning: true,
		OnlineUsers: 2,
	})
	m, ok := h.GetLatestMetrics(7)
	if !ok || m.CPU != 11 || m.Mem != 222 || m.RxRate != 44 {
		t.Fatalf("旧 agent 心跳未写入快照: %+v", m)
	}

	// ② 快车道首帧
	feed(t, h, conn, protocol.MsgMetrics, protocol.MetricsPayload{
		CPU: 77, RxRate: 100, TxRate: 200, RxBytes: 1000, TxBytes: 2000,
		XrayRunning: true,
		Conns:       &protocol.ConnStatsPayload{TCPEstablished: 9, TCPTotal: 12, UDP: 1},
		UserRates: []protocol.UserRatePayload{
			{Email: "u1.i3@panel.local", Up: 10, Down: 20},
		},
		TS: 1,
	})

	// ③ 新 agent 的心跳：实时字段缺省（0 值），慢字段有值
	feed(t, h, conn, protocol.MsgHeartbeat, protocol.HeartbeatPayload{
		Mem: 999, Disk: 888, XrayRunning: true, OnlineUsers: 5,
	})

	m, _ = h.GetLatestMetrics(7)
	if m.CPU != 77 || m.RxRate != 100 || m.TxRate != 200 || m.RxBytes != 1000 || m.TxBytes != 2000 {
		t.Fatalf("心跳把快车道实时字段清掉了: %+v", m)
	}
	if len(m.UserRates) != 1 || m.UserRates[0].Up != 10 {
		t.Fatalf("用户速率被心跳清掉: %+v", m.UserRates)
	}
	if m.Mem != 999 || m.Disk != 888 || m.OnlineUsers != 5 {
		t.Fatalf("快车道不应影响慢字段: %+v", m)
	}
	if m.MetricsAt.IsZero() {
		t.Fatal("快车道时间戳未记录")
	}
	if m.Conns == nil || m.Conns.TCPEstablished != 9 {
		t.Fatalf("连接数丢失: %+v", m.Conns)
	}

	// ④ 窗口聚合不因心跳里的 0 值而失真：快车道连接上的心跳不再贡献 CPU 样本，
	//    否则 30s 一帧的 0 会把 3s 一帧的真实 CPU 均值稀释掉。先挡住 1 分钟落库把窗口清空。
	conn.lastReportAt = time.Now()
	conn.windowCPUSum = 0
	conn.windowCPUCount = 0
	feed(t, h, conn, protocol.MsgHeartbeat, protocol.HeartbeatPayload{Mem: 111, XrayRunning: true})
	if conn.windowCPUCount != 0 {
		t.Fatalf("快车道连接上心跳不应计入 CPU 均值，实际采样数 %d", conn.windowCPUCount)
	}
}

// TestHubMetricsLaneSurvivesNoSlowFrame 只有快车道帧时也保持在线与指标可读
// （拆分最容易翻车的点：只认慢车道会让节点在面板上闪成离线）。
func TestHubMetricsLaneSurvivesNoSlowFrame(t *testing.T) {
	h := &Hub{
		conns:         make(map[uint64]*Conn),
		latestMetrics: make(map[uint64]*NodeMetricsSnapshot),
	}
	conn := newTestConn(8)
	h.conns[8] = conn

	feed(t, h, conn, protocol.MsgMetrics, protocol.MetricsPayload{CPU: 5, XrayRunning: true})
	if !h.IsOnline(8) {
		t.Fatal("仅收到快车道帧也应视为在线")
	}
	m, ok := h.GetLatestMetrics(8)
	if !ok || m.CPU != 5 || !m.XrayRunning {
		t.Fatalf("快车道首帧未建快照: %+v", m)
	}
}
