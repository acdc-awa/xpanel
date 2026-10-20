// Package protocols 协议插件注册表：同一协议的服务端注入与客户端映射内聚于一个插件。
// 新增协议 = 新增一个实现 contracts.ProtocolPlugin 的文件并在 init 注册，
// 生成（xray driver）、订阅（subscribe）、取数（services）三侧自动获得该协议能力。
package protocols

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/contracts"
)

var registry = contracts.NewPluginRegistry()

func init() {
	registry.Register(VLESSPlugin{})
	registry.Register(HysteriaPlugin{})
}

// Find 按协议名查找插件；未注册返回 nil。
func Find(proto string) contracts.ProtocolPlugin {
	return registry.Find(proto)
}

// FindByScheme 按 URL scheme 查找匹配的协议插件；未注册返回 nil。
func FindByScheme(scheme string) contracts.ProtocolPlugin {
	return registry.FindByScheme(scheme)
}

// ParseLink 将经典代理订阅链接（vless://, hysteria2://, socks5://, http:// 等）解析为统一中性 ProxyNodeDTO。
func ParseLink(rawURL string) (*contracts.ProxyNodeDTO, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("代理链接不能为空")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("解析 URL 失败: %w", err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("缺少协议 Scheme")
	}
	p := FindByScheme(u.Scheme)
	if p == nil {
		return nil, fmt.Errorf("暂不支持协议 scheme %q 的代理链接", u.Scheme)
	}
	return p.ParseLink(u)
}

// RenderOutbound 将统一中性 ProxyNodeDTO 渲染为 Xray 客户端出站结构 map。
func RenderOutbound(dto *contracts.ProxyNodeDTO) (map[string]any, error) {
	if dto == nil {
		return nil, fmt.Errorf("DTO 不能为空")
	}
	p := Find(dto.Protocol)
	if p == nil {
		return nil, fmt.Errorf("暂不支持将协议 %q 渲染为出站配置", dto.Protocol)
	}
	return p.RenderOutbound(dto)
}

// FormatLink 将统一中性 ProxyNodeDTO 导出为对应协议的标准订阅链接。
func FormatLink(dto *contracts.ProxyNodeDTO) (string, error) {
	if dto == nil {
		return "", fmt.Errorf("DTO 不能为空")
	}
	p := Find(dto.Protocol)
	if p == nil {
		return "", fmt.Errorf("暂不支持将协议 %q 格式化为订阅链接", dto.Protocol)
	}
	return p.FormatLink(dto)
}

// AllDescriptors 获取所有已注册协议的描述符。
func AllDescriptors() []contracts.ProtocolDescriptor {
	return registry.AllDescriptors()
}

// RegisteredProtocols 获取所有已注册的协议名列表。
func RegisteredProtocols() []string {
	return registry.RegisteredProtocols()
}

// ServerUsers 按协议生成服务端入站用户数组及挂载键名。
// 未注册协议显式报错拒绝（Fail-Closed 铁律，坚决杜绝静默注入假 UUID 或丢节点）。
func ServerUsers(proto string, users []protocol.User, spec *contracts.InboundSpec) (string, []any, error) {
	p := Find(proto)
	if p == nil {
		return "", nil, fmt.Errorf("协议 %q 未注册协议插件", proto)
	}
	return p.ServerUsers(users, spec)
}

// ServerClients 兼容保留：按协议生成服务端入站 settings.clients。
func ServerClients(proto string, users []protocol.User, spec *contracts.InboundSpec) []any {
	_, items, _ := ServerUsers(proto, users, spec)
	return items
}

// ResolveFlow 按协议决议用户 flow（生成侧盖戳与订阅侧同源）；
// 未注册协议返回空（flow 是 VLESS 语义，不应泄漏到其他协议）。
func ResolveFlow(proto string, spec *contracts.InboundSpec, inboundFlow string) string {
	if p := Find(proto); p != nil {
		return p.ResolveFlow(spec, inboundFlow)
	}
	return ""
}

// ApplyPeerTLSValidation 统一处理机间互联 Peer 出站的 TLS 校验配置（支持 Pin 与 托管 CA 验签）。
func ApplyPeerTLSValidation(dto *contracts.ProxyNodeDTO, in *contracts.ClientProjectionInput, defaultServerName string) {
	if dto == nil || in == nil || in.RefTarget == nil || dto.Security == nil || dto.Security.Type != "tls" {
		return
	}
	target := in.RefTarget
	serverName := defaultServerName
	if target.TargetCertDomain != "" {
		serverName = target.TargetCertDomain
	}
	dto.Security.SNI = serverName

	useCA := false
	if in.TlsVerifyMode == "ca" {
		useCA = (target.CACert != "")
	} else if in.TlsVerifyMode == "pin" {
		useCA = false
	} else {
		// 自动模式：目标入站绑定证书若关联托管 CA，则优先采用 CA 验签
		useCA = (target.CACert != "")
	}

	if useCA {
		dto.Security.CACert = target.CACert
		dto.Security.DisableSystemRoot = true
		dto.Security.VerifyPeerCertByName = serverName
		if dto.Auth != nil && dto.Auth.Extra != nil {
			delete(dto.Auth.Extra, "CertPin")
		}
	} else if target.CertPin != "" {
		if dto.Auth != nil && dto.Auth.Extra != nil {
			dto.Auth.Extra["CertPin"] = target.CertPin
		}
	}
}
