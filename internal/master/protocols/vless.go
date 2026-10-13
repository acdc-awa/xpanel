package protocols

import (
	"fmt"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/contracts"
)

// VLESSPlugin VLESS 协议插件（tcp/xhttp + reality/tls/none）。
// 收拢历史上散在 xray.buildClients、services.protoUsersFor、subscribe.BuildProxyItem、
// xray.buildRefOutbound 诸处的 VLESS 知识，生成侧与订阅侧严格同源。
type VLESSPlugin struct{}

func (VLESSPlugin) Protocol() string { return "vless" }

// Descriptor 声明 VLESS 的能力描述符。
func (VLESSPlugin) Descriptor() contracts.ProtocolDescriptor {
	return contracts.ProtocolDescriptor{
		Name:         "vless",
		Label:        "VLESS",
		Networks:     []string{"tcp", "xhttp"},
		Securities:   []string{"reality", "tls", "none"},
		Credential:   contracts.CredentialUUID,
		SupportsUDP:  true,
		HotSync:      true,
		AccountType:  "vless",
		InboundTypes: []string{"user"},
		Segments: []contracts.LayerSegment{
			{Layer: "settings", Key: "decryption", Role: contracts.RoleServerOnly},
			{Layer: "settings", Key: "encryption", Role: contracts.RoleClientOnly},
			{Layer: "settings", Key: "fallbacks", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "tcpSettings", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "xhttpSettings", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "tlsSettings", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "realitySettings", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "sockopt", Role: contracts.RoleBoth},
		},
	}
}

// ServerUsers 生成服务端入站的用户数组，返回挂载键 clients 及用户元素。
func (VLESSPlugin) ServerUsers(users []protocol.User, _ *contracts.InboundSpec) (string, []any, error) {
	clients := make([]any, 0, len(users))
	for _, u := range users {
		if u.UUID == "" {
			continue
		}
		c := map[string]any{
			"id":    u.UUID,
			"email": u.Email,
		}
		if u.Flow != "" {
			c["flow"] = u.Flow
		}
		if u.Limit > 0 {
			c["limit"] = u.Limit
		}
		clients = append(clients, c)
	}
	return "clients", clients, nil
}

// ServerClients 兼容保留：旧版服务端 clients 注入，内部调用 ServerUsers。
func (p VLESSPlugin) ServerClients(users []protocol.User, spec *contracts.InboundSpec) []any {
	_, items, _ := p.ServerUsers(users, spec)
	return items
}

// ResolveFlow 用户 flow 决议（生成侧盖戳与订阅侧同源，入参为入站自身的传输/安全）。
func (VLESSPlugin) ResolveFlow(spec *contracts.InboundSpec, inboundFlow string) string {
	return resolveVLESSFlow(spec.Network, spec.Security, inboundFlow)
}

// resolveVLESSFlow flow 决议规则：
// 显式 none → 空（禁用自动注入，订阅与服务端保持一致否则握手不匹配）；
// 空 + tcp+reality → 自动 xtls-rprx-vision；其余原样透传。
// security 允许传入分享覆写后的值（订阅侧），服务端注入传入 streamSettings 原值。
func resolveVLESSFlow(network, security, inboundFlow string) string {
	if inboundFlow == "none" {
		return ""
	}
	if inboundFlow == "" && network == "tcp" && security == "reality" {
		return "xtls-rprx-vision"
	}
	return inboundFlow
}

// ValidateInbound 协议级校验（JSON 可解析性与 REALITY 密钥预检由上层/xray 校验承担）。
func (VLESSPlugin) ValidateInbound(_ *contracts.InboundSpec) error { return nil }

