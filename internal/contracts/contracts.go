// Package contracts 定义系统统一纯数据契约（DTO）与扩展能力接口（Interfaces）。
// 契约层不依赖具体框架或 GORM 实现，也不依赖具体 Xray / Sing-box 实现；
// 为了承载业务接口，允许引用领域模型与协议 DTO，但保持单向依赖：
// contracts -> models/protocol，不能被上层具体实现反向依赖。
package contracts

import (
	"context"
	"time"
)

// ProxyNodeDTO 代理节点标准纯数据契约（订阅与代理导出时的公共模型）。
// 公共字段承载所有协议共有的连接信息；协议差异通过 Transport/Security/Auth/Features 扩展。
type ProxyNodeDTO struct {
	ID         uint64
	Name       string
	ServerHost string // 客户端连接的主机名/IP（已由 share_addr 决策）
	ServerPort int    // 客户端连接的端口（已由 share_port 决策）
	Protocol   string // vless / vmess / trojan / ss / hysteria2 ...

	Transport *TransportOptions    // 传输层能力（tcp/xhttp/ws/grpc/quic...）
	Security  *SecurityOptions     // 安全层能力（none/tls/reality/ech...）
	Auth      *ClientCredentialDTO // 认证凭证（UUID/Password/Cipher...）
	Features  []string             // 专有特性，如 ["vision", "mux", "brutal"]
}

// TransportOptions 传输层通用扩展点。
type TransportOptions struct {
	Network string            // tcp, xhttp, ws, grpc, quic ...
	Path    string            // 已由 share_path 或传输设置决策
	Host    string            // HTTP Host 头（已由 share_host 或传输设置决策）
	Mode    string            // XHTTP mode (auto, stream-up, packet-up)
	Opts    map[string]string // 附加传输参数

	// 后续新传输扩展字段追加到这里，不污染 ProxyNodeDTO 公共结构。
}

// SecurityOptions 安全层通用扩展点。
type SecurityOptions struct {
	Type          string // none, tls, reality, ech ...
	SNI           string // 已由 share_sni 或安全设置决策
	AllowInsecure bool   // 是否跳过证书校验
	Reality       *RealityOptions
}

// RealityOptions REALITY 专有扩展参数。
type RealityOptions struct {
	PublicKey string
	ShortID   string
}

// ClientCredentialDTO 客户端认证凭证 DTO。
// 这里放“大多数代理协议会用到的通用认证字段”，具体协议用不到就留空；
// 更冷门的协议专属字段放入 Extra，避免公共结构被协议写死。
type ClientCredentialDTO struct {
	UUID           string         // VLESS / VMess
	Password       string         // Trojan / Shadowsocks / SOCKS / HTTP
	Cipher         string         // VMess cipher / Shadowsocks cipher
	AlterID        int            // VMess alterId
	Flow           string         // VLESS flow (xtls-rprx-vision, ...)
	Encryption     string         // VLESS 新 encryption / 其他协议的 encryption 选项
	PacketEncoding string         // vless/vmess packet-encoding
	Plugin         string         // Shadowsocks plugin
	PluginOpts     map[string]any // Shadowsocks plugin-opts
	Extra          map[string]any // 未来协议特别字段
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

// ExportOptions 订阅导出器选项。
type ExportOptions struct {
	Template  string // Clash/订阅模板；其他导出器可忽略
	PanelHost string // 面板域名，用于模板中的 $PANEL_HOST$ 等占位符
}

// SubscriptionExporter 订阅导出器插件接口。
type SubscriptionExporter interface {
	FormatKey() string // "clash", "sing-box", "base64", "surge"
	MatchUserAgent(ua string) bool
	Export(ctx context.Context, user UserSummaryDTO, nodes []ProxyNodeDTO, opts ExportOptions) (content string, contentType string, err error)
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
