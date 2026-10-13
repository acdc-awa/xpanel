package contracts

import (
	"net/url"
	"strings"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
)

// ShareOverride 订阅分享覆写（入站 share_* 字段的纯数据快照）。
// 服务端物理监听与外部反代/订阅视图解耦：覆写只影响客户端可见参数。
type ShareOverride struct {
	Security      string // "" = 跟随服务端 streamSettings；tls / none 强制覆写
	SNI           string
	Host          string
	Path          string
	AllowInsecure bool
}

// EndpointRole 区分客户端投影语义。
type EndpointRole string

const (
	RoleSubscription EndpointRole = "subscription" // 用户客户端订阅（消费 share_* 门面覆写）
	RolePeer         EndpointRole = "peer"         // 节点间互联（消费 RefTarget 内部真实地址与 InternalUUID）
)

// CredentialKind 凭证语义（决定服务端用户元素与客户端凭证字段）。
type CredentialKind string

const (
	CredentialUUID     CredentialKind = "uuid"     // vless / vmess
	CredentialPassword CredentialKind = "password" // hysteria / trojan / ss / socks / http
)

// FieldRole 字段角色：决定 sanitize 剥离与订阅消费。
type FieldRole string

const (
	RoleServerOnly FieldRole = "server" // 只进服务端配置（如 realitySettings.privateKey）
	RoleClientOnly FieldRole = "client" // 只进客户端（如 fingerprint）
	RoleBoth       FieldRole = "both"
)

// LayerSegment 某一层上的协议专有具名段。
type LayerSegment struct {
	Layer string    // "settings" | "stream" | "sniffing"
	Key   string    // "xhttpSettings" / "hysteriaSettings" / "finalmask" / "realitySettings"
	Role  FieldRole
}

// ProtocolDescriptor 协议能力描述符（纯数据；生成/校验/订阅/前端共用）。
type ProtocolDescriptor struct {
	Name  string // Xray 的 protocol 字符串："vless" / "hysteria" / ...
	Label string // 前端显示名

	Networks   []string // 允许的传输：{"tcp","xhttp"} / {"hysteria"}
	Securities []string // 允许的安全层：{"reality","tls","none"} / {"tls"}

	// 非空 = 该协议强绑定此值，前端不可选、生成期强制。
	ForceNetwork  string // hysteria → "hysteria"
	ForceSecurity string // hysteria → "tls"

	Credential  CredentialKind
	SupportsUDP bool
	HotSync     bool   // 能否 gRPC 热更（决定用户变更走热更还是整配置重推）
	AccountType string // agent 侧 account 的 Go 类型标识："vless" / "hysteria"

	InboundTypes  []string       // 允许的入站角色：{"user","relay"} / {"user"}
	Segments      []LayerSegment // 该协议的专有段（供 sanitize/订阅/前端）
	ClientSchemes []string       // 支持的客户端订阅 URL Scheme 列表，如 ["vless"] / ["hysteria2", "hy2"] / ["socks5", "socks", "http", "https"]
}

// ClientProjectionInput 统一客户端投影输入。
type ClientProjectionInput struct {
	Role EndpointRole // subscription | peer

	// 订阅视角（RoleSubscription）：节点名/对外地址/端口已由订阅服务决议
	Name        string
	Host        string
	Port        int
	Spec        *InboundSpec  // 入站统一解码结果
	Share       ShareOverride // 分享覆写
	InboundFlow string        // 入站级 Flow
	UserUUID    string        // 用户凭证原料

	// 节点互联视角（RolePeer）：中转出站引用的真实目标节点信息
	RefTarget *RefTarget
	// 出站专属机间认证凭据（Per-Outbound InternalUUID）
	OutboundUUID string
	// TlsVerifyMode 出站显式配置的验证模式（"" = 自动/优先CA, "ca" = 强制CA验签, "pin" = 强制Pin）
	TlsVerifyMode string
}

// ClientNodeInput 兼容保留：旧版订阅节点组装输入（内部桥接至 ClientProjectionInput）。
type ClientNodeInput struct {
	Name string
	Host string
	Port int

	Spec        *InboundSpec
	Share       ShareOverride
	InboundFlow string
	UserUUID    string
}

