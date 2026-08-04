// Package protocol 定义主控-节点通信协议（主控与 Agent 共用）。
//
// 传输：WebSocket（生产 wss 由 Caddy 终止 TLS 后以明文 ws 转发给主控，主控恒监听 ws）。
// 消息：JSON 文本帧，{type, id, payload}，请求-响应用 id 配对。
package protocol

import (
	"encoding/json"
	"errors"
)

// 消息类型常量（与《系统设计方案》§4.3 对齐）。
const (
	// 节点 → 主控
	MsgAuth          = "auth"
	MsgHeartbeat     = "heartbeat"
	MsgTrafficReport = "traffic_report"
	MsgResult        = "result"

	// 主控 → 节点
	MsgPushConfig  = "push_config"
	MsgSyncUsers   = "sync_users"
	MsgRestartXray = "restart_xray"
	MsgGetStatus   = "get_status"
	MsgGetLogs     = "get_logs"
)

// Message 为统一消息帧。
type Message struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"` // 请求 ID，result 回执时回填
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Encode 编码消息帧。
func Encode(typ, id string, payload any) ([]byte, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = b
	}
	return json.Marshal(Message{Type: typ, ID: id, Payload: raw})
}

// Decode 解码消息帧。
func Decode(data []byte) (*Message, error) {
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Type == "" {
		return nil, errors.New("消息缺少 type 字段")
	}
	return &m, nil
}

// Payload 解析消息负载到 v。
func (m *Message) PayloadTo(v any) error {
	if len(m.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(m.Payload, v)
}