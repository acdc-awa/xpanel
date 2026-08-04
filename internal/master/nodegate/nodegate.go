// Package nodegate 实现主控侧的节点 WebSocket 网关：
// 接收节点连接/认证/心跳，维护在线注册表，并向指定节点下发指令。
package nodegate

import (
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/master/services"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/protocol"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// 心跳与超时参数
const (
	HeartbeatTimeout = 90 * time.Second // 超过此时长无心跳视为失联
	WriteTimeout     = 10 * time.Second
	AskTimeout       = 15 * time.Second // 指令等待回执超时
)

// Conn 一条节点连接。
type Conn struct {
	ServerID uint64
	NodeID   string
	WS       *websocket.Conn
	Send     chan []byte
	LastSeen int64 // unix 秒
	mu       sync.Mutex
	closed   bool
}

// touch 更新最近活跃时间。
func (c *Conn) touch() { c.LastSeen = time.Now().Unix() }

// closeSafe 幂等关闭。
func (c *Conn) closeSafe() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		_ = c.WS.Close()
	}
}

// Hub 节点连接注册表。
type Hub struct {
	DB       *gorm.DB
	Traffic  *services.TrafficService
	Config   *services.ConfigService
	Upgrader websocket.Upgrader

	mu      sync.RWMutex
	conns   map[uint64]*Conn                      // by server_id
	pending map[string]chan *protocol.ResultPayload // 请求 id → 回执通道
}

// NewHub 构造网关。
func NewHub(db *gorm.DB, traffic *services.TrafficService, config *services.ConfigService) *Hub {
	h := &Hub{
		DB:      db,
		Traffic: traffic,
		Config:  config,
		Upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// dev 环境放开跨域；生产由 Nginx/Caddy 同域反代并收紧
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		conns:   make(map[uint64]*Conn),
		pending: make(map[string]chan *protocol.ResultPayload),
	}
	go h.watchdog()
	return h
}

// ServeWS 处理节点 WebSocket 连接。
func (h *Hub) ServeWS(c *gin.Context) {
	ws, err := h.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	// 第一步必须是 auth
	_ = ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil {
		_ = ws.Close()
		return
	}
	msg, err := protocol.Decode(data)
	if err != nil || msg.Type != protocol.MsgAuth {
		_ = h.writeRaw(ws, "bad_auth", "", protocol.ResultPayload{OK: false, Error: "首条消息必须是 auth"})
		_ = ws.Close()
		return
	}
	var auth protocol.AuthPayload
	if err := msg.PayloadTo(&auth); err != nil {
		_ = ws.Close()
		return
	}

	server, err := h.authenticate(auth)
	if err != nil {
		_ = h.writeRaw(ws, "bad_auth", msg.ID, protocol.ResultPayload{OK: false, Error: err.Error()})
		_ = ws.Close()
		return
	}

	conn := &Conn{
		ServerID: server.ID,
		NodeID:   server.NodeID,
		WS:       ws,
		Send:     make(chan []byte, 64),
		LastSeen: time.Now().Unix(),
	}
	h.register(conn)
	_ = h.writeRaw(ws, "auth_ok", msg.ID, protocol.ResultPayload{OK: true})
	_ = ws.SetReadDeadline(time.Time{})

	// 标记在线
	h.DB.Model(&models.Server{}).Where("id = ?", server.ID).
		Updates(map[string]any{"status": 1, "last_seen_at": time.Now()})

	// 节点上线：自动补推待推送配置（非阻塞）
	go h.PushPending(server.ID)

	go h.writePump(conn)
	h.readPump(conn)
}

// authenticate 校验 node_id + secret。
func (h *Hub) authenticate(a protocol.AuthPayload) (*models.Server, error) {
	var server models.Server
	if err := h.DB.Where("node_id = ?", a.NodeID).First(&server).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("节点不存在")
		}
		return nil, errors.New("数据库错误")
	}
	if util.HashSecret(a.Secret) != server.Secret {
		return nil, errors.New("节点密钥错误")
	}
	return &server, nil
}

func (h *Hub) register(conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// 同节点旧连接替换
	if old, ok := h.conns[conn.ServerID]; ok {
		old.closeSafe()
	}
	h.conns[conn.ServerID] = conn
}

func (h *Hub) unregister(conn *Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cur, ok := h.conns[conn.ServerID]; ok && cur == conn {
		delete(h.conns, conn.ServerID)
	}
	close(conn.Send)
	h.DB.Model(&models.Server{}).Where("id = ?", conn.ServerID).
		Update("status", 0)
}

// readPump 读取消息循环。
func (h *Hub) readPump(conn *Conn) {
	defer func() {
		h.unregister(conn)
		conn.closeSafe()
	}()
	for {
		_, data, err := conn.WS.ReadMessage()
		if err != nil {
			return
		}
		msg, err := protocol.Decode(data)
		if err != nil {
			continue
		}
		conn.touch()
		switch msg.Type {
		case protocol.MsgHeartbeat:
			h.handleHeartbeat(conn, msg)
		case protocol.MsgResult:
			h.handleResult(msg)
		case protocol.MsgTrafficReport:
			h.handleTrafficReport(conn, msg)
		}
	}
}

