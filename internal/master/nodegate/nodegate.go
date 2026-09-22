// Package nodegate 实现主控侧的节点 WebSocket 网关：
// 接收节点连接/认证/心跳，维护在线注册表，并向指定节点下发指令。
package nodegate

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// 心跳与超时参数
const (
	HeartbeatTimeout = 90 * time.Second // 超过此时长无心跳视为失联
	WriteTimeout     = 10 * time.Second
	AskTimeout       = 30 * time.Second // 指令等待回执超时（需 > testTimeout + stopGracePeriod + RTT）
	// SetupInternalAskTimeout setup-internal 指令等待回执超时：节点侧仅幂等读文件或原子写盘，
	// 刻意短于 AskTimeout 以免管理端保存/创建入站被慢节点拖住；超时由主控生成 UUID 兜底，
	// 配置以主控 DB 为准，晚到的节点回执被忽略无实际影响。rotate 走管理端显式端点仍用 AskTimeout。
	SetupInternalAskTimeout = 3 * time.Second
	// UpgradeAskTimeout 自升级指令等待回执超时：节点需从 GitHub Releases 拉取二进制
	//（网络慢时可达数分钟），且成功回执在二进制替换完成后才发出。
	UpgradeAskTimeout = 5 * time.Minute
	PongWait          = 60 * time.Second // 等待 pong 回复超时
	PingPeriod        = (PongWait * 9) / 10

	// enforcedThrottle 事件驱动超额/到期处置的节流窗口：同一用户命中后 3 分钟内不重复
	// 触发全节点同步（用户被移除后不再产生上报，正常只触发一次；此为重试/多节点并发兜底）。
	enforcedThrottle = 3 * time.Minute

	// nodeReportSampleInterval node_reports 落库采样间隔。心跳仍按 heartbeat_interval（默认 30s）
	// 保活并即时更新 servers.last_seen_at/status，但监控指标行按此间隔抽稀落库——30s 一行会让
	// 每个节点每天产生 2880 行（含索引约 0.4MB/天/节点），是 node_reports 膨胀的主因。
	// 取 1 分钟：节点监控曲线最长回看 7 天、读取时再按 1m/3m/10m/1h 二次分桶，
	// 1 分钟采样对全部区间都不降分辨率。
	nodeReportSampleInterval = time.Minute
)

// NodeMetricsSnapshot 节点实时指标纯内存快照（供实时大盘毫秒级无锁读取，0 磁盘 I/O）。
type NodeMetricsSnapshot struct {
	ServerID    uint64    `json:"server_id"`
	CPU         float64   `json:"cpu"`
	Mem         float64   `json:"mem"`
	MemTotal    uint64    `json:"mem_total"`
	Disk        float64   `json:"disk"`
	DiskTotal   uint64    `json:"disk_total"`
	OnlineUsers int       `json:"online_users"`
	RxRate      float64   `json:"rx_rate"`
	TxRate      float64   `json:"tx_rate"`
	RxBytes     uint64    `json:"rx_bytes"`
	TxBytes     uint64    `json:"tx_bytes"`
	XrayRunning bool      `json:"xray_running"`
	ReportedAt  time.Time `json:"reported_at"`
}

