package protocols

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/contracts"
)

// HysteriaPlugin Hysteria 2 协议插件（强制 network: hysteria, security: tls）。
// 契约：
// 1. 服务端：用户认证全部由 settings.users[]（每用户独立 auth + email 计费锚点）承担，不使用共享密码；
// 2. 客户端/节点互联：密码位于 streamSettings.hysteriaSettings.auth；
// 3. 订阅层：导出为标准 hysteria2 (Clash) 与 hysteria2:// 链接。
type HysteriaPlugin struct{}

func (HysteriaPlugin) Protocol() string { return "hysteria" }

// Descriptor 声明 Hysteria 2 的能力描述符。
func (HysteriaPlugin) Descriptor() contracts.ProtocolDescriptor {
	return contracts.ProtocolDescriptor{
		Name:          "hysteria",
		Label:         "Hysteria 2",
		Networks:      []string{"hysteria"},
		Securities:    []string{"tls"},
		ForceNetwork:  "hysteria",
		ForceSecurity: "tls",
		Credential:    contracts.CredentialPassword,
		SupportsUDP:   true,
		HotSync:       true,
		AccountType:   "hysteria",
		InboundTypes:  []string{"user"},
		ClientSchemes: []string{"hysteria2", "hy2"},
		Segments: []contracts.LayerSegment{
			{Layer: "settings", Key: "version", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "hysteriaSettings", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "tlsSettings", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "finalmask", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "sockopt", Role: contracts.RoleBoth},
		},
	}
}

// NormalizeServer 服务端规范化：Hysteria 2 强制 settings.version=2、
// network=hysteria、security=tls，并补齐 hysteriaSettings 默认值、剥离共享密码 auth。
func (HysteriaPlugin) NormalizeServer(spec *contracts.InboundSpec) (*contracts.ServerOverrides, error) {
	settings := make(map[string]any, len(spec.Settings)+1)
	for k, v := range spec.Settings {
		settings[k] = v
	}
	settings["version"] = 2

	hy := map[string]any{"version": 2, "udpIdleTimeout": 60}
	if existing, ok := spec.Stream["hysteriaSettings"].(map[string]any); ok {
		hy = make(map[string]any, len(existing)+1)
		for k, v := range existing {
			hy[k] = v
		}
		hy["version"] = 2
		delete(hy, "auth")
	}
	return &contracts.ServerOverrides{
		Settings: settings,
		Stream: map[string]any{
			"network":          "hysteria",
			"security":         "tls",
			"hysteriaSettings": hy,
		},
	}, nil
}

// ServerUsers 生成服务端入站用户数组，返回挂载键 users 及用户元素。
func (HysteriaPlugin) ServerUsers(users []protocol.User, _ *contracts.InboundSpec) (string, []any, error) {
	items := make([]any, 0, len(users))
	for _, u := range users {
		if u.UUID == "" {
			continue
		}
		items = append(items, map[string]any{
			"auth":  u.UUID,
			"email": u.Email,
			"level": u.Level,
		})
	}
	return "users", items, nil
}

// ServerClients 兼容保留：旧版服务端 clients 注入，内部调用 ServerUsers。
func (p HysteriaPlugin) ServerClients(users []protocol.User, spec *contracts.InboundSpec) []any {
	_, items, _ := p.ServerUsers(users, spec)
	return items
}

// ResolveFlow Hysteria 2 无 flow，固定返回空。
func (HysteriaPlugin) ResolveFlow(_ *contracts.InboundSpec, _ string) string {
	return ""
}

// ResolveRelayFlow Hysteria 2 无 flow，中继凭据同样固定返回空。
func (HysteriaPlugin) ResolveRelayFlow(_ *contracts.InboundSpec, _ string) string {
	return ""
}

// ValidateInbound 入站保存时的协议级校验。
func (HysteriaPlugin) ValidateInbound(spec *contracts.InboundSpec) error {
	if spec == nil {
		return nil
	}
	if spec.Network != "" && spec.Network != "hysteria" {
		return fmt.Errorf("Hysteria 2 传输协议只能为 hysteria，got: %s", spec.Network)
	}
	if spec.Security != "" && spec.Security != "tls" {
		return fmt.Errorf("Hysteria 2 安全协议只能为 tls，got: %s", spec.Security)
	}
	return nil
}

