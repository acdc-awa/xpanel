// Package client 实现 Agent 到主控的 WebSocket 客户端：
// 认证、心跳、断线指数退避重连、指令处理与回执、流量采集上报。
package client

import (
	"context"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/zhx/xray-panel/internal/agent/collector"
	"github.com/zhx/xray-panel/internal/agent/stats"
	"github.com/zhx/xray-panel/internal/agent/xrayproc"
	"github.com/zhx/xray-panel/internal/pkg/protocol"
)

// pendingEntry 待上报的累积流量。
type pendingEntry struct {
	Up   int64
	Down int64
}

// Client 节点端客户端。
type Client struct {
	BaseURL        string // ws://host/api/v1/node/ws
	NodeID         string
	Secret         string
	Heartbeat      time.Duration
	ReconnectMax   time.Duration
	Xray           *xrayproc.Proc
	Collector      *collector.Collector
	Stats          *stats.Collector
	CollectInterval time.Duration
	ReportInterval  time.Duration

	ws      *websocket.Conn
	writeMu sync.Mutex // 保护 ws 写（心跳/上报/回执并发）
	pendingMu sync.Mutex
	pending   map[string]*pendingEntry // by email
}

// Run 常驻运行：流量采集上报 + 连接/服务/重连。
func (c *Client) Run(ctx context.Context) {
	c.pending = make(map[string]*pendingEntry)
	go c.collectLoop(ctx)
	go c.reportLoop(ctx)

	backoff := time.Second
	for {
		err := c.connectAndServe(ctx, &backoff)
		if err != nil {
			log.Printf("agent: 连接断开: %v（%s 后重连）", err, backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		// 指数退避 (带抖动)
		backoff = time.Duration(float64(backoff) * 1.5)
		jitter := time.Duration((float64(backoff) * 0.2) * (float64(time.Now().UnixNano()%100) / 100.0))
		backoff += jitter
		if backoff > c.ReconnectMax {
			backoff = c.ReconnectMax
		}
	}
}

// collectLoop 周期性采集 xray stats 并累积到 pending。
func (c *Client) collectLoop(ctx context.Context) {
	ticker := time.NewTicker(c.CollectInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			entries, err := c.Stats.Collect(ctx)
			if err != nil {
				log.Printf("agent: stats 采集失败: %v", err)
				c.Stats.Close()
				continue
			}
			if len(entries) == 0 {
				continue
			}
			total := int64(0)
			for _, e := range entries {
				total += e.Up + e.Down
			}
			if total > 0 {
				log.Printf("agent: 采集到流量 delta=%d 字节（%d 用户）", total, len(entries))
			}
			c.pendingMu.Lock()
			for _, e := range entries {
				p, ok := c.pending[e.Email]
				if !ok {
					p = &pendingEntry{}
					c.pending[e.Email] = p
				}
				p.Up += e.Up
				p.Down += e.Down
			}
			c.pendingMu.Unlock()
		}
	}
}

// reportLoop 周期上报 pending；失败保留（重连后补报，不丢数据）。
func (c *Client) reportLoop(ctx context.Context) {
	ticker := time.NewTicker(c.ReportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.reportPending()
		}
	}
}

func (c *Client) reportPending() {
	c.pendingMu.Lock()
	if len(c.pending) == 0 {
		c.pendingMu.Unlock()
		return
	}
	entries := make([]protocol.TrafficEntry, 0, len(c.pending))
	for email, p := range c.pending {
		entries = append(entries, protocol.TrafficEntry{
			UserID: 0, // 主控按 email 匹配 user_id
			Email:  email,
			UpBytes: p.Up,
			DownBytes: p.Down,
		})
	}
	period := time.Now().UTC().Format(time.RFC3339)
	// 清空 pending（发送失败再放回）
	c.pending = make(map[string]*pendingEntry)
	c.pendingMu.Unlock()

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.ws == nil {
		c.restorePending(entries)
		return
	}
	err := c.sendLocked(protocol.MsgTrafficReport, "", protocol.TrafficReportPayload{
		Entries: entries,
		Period:  period,
	})
	if err != nil {
		log.Printf("agent: traffic_report 发送失败，保留待补报: %v", err)
		c.restorePending(entries)
	} else {
		log.Printf("agent: 已上报流量 %d 条（period=%s）", len(entries), period)
	}
}

// restorePending 上报失败后把数据放回 pending。
func (c *Client) restorePending(entries []protocol.TrafficEntry) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for _, e := range entries {
		key := e.Email
		p, ok := c.pending[key]
		if !ok {
			p = &pendingEntry{}
			c.pending[key] = p
		}
		p.Up += e.UpBytes
		p.Down += e.DownBytes
	}
}

func (c *Client) wsURL() string {
	u, _ := url.Parse(c.BaseURL)
	q := u.Query()
	q.Set("node_id", c.NodeID)
	u.RawQuery = q.Encode()
	return u.String()
}