// Conn 一条节点连接。
type Conn struct {
	ServerID uint64
	NodeID   string
	WS       *websocket.Conn
	Send     chan []byte
	done     chan struct{} // 关闭信号：通知 writePump 退出（代替关闭 Send，避免与并发 Send/Ask 竞态）
	LastSeen atomic.Int64  // unix 秒：readPump/pong 写，IsOnline/watchdog 读，必须原子
	// settingsSynced 运行时设置（上报/心跳周期）是否已成功下发至当前连接：建连为 false，
	// PushAgentSettings 成功置 true；watchdog 2 分钟补推循环据此决定是否重试（失败不置位）。
	settingsSynced atomic.Bool
	// usersSynced 用户名单与账期映射是否已成功下发至当前连接：建连为 false，
	// SyncUsers 成功置 true；watchdog 2 分钟补推循环据此决定是否重试（失败不置位）。
	// 用户变更 / 账期切换广播（SyncUsersToAll）时重置为 false，确保在线节点必定补推成功。
	usersSynced atomic.Bool
	// lastReportAt 上次 node_reports 落库时刻（采样节流）。仅 readPump 所在 goroutine
	// 读写，无需加锁；重连新建 Conn 时为零值，首帧心跳立即落库。
	lastReportAt time.Time
	// lastDBUpdate 上次向 SQLite servers 表同步 last_seen_at 的时刻（节流保护，避免高频心跳压满单连接池）。
	lastDBUpdate time.Time
	// lastXrayState 上次上报的 xray 运行状态（内存缓存，用于避免高频心跳每帧查库判断跃迁）。
	lastXrayState string
	// lastDiskHash 上次上报的磁盘配置哈希（内存缓存，用于避免高频心跳每帧查库判断偏离）。
	lastDiskHash string

	// 1 分钟落库采样窗口内的聚合指标跟踪（避免单点采样漏采瞬时尖峰）：
	windowRxRateMax float64
	windowTxRateMax float64
	windowCPUSum    float64
	windowCPUCount  int
	windowMemLast   float64
	windowMemTotal  uint64
	windowDiskLast  float64
	windowDiskTotal uint64
	windowUsersMax  int
	windowRxBytes   uint64
	windowTxBytes   uint64

	mu     sync.Mutex
	closed bool
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
	conns   map[uint64]*Conn       // by server_id
	pending map[string]*pendingReq // 请求 id → 回执请求
	wg      sync.WaitGroup
	quit    chan struct{}

	metricsMu     sync.RWMutex
	latestMetrics map[uint64]*NodeMetricsSnapshot // server_id → 最新内存指标快照（0 磁盘 I/O）

	upgradeMu     sync.RWMutex
	upgradeStatus map[uint64]*protocol.UpgradeProgressPayload // server_id → 最新升级进度

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
	h.latestMetrics = make(map[uint64]*NodeMetricsSnapshot)
	h.enforcedAt = make(map[uint64]time.Time)
	h.upgradeStatus = make(map[uint64]*protocol.UpgradeProgressPayload)
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
		ServerID:      server.ID,
		NodeID:        server.NodeID,
		WS:            ws,
		Send:          make(chan []byte, 64),
		done:          make(chan struct{}),
		lastXrayState: server.XrayState,
		lastDiskHash:  server.XrayDiskHash,
	}
	conn.LastSeen.Store(time.Now().Unix())
	h.register(conn)
	// 认证回执携带能力声明：节点据此决定流量批次是「等落库回执才删」还是「发完即删」。
	// 旧节点忽略 caps；本字段是新节点防重复计量的开关，不能省。
	_ = h.writeRaw(ws, protocol.MsgAuthOK, msg.ID, protocol.AuthOKPayload{
		OK:   true,
		Caps: []string{protocol.CapTrafficAck},
	})
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
			return nil, errors.New("服务器不存在")
		}
		return nil, errors.New("数据库错误")
	}
	// 无认证端点上的密钥比较必须恒定时间，防按字节猜解的时序侧信道。
	if subtle.ConstantTimeCompare([]byte(util.HashSecret(a.Secret)), []byte(server.Secret)) != 1 {
		return nil, errors.New("服务器密钥错误")
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
				case req.ch <- &protocol.ResultPayload{OK: false, Error: "服务器连接已断开"}:
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
		if h.DB != nil {
			h.DB.Model(&models.Server{}).Where("id = ?", conn.ServerID).
				Update("status", 0)
		}
		// 节点彻底离线时，内存快照中的瞬时速率与在线人数归零
		h.metricsMu.Lock()
		if h.latestMetrics != nil {
			if m, ok := h.latestMetrics[conn.ServerID]; ok && m != nil {
				m.RxRate = 0
				m.TxRate = 0
				m.OnlineUsers = 0
				m.XrayRunning = false
			}
		}
		h.metricsMu.Unlock()
	}
}

// GetLatestMetrics 获取单个节点的最新纯内存监控快照。
func (h *Hub) GetLatestMetrics(serverID uint64) (*NodeMetricsSnapshot, bool) {
	h.metricsMu.RLock()
	defer h.metricsMu.RUnlock()
	if h.latestMetrics == nil {
		return nil, false
	}
	m, ok := h.latestMetrics[serverID]
	if !ok || m == nil {
		return nil, false
	}
	cp := *m
	if !h.IsOnline(serverID) {
		cp.RxRate = 0
		cp.TxRate = 0
		cp.OnlineUsers = 0
		cp.XrayRunning = false
	}
	return &cp, true
}

// GetAllLatestMetrics 获取所有节点的最新纯内存监控快照映射（副本）。
func (h *Hub) GetAllLatestMetrics() map[uint64]*NodeMetricsSnapshot {
	h.metricsMu.RLock()
	defer h.metricsMu.RUnlock()
	res := make(map[uint64]*NodeMetricsSnapshot, len(h.latestMetrics))
	for id, m := range h.latestMetrics {
		if m != nil {
			cp := *m
			if !h.IsOnline(id) {
				cp.RxRate = 0
				cp.TxRate = 0
				cp.OnlineUsers = 0
				cp.XrayRunning = false
			}
			res[id] = &cp
		}
	}
	return res
}