// ProtocolPlugin 协议插件：同一协议的「服务端用户注入 / 用户 flow 决议 /
// 客户端投影 / 入站校验 / 能力描述」内聚一处。新增协议 = 实现本接口并注册。
type ProtocolPlugin interface {
	Protocol() string // "vless" / "hysteria" ...
	Descriptor() ProtocolDescriptor

	// ServerUsers 生成服务端入站用户数组：返回「挂载键名」+「元素切片」+「error」。
	// 如 vless 返回 key="clients", items=[{id,email,flow,limit}], err=nil。
	ServerUsers(users []protocol.User, spec *InboundSpec) (key string, items []any, err error)

	// ServerClients 兼容保留：旧版服务端 clients 注入，内部桥接调用 ServerUsers。
	ServerClients(users []protocol.User, spec *InboundSpec) []any

	// ProjectClient 统一客户端端点投影（按 role 区分 subscription / peer）。
	ProjectClient(in *ClientProjectionInput) (*ProxyNodeDTO, error)

	// BuildClientNode 兼容保留：组装订阅导出用 ProxyNodeDTO，内部桥接调用 ProjectClient。
	BuildClientNode(in *ClientNodeInput) *ProxyNodeDTO

	// ResolveFlow 决议用户在该入站上的 flow（生成侧盖戳与订阅侧同源）。
	ResolveFlow(spec *InboundSpec, inboundFlow string) string

	// ValidateInbound 入站保存时的协议级校验（在 JSON 可解析校验之上）。
	ValidateInbound(spec *InboundSpec) error

	// FormatLink 格式化为该协议的标准订阅 URL（如 vless://...）。
	FormatLink(dto *ProxyNodeDTO) (string, error)

	// ParseLink 解析订阅 URL 为中性 ProxyNodeDTO。
	ParseLink(u *url.URL) (*ProxyNodeDTO, error)

	// RenderOutbound 渲染为 Xray 客户端出站结构 map（含 settings 与 streamSettings）。
	RenderOutbound(dto *ProxyNodeDTO) (map[string]any, error)
}

// PluginRegistry 协议插件注册表：按协议名注册/查找。
type PluginRegistry struct {
	plugins []ProtocolPlugin
}

// NewPluginRegistry 创建空注册表。
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{}
}

// Register 注册插件；后注册的同名插件覆盖之前的同名项。
func (r *PluginRegistry) Register(p ProtocolPlugin) {
	if p == nil {
		return
	}
	for i, old := range r.plugins {
		if old.Protocol() == p.Protocol() {
			r.plugins[i] = p
			return
		}
	}
	r.plugins = append(r.plugins, p)
}

// Find 按协议名查找插件；未找到返回 nil。
func (r *PluginRegistry) Find(proto string) ProtocolPlugin {
	for _, p := range r.plugins {
		if p.Protocol() == proto {
			return p
		}
	}
	return nil
}

// FindByScheme 按 URL scheme 查找匹配的协议插件；未找到返回 nil。
func (r *PluginRegistry) FindByScheme(scheme string) ProtocolPlugin {
	sLower := strings.ToLower(scheme)
	for _, p := range r.plugins {
		for _, s := range p.Descriptor().ClientSchemes {
			if strings.ToLower(s) == sLower {
				return p
			}
		}
	}
	return nil
}

// AllDescriptors 返回已注册协议的所有能力描述符。
func (r *PluginRegistry) AllDescriptors() []ProtocolDescriptor {
	res := make([]ProtocolDescriptor, 0, len(r.plugins))
	for _, p := range r.plugins {
		res = append(res, p.Descriptor())
	}
	return res
}

// RegisteredProtocols 返回所有已注册的协议名。
func (r *PluginRegistry) RegisteredProtocols() []string {
	res := make([]string, 0, len(r.plugins))
	for _, p := range r.plugins {
		res = append(res, p.Protocol())
	}
	return res
}
