// Package client 实现 Agent 到主控的 WebSocket 客户端：
// 认证、心跳、断线指数退避重连、指令处理与回执。
package client

import (
	"context"
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"

	"github.com/zhx/xray-panel/internal/agent/collector"
	"github.com/zhx/xray-panel/internal/agent/xrayproc"
	"github.com/zhx/xray-panel/internal/pkg/protocol"
)

// Client 节点端客户端。
type Client struct {
	BaseURL   string // ws://host/api/v1/node/ws
	NodeID    string
	Secret    string
	Heartbeat time.Duration
	ReconnectMax time.Duration
	Xray      *xrayproc.Proc
	Collector *collector.Collector

	ws *websocket.Conn
}

// Run 常驻运行：连接 → 服务 → 断线重连。
func (c *Client) Run(ctx context.Context) {
	backoff := time.Second
	for {
		err := c.connectAndServe(ctx)
		if err != nil {
			log.Printf("agent: 连接断开: %v（%s 后重连）", err, backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < c.ReconnectMax {
			backoff *= 2
		}
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
func (c *Client) connectAndServe(ctx context.Context) error {
	ws, _, err := websocket.DefaultDialer.Dial(c.wsURL(), nil)
	if err != nil {
		return err
	}
	c.ws = ws
	defer ws.Close()

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

func (c *Client) send(typ, id string, payload any) error {
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
			running, pid, _, uptime := c.Xray.Status()
			_ = running
			_ = pid
			_ = uptime
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