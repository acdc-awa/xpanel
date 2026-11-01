package nodegate

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// TestServeWSMetricsLaneWire 快车道端到端（2026-11-01）：走真实 ServeWS 握手与真实 WS 帧，
// 把两侧的 JSON 线格式钉住——单元测试用同一份 struct 拼装，测不到 tag 写错这类问题。
//
//	① auth_ok 必须声明 metrics_split（否则节点永不启用快车道，面板永远没有速率）；
//	② 节点发来的 metrics 帧要能落到内存快照（CPU/速率/连接数/每用户速率/采样时刻）；
//	③ 随后的慢心跳（实时字段缺省）不得把这些值清掉。
func TestServeWSMetricsLaneWire(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wire.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Server{}, &models.NodeReport{}, &models.Setting{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	h := NewHub(db, nil, nil)
	t.Cleanup(h.Shutdown)
	srv := models.Server{NodeID: "node-metrics", Secret: util.HashSecret("sec"), Name: "m"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/node/ws", h.ServeWS)
	ts := httptest.NewServer(r)
	defer ts.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http")+"/node/ws?node_id=node-metrics", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	auth, _ := protocol.Encode(protocol.MsgAuth, "", protocol.AuthPayload{NodeID: "node-metrics", Secret: "sec"})
	if err := ws.WriteMessage(websocket.TextMessage, auth); err != nil {
		t.Fatal(err)
	}
	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	msg, err := protocol.Decode(data)
	if err != nil || msg.Type != protocol.MsgAuthOK {
		t.Fatalf("应回 auth_ok: %v %s", err, string(data))
	}
	var caps protocol.AuthOKPayload
	if err := msg.PayloadTo(&caps); err != nil {
		t.Fatal(err)
	}
	if !caps.HasCap(protocol.CapMetricsSplit) {
		t.Fatalf("auth_ok 必须声明 %s，实际 %s", protocol.CapMetricsSplit, string(msg.Payload))
	}

	// ② 快车道帧
	metrics, _ := protocol.Encode(protocol.MsgMetrics, "", protocol.MetricsPayload{
		CPU: 12.5, RxRate: 2048, TxRate: 1024, RxBytes: 111, TxBytes: 222, XrayRunning: true,
		Conns:     &protocol.ConnStatsPayload{TCPEstablished: 3, TCPTotal: 8, UDP: 2},
		UserRates: []protocol.UserRatePayload{{Email: "u1.i2@panel.local", Up: 10, Down: 20}},
		TS:        1,
	})
	if err := ws.WriteMessage(websocket.TextMessage, metrics); err != nil {
		t.Fatal(err)
	}
	waitMetrics(t, h, srv.ID, func(m *NodeMetricsSnapshot) bool {
		return m.CPU == 12.5 && len(m.UserRates) == 1 && !m.MetricsAt.IsZero()
	})

	// ③ 慢心跳（实时字段缺省，模拟新 agent）
	hb, _ := protocol.Encode(protocol.MsgHeartbeat, "", protocol.HeartbeatPayload{
		Mem: 4096, MemTotal: 8192, Disk: 1, DiskTotal: 2, XrayRunning: true, OnlineUsers: 1,
		OnlineIPs: []protocol.OnlineUserIPs{{Email: "u1.i2@panel.local", IPs: []string{"1.2.3.4"}}},
		Conns:     &protocol.ConnStatsPayload{TCPEstablished: 4, TCPTotal: 9, UDP: 2},
		TS:        2,
	})
	if err := ws.WriteMessage(websocket.TextMessage, hb); err != nil {
		t.Fatal(err)
	}
	waitMetrics(t, h, srv.ID, func(m *NodeMetricsSnapshot) bool {
		return m.Mem == 4096 && m.CPU == 12.5 && m.RxRate == 2048 &&
			len(m.UserRates) == 1 && m.OnlineUsers == 1
	})

	// 收尾：等网关注销连接，避免清理与在线落库/后台推送并发（见 closeWSAndWait）。
	closeWSAndWait(t, h, ws, srv.ID)
}

// waitMetrics 等网关异步处理帧（读循环在另一个 goroutine），超时前轮询内存快照。
func waitMetrics(t *testing.T, h *Hub, id uint64, ok func(*NodeMetricsSnapshot) bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if m, found := h.GetLatestMetrics(id); found && ok(m) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	m, _ := h.GetLatestMetrics(id)
	t.Fatalf("快照未达预期: %+v", m)
}
