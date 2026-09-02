// Package nodegate 实现主控侧的节点 WebSocket 网关：
// 接收节点连接/认证/心跳，维护在线注册表，并向指定节点下发指令。
package nodegate

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// 心跳与超时参数
const (
	HeartbeatTimeout = 90 * time.Second // 超过此时长无心跳视为失联
	WriteTimeout     = 10 * time.Second
	AskTimeout       = 30 * time.Second // 指令等待回执超时（需 > testTimeout + stopGracePeriod + RTT）
	// UpgradeAskTimeout 自升级指令等待回执超时：节点需从 GitHub Releases 拉取二进制
	//（网络慢时可达数分钟），且成功回执在二进制替换完成后才发出。
	UpgradeAskTimeout = 5 * time.Minute
	PongWait          = 60 * time.Second // 等待 pong 回复超时
	PingPeriod        = (PongWait * 9) / 10

	// enforcedThrottle 事件驱动超额/到期处置的节流窗口：同一用户命中后 3 分钟内不重复
	// 触发全节点同步（用户被移除后不再产生上报，正常只触发一次；此为重试/多节点并发兜底）。
	enforcedThrottle = 3 * time.Minute
)

// Conn 一条节点连接。
type Conn struct {
	ServerID uint64
	NodeID   string
	WS       *websocket.Conn
	Send     chan []byte
	done     chan struct{} // 关闭信号：通知 writePump 退出（代替关闭 Send，避免与并发 Send 竞态）
	LastSeen atomic.Int64  // unix 秒：readPump/pong 写，IsOnline/watchdog 读，必须原子
	mu       sync.Mutex
	closed   bool
}

// touch 更新最近活跃时间。
func (c *Conn) touch() { c.LastSeen.Store(time.Now().Unix()) }

// closeSafe 幂等关闭：置 closed、关闭 done 通知 writePump 退出、关闭 WS。
// 注意：这里刻意不 close(c.Send)——Send 通道可能正被并发 Send/Ask 选中发送，
// 关闭它会触发 "send on closed channel" panic（详见 reliability_test.go 的竞态说明）。
func (c *Conn) closeSafe() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.done)
		if c.WS != nil {
			_ = c.WS.Close()
		}
	}
}

type pendingReq struct {
	serverID uint64
	ch       chan *protocol.ResultPayload
}

// Hub 节点连接注册表。
type Hub struct {
	DB       *gorm.DB
	Traffic  contracts.TrafficService
	Config   contracts.ConfigService
	Upgrader websocket.Upgrader
	// CertPusher 节点上线后补推待推证书的回调（U7，由 main 注入 api 层实现）。
	CertPusher func(serverID uint64)

	mu      sync.RWMutex
	conns   map[uint64]*Conn               // by server_id
	pending map[string]*pendingReq         // 请求 id → 回执请求
	wg      sync.WaitGroup
	quit    chan struct{}

	enforcedMu sync.Mutex
	enforcedAt map[uint64]time.Time // 事件驱动处置节流：userID → 上次触发时刻（watchdog 定期清理）
}

// NewHub 构造网关。
func NewHub(db *gorm.DB, traffic contracts.TrafficService, config contracts.ConfigService) *Hub {
	h := &Hub{
		DB:      db,
		Traffic: traffic,
		Config:  config,
		Upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// P2-4：无 Origin 的 agent/CLI 放行；浏览器带 Origin 时仅允许与请求 Host 同源。
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				u, err := url.Parse(origin)
				if err != nil || u.Host == "" {
					return false
				}
				return strings.EqualFold(u.Host, r.Host)
			},
		},
		conns:   make(map[uint64]*Conn),
		pending: make(map[string]*pendingReq),
		quit:    make(chan struct{}),
	}
	h.enforcedAt = make(map[uint64]time.Time)
	h.wg.Add(1)
	go h.watchdog()
	return h
}

// Shutdown 优雅关闭：停止 watchdog，断开所有节点连接。
func (h *Hub) Shutdown() {
	close(h.quit)
	h.wg.Wait()
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.conns {
		c.closeSafe()
	}
}

