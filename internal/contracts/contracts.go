// Package contracts 定义系统统一纯数据契约（DTO）与扩展能力接口（Interfaces）。
// 契约层零外部依赖，不依赖 GORM 模型，不依赖具体 Xray / Sing-box 实现，
// 作为内核宿主与所有插件、适配器交互的统一语言。
package contracts

import (
	"context"
	"time"
)

// ProxyNodeDTO 代理节点标准纯数据契约（订阅与代理导出时的公共模型）。
type ProxyNodeDTO struct {
	ID            uint64
	Name          string
	ServerHost    string // 客户端连接的主机名/IP（已由 share_addr 决策）
	ServerPort    int    // 客户端连接的端口（已由 share_port 决策）
	Protocol      string // vless
	Network       string // tcp, xhttp
	TLSType       string // none, tls, reality
	SNI           string // TLS SNI 握手域名（已由 share_sni 或 tlsSettings 决策）
	AllowInsecure bool   // 是否跳过证书校验

	// 传输层参数 (xhttp)
	Path string            // XHTTP 路径（已由 share_path 或 xhttpSettings 决策）
	Host string            // HTTP Host 头（已由 share_host 或 xhttpSettings 决策）
	Mode string            // XHTTP mode (auto, stream-up, packet-up)
	Opts map[string]string // 附加传输参数

	// 专有特性扩展（Capabilities）
	Flow       string // 如 xtls-rprx-vision
	NoAutoFlow bool   // 是否禁止自动注入 vision
	Reality    *RealityOptions
}

// RealityOptions REALITY 专有扩展参数。
type RealityOptions struct {
	PublicKey string
	ShortID   string
}

// ClientCredentialDTO 客户端认证凭证 DTO。
type ClientCredentialDTO struct {
	UserID      uint64
	Email       string
	UUID        string
	Password    string
	Flow        string
	DeviceLimit int
	SpeedLimit  int64 // 字节/秒
}

// UserSummaryDTO 用户状态摘要 DTO。
type UserSummaryDTO struct {
	ID          uint64
	Username    string
	Email       string
	UUID        string
	Status      int // 1: 正常, 0: 禁用
	PlanID      uint64
	TrafficUsed int64
	TrafficMax  int64
	ExpireTime  *time.Time
}

// SubscriptionExporter 订阅导出器插件接口。
type SubscriptionExporter interface {
	FormatKey() string // "clash", "sing-box", "base64", "surge"
	MatchUserAgent(ua string) bool
	Export(ctx context.Context, user UserSummaryDTO, nodes []ProxyNodeDTO, template string) (content string, contentType string, err error)
}

// CoreDriver 代理核心配置驱动接口。
type CoreDriver interface {
	Name() string // "xray", "sing-box"
	ValidateConfig(ctx context.Context, rawConfig string) error
}

// IngressTopology 入站拓扑适配器接口。
type IngressTopology interface {
	Type() string // "standalone_direct", "caddy_reverse_proxy", "cdn_proxy"
	BuildListen(inbound any) (listenHost string, listenPort int, security string, err error)
	BuildShare(inbound any, serverHost string) (shareHost string, sharePort int, shareSecurity string, err error)
}
