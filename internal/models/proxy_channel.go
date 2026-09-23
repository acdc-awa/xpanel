package models

import (
	"time"
)

// ProxyChannel 独立通道（四层端口转发 / SOCKS5 独立代理 / HTTP 独立代理）。
// 拥有独立的计费账本（配额、已用流量、重置周期）与生命周期状态机，
// 底层自动联动托管一个 Inbound (type="channel")。
// 预留 UserID 字段，支持未来平滑接入用户账户体系。
type ProxyChannel struct {
	ID        uint64 `gorm:"primaryKey" json:"id"`
	Name      string `gorm:"size:64;not null" json:"name"`     // 通道名称/备注（如"海外爬虫专线"）
	ServerID  uint64 `gorm:"index;not null" json:"server_id"`  // 所属服务器
	InboundID uint64 `gorm:"index;not null" json:"inbound_id"` // 对应底层 Inbound ID
	Port      int    `gorm:"not null" json:"port"`             // 监听端口
	Listen    string `gorm:"size:64" json:"listen"`            // 监听地址（空=0.0.0.0）
	Protocol  string `gorm:"size:16;not null" json:"protocol"` // tunnel / socks5 / http

	// 业务参数
	TargetAddress string `gorm:"size:255" json:"target_address,omitempty"` // 仅 tunnel: 目标地址
	TargetPort    int    `gorm:"default:0" json:"target_port,omitempty"`    // 仅 tunnel: 目标端口
	ProxyProtocol bool   `gorm:"default:false" json:"proxy_protocol"`       // 仅 tunnel: 是否透传 PROXY Protocol v2
	Username      string `gorm:"size:64" json:"username,omitempty"`         // 仅 socks5/http: 认证用户名
	Password      string `gorm:"size:64" json:"password,omitempty"`         // 仅 socks5/http: 认证密码
	AllowUDP      bool   `gorm:"default:true" json:"allow_udp"`             // 仅 socks5: 是否支持 UDP

	// 独立计费账户与生命周期
	TrafficLimitGB   int64      `gorm:"default:0" json:"traffic_limit_gb"`          // 周期总配额（GB，0=不限）
	TrafficUsedBytes int64      `gorm:"default:0" json:"traffic_used_bytes"`        // 本周期已用字节（up+down）
	UpBytes          int64      `gorm:"default:0" json:"up_bytes"`                  // 本周期上传字节
	DownBytes        int64      `gorm:"default:0" json:"down_bytes"`                // 本周期下载字节
	TrafficReset     string     `gorm:"size:16;default:never" json:"traffic_reset"` // never / daily / weekly / monthly
	LastResetDate    string     `gorm:"size:10" json:"-"`                           // 上次重置标记（YYYY-MM-DD / YYYY-MM）
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`                       // 到期时间（nil=不过期）
	AutoDisable      bool       `gorm:"default:true" json:"auto_disable"`           // 超额/到期是否自动停用端口
	Status           string     `gorm:"size:16;default:active" json:"status"`       // active / quota_exceeded / expired / disabled
	Enabled          bool       `gorm:"default:true" json:"enabled"`                // 管理员手动开关

	// 预留接缝（未来融入用户中心）
	UserID *uint64 `gorm:"index" json:"user_id,omitempty"` // 所属用户 ID（当前为 nil）

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