// ServeWS 处理节点 WebSocket 连接。
func (h *Hub) ServeWS(c *gin.Context) {
	ws, err := h.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	ws.SetReadLimit(1024 * 1024)

	// 第一步必须是 auth
	_ = ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil {
		_ = ws.Close()
		return
	}
	msg, err := protocol.Decode(data)
	if err != nil || msg.Type != protocol.MsgAuth {
		_ = h.writeRaw(ws, protocol.MsgAuthBad, "", protocol.ResultPayload{OK: false, Error: "首条消息必须是 auth"})
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
		_ = h.writeRaw(ws, protocol.MsgAuthBad, msg.ID, protocol.ResultPayload{OK: false, Error: err.Error()})
		_ = ws.Close()
		return
	}

	conn := &Conn{
		ServerID: server.ID,
		NodeID:   server.NodeID,
		WS:       ws,
		Send:     make(chan []byte, 64),
		done:     make(chan struct{}),
	}
	conn.LastSeen.Store(time.Now().Unix())
	h.register(conn)
	_ = h.writeRaw(ws, protocol.MsgAuthOK, msg.ID, protocol.ResultPayload{OK: true})
	_ = ws.SetReadDeadline(time.Time{})

	// 标记在线
	h.DB.Model(&models.Server{}).Where("id = ?", server.ID).
		Updates(map[string]any{"status": 1, "last_seen_at": time.Now()})

	// 节点上线：自动补推待推送配置 + 待推证书（非阻塞）+ 对齐最新用户名单 + 下发运行时设置
	go h.PushPending(server.ID)
	if h.CertPusher != nil {
		go h.CertPusher(server.ID)
	}
	// 重连即对齐最新有效用户名单：离线期间发生的用户超额/到期/禁用，PushPending 推的
	// 可能是旧配置，热更一次立即移除，不必等 1h 校准（2026-09-01）
	go func(id uint64) {
		if err := h.SyncUsers(id); err != nil {
			log.Printf("nodegate: 节点 %d 上线同步用户列表失败: %v", id, err)
		}
	}(server.ID)
	// 下发当前运行时设置（上报/心跳周期，设置页可调；旧 agent 静默忽略，yaml 兜底）
	go h.PushAgentSettings(server.ID)

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
	// 无认证端点上的密钥比较必须恒定时间，防按字节猜解的时序侧信道。
	if subtle.ConstantTimeCompare([]byte(util.HashSecret(a.Secret)), []byte(server.Secret)) != 1 {
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
	if cur, ok := h.conns[conn.ServerID]; ok && cur == conn {
		delete(h.conns, conn.ServerID)
	}
	_, hasReplacement := h.conns[conn.ServerID]

	// 当节点连接断开时，若该节点有等待回执的 Ask 指令，立即以错误唤醒，
	// 避免调用方（如自升级等待 5 分钟、配置推送等待 30 秒）在连接断开后死等满超时
	if !hasReplacement {
		for id, req := range h.pending {
			if req.serverID == conn.ServerID {
				select {
				case req.ch <- &protocol.ResultPayload{OK: false, Error: "节点连接已断开"}:
				default:
				}
				delete(h.pending, id)
			}
		}
	}
	h.mu.Unlock()
	conn.closeSafe() // 幂等；只关闭 done/WS，不关闭 Send 通道（避免与并发 Send 竞态 panic）

	// P2-10：仅当注册表中已无该节点的新连接时才把 DB 状态置 0，
	// 避免旧连接退出与新连接注册交错时把在线节点短暂标离线。
	if !hasReplacement {
		h.DB.Model(&models.Server{}).Where("id = ?", conn.ServerID).
			Update("status", 0)
	}
}

// readPump 读取消息循环。
func (h *Hub) readPump(conn *Conn) {
	defer func() {
		h.unregister(conn)
		conn.closeSafe()
	}()
	_ = conn.WS.SetReadDeadline(time.Now().Add(PongWait))
	conn.WS.SetPongHandler(func(string) error {
		_ = conn.WS.SetReadDeadline(time.Now().Add(PongWait))
		conn.touch()
		return nil
	})
	for {
		_, data, err := conn.WS.ReadMessage()
		if err != nil {
			return
		}
		_ = conn.WS.SetReadDeadline(time.Now().Add(PongWait)) // 每次收到有效数据帧均续期
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
		case protocol.MsgInternalUUIDReport:
			h.handleInternalUUIDReport(conn, msg)
		}
	}
}

