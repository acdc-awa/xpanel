package protocol

import "time"

// AuthPayload 节点认证（node_secret 不放入 URL，避免日志泄露）。
type AuthPayload struct {
	NodeID string `json:"node_id"`
	Secret string `json:"secret"`
}

// HeartbeatPayload 心跳与节点状态。
type HeartbeatPayload struct {
	CPU         float64 `json:"cpu"`          // 百分比
	Mem         float64 `json:"mem"`          // 已用内存（字节）
	MemTotal    float64 `json:"mem_total"`    // 总内存（字节）
	Disk        float64 `json:"disk"`         // 已用磁盘（字节）
	DiskTotal   float64 `json:"disk_total"`   // 总磁盘（字节）
	XrayRunning bool    `json:"xray_running"` // xray 是否在运行
	OnlineUsers int     `json:"online_users"` // 在线用户数（P2 流量接入后填）
	RxRate      float64 `json:"rx_rate"`      // 实时速率（字节/秒）
	TxRate      float64 `json:"tx_rate"`
	TS          int64   `json:"ts"` // unix 秒
}

// TrafficEntry 单条流量记录。
// P2：UserID=0 时主控按 Email 匹配用户；P5 接入入站维度后可填 Inbound。
type TrafficEntry struct {
	UserID    uint64 `json:"user_id"`
	Email     string `json:"email,omitempty"`
	Inbound   string `json:"inbound,omitempty"`
	UpBytes   int64  `json:"up_bytes"`
	DownBytes int64  `json:"down_bytes"`
}

// TrafficReportPayload 流量批量上报（P2 使用）。
type TrafficReportPayload struct {
	Entries []TrafficEntry `json:"entries"`
	Period  string         `json:"period"` // 上报周期起始，RFC3339
}

// User 节点同步的用户信息。
type User struct {
	UUID  string `json:"uuid"`
	Email string `json:"email"`
	Flow  string `json:"flow,omitempty"`
	Level uint32 `json:"level,omitempty"`
}

// SyncUsersPayload 全量用户同步负载（InboundTag -> []User）。
type SyncUsersPayload struct {
	Users map[string][]User `json:"users"`
}

// PushConfigPayload 下发 Xray 配置（P1 为完整 config JSON 透传，
// 模板生成器放 P3/P5）。
type PushConfigPayload struct {
	ConfigJSON string `json:"config_json"`
}

// GetLogsPayload 请求最近日志。
type GetLogsPayload struct {
	Lines int `json:"lines"`
}

// ResultPayload 指令回执（id 回填请求 ID）。
type ResultPayload struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

// GetStatusPayload 查询完整状态。
type GetStatusPayload struct{}

// StatusData Agent 返回的完整状态。
type StatusData struct {
	XrayRunning bool      `json:"xray_running"`
	Pid         int       `json:"pid,omitempty"`
	UptimeSec   int64     `json:"uptime_sec,omitempty"`
	ConfigPath  string    `json:"config_path,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
}