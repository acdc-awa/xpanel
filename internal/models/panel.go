package models

import "time"

// Server 节点服务器（对应 §5 servers）。
type Server struct {
	ID                    uint64     `gorm:"primaryKey" json:"id"`
	ServerType            string     `gorm:"size:32;default:xray" json:"server_type"`             // xray（托管节点）/ l4_relay（纯4层中转）
	Name                  string     `gorm:"size:64;not null" json:"name"`
	Host                  string     `gorm:"size:255;not null" json:"host"`
	NodeID                string     `gorm:"size:32;uniqueIndex;not null" json:"node_id"`
	Secret                string     `gorm:"size:64;not null" json:"-"`
	Location              string     `gorm:"size:64" json:"location"`
	Remark                string     `gorm:"size:255" json:"remark"`
	Status                int        `gorm:"default:0;index" json:"status"`                       // 0 离线 1 在线
	DefaultOutboundTag    string     `gorm:"size:64;default:direct" json:"default_outbound_tag"`  // 默认出口（路由未命中时的出站标签）
	RoutingDomainStrategy string     `gorm:"size:32;default:AsIs" json:"routing_domain_strategy"` // 路由域名策略 AsIs/IPIfNonMatch/IPOnDemand
	// 默认出口（freedom）的出站域名解析策略：AsIs/UseIP/UseIPv4/UseIPv6——作用于出站连接阶段
	// （与 routing_domain_strategy 语义不同：前者路由匹配阶段，后者出站解析阶段）
	DefaultOutboundDS string `gorm:"size:16;default:AsIs" json:"default_outbound_domain_strategy"`
	LastSeenAt            *time.Time `json:"last_seen_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// Inbound 入站（接入点），每节点可配多个。
// settings_json 存协议层 JSON（clients 动态注入 / fallbacks / decryption）；
// stream_settings 存传输层 JSON（network / security / realitySettings / wsSettings / tlsSettings 等）；
// sniffing 存流量嗅探 JSON。
type Inbound struct {
	ID             uint64  `gorm:"primaryKey" json:"id"`
	ServerID       uint64  `gorm:"index;not null" json:"server_id"`
	Tag            string  `gorm:"size:64;not null" json:"tag"`
	Protocol       string  `gorm:"size:16;not null" json:"protocol"` // vless / vmess / trojan / shadowsocks
	Port           int     `gorm:"not null" json:"port"`
	Listen         string  `gorm:"size:64" json:"listen"`            // 监听地址，空 = 0.0.0.0
	SettingsJSON   string  `gorm:"type:text" json:"settings_json"`   // 协议 settings（透传，clients 由后端注入）
	StreamSettings string  `gorm:"type:text" json:"stream_settings"` // 传输 streamSettings（透传）
	Sniffing       string  `gorm:"type:text" json:"sniffing"`        // 嗅探配置（透传）
	Ratio          float64 `gorm:"default:1" json:"ratio"`
	// 流量统计（冗余计数器，避免每次 SUM traffic_logs）
	Up           int64      `gorm:"default:0" json:"up"`
	Down         int64      `gorm:"default:0" json:"down"`
	Total        int64      `gorm:"default:0" json:"total"`                     // 入站总流量上限（0=不限）
	TrafficReset string     `gorm:"size:16;default:never" json:"traffic_reset"` // never / daily / weekly / monthly
	LastResetDate string    `gorm:"size:10" json:"-"`                           // ISSUE-05：上次清零周期键（YYYY-MM-DD / YYYY-MM），防止同周期重复清零
	ExpiryTime   *time.Time `json:"expiry_time,omitempty"`                      // 入站自身到期时间
	// 分享地址（订阅专用，与节点监听解耦：四层转发场景监听为内网，订阅给用户的是转发端点）
	ShareAddrStrategy string    `gorm:"size:16;default:node" json:"share_addr_strategy"` // node / listen / custom
	ShareAddr         string    `gorm:"size:255" json:"share_addr"`                      // 自定义分享地址（域名/IP，不带端口）
	SharePort         int       `gorm:"default:0" json:"share_port"`                     // 自定义分享端口（0 = 使用入站端口）
	// 外部反代与订阅覆写字段（与本地物理监听解耦，支持 Caddy/CDN TLS 卸载模式）
	ShareSecurity      string `gorm:"size:16;default:auto" json:"share_security"` // auto（跟随stream_settings）/ tls / none
	ShareSNI           string `gorm:"size:255" json:"share_sni"`                  // 订阅 SNI 覆写（如反代域名）
	ShareHost          string `gorm:"size:255" json:"share_host"`                 // 订阅 HTTP/WS Host 覆写
	SharePath          string `gorm:"size:255" json:"share_path"`                 // 订阅 WS/XHTTP Path 覆写
	ShareAllowInsecure bool   `gorm:"default:false" json:"share_allow_insecure"`  // 订阅是否跳过证书检查
	// Phase T 拓扑化：入站三态
	Type          string  `gorm:"size:16;default:user" json:"type"`       // user（进订阅）/ relay（内部转发）/ idle（闲置）
	PreviousType  string  `gorm:"size:16" json:"-"`                       // 被自动标 relay 前的类型（解绑引用后回退；空 = 原本即 relay/idle，保持不动）
	InternalUUID  string  `gorm:"size:36" json:"internal_uuid,omitempty"` // relay 入站 UUID（节点生成上报，主控只读）
	CertID        *uint64 `gorm:"index" json:"cert_id,omitempty"`         // TLS 入站选择证书（certs 表）
	Enabled       bool    `gorm:"default:true" json:"enabled"`
	// 入站级流控（写进生成的 clients，VLESS settings 无顶层 flow，不入 settings_json）：
	// 空 = 自动（TCP+REALITY 自动注入 xtls-rprx-vision）；
	// xtls-rprx-vision = 为该入站用户全部开启；
	// none = 禁用自动注入。（UserInbound per-user 覆盖已随 2026-08-14 批2 冻结删除）
	Flow          string    `gorm:"size:32" json:"flow"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Plan 套餐。
type Plan struct {
	ID                uint64    `gorm:"primaryKey" json:"id"`
	Name              string    `gorm:"size:64;not null" json:"name"`
	Description       string    `gorm:"type:text" json:"description"`                // 自定义文案/套餐特性说明
	PriceCents        int64     `gorm:"not null" json:"price_cents"` // 价格（分）
	TrafficGB         int64     `gorm:"not null" json:"traffic_gb"`
	DurationDays      int       `gorm:"not null" json:"duration_days"`
	DeviceLimit       int       `gorm:"default:0" json:"device_limit"`              // 最大在线设备数（0=不限）
	PermissionGroupID uint64    `gorm:"index;default:0" json:"permission_group_id"` // 绑定权限组（0=不绑定）
	Enabled           bool      `gorm:"default:true" json:"enabled"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Order 订单（余额直付即时生效：paid）。人工确认收款已去除（2026-08-14 方向④）。
type Order struct {
	ID            uint64     `gorm:"primaryKey" json:"id"`
	OrderNo       string     `gorm:"size:32;uniqueIndex;not null" json:"order_no"`
	UserID        uint64     `gorm:"index;not null" json:"user_id"`
	PlanID        uint64     `gorm:"index;not null" json:"plan_id"`
	AmountCents   int64      `gorm:"not null" json:"amount_cents"`
	PaymentMethod string     `gorm:"size:32;default:balance;index" json:"payment_method"` // balance（唯一）
	Status        string     `gorm:"size:16;default:paid;index" json:"status"`            // paid（唯一）
	CreatedAt     time.Time  `json:"created_at"`
	PaidAt        *time.Time `json:"paid_at"`
}

// TrafficLog 节点上报的流量明细（按 用户×入站×周期）。
// (user_id, inbound_id, period_start) 三列唯一：同一上报周期重复投递时覆盖合并（补报幂等）。
// 2026-08-14 U1 修复：原仅 period_start 单列唯一索引 → 多用户共周期上报时互相冲突丢数据。
type TrafficLog struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	UserID      uint64    `gorm:"uniqueIndex:idx_traffic_uid_inb_period,priority:1;not null" json:"user_id"`
	InboundID   uint64    `gorm:"uniqueIndex:idx_traffic_uid_inb_period,priority:2;index" json:"inbound_id"`
	UpBytes     int64     `gorm:"not null" json:"up_bytes"`
	DownBytes   int64     `gorm:"not null" json:"down_bytes"`
	PeriodStart time.Time `gorm:"uniqueIndex:idx_traffic_uid_inb_period,priority:3;index:idx_traffic_period_start" json:"period_start"`
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
	ServerID    uint64    `gorm:"index:idx_server_reported,priority:1;not null" json:"server_id"`
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
	ReportedAt  time.Time `gorm:"index:idx_server_reported,priority:2;index" json:"reported_at"`
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

// PendingCert 证书待推记录（U7：节点离线时上传的证书，上线后补推）。
type PendingCert struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	ServerID  uint64    `gorm:"uniqueIndex;not null" json:"server_id"`
	CertID    uint64    `gorm:"not null" json:"cert_id"`
	Status    string    `gorm:"size:16;default:pending" json:"status"` // pending / pushed
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
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
	// Phase T 拓扑化：引用目标入站（落地），vnext 由生成器自动构造；空 = 沿用透传 settings_json
	InboundRef *uint64 `gorm:"index" json:"inbound_ref,omitempty"`
	Enabled    bool    `gorm:"default:true" json:"enabled"`
	Priority   int     `gorm:"default:0" json:"priority"`
	Remark     string  `gorm:"size:255" json:"remark"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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
	Protocol    string    `gorm:"size:64" json:"protocol,omitempty"`      // bittorrent / http / tls / quic，逗号分隔多选
	InboundTag  string    `gorm:"type:text" json:"inbound_tag,omitempty"` // 逗号/换行分隔或 JSON 数组
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	Priority    int       `gorm:"default:0" json:"priority"`
	Remark      string    `gorm:"size:255" json:"remark"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
