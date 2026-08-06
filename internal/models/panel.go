package models

import "time"

// Server 节点服务器（对应 §5 servers）。
type Server struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	Name       string     `gorm:"size:64;not null" json:"name"`
	Host       string     `gorm:"size:255;not null" json:"host"`
	NodeID     string     `gorm:"size:32;uniqueIndex;not null" json:"node_id"`
	Secret     string     `gorm:"size:64;not null" json:"-"`
	Location   string     `gorm:"size:64" json:"location"`
	Remark     string     `gorm:"size:255" json:"remark"`
	Status     int        `gorm:"default:0;index" json:"status"` // 0 离线 1 在线
	LastSeenAt *time.Time `json:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Inbound 入站（接入点），每节点可配多个。
// settings_json 存协议层 JSON（clients 动态注入 / fallbacks / decryption）；
// stream_settings 存传输层 JSON（network / security / realitySettings / wsSettings / tlsSettings 等）；
// sniffing 存流量嗅探 JSON。
type Inbound struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	ServerID       uint64    `gorm:"index;not null" json:"server_id"`
	Tag            string    `gorm:"size:64;not null" json:"tag"`
	Protocol       string    `gorm:"size:16;not null" json:"protocol"` // vless / vmess / trojan / shadowsocks
	Port           int       `gorm:"not null" json:"port"`
	Listen         string    `gorm:"size:64" json:"listen"`            // 监听地址，空 = 0.0.0.0
	SettingsJSON   string    `gorm:"type:text" json:"settings_json"`   // 协议 settings（透传，clients 由后端注入）
	StreamSettings string    `gorm:"type:text" json:"stream_settings"` // 传输 streamSettings（透传）
	Sniffing       string    `gorm:"type:text" json:"sniffing"`        // 嗅探配置（透传）
	Ratio          float64   `gorm:"default:1" json:"ratio"`
	Enabled        bool      `gorm:"default:true" json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// UserInbound 用户-入站授权关系。
type UserInbound struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	UserID    uint64    `gorm:"index;not null" json:"user_id"`
	InboundID uint64    `gorm:"index;not null" json:"inbound_id"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	Remark    string    `gorm:"size:255" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
}

// Plan 套餐。
type Plan struct {
	ID             uint64    `gorm:"primaryKey" json:"id"`
	Name           string    `gorm:"size:64;not null" json:"name"`
	PriceCents     int64     `gorm:"not null" json:"price_cents"` // 价格（分）
	TrafficGB      int64     `gorm:"not null" json:"traffic_gb"`
	DurationDays   int       `gorm:"not null" json:"duration_days"`
	SpeedLimitKbps     int64  `json:"speed_limit_kbps"` // 0 = 不限速
	PermissionGroupID  uint64 `gorm:"index;default:0" json:"permission_group_id"` // 绑定权限组（0=不绑定）
	Enabled            bool   `gorm:"default:true" json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Order 订单（人工确认制：pending → paid / cancelled）。
type Order struct {
	ID             uint64     `gorm:"primaryKey" json:"id"`
	OrderNo        string     `gorm:"size:32;uniqueIndex;not null" json:"order_no"`
	UserID         uint64     `gorm:"index;not null" json:"user_id"`
	PlanID         uint64     `gorm:"index;not null" json:"plan_id"`
	AmountCents    int64      `gorm:"not null" json:"amount_cents"`
	Status         string     `gorm:"size:16;default:pending;index" json:"status"`
	ConfirmAdminID uint64     `gorm:"index" json:"confirm_admin_id"`
	CreatedAt      time.Time  `json:"created_at"`
	PaidAt         *time.Time `json:"paid_at"`
}

// TrafficLog 节点上报的流量明细（按 用户×入站×周期）。
// (user_id, inbound_id, period_start) 唯一：同一上报周期重复投递时覆盖合并（补报幂等）。
type TrafficLog struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	UserID      uint64    `gorm:"index;not null" json:"user_id"`
	InboundID   uint64    `gorm:"index" json:"inbound_id"`
	UpBytes     int64     `gorm:"not null" json:"up_bytes"`
	DownBytes   int64     `gorm:"not null" json:"down_bytes"`
	PeriodStart time.Time `gorm:"uniqueIndex:idx_traffic_period" json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	CreatedAt   time.Time `json:"created_at"`
}