// SetMetricsForTest 供测试注入模拟指标快照。
func (h *Hub) SetMetricsForTest(serverID uint64, m *NodeMetricsSnapshot) {
	h.metricsMu.Lock()
	if h.latestMetrics == nil {
		h.latestMetrics = make(map[uint64]*NodeMetricsSnapshot)
	}
	h.latestMetrics[serverID] = m
	h.metricsMu.Unlock()
}

// SetOnlineForTest 供测试注册模拟在线连接。
func (h *Hub) SetOnlineForTest(serverID uint64, lastSeen time.Time) {
	h.mu.Lock()
	conn := &Conn{ServerID: serverID}
	conn.LastSeen.Store(lastSeen.Unix())
	h.conns[serverID] = conn
	h.mu.Unlock()
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
		case protocol.MsgUpgradeProgress:
			h.handleUpgradeProgress(conn, msg)
		}
	}
}

// SetUpgradeStatus 记录或更新指定节点的升级进度。
func (h *Hub) SetUpgradeStatus(serverID uint64, p *protocol.UpgradeProgressPayload) {
	h.upgradeMu.Lock()
	defer h.upgradeMu.Unlock()
	h.upgradeStatus[serverID] = p
}

// GetUpgradeStatus 获取指定节点的最新升级进度（空表示未在升级）。
func (h *Hub) GetUpgradeStatus(serverID uint64) *protocol.UpgradeProgressPayload {
	h.upgradeMu.RLock()
	defer h.upgradeMu.RUnlock()
	if st, ok := h.upgradeStatus[serverID]; ok {
		cp := *st
		return &cp
	}
	return nil
}

// GetUpgradeStatuses 批量获取升级进度快照（ids 为空 = 全部有记录的节点），供批量升级总览轮询。
func (h *Hub) GetUpgradeStatuses(ids []uint64) map[uint64]*protocol.UpgradeProgressPayload {
	h.upgradeMu.RLock()
	defer h.upgradeMu.RUnlock()
	want := make(map[uint64]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := make(map[uint64]*protocol.UpgradeProgressPayload, len(h.upgradeStatus))
	for id, st := range h.upgradeStatus {
		if len(want) > 0 && !want[id] {
			continue
		}
		cp := *st
		out[id] = &cp
	}
	return out
}

// handleUpgradeProgress 接收节点升级进度上报并缓存。
func (h *Hub) handleUpgradeProgress(conn *Conn, msg *protocol.Message) {
	var p protocol.UpgradeProgressPayload
	if err := msg.PayloadTo(&p); err != nil {
		return
	}
	h.SetUpgradeStatus(conn.ServerID, &p)
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
		// UUID 变更 → 重新生成并**落待推送**再推（旧实现只调 Generate 丢弃返回值就直接
		// PushPending：有 pending 行时推的是旧内容、没有 pending 行时 PushPending 直接返回，
		// 整个推送是空操作，要等下一次每小时校准才生效）。
		h.enqueueConfig(conn.ServerID)
		// 扇出：引用该落地入站的出站 client id 也随之变了（xray buildRefOutbound 按入站现状
		// 现场派生 internal_uuid），这些服务器不在上面的重推范围内，必须按 InboundRef 补齐
		// （与换证联动 reenqueueRelayConfigsForCert 同构）。
		h.enqueueRefOutboundServers(inb.ID)
	}
}

// enqueueConfig 生成 → 落待推送 → 非阻塞推送（与 api.enqueueConfig 同构的网关侧版本）。
func (h *Hub) enqueueConfig(serverID uint64) {
	if h.Config == nil || serverID == 0 {
		return
	}
	cfg, err := h.Config.Generate(serverID)
	if err != nil {
		log.Printf("nodegate: 生成服务器 %d 配置失败: %v", serverID, err)
		return
	}
	if err := h.Config.SavePending(serverID, cfg); err != nil {
		log.Printf("nodegate: 保存服务器 %d 待推送配置失败: %v", serverID, err)
		return
	}
	go h.PushPending(serverID)
}

// enqueueRefOutboundServers 重推所有「出站 InboundRef 指向该入站」的服务器。
func (h *Hub) enqueueRefOutboundServers(inboundID uint64) {
	var obs []models.ServerOutbound
	if err := h.DB.Select("server_id").Distinct("server_id").
		Where("inbound_ref = ?", inboundID).Find(&obs).Error; err != nil {
		log.Printf("nodegate: 查询引用入站 %d 的出站失败: %v", inboundID, err)
		return
	}
	for _, ob := range obs {
		h.enqueueConfig(ob.ServerID)
	}
}