// handleInternalUUIDReport 节点侧内部 UUID 变更主动上报（如 CLI 轮换）：
// 更新入站 internal_uuid，并按 tag 匹配本服务器 relay 入站。
func (h *Hub) handleInternalUUIDReport(conn *Conn, msg *protocol.Message) {
	var p protocol.InternalUUIDReportPayload
	if err := msg.PayloadTo(&p); err != nil {
		return
	}
	if p.Tag == "" || p.UUID == "" {
		return
	}
	var inb models.Inbound
	err := h.DB.Where("server_id = ? AND tag = ? AND type = ?", conn.ServerID, p.Tag, models.InboundTypeRelay).First(&inb).Error
	if err != nil || inb.InternalUUID == p.UUID {
		return
	}
	if err := h.DB.Model(&inb).Update("internal_uuid", p.UUID).Error; err == nil && h.Config != nil {
		// UUID 变更 → 重新生成配置（引用该落地出站的服务器配置会随之更新）
		if _, err := h.Config.Generate(conn.ServerID); err == nil {
			h.PushPending(conn.ServerID)
		}
	}
}

// handleTrafficReport 处理节点流量上报（幂等落库）+ 事件驱动超额/到期处置：
// 落库后对本帧涉及用户做增量限额判定，命中即热更全节点用户列表（gRPC 秒级移除，
// 不重启 xray）——「超额后还能跑流量 ~1h」问题的根治路径（2026-09-01），
// 最坏端到端时延从 1h 校准兜底降为 ≈1 个上报周期。
func (h *Hub) handleTrafficReport(conn *Conn, msg *protocol.Message) {
	if h.Traffic == nil {
		return
	}
	var tr protocol.TrafficReportPayload
	if err := msg.PayloadTo(&tr); err != nil {
		return
	}
	ids, err := h.Traffic.Save(tr, conn.ServerID)
	if err != nil {
		log.Printf("nodegate: 流量落库失败 (server=%d): %v", conn.ServerID, err)
		return
	}
	if len(ids) == 0 {
		return
	}
	violators, err := h.Traffic.FindViolators(ids)
	if err != nil {
		log.Printf("nodegate: 超额/到期判定失败 (server=%d): %v", conn.ServerID, err)
		return
	}
	if len(violators) == 0 {
		return
	}
	// 节流：同一用户 3 分钟内不重复触发（用户被移除后不再产生上报，正常只触发一次）
	now := time.Now()
	fire := false
	h.enforcedMu.Lock()
	for _, id := range violators {
		if last, ok := h.enforcedAt[id]; !ok || now.Sub(last) > enforcedThrottle {
			h.enforcedAt[id] = now
			fire = true
		}
	}
	h.enforcedMu.Unlock()
	if !fire {
		return
	}
	log.Printf("nodegate: 流量上报触发违规处置（超额/到期）共 %d 用户，热更全节点用户列表: %v", len(violators), violators)
	h.SyncUsersToAll()
}

// pruneEnforced 清理过期的处置节流记录，防 map 无界增长（watchdog 15s tick 调用）。
func (h *Hub) pruneEnforced() {
	cut := time.Now().Add(-10 * time.Minute)
	h.enforcedMu.Lock()
	defer h.enforcedMu.Unlock()
	for id, at := range h.enforcedAt {
		if at.Before(cut) {
			delete(h.enforcedAt, id)
		}
	}
}

