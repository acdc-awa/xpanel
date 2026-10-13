package protocols

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/contracts"
)

// MixedPlugin Mixed 协议插件（SOCKS5 与 HTTP 代理合一入站，同时支持 SOCKS5 与 HTTP 客户端出站与链接互转）。
type MixedPlugin struct{}

func (MixedPlugin) Protocol() string { return "mixed" }

// Descriptor 声明 Mixed 协议能力描述符。
func (MixedPlugin) Descriptor() contracts.ProtocolDescriptor {
	return contracts.ProtocolDescriptor{
		Name:          "mixed",
		Label:         "Mixed (SOCKS5 / HTTP)",
		Networks:      []string{"tcp"},
		Securities:    []string{"none", "tls"},
		Credential:    contracts.CredentialPassword,
		SupportsUDP:   true,
		HotSync:       false,
		AccountType:   "mixed",
		InboundTypes:  []string{"user", "forward"},
		ClientSchemes: []string{"socks5", "socks", "socks5s", "http", "https"},
		Segments: []contracts.LayerSegment{
			{Layer: "settings", Key: "accounts", Role: contracts.RoleBoth},
			{Layer: "settings", Key: "auth", Role: contracts.RoleBoth},
			{Layer: "settings", Key: "udp", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "tlsSettings", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "sockopt", Role: contracts.RoleBoth},
		},
	}
}

// ServerUsers 生成服务端入站用户数组，返回挂载键 accounts。
func (MixedPlugin) ServerUsers(users []protocol.User, _ *contracts.InboundSpec) (string, []any, error) {
	accounts := make([]any, 0, len(users))
	for _, u := range users {
		if u.UUID == "" {
			continue
		}
		accounts = append(accounts, map[string]any{
			"user":  u.UUID,
			"pass":  u.Email,
			"email": u.Email,
		})
	}
	return "accounts", accounts, nil
}

// ServerClients 兼容保留：内部调用 ServerUsers。
func (p MixedPlugin) ServerClients(users []protocol.User, spec *contracts.InboundSpec) []any {
	_, items, _ := p.ServerUsers(users, spec)
	return items
}

// ResolveFlow Mixed 无 flow，固定返回空。
func (MixedPlugin) ResolveFlow(_ *contracts.InboundSpec, _ string) string {
	return ""
}

// ValidateInbound 入站保存校验。
func (MixedPlugin) ValidateInbound(spec *contracts.InboundSpec) error {
	if spec == nil {
		return nil
	}
	if spec.Network != "" && spec.Network != "tcp" {
		return fmt.Errorf("Mixed 传输协议仅支持 tcp，got: %s", spec.Network)
	}
	if spec.Security != "" && spec.Security != "none" && spec.Security != "tls" {
		return fmt.Errorf("Mixed 安全协议仅支持 none 或 tls，got: %s", spec.Security)
	}
	return nil
}

// ProjectClient 统一客户端端点投影（按 role 区分 subscription / peer）。
func (p MixedPlugin) ProjectClient(in *contracts.ClientProjectionInput) (*contracts.ProxyNodeDTO, error) {
	if in == nil {
		return nil, fmt.Errorf("projection input 不能为空")
	}

	switch in.Role {
	case contracts.RolePeer:
		return p.projectPeer(in)
	case contracts.RoleSubscription:
		return p.projectSubscription(in), nil
	default:
		return nil, fmt.Errorf("未知投影角色: %s", in.Role)
	}
}

// projectSubscription 用户订阅端点投影：默认导出 SOCKS5 DTO。
func (p MixedPlugin) projectSubscription(in *contracts.ClientProjectionInput) *contracts.ProxyNodeDTO {
	spec := in.Spec
	if spec == nil {
		return nil
	}

	sec := spec.Security
	if in.Share.Security == "tls" {
		sec = "tls"
	} else if in.Share.Security == "none" {
		sec = "none"
	}

	sni := in.Host
	if in.Share.SNI != "" {
		sni = in.Share.SNI
	} else if spec.TLS != nil && spec.TLS.ServerName != "" {
		sni = spec.TLS.ServerName
	}

	return &contracts.ProxyNodeDTO{
		ID:         spec.ID,
		Name:       in.Name,
		ServerHost: in.Host,
		ServerPort: in.Port,
		Protocol:   "socks",
		Transport:  &contracts.TransportOptions{Network: "tcp"},
		Security: &contracts.SecurityOptions{
			Type:          sec,
			SNI:           sni,
			AllowInsecure: in.Share.AllowInsecure,
		},
		Auth: &contracts.ClientCredentialDTO{
			User:     in.UserUUID,
			Password: in.UserUUID,
		},
	}
}