// handleTrafficReport 处理节点流量上报（批次去重 + 落库确认）+ 事件驱动超额/到期处置：
// 落库后对本帧涉及用户做增量限额判定，命中即热更全节点用户列表（gRPC 秒级移除，
// 不重启 xray）——「超额后还能跑流量 ~1h」问题的根治路径（2026-09-01），
// 最坏端到端时延从 1h 校准兜底降为 ≈1 个上报周期。
//
// 投递确认（审计 F1）：Save 把「批次去重记录 + 流量写入」放在同一事务，提交成功才回
// traffic_ack(ok)。节点只有收到 ok 才删本地 outbox 批次——主控写库失败/崩溃/回执丢失时
// 节点保留并重发，重复投递由 BatchID 去重兜住。失败回 ok=false 仅供节点日志定位原因
// （节点按自己的节奏重试，不依赖本回执调度）。旧 agent 不带 BatchID：不回执，维持旧行为。
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
		h.ackTraffic(conn, tr.BatchID, false, "流量落库失败")
		return
	}
	// 落库成功（含「批次重复投递、本次无新增」）→ 确认，节点据此删批
	h.ackTraffic(conn, tr.BatchID, true, "")
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

// ackTraffic 回流量批次落库回执（审计 F1）。batchID 为空（旧 agent 不带批次号）时静默跳过。
// 走 conn.Send 队列而非直写 WS：本函数在 readPump goroutine 上执行，直写会与 writePump 争用
// 连接并可能阻塞读循环。带超时与 done 兜底，连接关闭后不空等。
func (h *Hub) ackTraffic(conn *Conn, batchID string, ok bool, errMsg string) {
	if batchID == "" {
		return
	}
	data, err := protocol.Encode(protocol.MsgTrafficAck, "",
		protocol.TrafficAckPayload{BatchID: batchID, OK: ok, Error: errMsg})
	if err != nil {
		return
	}
	select {
	case conn.Send <- data:
	case <-conn.done:
	case <-time.After(time.Second):
		log.Printf("nodegate: 流量回执入队超时 (server=%d batch=%s ok=%v)", conn.ServerID, batchID, ok)
	}
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
	now := time.Now()
	conn.touch()

	// 在线数按去重用户口径重算：统计键按入站区分后同一用户每入站一个 email 条目，
	// agent 直接计数会重复计人；快照为空（旧 agent）沿用其计数。
	onlineUsers := hb.OnlineUsers
	if len(hb.OnlineIPs) > 0 {
		onlineUsers = xray.CountDistinctOnlineUsers(hb.OnlineIPs)
	}

	// 1. 无论是否到达 DB 抽稀落库间隔，每一帧心跳都无条件立即更新纯内存快照（0 磁盘 I/O）
	snapshot := &NodeMetricsSnapshot{
		ServerID:    conn.ServerID,
		CPU:         hb.CPU,
		Mem:         hb.Mem,
		MemTotal:    uint64(hb.MemTotal),
		Disk:        hb.Disk,
		DiskTotal:   uint64(hb.DiskTotal),
		OnlineUsers: onlineUsers,
		RxRate:      hb.RxRate,
		TxRate:      hb.TxRate,
		RxBytes:     hb.RxBytes,
		TxBytes:     hb.TxBytes,
		XrayRunning: hb.XrayRunning,
		ReportedAt:  now,
	}
	h.metricsMu.Lock()
	if h.latestMetrics == nil {
		h.latestMetrics = make(map[uint64]*NodeMetricsSnapshot)
	}
	h.latestMetrics[conn.ServerID] = snapshot
	h.metricsMu.Unlock()

	// 2. 窗口聚合：累积 1 分钟内的峰值与总量（避免点采样漏采瞬时尖峰）
	if hb.RxRate > conn.windowRxRateMax {
		conn.windowRxRateMax = hb.RxRate
	}
	if hb.TxRate > conn.windowTxRateMax {
		conn.windowTxRateMax = hb.TxRate
	}
	if onlineUsers > conn.windowUsersMax {
		conn.windowUsersMax = onlineUsers
	}
	conn.windowCPUSum += hb.CPU
	conn.windowCPUCount++
	conn.windowMemLast = hb.Mem
	conn.windowMemTotal = uint64(hb.MemTotal)
	conn.windowDiskLast = hb.Disk
	conn.windowDiskTotal = uint64(hb.DiskTotal)
	conn.windowRxBytes = hb.RxBytes
	conn.windowTxBytes = hb.TxBytes

	// 3. 构建 servers 表更新字段
	updates := map[string]any{
		"status":       1,
		"last_seen_at": now,
		"xray_running": hb.XrayRunning,
	}
	if hb.Version != "" { // 旧 agent 不上报版本，不覆盖已有值
		updates["agent_version"] = hb.Version
	}
	prevXrayState := conn.lastXrayState
	stateChanged := false
	if hb.XrayState != "" {
		if hb.XrayState != conn.lastXrayState {
			stateChanged = true
			conn.lastXrayState = hb.XrayState
		}
		updates["xray_state"] = hb.XrayState
		updates["xray_failures"] = hb.XrayFailures
		if hb.XrayLastError != "" {
			updates["xray_last_error"] = truncateRunes(hb.XrayLastError, 500)
			if hb.XrayErrorAt > 0 {
				updates["xray_error_at"] = time.Unix(hb.XrayErrorAt, 0)
			}
		}
	}
	if hb.DiskHash != "" {
		updates["xray_disk_hash"] = hb.DiskHash
		updates["xray_running_hash"] = hb.RunningHash
		// 仅当节点上报的磁盘哈希与上次内存记录不一致时才查库判定偏离，避免高频心跳每帧打库
		if hb.DiskHash != conn.lastDiskHash {
			conn.lastDiskHash = hb.DiskHash
			if h.Config != nil {
				if pend, perr := h.Config.GetPending(conn.ServerID); perr == nil && pend != nil &&
					pend.Status == "pushed" && pend.AppliedHash != "" && hb.DiskHash != pend.AppliedHash {
					if conn.usersSynced.Load() {
						conn.usersSynced.Store(false)
						go func(sid uint64) {
							if err := h.SyncUsers(sid); err != nil {
								log.Printf("nodegate: 节点 %d 磁盘偏离自动修复失败: %v（将由看门狗重试）", sid, err)
							}
						}(conn.ServerID)
					}
				}
			}
		}
	}
	if len(hb.OnlineIPs) == 0 {
		updates["online_ips"] = "[]"
	} else if b, err := json.Marshal(hb.OnlineIPs); err == nil {
		updates["online_ips"] = string(b)
	}

	// 节流写入 SQLite servers 表：首次心跳、状态跃迁立即写库；常规心跳节流至 30s 一次，
	// 避免高频心跳（3-5s）打爆 SQLite 单连接池。
	needDBUpdate := conn.lastDBUpdate.IsZero() ||
		now.Sub(conn.lastDBUpdate) >= 30*time.Second ||
		stateChanged
	if needDBUpdate && h.DB != nil {
		conn.lastDBUpdate = now
		h.DB.Model(&models.Server{}).Where("id = ?", conn.ServerID).Updates(updates)
	}

	// 报警：状态跃迁才记一条系统审计
	if stateChanged {
		h.raiseXrayAlarm(conn.ServerID, prevXrayState, hb)
	}

	// 4. node_reports 采样落库（见 nodeReportSampleInterval）：
	// 监控指标行按固定间隔（默认 1 分钟）抽稀，写入窗口内的真实峰值与均值，零额外存储增长。
	if now.Sub(conn.lastReportAt) < nodeReportSampleInterval {
		return
	}
	conn.lastReportAt = now
	rxRate := hb.RxRate
	if conn.windowRxRateMax > rxRate {
		rxRate = conn.windowRxRateMax
	}
	txRate := hb.TxRate
	if conn.windowTxRateMax > txRate {
		txRate = conn.windowTxRateMax
	}
	maxUsers := onlineUsers
	if conn.windowUsersMax > maxUsers {
		maxUsers = conn.windowUsersMax
	}
	avgCPU := hb.CPU
	if conn.windowCPUCount > 0 {
		avgCPU = conn.windowCPUSum / float64(conn.windowCPUCount)
	}
	if h.DB != nil {
		_ = h.DB.Create(&models.NodeReport{
			ServerID:    conn.ServerID,
			CPU:         avgCPU,
			Mem:         hb.Mem,
			MemTotal:    uint64(hb.MemTotal),
			Disk:        hb.Disk,
			DiskTotal:   uint64(hb.DiskTotal),
			OnlineUsers: maxUsers,
			RxRate:      rxRate,
			TxRate:      txRate,
			RxBytes:     hb.RxBytes,
			TxBytes:     hb.TxBytes,
			ReportedAt:  now,
		}).Error
	}

	// 重置窗口统计
	conn.windowRxRateMax = 0
	conn.windowTxRateMax = 0
	conn.windowCPUSum = 0
	conn.windowCPUCount = 0
	conn.windowUsersMax = 0
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
		return errors.New("服务器离线")
	}
	select {
	case conn.Send <- msg:
		return nil
	case <-time.After(WriteTimeout):
		return errors.New("服务器发送超时")
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
		return nil, errors.New("等待服务器回执超时")
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
		h.recordPushFailure(p.ID, "服务器离线，等待上线自动补推")
		return
	}
	// P1b：推送前现场重算待推内容（见 refreshPending）
	p = h.refreshPending(serverID, p)
	if p == nil || p.Status == "pushed" {
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
		// 同一原因会按补推周期反复出现（如节点对同一份配置的冷却拒绝），原因未变化时不再刷日志；
		// 面板仍展示原因与累计失败次数。比较前先按列宽截断，否则超长原因会每轮都判为"变了"。
		failure := truncateRunes("服务器拒绝: "+msg, 500)
		if failure != p.LastError {
			log.Printf("nodegate: 节点 %d 拒绝推送的配置: %s（保留待推送）", serverID, msg)
		}
		h.recordPushFailure(p.ID, failure)
		return
	}
	marked, merr := h.Config.MarkPushedIfSame(p.ID, p.ConfigJSON)
	if merr != nil {
		log.Printf("nodegate: 标记已推送失败 (server=%d): %v", serverID, merr)
		return
	}
	if marked {
		log.Printf("nodegate: 已自动推送配置到节点 %d", serverID)
		// 记账：主控记录的「节点磁盘内容」= 刚推送的这份（面板据此对账 disk_hash）
		if aerr := h.Config.MarkApplied(serverID, p.ConfigJSON); aerr != nil {
			log.Printf("nodegate: 记录已应用配置失败 (server=%d): %v", serverID, aerr)
		}
		// 冷推把「生成那一刻」的用户集写进了 xray，而推送前后可能已有新的用户变更走热更
		// 下发过（热更先到、冷推后到 → 旧用户集覆盖了 xray）。补一次热更把最新用户集打回去，
		// 消灭这个竞态残留窗口。异步执行：它要再走一次 Ask（最长 30s），不能拖住调用方。
		go func(id uint64) {
			if err := h.SyncUsers(id); err != nil {
				log.Printf("nodegate: 冷推成功后补热更失败 (server=%d): %v", id, err)
			}
		}(serverID)
	} else {
		// 推送期间 pending 已被更新（如用户编辑/每小时校准），保持 pending 待下一轮推送
		log.Printf("nodegate: 节点 %d 推送成功但 pending 内容已被更新，保留待推送", serverID)
	}
}

