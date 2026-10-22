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
	u, hopPorts, err := parseURLWithHopPort(rawURL)
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
	dto, err := p.ParseLink(u)
	if err != nil || dto == nil || hopPorts == "" {
		return dto, err
	}
	// 端口跳跃集合由外层预解析补回（插件只看到首个端口）。
	if dto.Transport == nil {
		dto.Transport = &contracts.TransportOptions{}
	}
	if dto.Transport.Opts == nil {
		dto.Transport.Opts = make(map[string]string)
	}
	dto.Transport.Opts[Hy2OptPorts] = hopPorts
	return dto, nil
}

// parseURLWithHopPort 兼容 hysteria2 的端口跳跃链接：authority 的端口位置是集合
// （如 host:443,5000-6000），标准 url.Parse 会因端口非数字而失败。
// 解析失败时截出首个端口交给标准解析，集合同样返回给调用方。
func parseURLWithHopPort(rawURL string) (*url.URL, string, error) {
	if u, err := url.Parse(rawURL); err == nil {
		return u, "", nil
	}
	scheme, rest, ok := strings.Cut(rawURL, "://")
	if !ok {
		return nil, "", fmt.Errorf("无效 URL: %s", rawURL)
	}
	ls := strings.ToLower(scheme)
	if ls != "hysteria2" && ls != "hy2" {
		_, err := url.Parse(rawURL)
		return nil, "", err
	}
	end := strings.IndexAny(rest, "/?#")
	authority, tail := rest, ""
	if end >= 0 {
		authority, tail = rest[:end], rest[end:]
	}
	at := strings.LastIndex(authority, "@")
	userinfo, hostPort := "", authority
	if at >= 0 {
		userinfo, hostPort = authority[:at+1], authority[at+1:]
	}
	colon := strings.LastIndex(hostPort, ":")
	if colon < 0 {
		return nil, "", fmt.Errorf("端口跳跃链接缺少端口: %s", rawURL)
	}
	host, ports := hostPort[:colon], hostPort[colon+1:]
	if !strings.ContainsAny(ports, ",-") {
		_, err := url.Parse(rawURL)
		return nil, "", err
	}
	first := ports
	if i := strings.IndexAny(ports, ",-"); i > 0 {
		first = ports[:i]
	}
	if first == "" {
		return nil, "", fmt.Errorf("端口跳跃链接端口非法: %s", ports)
	}
	u, err := url.Parse(scheme + "://" + userinfo + host + ":" + first + tail)
	if err != nil {
		return nil, "", err
	}
	return u, ports, nil
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

// NormalizeServer 调用协议插件的服务端规范化；未注册协议原样透传
// （与历史行为一致：真正拒绝发生在 ServerUsers，未知协议无法注入用户）。
func NormalizeServer(proto string, spec *contracts.InboundSpec) (*contracts.ServerOverrides, error) {
	if p := Find(proto); p != nil {
		return p.NormalizeServer(spec)
	}
	return nil, nil
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

// ResolveRelayFlow 按协议决议机间中继凭据的 flow（服务端注入）。
// 未注册协议返回空。
func ResolveRelayFlow(proto string, spec *contracts.InboundSpec, inboundFlow string) string {
	if p := Find(proto); p != nil {
		return p.ResolveRelayFlow(spec, inboundFlow)
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

	// 托管 CA 验签材料：新 agent 走文件（push_ca 落盘），旧 agent 走内联 PEM。
	hasCA := target.CACert != "" || target.CACertFile != ""
	useCA := false
	if in.TlsVerifyMode == "ca" {
		useCA = hasCA
	} else if in.TlsVerifyMode == "pin" {
		useCA = false
	} else {
		// 自动模式：目标入站绑定证书若关联托管 CA，则优先采用 CA 验签
		useCA = hasCA
	}

	if useCA {
		if target.CACertFile != "" {
			dto.Security.CACertFile = target.CACertFile
			dto.Security.CACert = ""
		} else {
			dto.Security.CACert = target.CACert
		}
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
