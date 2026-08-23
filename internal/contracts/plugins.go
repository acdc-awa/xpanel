package contracts

import "github.com/acdc-awa/xpanel-node/pkg/protocol"

// ShareOverride 订阅分享覆写（入站 share_* 字段的纯数据快照）。
// 服务端物理监听与外部反代/订阅视图解耦：覆写只影响客户端可见参数。
type ShareOverride struct {
	Security      string // "" = 跟随服务端 streamSettings；tls / none 强制覆写
	SNI           string
	Host          string
	Path          string
	AllowInsecure bool
}

// ClientNodeInput 协议插件组装订阅节点 DTO 的输入。
// 节点名/对外地址/端口已由订阅服务决议（ShareAddrOf/NodeName），插件只负责协议知识。
type ClientNodeInput struct {
	Name string // 已决议节点名
	Host string // 已决议对外地址
	Port int    // 已决议对外端口

	Spec        *InboundSpec  // 入站统一解码结果
	Share       ShareOverride // 分享覆写
	InboundFlow string        // 入站级 Flow（"" 自动 / none 禁用 / 显式值）
	UserUUID    string        // 用户凭证原料（各协议派生规则见插件实现）
}

// ProtocolPlugin 协议插件：同一协议的「服务端 clients 注入 / 用户 flow 决议 /
// 订阅 DTO 组装 / 入站校验」内聚一处。新增协议 = 实现本接口并注册。
type ProtocolPlugin interface {
	Protocol() string // "vless" / "vmess" / "trojan" / "ss" ...

	// ServerClients 生成服务端入站 settings.clients（用户态入站）。
	// users 已经过权限组过滤与 flow 盖戳（ResolveFlow 同源）。
	ServerClients(users []protocol.User, spec *InboundSpec) []any

	// ResolveFlow 决议用户在该入站上的 flow（生成侧盖戳与订阅侧同源）。
	// inboundFlow 为入站级 Flow 字段（"" 自动 / none 禁用 / 显式值）。
	ResolveFlow(spec *InboundSpec, inboundFlow string) string

	// BuildClientNode 组装订阅导出用 ProxyNodeDTO；
	// 返回 nil 表示该入站不足以产出订阅节点（如 reality 缺 SNI），调用方跳过。
	BuildClientNode(in *ClientNodeInput) *ProxyNodeDTO

	// ValidateInbound 入站保存时的协议级校验（在 JSON 可解析校验之上）。
	ValidateInbound(spec *InboundSpec) error
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