func (h *Hub) handleHeartbeat(conn *Conn, msg *protocol.Message) {
	var hb protocol.HeartbeatPayload
	_ = msg.PayloadTo(&hb)
	updates := map[string]any{
		"status":       1,
		"last_seen_at": time.Now(),
		"xray_running": hb.XrayRunning,
	}
	if hb.Version != "" { // 旧 agent 不上报版本，不覆盖已有值
		updates["agent_version"] = hb.Version
	}
	// 在线用户 IP 快照每次覆写：新版 agent 心跳携带；旧 agent 或无人在线为空列表（如实清空）
	if len(hb.OnlineIPs) == 0 {
		updates["online_ips"] = "[]"
	} else if b, err := json.Marshal(hb.OnlineIPs); err == nil {
		updates["online_ips"] = string(b)
	}
	h.DB.Model(&models.Server{}).Where("id = ?", conn.ServerID).Updates(updates)
	// node_reports 落库（供仪表盘趋势）
	_ = h.DB.Create(&models.NodeReport{
		ServerID:    conn.ServerID,
		CPU:         hb.CPU,
		Mem:         hb.Mem,
		MemTotal:    uint64(hb.MemTotal),
		Disk:        hb.Disk,
		DiskTotal:   uint64(hb.DiskTotal),
		OnlineUsers: hb.OnlineUsers,
		RxRate:      hb.RxRate,
		TxRate:      hb.TxRate,
		RxBytes:     hb.RxBytes,
		TxBytes:     hb.TxBytes,
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
	req, ok := h.pending[msg.ID]
	if ok {
		delete(h.pending, msg.ID)
	}
	h.mu.Unlock()
	if ok && req != nil && req.ch != nil {
		select {
		case req.ch <- &res:
		default:
		}
	}
}

// Disconnect 主动断开指定服务器的 WebSocket 连接（如删除服务器或重置密钥时调用）。
func (h *Hub) Disconnect(serverID uint64) {
	h.mu.Lock()
	conn, ok := h.conns[serverID]
	h.mu.Unlock()
	if ok && conn != nil {
		conn.closeSafe()
	}
}

// writePump 从 Send 通道写消息。
func (h *Hub) writePump(conn *Conn) {
	ticker := time.NewTicker(PingPeriod)
	defer ticker.Stop()
	for {
		select {
		case data, ok := <-conn.Send:
			if !ok {
				_ = conn.WS.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = conn.WS.SetWriteDeadline(time.Now().Add(WriteTimeout))
			if err := conn.WS.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-conn.done:
			return
		case <-ticker.C:
			_ = conn.WS.SetWriteDeadline(time.Now().Add(WriteTimeout))
			if err := conn.WS.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
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
	return time.Since(time.Unix(conn.LastSeen.Load(), 0)) < HeartbeatTimeout
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
	req := &pendingReq{serverID: serverID, ch: ch}
	h.mu.Lock()
	h.pending[id] = req
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
// 2026-08-31：失败路径补日志——此前 Ask 出错/节点拒绝均静默，生成后"没推送"无从诊断。
func (h *Hub) PushPending(serverID uint64) {
	if h.Config == nil {
		return
	}
	p, err := h.Config.GetPending(serverID)
	if err != nil {
		log.Printf("nodegate: 读取待推送配置失败 (server=%d): %v", serverID, err)
		return
	}
	if p == nil || p.Status == "pushed" {
		return
	}
	if !h.IsOnline(serverID) {
		log.Printf("nodegate: 节点 %d 离线，配置待推送（上线时由 ServeWS 自动补推）", serverID)
		h.recordPushFailure(p.ID, "节点离线，等待上线自动补推")
		return
	}
	res, err := h.Ask(serverID, protocol.MsgPushConfig, protocol.PushConfigPayload{ConfigJSON: p.ConfigJSON}, AskTimeout)
	if err != nil {
		log.Printf("nodegate: 推送配置失败 (server=%d): %v（保留待推送）", serverID, err)
		h.recordPushFailure(p.ID, err.Error())
		return
	}
	if res == nil || !res.OK {
		msg := "无回执"
		if res != nil && res.Error != "" {
			msg = res.Error
		}
		log.Printf("nodegate: 节点 %d 拒绝推送的配置: %s（保留待推送）", serverID, msg)
		h.recordPushFailure(p.ID, "节点拒绝: "+msg)
		return
	}
	marked, merr := h.Config.MarkPushedIfSame(p.ID, p.ConfigJSON)
	if merr != nil {
		log.Printf("nodegate: 标记已推送失败 (server=%d): %v", serverID, merr)
		return
	}
	if marked {
		log.Printf("nodegate: 已自动推送配置到节点 %d", serverID)
	} else {
		// 推送期间 pending 已被更新（如用户编辑/每小时校准），保持 pending 待下一轮推送
		log.Printf("nodegate: 节点 %d 推送成功但 pending 内容已被更新，保留待推送", serverID)
	}
}

// recordPushFailure 记录一次配置推送失败：last_error/attempts/last_attempt_at 落到
// PendingConfig 行上，面板"待推送"旁直接展示原因，不必翻主控日志。失败不改变 pending 状态。
func (h *Hub) recordPushFailure(pendingID uint64, reason string) {
	if len(reason) > 500 { // 与模型列宽一致，防 agent 回执里的配置报错细节超长；按 rune 截避免切断多字节字符
		reason = string([]rune(reason)[:500])
	}
	if err := h.DB.Model(&models.PendingConfig{}).Where("id = ?", pendingID).
		Updates(map[string]any{
			"last_error":      reason,
			"attempts":        gorm.Expr("attempts + 1"),
			"last_attempt_at": time.Now(),
		}).Error; err != nil {
		log.Printf("nodegate: 记录推送失败状态出错 (pending=%d): %v", pendingID, err)
	}
}

// SyncUsers 计算指定节点最新有效用户并发送 MsgSyncUsers。
func (h *Hub) SyncUsers(serverID uint64) error {
	if h.Config == nil {
		return errors.New("config service not initialized")
	}
	usersMap, err := h.Config.GetValidUsers(serverID)
	if err != nil {
		return err
	}
	payload := protocol.SyncUsersPayload{
		Users: usersMap,
	}
	res, err := h.Ask(serverID, protocol.MsgSyncUsers, payload, AskTimeout)
	if err != nil {
		return err
	}
	if res != nil && !res.OK {
		return errors.New(res.Error)
	}
	return nil
}

// SyncUsersToAll 广播给所有在线节点增量/全量同步最新用户列表（非阻塞）。
func (h *Hub) SyncUsersToAll() {
	h.mu.RLock()
	var serverIDs []uint64
	for id := range h.conns {
		serverIDs = append(serverIDs, id)
	}
	h.mu.RUnlock()

	for _, id := range serverIDs {
		go func(sid uint64) {
			if err := h.SyncUsers(sid); err != nil {
				log.Printf("nodegate: 向节点 %d 动态同步用户失败: %v", sid, err)
			}
		}(id)
	}
}

// PushAgentSettings 向单个节点下发运行时设置（上报/心跳周期，设置页「节点上报」可调）。
// 节点连接建立与设置保存时调用；离线报错仅记日志（重连时 ServeWS 会重新下发）。
func (h *Hub) PushAgentSettings(serverID uint64) {
	payload := protocol.AgentSettingsPayload{
		ReportIntervalSec:    services.AgentReportIntervalSec(h.DB),
		HeartbeatIntervalSec: services.AgentHeartbeatIntervalSec(h.DB),
	}
	res, err := h.Ask(serverID, protocol.MsgAgentSettings, payload, AskTimeout)
	if err != nil {
		log.Printf("nodegate: 下发运行时设置失败 (server=%d): %v", serverID, err)
		return
	}
	if res != nil && !res.OK {
		log.Printf("nodegate: 节点 %d 拒绝运行时设置: %s", serverID, res.Error)
	}
}

// BroadcastAgentSettings 向所有在线节点下发运行时设置（设置保存后调用，非阻塞）。
func (h *Hub) BroadcastAgentSettings() {
	h.mu.RLock()
	var serverIDs []uint64
	for id := range h.conns {
		serverIDs = append(serverIDs, id)
	}
	h.mu.RUnlock()
	for _, id := range serverIDs {
		go h.PushAgentSettings(id)
	}
}

func (h *Hub) watchdog() {
	defer h.wg.Done()
	ticker := time.NewTicker(15 * time.Second)
	retryTicker := time.NewTicker(2 * time.Minute) // 待推送配置短周期补推（2026-08-31）
	alignTicker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	defer retryTicker.Stop()
	defer alignTicker.Stop()
	for {
		select {
		case <-h.quit:
			return
		case <-retryTicker.C:
			// 在线节点的待推送配置每 2 分钟补推一次：推送失败（Ask 出错/节点拒绝）不再依赖
			// 1 小时校准兜底。推的是已保存的 config_json（不重新 Generate），绕开 Generate
			// 失败导致的重推死锁；离线节点由上线时 ServeWS 自动补推覆盖，故只遍历注册表。
			h.mu.RLock()
			var serverIDs []uint64
			for id := range h.conns {
				serverIDs = append(serverIDs, id)
			}
			h.mu.RUnlock()
			for _, id := range serverIDs {
				go h.PushPending(id)
			}
		case <-ticker.C:
			h.pruneEnforced()
			h.mu.RLock()
			var stale []*Conn
			for _, c := range h.conns {
				if time.Since(time.Unix(c.LastSeen.Load(), 0)) > HeartbeatTimeout {
					stale = append(stale, c)
				}
			}
			h.mu.RUnlock()
			for _, c := range stale {
				log.Printf("nodegate: 节点 %s 心跳超时，断开", c.NodeID)
				c.closeSafe()
			}
		case <-alignTicker.C:
			// 定期 1 小时 100% 状态校准全量推送
			h.mu.RLock()
			var serverIDs []uint64
			for id := range h.conns {
				serverIDs = append(serverIDs, id)
			}
			h.mu.RUnlock()
			for _, id := range serverIDs {
				go func(sid uint64) {
					log.Printf("nodegate: 执行定期全量状态校准 (server=%d)", sid)
					if h.Config != nil {
						cfgStr, err := h.Config.Generate(sid)
						if err == nil {
							_ = h.Config.SavePending(sid, cfgStr)
							h.PushPending(sid)
						}
					}
				}(id)
			}
		}
	}
}