// handleTrafficReport 处理节点流量上报（幂等落库）。
func (h *Hub) handleTrafficReport(conn *Conn, msg *protocol.Message) {
	if h.Traffic == nil {
		return
	}
	var tr protocol.TrafficReportPayload
	if err := msg.PayloadTo(&tr); err != nil {
		return
	}
	if err := h.Traffic.Save(tr); err != nil {
		log.Printf("nodegate: 流量落库失败 (server=%d): %v", conn.ServerID, err)
	}
}

func (h *Hub) handleHeartbeat(conn *Conn, msg *protocol.Message) {
	var hb protocol.HeartbeatPayload
	_ = msg.PayloadTo(&hb)
	h.DB.Model(&models.Server{}).Where("id = ?", conn.ServerID).Updates(map[string]any{
		"status":       1,
		"last_seen_at": time.Now(),
	})
	// node_reports 落库（供仪表盘趋势）
	_ = h.DB.Create(&models.NodeReport{
		ServerID:    conn.ServerID,
		CPU:         hb.CPU,
		Mem:         hb.Mem,
		OnlineUsers: hb.OnlineUsers,
		RxRate:      hb.RxRate,
		TxRate:      hb.TxRate,
		ReportedAt:  time.Now(),
	}).Error
}

func (h *Hub) handleResult(msg *protocol.Message) {
	if msg.ID == "" {
		return
	}
	var res protocol.ResultPayload
	_ = msg.PayloadTo(&res)
	h.mu.Lock()
	ch, ok := h.pending[msg.ID]
	if ok {
		delete(h.pending, msg.ID)
	}
	h.mu.Unlock()
	if ok && ch != nil {
		select {
		case ch <- &res:
		default:
		}
	}
}

// writePump 从 Send 通道写消息。
func (h *Hub) writePump(conn *Conn) {
	for data := range conn.Send {
		_ = conn.WS.SetWriteDeadline(time.Now().Add(WriteTimeout))
		if err := conn.WS.WriteMessage(websocket.TextMessage, data); err != nil {
			return
		}
	}
}

func (h *Hub) writeRaw(ws *websocket.Conn, typ, id string, payload any) error {
	data, err := protocol.Encode(typ, id, payload)
	if err != nil {
		return err
	}
	_ = ws.SetWriteDeadline(time.Now().Add(WriteTimeout))
	return ws.WriteMessage(websocket.TextMessage, data)
}

// IsOnline 节点是否在线（连接存在且心跳新鲜）。
func (h *Hub) IsOnline(serverID uint64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, ok := h.conns[serverID]
	if !ok {
		return false
	}
	return time.Since(time.Unix(conn.LastSeen, 0)) < HeartbeatTimeout
}

// Send 向节点发送消息（不等待回执）。
func (h *Hub) Send(serverID uint64, msg []byte) error {
	h.mu.RLock()
	conn, ok := h.conns[serverID]
	h.mu.RUnlock()
	if !ok {
		return errors.New("节点离线")
	}
	select {
	case conn.Send <- msg:
		return nil
	case <-time.After(WriteTimeout):
		return errors.New("节点发送超时")
	}
}

// Ask 向节点发送指令并等待 result 回执（带超时）。
func (h *Hub) Ask(serverID uint64, typ string, payload any, timeout time.Duration) (*protocol.ResultPayload, error) {
	id := util.RandomID(8)
	ch := make(chan *protocol.ResultPayload, 1)
	h.mu.Lock()
	h.pending[id] = ch
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
	}()

	data, err := protocol.Encode(typ, id, payload)
	if err != nil {
		return nil, err
	}
	if err := h.Send(serverID, data); err != nil {
		return nil, err
	}
	select {
	case res := <-ch:
		return res, nil
	case <-time.After(timeout):
		return nil, errors.New("等待节点回执超时")
	}
}

// PushPending 若存在待推送配置且节点在线，则下发并标记已推送（非阻塞，调用方用 goroutine）。
func (h *Hub) PushPending(serverID uint64) {
	if h.Config == nil {
		return
	}
	p, err := h.Config.GetPending(serverID)
	if err != nil || p == nil || p.Status == "pushed" {
		return
	}
	if !h.IsOnline(serverID) {
		return // 节点离线，保留待推送（上线时由 ServeWS 再次触发）
	}
	res, err := h.Ask(serverID, protocol.MsgPushConfig, protocol.PushConfigPayload{ConfigJSON: p.ConfigJSON}, AskTimeout)
	if err == nil && res != nil && res.OK {
		if merr := h.Config.MarkPushed(p.ID); merr == nil {
			log.Printf("nodegate: 已自动推送配置到节点 %d", serverID)
		}
	}
}

// watchdog 扫描心跳超时连接并关闭（触发 unregister → 离线）。
func (h *Hub) watchdog() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		h.mu.RLock()
		var stale []*Conn
		for _, c := range h.conns {
			if time.Since(time.Unix(c.LastSeen, 0)) > HeartbeatTimeout {
				stale = append(stale, c)
			}
		}
		h.mu.RUnlock()
		for _, c := range stale {
			log.Printf("nodegate: 节点 %s 心跳超时，断开", c.NodeID)
			c.closeSafe()
		}
	}
}