// connectAndServe 建立连接并进入消息循环（返回时连接已关闭）。
func (c *Client) connectAndServe(ctx context.Context, backoff *time.Duration) error {
	ws, _, err := websocket.DefaultDialer.Dial(c.wsURL(), nil)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	c.ws = ws
	c.writeMu.Unlock()
	defer func() {
		c.writeMu.Lock()
		c.ws = nil
		c.writeMu.Unlock()
		_ = ws.Close()
	}()
	ws.SetReadLimit(1024 * 1024)

	// 认证
	if err := c.send(protocol.MsgAuth, "", protocol.AuthPayload{NodeID: c.NodeID, Secret: c.Secret}); err != nil {
		return err
	}
	_ = ws.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, data, err := ws.ReadMessage()
	if err != nil {
		return err
	}
	msg, err := protocol.Decode(data)
	if err != nil || msg.Type != "auth_ok" {
		return errAuthRejected
	}
	_ = ws.SetReadDeadline(time.Time{})
	log.Printf("agent: 已连上主控（node=%s）", c.NodeID)
	
	// 连接成功，重置退避时间
	*backoff = time.Second

	// ctx 取消时关闭连接，中断阻塞中的 ReadMessage（否则 SIGTERM 无法退出）
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = ws.Close()
		case <-done:
		}
	}()

	// 心跳
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go c.heartbeatLoop(hbCtx)

	// 消息循环
	for {
		_, data, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		m, err := protocol.Decode(data)
		if err != nil {
			continue
		}
		c.handle(m)
	}
}

var errAuthRejected = &wsError{msg: "认证被拒绝"}

type wsError struct{ msg string }

func (e *wsError) Error() string { return e.msg }

// send 写消息（并发安全）。
func (c *Client) send(typ, id string, payload any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.ws == nil {
		return errNoConn
	}
	return c.sendLocked(typ, id, payload)
}

var errNoConn = &wsError{msg: "未连接"}

// sendLocked 写消息（调用方需持有 writeMu）。
func (c *Client) sendLocked(typ, id string, payload any) error {
	data, err := protocol.Encode(typ, id, payload)
	if err != nil {
		return err
	}
	return c.ws.WriteMessage(websocket.TextMessage, data)
}

func (c *Client) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(c.Heartbeat)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snap := c.Collector.Snapshot()
			hb := protocol.HeartbeatPayload{
				CPU:         snap.CPU,
				Mem:         snap.Mem,
				MemTotal:    snap.MemTotal,
				Disk:        snap.Disk,
				DiskTotal:   snap.DiskTotal,
				XrayRunning: c.Xray.IsRunning(),
				OnlineUsers: 0,
				TS:          time.Now().Unix(),
			}
			if err := c.send(protocol.MsgHeartbeat, "", hb); err != nil {
				return // 连接已断，主循环会重连
			}
		}
	}
}

// handle 处理主控指令并回执。
func (c *Client) handle(m *protocol.Message) {
	var res protocol.ResultPayload
	switch m.Type {
	case protocol.MsgPushConfig:
		var p protocol.PushConfigPayload
		if err := m.PayloadTo(&p); err != nil {
			res = protocol.ResultPayload{OK: false, Error: "解析 push_config 失败"}
		} else if err := c.Xray.RestartWithConfig(p.ConfigJSON); err != nil {
			res = protocol.ResultPayload{OK: false, Error: err.Error()}
		} else {
			res = protocol.ResultPayload{OK: true, Data: "xray 已按新配置重启"}
		}
	case protocol.MsgRestartXray:
		if err := c.Xray.Stop(); err != nil {
			res = protocol.ResultPayload{OK: false, Error: err.Error()}
		} else if err := c.Xray.Start(); err != nil {
			res = protocol.ResultPayload{OK: false, Error: err.Error()}
		} else {
			res = protocol.ResultPayload{OK: true, Data: "xray 已重启"}
		}
	case protocol.MsgGetStatus:
		running, pid, startedAt, uptime := c.Xray.Status()
		res = protocol.ResultPayload{OK: true, Data: protocol.StatusData{
			XrayRunning: running,
			Pid:         pid,
			UptimeSec:   uptime,
			ConfigPath:  c.Xray.ConfigPath,
			StartedAt:   startedAt,
		}}
	case protocol.MsgGetLogs:
		var p protocol.GetLogsPayload
		_ = m.PayloadTo(&p)
		logs, err := c.Xray.Logs(p.Lines)
		if err != nil {
			res = protocol.ResultPayload{OK: false, Error: err.Error()}
		} else {
			res = protocol.ResultPayload{OK: true, Data: logs}
		}
	default:
		return // 不认识的类型不回应
	}
	_ = c.send(protocol.MsgResult, m.ID, res)
}