// TrafficDaily 每日汇总（仪表盘用）。
type TrafficDaily struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	UserID    uint64    `gorm:"index;not null" json:"user_id"`
	Date      string    `gorm:"size:10;index" json:"date"` // YYYY-MM-DD
	UpBytes   int64     `gorm:"not null" json:"up_bytes"`
	DownBytes int64     `gorm:"not null" json:"down_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

// NodeReport 节点心跳/状态上报。
type NodeReport struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	ServerID    uint64    `gorm:"index;not null" json:"server_id"`
	CPU         float64   `json:"cpu"`
	Mem         float64   `json:"mem"`
	OnlineUsers int       `json:"online_users"`
	RxRate      float64   `json:"rx_rate"`
	TxRate      float64   `json:"tx_rate"`
	ReportedAt  time.Time `gorm:"index" json:"reported_at"`
}

// AuditLog 审计日志（管理员操作、登录、订单确认等）。
type AuditLog struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	OperatorType string    `gorm:"size:16;index" json:"operator_type"` // admin / user / system
	OperatorID   uint64    `gorm:"index" json:"operator_id"`
	Action       string    `gorm:"size:64;index" json:"action"`
	Detail       string    `gorm:"type:text" json:"detail"`
	IP           string    `gorm:"size:64" json:"ip"`
	CreatedAt    time.Time `json:"created_at"`
}

// PendingConfig 服务器待推送的 Xray 配置（每服务器一条最新；节点离线时保留，上线后自动补推）。
type PendingConfig struct {
	ID         uint64     `gorm:"primaryKey" json:"id"`
	ServerID   uint64     `gorm:"uniqueIndex;not null" json:"server_id"`
	ConfigJSON string     `gorm:"type:text" json:"-"`
	Status     string     `gorm:"size:16;default:pending" json:"status"` // pending / pushed
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	PushedAt   *time.Time `json:"pushed_at"`
}

// Setting 站点配置（公告等键值对）。
type Setting struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"size:64;uniqueIndex;not null" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ServerOutbound 服务器独立出站规则（§多节点 3x-ui 架构）。
type ServerOutbound struct {
	ID                 uint64    `gorm:"primaryKey" json:"id"`
	ServerID           uint64    `gorm:"index;not null" json:"server_id"`
	Tag                string    `gorm:"size:64;not null" json:"tag"`
	Protocol           string    `gorm:"size:32;not null" json:"protocol"` // freedom / blackhole / socks / vmess / etc.
	SettingsJSON       string    `gorm:"type:text" json:"settings_json"`
	StreamSettingsJSON string    `gorm:"type:text" json:"stream_settings_json,omitempty"`
	SendThrough        string    `gorm:"size:64" json:"send_through,omitempty"`
	Enabled            bool      `gorm:"default:true" json:"enabled"`
	Priority           int       `gorm:"default:0" json:"priority"`
	Remark             string    `gorm:"size:255" json:"remark"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// ServerRoutingRule 服务器独立路由规则（§多节点 3x-ui 架构）。
type ServerRoutingRule struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	ServerID    uint64    `gorm:"index;not null" json:"server_id"`
	OutboundTag string    `gorm:"size:64;not null" json:"outbound_tag"`
	RuleJSON    string    `gorm:"type:text" json:"rule_json,omitempty"` // 自定义完整 Rule JSON
	Domain      string    `gorm:"type:text" json:"domain,omitempty"`    // 逗号/换行分隔或 JSON 数组
	IP          string    `gorm:"type:text" json:"ip,omitempty"`        // 逗号/换行分隔或 JSON 数组
	Port        string    `gorm:"size:64" json:"port,omitempty"`
	Network     string    `gorm:"size:32" json:"network,omitempty"`
	InboundTag  string    `gorm:"type:text" json:"inbound_tag,omitempty"` // 逗号/换行分隔或 JSON 数组
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	Priority    int       `gorm:"default:0" json:"priority"`
	Remark      string    `gorm:"size:255" json:"remark"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