// ProjectClient 统一客户端端点投影（按 role 区分 subscription / peer）。
func (p VLESSPlugin) ProjectClient(in *contracts.ClientProjectionInput) (*contracts.ProxyNodeDTO, error) {
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

// projectSubscription 用户订阅端点投影：消费分享覆写（ShareSecurity/SNI/Host/Path/AllowInsecure）。
// reality 入站缺 SNI/公钥时返回 nil（与历史上 FormatProxiesYAML 跳过坏节点同源）。
func (p VLESSPlugin) projectSubscription(in *contracts.ClientProjectionInput) *contracts.ProxyNodeDTO {
	spec := in.Spec
	if spec == nil {
		return nil
	}

	// 安全层覆写（ShareSecurity: "" 跟随服务端 / tls / none）
	sec := spec.Security
	switch in.Share.Security {
	case "tls":
		sec = "tls"
	case "none":
		sec = "none"
	}

	dto := &contracts.ProxyNodeDTO{
		ID:         spec.ID,
		Name:       in.Name,
		ServerHost: in.Host,
		ServerPort: in.Port,
		Protocol:   "vless",
		Transport:  &contracts.TransportOptions{Network: spec.Network},
		Security:   &contracts.SecurityOptions{Type: sec},
		Auth:       &contracts.ClientCredentialDTO{UUID: in.UserUUID},
	}
	// flow 与服务端注入同源（订阅侧用覆写后的安全类型判定 reality）
	dto.Auth.Flow = resolveVLESSFlow(spec.Network, sec, in.InboundFlow)

	// 指纹为客户端角色字段（streamSettings.fingerprint），tls/reality 均消费，空兜底 chrome
	fp := spec.Fingerprint
	if fp == "" {
		fp = "chrome"
	}

	switch sec {
	case "tls":
		sni, allowInsecure := "", false
		if spec.TLS != nil {
			sni, allowInsecure = spec.TLS.ServerName, spec.TLS.AllowInsecure
		}
		if in.Share.SNI != "" {
			sni = in.Share.SNI
		}
		if in.Share.AllowInsecure {
			allowInsecure = true
		}
		dto.Security.SNI = sni
		dto.Security.AllowInsecure = allowInsecure
		dto.Security.Fingerprint = fp
	case "reality":
		r := spec.Reality
		if r == nil || r.ServerName == "" {
			return nil
		}
		dto.Security.SNI = r.ServerName
		dto.Security.Fingerprint = fp
		dto.Security.Reality = &contracts.RealityOptions{
			PublicKey: r.PublicKey,
			ShortID:   r.ShortID,
		}
	}

	// XHTTP 传输参数与覆写（mode 为空且有 path/host 覆写时按 auto 兜底）
	if spec.Network == "xhttp" {
		mode, path, host := "", "", ""
		if spec.XHTTP != nil {
			mode, path, host = spec.XHTTP.Mode, spec.XHTTP.Path, spec.XHTTP.Host
		}
		if in.Share.Path != "" {
			path = in.Share.Path
		}
		if in.Share.Host != "" {
			host = in.Share.Host
		}
		if mode == "" && (in.Share.Path != "" || in.Share.Host != "") {
			mode = "auto"
			if path == "" {
				path = "/"
			}
		}
		dto.Transport.Mode = mode
		dto.Transport.Path = path
		dto.Transport.Host = host
		if spec.XHTTP != nil && len(spec.XHTTP.Extra) > 0 {
			dto.Transport.Opts = make(map[string]string, len(spec.XHTTP.Extra))
			for k, v := range spec.XHTTP.Extra {
				dto.Transport.Opts[k] = v
			}
		}
	}

	return dto
}

// projectPeer 节点间互联投影：消费真实监听与内部凭证（RefTarget），忽略门面覆写。
func (p VLESSPlugin) projectPeer(in *contracts.ClientProjectionInput) (*contracts.ProxyNodeDTO, error) {
	if in.RefTarget == nil {
		return nil, fmt.Errorf("节点互联投影必须包含 RefTarget")
	}
	target := in.RefTarget
	inb := target.Inbound
	spec := contracts.DecodeInbound(&inb)
	net := spec.Network
	sec := spec.Security

	encryption := "none"
	dec, err := VlessDecryptionFromSettings(inb.SettingsJSON)
	if err != nil {
		return nil, fmt.Errorf("目标入站 %s settings 解析失败: %w", inb.Tag, err)
	}
	if VlessEncEnabled(dec) {
		if encryption, err = VlessEncClientFromDecryption(dec); err != nil {
			return nil, fmt.Errorf("目标入站 %s VLESS Encryption 派生失败: %w", inb.Tag, err)
		}
	}

	flow := ""
	if net == "tcp" && (sec == "reality" || sec == "tls" || VlessEncEnabled(dec)) {
		flow = "xtls-rprx-vision"
	}

	peerUUID := in.OutboundUUID
	if peerUUID == "" {
		peerUUID = inb.InternalUUID
	}

	dto := &contracts.ProxyNodeDTO{
		ID:         inb.ID,
		Name:       inb.Tag,
		ServerHost: target.ServerHost,
		ServerPort: inb.Port,
		Protocol:   "vless",
		Transport:  &contracts.TransportOptions{Network: net},
		Security:   &contracts.SecurityOptions{Type: sec},
		Auth: &contracts.ClientCredentialDTO{
			UUID:       peerUUID,
			Flow:       flow,
			Encryption: encryption,
			Extra:      make(map[string]any),
		},
	}
	if target.CertPin != "" {
		dto.Auth.Extra["CertPin"] = target.CertPin
	}

	// 传输层参数
	if net == "xhttp" && spec.XHTTP != nil {
		dto.Transport.Mode = spec.XHTTP.Mode
		dto.Transport.Path = spec.XHTTP.Path
		dto.Transport.Host = spec.XHTTP.Host
	}

	// 安全层参数
	fp := spec.Fingerprint
	if fp == "" {
		fp = "chrome"
	}
	dto.Security.Fingerprint = fp

	switch sec {
	case "reality":
		if r := spec.Reality; r != nil && r.ServerName != "" {
			dto.Security.SNI = r.ServerName
			dto.Security.Reality = &contracts.RealityOptions{
				PublicKey: r.PublicKey,
				ShortID:   r.ShortID,
			}
		}
	case "tls":
		serverName := target.ServerHost
		if t := spec.TLS; t != nil && t.ServerName != "" {
			serverName = t.ServerName
		}
		dto.Security.SNI = serverName
	}

	return dto, nil
}

// BuildClientNode 兼容保留：组装订阅导出用 ProxyNodeDTO，内部桥接调用 ProjectClient。
func (p VLESSPlugin) BuildClientNode(in *contracts.ClientNodeInput) *contracts.ProxyNodeDTO {
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