// refreshPending 推送前现场重算待推内容并返回「本次要推的那一份」。
//
// 为什么必须重算（P1b）：待推内容冻结于 SavePending 那一刻，而用户变更只走热更、从不更新它
// —— 推送被延迟（节点离线 / 冷却拒绝 / 环境故障）期间被删 / 被封 / 超期的用户仍留在里面，
// 冷推成功就把他们"复活"（能连、照常计费）。Generate 是确定的纯函数，重算即得当前用户集。
//
// 写回用内容 CAS（SavePendingIfSame）：重算期间若来了新编辑，替换会失败，此时改用最新内容
// 推送，绝不覆盖它。任何一步失败都退回已保存的内容——重算失败不该让推送停摆。
func (h *Hub) refreshPending(serverID uint64, p *models.PendingConfig) *models.PendingConfig {
	fresh, err := h.Config.Generate(serverID)
	if err != nil {
		log.Printf("nodegate: 推送前重算配置失败 (server=%d)，改用已保存内容: %v", serverID, err)
		return p
	}
	if fresh == p.ConfigJSON {
		return p
	}
	ok, serr := h.Config.SavePendingIfSame(serverID, p.ConfigJSON, fresh)
	switch {
	case serr != nil:
		log.Printf("nodegate: 保存重算后的配置失败 (server=%d): %v", serverID, serr)
	case ok:
		log.Printf("nodegate: 推送前重算发现待推内容已陈旧（用户集有变更），改用新内容 (server=%d)", serverID)
		p.ConfigJSON = fresh
	default:
		if np, nerr := h.Config.GetPending(serverID); nerr == nil && np != nil {
			log.Printf("nodegate: 重算期间待推内容被新编辑覆盖，改用最新内容 (server=%d)", serverID)
			return np
		}
	}
	return p
}