// projectPeer 节点间互联投影：消费真实监听与机间认证凭据（InternalUUID）。
func (p MixedPlugin) projectPeer(in *contracts.ClientProjectionInput) (*contracts.ProxyNodeDTO, error) {
	if in.RefTarget == nil {
		return nil, fmt.Errorf("节点互联投影必须包含 RefTarget")
	}
	target := in.RefTarget
	inb := target.Inbound
	spec := contracts.DecodeInbound(&inb)

	peerPass := in.OutboundUUID
	if peerPass == "" {
		peerPass = inb.InternalUUID
	}
	peerUser := fmt.Sprintf("relay_out_%d", inb.ID)

	dto := &contracts.ProxyNodeDTO{
		ID:         inb.ID,
		Name:       inb.Tag,
		ServerHost: target.ServerHost,
		ServerPort: inb.Port,
		Protocol:   "socks", // 机间互联默认优先以 SOCKS5 连接
		Transport:  &contracts.TransportOptions{Network: "tcp"},
		Security:   &contracts.SecurityOptions{Type: spec.Security},
		Auth: &contracts.ClientCredentialDTO{
			User:     peerUser,
			Password: peerPass,
			Extra:    make(map[string]any),
		},
	}
	if spec.Security == "tls" {
		sni := target.ServerHost
		if spec.TLS != nil && spec.TLS.ServerName != "" {
			sni = spec.TLS.ServerName
		}
		ApplyPeerTLSValidation(dto, in, sni)
	}
	return dto, nil
}

// BuildClientNode 兼容保留。
func (p MixedPlugin) BuildClientNode(in *contracts.ClientNodeInput) *contracts.ProxyNodeDTO {
	if in == nil {
		return nil
	}
	dto, _ := p.ProjectClient(&contracts.ClientProjectionInput{
		Role:     contracts.RoleSubscription,
		Name:     in.Name,
		Host:     in.Host,
		Port:     in.Port,
		Spec:     in.Spec,
		Share:    in.Share,
		UserUUID: in.UserUUID,
	})
	return dto
}

// FormatLink 格式化为标准 SOCKS5 (socks5://) 或 HTTP (http://) 链接。
func (MixedPlugin) FormatLink(dto *contracts.ProxyNodeDTO) (string, error) {
	if dto == nil {
		return "", fmt.Errorf("DTO 不能为空")
	}
	proto := "socks"
	if strings.EqualFold(dto.Protocol, "http") {
		proto = "http"
	}

	isTLS := dto.Security != nil && strings.EqualFold(dto.Security.Type, "tls")
	scheme := "socks5"
	if proto == "http" {
		if isTLS {
			scheme = "https"
		} else {
			scheme = "http"
		}
	} else {
		if isTLS {
			scheme = "socks5s"
		} else {
			scheme = "socks5"
		}
	}

	u := &url.URL{
		Scheme:   scheme,
		Host:     fmt.Sprintf("%s:%d", dto.ServerHost, dto.ServerPort),
		Fragment: dto.Name,
	}

	if dto.Auth != nil && dto.Auth.User != "" {
		u.User = url.UserPassword(dto.Auth.User, dto.Auth.Password)
	}

	// 若有 TLS SNI 且不同于 Host，带上 query
	if isTLS && dto.Security != nil && dto.Security.SNI != "" && dto.Security.SNI != dto.ServerHost {
		q := url.Values{}
		q.Set("sni", dto.Security.SNI)
		u.RawQuery = q.Encode()
	}

	return u.String(), nil
}