// ProjectClient 统一客户端端点投影（按 role 区分 subscription / peer）。
func (p HysteriaPlugin) ProjectClient(in *contracts.ClientProjectionInput) (*contracts.ProxyNodeDTO, error) {
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

// projectSubscription 用户订阅端点投影：生成中性 ProxyNodeDTO。
func (p HysteriaPlugin) projectSubscription(in *contracts.ClientProjectionInput) *contracts.ProxyNodeDTO {
	spec := in.Spec
	if spec == nil {
		return nil
	}

	dto := &contracts.ProxyNodeDTO{
		ID:         spec.ID,
		Name:       in.Name,
		ServerHost: in.Host,
		ServerPort: in.Port,
		Protocol:   "hysteria",
		Transport:  &contracts.TransportOptions{Network: "hysteria"},
		Security:   &contracts.SecurityOptions{Type: "tls"},
		Auth: &contracts.ClientCredentialDTO{
			Password: in.UserUUID,
			UUID:     in.UserUUID,
		},
	}

	sni := in.Host
	if spec.TLS != nil && spec.TLS.ServerName != "" {
		sni = spec.TLS.ServerName
	}
	if in.Share.SNI != "" {
		sni = in.Share.SNI
	}
	dto.Security.SNI = sni
	dto.Security.AllowInsecure = in.Share.AllowInsecure
	if spec.TLS != nil && spec.TLS.AllowInsecure {
		dto.Security.AllowInsecure = true
	}

	// Hysteria 2 走 QUIC，不存在 TLS ClientHello 指纹模拟，Fingerprint 对本协议没有意义。
	// 不写入 DTO：mihomo 的 hysteria2.fingerprint 是证书 pin，填浏览器名会让整份配置加载失败。

	return dto
}

// projectPeer 节点间互联投影：消费真实监听与内部凭证（RefTarget），忽略门面覆写。
func (p HysteriaPlugin) projectPeer(in *contracts.ClientProjectionInput) (*contracts.ProxyNodeDTO, error) {
	if in.RefTarget == nil {
		return nil, fmt.Errorf("节点互联投影必须包含 RefTarget")
	}
	target := in.RefTarget
	inb := target.Inbound
	spec := contracts.DecodeInbound(&inb)

	peerAuth := in.OutboundUUID
	if peerAuth == "" {
		peerAuth = inb.InternalUUID
	}

	dto := &contracts.ProxyNodeDTO{
		ID:         inb.ID,
		Name:       inb.Tag,
		ServerHost: target.ServerHost,
		ServerPort: inb.Port,
		Protocol:   "hysteria",
		Transport:  &contracts.TransportOptions{Network: "hysteria"},
		Security:   &contracts.SecurityOptions{Type: "tls"},
		Auth: &contracts.ClientCredentialDTO{
			UUID:     peerAuth,
			Password: peerAuth,
			Extra:    make(map[string]any),
		},
	}
	serverName := target.ServerHost
	if spec.TLS != nil && spec.TLS.ServerName != "" {
		serverName = spec.TLS.ServerName
	}
	ApplyPeerTLSValidation(dto, in, serverName)

	return dto, nil
}

// BuildClientNode 兼容保留：旧版订阅节点组装输入，内部桥接调用 ProjectClient。
func (p HysteriaPlugin) BuildClientNode(in *contracts.ClientNodeInput) *contracts.ProxyNodeDTO {
	if in == nil {
		return nil
	}
	dto, _ := p.ProjectClient(&contracts.ClientProjectionInput{
		Role:        contracts.RoleSubscription,
		Name:        in.Name,
		Host:        in.Host,
		Port:        in.Port,
		Spec:        in.Spec,
		Share:       in.Share,
		InboundFlow: in.InboundFlow,
		UserUUID:    in.UserUUID,
	})
	return dto
}

// FormatLink 格式化为标准 hysteria2:// 订阅链接。
func (HysteriaPlugin) FormatLink(dto *contracts.ProxyNodeDTO) (string, error) {
	if dto == nil || dto.Auth == nil {
		return "", fmt.Errorf("hysteria DTO 不能为空")
	}
	password := dto.Auth.Password
	if password == "" {
		password = dto.Auth.UUID
	}
	if password == "" {
		return "", fmt.Errorf("hysteria DTO 缺少认证密码")
	}
	q := url.Values{}
	if dto.Security != nil {
		if dto.Security.SNI != "" {
			q.Set("sni", dto.Security.SNI)
		}
		if dto.Security.AllowInsecure {
			q.Set("insecure", "1")
		}
	}
	u := url.URL{
		Scheme:   "hysteria2",
		User:     url.User(password),
		Host:     fmt.Sprintf("%s:%d", dto.ServerHost, dto.ServerPort),
		RawQuery: q.Encode(),
		Fragment: dto.Name,
	}
	return u.String(), nil
}

// ParseLink 解析 hysteria2:// 或 hy2:// 订阅链接为中性 ProxyNodeDTO。
func (HysteriaPlugin) ParseLink(u *url.URL) (*contracts.ProxyNodeDTO, error) {
	if u == nil {
		return nil, fmt.Errorf("url 不能为空")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "hysteria2" && scheme != "hy2" {
		return nil, fmt.Errorf("协议 scheme 不匹配: %s, 期望 hysteria2 或 hy2", u.Scheme)
	}

	password := u.User.Username()
	if password == "" {
		if p, ok := u.User.Password(); ok {
			password = p
		}
	}
	if password == "" {
		return nil, fmt.Errorf("hysteria2 链接缺少认证密码")
	}

	host := u.Hostname()
	portStr := u.Port()
	if host == "" || portStr == "" {
		return nil, fmt.Errorf("hysteria2 链接缺少主机或端口: %s", u.Host)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("非法端口: %s", portStr)
	}

	q := u.Query()
	sni := q.Get("sni")
	if sni == "" {
		sni = host
	}
	allowInsecure := q.Get("insecure") == "1"

	name, _ := url.QueryUnescape(u.Fragment)
	if name == "" {
		name = fmt.Sprintf("hy2-%s-%d", host, port)
	}

	dto := &contracts.ProxyNodeDTO{
		Name:       name,
		ServerHost: host,
		ServerPort: port,
		Protocol:   "hysteria",
		Transport:  &contracts.TransportOptions{Network: "hysteria"},
		Security: &contracts.SecurityOptions{
			Type:          "tls",
			SNI:           sni,
			AllowInsecure: allowInsecure,
			Fingerprint:   "chrome",
		},
		Auth: &contracts.ClientCredentialDTO{
			Password: password,
			UUID:     password,
			Extra:    make(map[string]any),
		},
	}
	return dto, nil
}

// RenderOutbound 将中性 ProxyNodeDTO 渲染为 Xray 客户端出站结构 map。
func (HysteriaPlugin) RenderOutbound(dto *contracts.ProxyNodeDTO) (map[string]any, error) {
	if dto == nil || dto.Auth == nil {
		return nil, fmt.Errorf("DTO 不能为空")
	}
	auth := dto.Auth.Password
	if auth == "" {
		auth = dto.Auth.UUID
	}
	item := map[string]any{
		"protocol": "hysteria",
		"settings": map[string]any{
			"version": 2,
			"address": dto.ServerHost,
			"port":    dto.ServerPort,
		},
		"streamSettings": map[string]any{
			"network":  "hysteria",
			"security": "tls",
			"hysteriaSettings": map[string]any{
				"version": 2,
				"auth":    auth,
			},
		},
	}

	tlsSettings := map[string]any{}
	if dto.Security != nil {
		if dto.Security.SNI != "" {
			tlsSettings["serverName"] = dto.Security.SNI
		}
		if dto.Security.Fingerprint != "" {
			tlsSettings["fingerprint"] = dto.Security.Fingerprint
		}
		if dto.Auth.Extra != nil {
			if pin, ok := dto.Auth.Extra["CertPin"].(string); ok && pin != "" {
				tlsSettings["pinnedPeerCertSha256"] = pin
			}
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
	item["streamSettings"].(map[string]any)["tlsSettings"] = tlsSettings
	return item, nil
}