// buildSyncPayload 组装热更负载：用户集（按已生效结构过滤）+ 可选的整份配置（顺带落盘）。
// 拆出来是为了让「按 tags(S_a) 过滤」与「何时附带配置」这两条不变量有独立的回归用例
// （它们一旦退化，表现是整批同步中断或磁盘悄悄滞后，都不是一眼能看出来的）。
func (h *Hub) buildSyncPayload(serverID uint64) (protocol.SyncUsersPayload, error) {
	usersMap, err := h.Config.GetValidUsers(serverID)
	if err != nil {
		return protocol.SyncUsersPayload{}, err
	}
	// 不变量 I3：热更只使用「已生效结构 S_a」里的 tag。SYNCED 期由 GetValidUsers 的入站
	// 可用性过滤保证与运行中结构同源；PENDING 期新结构里的新入站还不存在于运行中的 xray，
	// 带着它下发会 handler not found 并整批中断（含末尾的 tag 清理）——恰恰砸在
	// 「结构变更卡住、用户变更更要紧」的场景上。取不到 S_a（无待推送行）时不过滤。
	if tags, ok := h.Config.AppliedTags(serverID); ok {
		for tag := range usersMap {
			if !tags[tag] {
				delete(usersMap, tag)
			}
		}
	}
	payload := protocol.SyncUsersPayload{Users: usersMap}
	// 不变量 I2：节点处于 SYNCED（无待生效结构）时附带现场生成的整份配置（= materialize(S_a,U)），
	// 节点在热更成功后把它落盘 —— 磁盘于是恒等于运行中配置的快照，节点重启（自愈/开机/手动）
	// 不会把用户集回退到上一次冷更。PENDING 期间不附带：磁盘不能留下未经验证的结构。
	// 内容与「主控记录的磁盘内容」一致且节点磁盘未偏离时不附带，避免让节点反复写盘 + 跑 -test。
	if synced, ok := h.Config.SyncedConfig(serverID); ok {
		applied := h.Config.AppliedConfig(serverID)
		diskHash := ""
		if h.DB != nil {
			var srv models.Server
			if err := h.DB.Select("xray_disk_hash").First(&srv, serverID).Error; err == nil {
				diskHash = srv.XrayDiskHash
			}
		}
		// 满足任一条件时附带整份配置修复磁盘：
		// 1. 主控记录的磁盘内容与现场重算不同（用户集有变更）
		// 2. 节点实际磁盘哈希与期望哈希不符（第三方改盘 / 落盘静默失败 / .good 回退触发磁盘偏离）
		if synced != applied || (diskHash != "" && diskHash != services.ContentHash(synced)) {
			payload.ConfigJSON = synced
		}
	}
	return payload, nil
}