// ParseLink 解析 socks5://, socks://, socks5s://, http://, https:// 订阅链接为中性 ProxyNodeDTO。
func (MixedPlugin) ParseLink(u *url.URL) (*contracts.ProxyNodeDTO, error) {
	if u == nil {
		return nil, fmt.Errorf("url 不能为空")
	}
	scheme := strings.ToLower(u.Scheme)

	proto := "socks"
	isTLS := false
	switch scheme {
	case "socks", "socks5":
		proto = "socks"
	case "socks5s":
		proto = "socks"
		isTLS = true
	case "http":
		proto = "http"
	case "https":
		proto = "http"
		isTLS = true
	default:
		return nil, fmt.Errorf("不支持的代理链接 scheme: %s", u.Scheme)
	}

	host := u.Hostname()
	portStr := u.Port()
	if host == "" {
		return nil, fmt.Errorf("代理链接缺少主机: %s", u.Host)
	}
	port := 1080
	if proto == "http" {
		port = 8080
		if isTLS {
			port = 443
		}
	}
	if portStr != "" {
		var err error
		port, err = strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			return nil, fmt.Errorf("非法端口: %s", portStr)
		}
	}

	var user, pass string
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
	}

	q := u.Query()
	if q.Get("tls") == "1" || strings.EqualFold(q.Get("security"), "tls") {
		isTLS = true
	}
	sec := "none"
	sni := ""
	if isTLS {
		sec = "tls"
		sni = q.Get("sni")
		if sni == "" {
			sni = host
		}
	}

	name, _ := url.QueryUnescape(u.Fragment)
	if name == "" {
		name = fmt.Sprintf("%s-%s-%d", proto, host, port)
	}

	dto := &contracts.ProxyNodeDTO{
		Name:       name,
		ServerHost: host,
		ServerPort: port,
		Protocol:   proto,
		Transport:  &contracts.TransportOptions{Network: "tcp"},
		Security: &contracts.SecurityOptions{
			Type: sec,
			SNI:  sni,
		},
		Auth: &contracts.ClientCredentialDTO{
			User:     user,
			Password: pass,
			Extra:    make(map[string]any),
		},
	}
	return dto, nil
}

// RenderOutbound 将中性 ProxyNodeDTO 渲染为 Xray 客户端出站结构 map (socks 或 http)。
func (MixedPlugin) RenderOutbound(dto *contracts.ProxyNodeDTO) (map[string]any, error) {
	if dto == nil {
		return nil, fmt.Errorf("DTO 不能为空")
	}

	proto := "socks"
	if strings.EqualFold(dto.Protocol, "http") {
		proto = "http"
	}

	serverMap := map[string]any{
		"address": dto.ServerHost,
		"port":    dto.ServerPort,
	}

	if dto.Auth != nil && (dto.Auth.User != "" || dto.Auth.Password != "") {
		serverMap["users"] = []any{
			map[string]any{
				"user": dto.Auth.User,
				"pass": dto.Auth.Password,
			},
		}
	}

	item := map[string]any{
		"protocol": proto,
		"settings": map[string]any{
			"servers": []any{serverMap},
		},
	}

	sec := "none"
	if dto.Security != nil && dto.Security.Type != "" {
		sec = dto.Security.Type
	}

	ss := map[string]any{
		"network":  "tcp",
		"security": sec,
	}

	if sec == "tls" {
		tlsSettings := map[string]any{}
		if dto.Security != nil {
			if dto.Security.SNI != "" {
				tlsSettings["serverName"] = dto.Security.SNI
			}
			if dto.Security.Fingerprint != "" {
				tlsSettings["fingerprint"] = dto.Security.Fingerprint
			}
			if dto.Security.PinnedPeerCertSha256 != "" {
				tlsSettings["pinnedPeerCertSha256"] = dto.Security.PinnedPeerCertSha256
			}
			if dto.Security.DisableSystemRoot {
				tlsSettings["disableSystemRoot"] = true
			}
			if dto.Security.VerifyPeerCertByName != "" {
				tlsSettings["verifyPeerCertByName"] = dto.Security.VerifyPeerCertByName
			}
			if dto.Security.CACertFile != "" || dto.Security.CACert != "" {
				var certEntry map[string]any
				if dto.Security.CACertFile != "" {
					certEntry = map[string]any{"certificateFile": dto.Security.CACertFile, "usage": "verify"}
				} else {
					certEntry = map[string]any{"certificate": strings.Split(dto.Security.CACert, "\n"), "usage": "verify"}
				}
				tlsSettings["certificates"] = []any{certEntry}
			}
		}
		ss["tlsSettings"] = tlsSettings
	}

	item["streamSettings"] = ss
	return item, nil
}