// truncateRunes 按 rune 截断，避免切断多字节字符（模型列宽以字符计）。
func truncateRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}

// raiseXrayAlarm xray 状态跃迁报警（2026-09-21）：只在"进入 failed"（连续失败达上限、
// 节点已停止自动拉起）与"failed → running"（已恢复）两个跃迁上记一条 system 审计 +
// 主控日志，不按心跳刷屏。面板审计页「服务器」分类可见。
func (h *Hub) raiseXrayAlarm(serverID uint64, prevState string, hb protocol.HeartbeatPayload) {
	if prevState == hb.XrayState {
		return
	}
	var srv models.Server
	if err := h.DB.Select("id", "name").First(&srv, serverID).Error; err != nil {
		return
	}
	var action, detail string
	switch {
	case hb.XrayState == "failed":
		action = "servers.xray_start_failed"
		detail = fmt.Sprintf("服务器「%s」(#%d) xray 连续 %d 次启动失败，已停止自动拉起：%s",
			srv.Name, serverID, hb.XrayFailures, hb.XrayLastError)
	case prevState == "failed" && hb.XrayState == "running":
		action = "servers.xray_recovered"
		detail = fmt.Sprintf("服务器「%s」(#%d) xray 已恢复运行", srv.Name, serverID)
	default:
		return
	}
	log.Printf("nodegate: %s", detail)
	_ = h.DB.Create(&models.AuditLog{
		OperatorType: "system",
		Action:       action,
		Detail:       detail,
		CreatedAt:    time.Now(),
	}).Error
}

// recordPushFailure 记录一次配置推送失败：last_error/attempts/last_attempt_at 落到
// PendingConfig 行上，面板"待推送"旁直接展示原因，不必翻主控日志。失败不改变 pending 状态。
func (h *Hub) recordPushFailure(pendingID uint64, reason string) {
	reason = truncateRunes(reason, 500) // 与模型列宽一致，防 agent 回执里的配置报错细节超长
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
		return errors.New("配置服务未初始化")
	}
	payload, err := h.buildSyncPayload(serverID)
	if err != nil {
		return err
	}
	res, err := h.Ask(serverID, protocol.MsgSyncUsers, payload, AskTimeout)
	if err != nil {
		return err
	}
	if res != nil && !res.OK {
		return errors.New(res.Error)
	}
	// 落盘成功 → 记账：主控记录的「节点磁盘内容」跟上，面板据此发现磁盘偏离
	// （第三方改过 / 落盘静默失败）。失败只记日志，不影响本次热更的结论。
	if payload.ConfigJSON != "" {
		prevApplied := h.Config.AppliedConfig(serverID)
		if ok, err := h.Config.MarkAppliedIfSame(serverID, prevApplied, payload.ConfigJSON); err != nil {
			log.Printf("nodegate: 记录热更落盘内容失败 (server=%d): %v", serverID, err)
		} else if !ok {
			log.Printf("nodegate: 热更落盘内容已被较新的变更更新，跳过旧版本记账 (server=%d)", serverID)
		}
	}
	h.mu.RLock()
	if c, ok := h.conns[serverID]; ok {
		c.usersSynced.Store(true)
	}
	h.mu.RUnlock()
	return nil
}

// SyncUsersToAll 广播给所有在线节点增量/全量同步最新用户列表（非阻塞）。
// 广播前将所有在线连接的 usersSynced 标志置为 false，若异步下发失败将由看门狗 2 分钟重试兜底。
func (h *Hub) SyncUsersToAll() {
	h.mu.RLock()
	var conns []*Conn
	for _, c := range h.conns {
		c.usersSynced.Store(false)
		conns = append(conns, c)
	}
	h.mu.RUnlock()

	for _, c := range conns {
		go func(conn *Conn) {
			if err := h.SyncUsers(conn.ServerID); err != nil {
				log.Printf("nodegate: 向节点 %d 动态同步用户失败: %v（将由看门狗自动重试）", conn.ServerID, err)
			}
		}(c)
	}
}

// PushAgentSettings 向单个节点下发运行时设置（上报/心跳周期，设置页「节点上报」可调）。
// 节点连接建立与设置保存时调用；失败仅记日志，由 watchdog 2 分钟补推循环重试
// （settingsSynced 未置位即补推，成功或重连重发后停止）。
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
		return
	}
	// 成功下发：标记当前连接已同步（连接重建时新 Conn 零值自动回到未同步）
	h.mu.RLock()
	if c, ok := h.conns[serverID]; ok {
		c.settingsSynced.Store(true)
	}
	h.mu.RUnlock()
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
			online := make([]*Conn, 0, len(h.conns))
			for _, c := range h.conns {
				online = append(online, c)
			}
			h.mu.RUnlock()
			for _, c := range online {
				go h.PushPending(c.ServerID)
				// 运行时设置建连下发失败的节点同样补推，直至节点确认（settingsSynced 置位）
				if !c.settingsSynced.Load() {
					go h.PushAgentSettings(c.ServerID)
				}
				// 用户名单与账期映射下发失败的节点每 2 分钟自动重试，直至节点确认（usersSynced 置位）
				if !c.usersSynced.Load() {
					go func(conn *Conn) {
						if err := h.SyncUsers(conn.ServerID); err != nil {
							log.Printf("nodegate: 补推用户名单与账期失败 (server=%d): %v", conn.ServerID, err)
						}
					}(c)
				}
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
			// 定期 1 小时状态校准：按内容对账，只推真正有变化的节点。
			// 旧实现无条件 SavePending + PushPending，等于每个在线节点每小时无条件断一次
			// （节点侧 RestartWithConfig 没有内容短路，实测同一份内容连推两次会完整走一遍
			// Stop+Start）。Generate 是确定的（无时间/随机源，json.MarshalIndent 对 map 键
			// 排序），所以"内容一字未变"可以直接用逐字节比较判定。
			h.mu.RLock()
			var serverIDs []uint64
			for id := range h.conns {
				serverIDs = append(serverIDs, id)
			}
			h.mu.RUnlock()
			for _, id := range serverIDs {
				go func(sid uint64) {
					if h.Config == nil {
						return
					}
					cfgStr, err := h.Config.Generate(sid)
					if err != nil {
						log.Printf("nodegate: 定期校准生成配置失败 (server=%d): %v", sid, err)
						return
					}
					applied := h.Config.AppliedConfig(sid)
					if p, perr := h.Config.GetPending(sid); perr == nil && p != nil &&
						p.Status == "pushed" && applied == cfgStr {
						// 检查节点磁盘是否偏离：若节点上报了磁盘哈希且与期望哈希不符，触发热更落盘对齐
						var srv models.Server
						if herr := h.DB.Select("xray_disk_hash").First(&srv, sid).Error; herr == nil &&
							srv.XrayDiskHash != "" && p.AppliedHash != "" && srv.XrayDiskHash != p.AppliedHash {
							log.Printf("nodegate: 定期校准发现节点 %d 磁盘配置偏离，触发热更落盘对齐", sid)
							_ = h.SyncUsers(sid)
						}
						return // 已生效内容与现场重算相同且磁盘未偏离：无事可做，不打扰节点
					}
					log.Printf("nodegate: 执行定期全量状态校准 (server=%d)", sid)
					if serr := h.Config.SavePending(sid, cfgStr); serr != nil {
						log.Printf("nodegate: 定期校准保存配置失败 (server=%d): %v", sid, serr)
						return
					}
					h.PushPending(sid)
				}(id)
			}
		}
	}
}
