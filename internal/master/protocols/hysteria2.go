package protocols

import (
	"fmt"

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
		Segments: []contracts.LayerSegment{
			{Layer: "settings", Key: "version", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "hysteriaSettings", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "tlsSettings", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "finalmask", Role: contracts.RoleBoth},
			{Layer: "stream", Key: "sockopt", Role: contracts.RoleBoth},
		},
	}
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

	fp := spec.Fingerprint
	if fp == "" {
		fp = "chrome"
	}
	dto.Security.Fingerprint = fp

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
	if target.CertPin != "" {
		dto.Auth.Extra["CertPin"] = target.CertPin
	}
	if spec.TLS != nil && spec.TLS.ServerName != "" {
		dto.Security.SNI = spec.TLS.ServerName
	}
	if dto.Security.SNI == "" {
		dto.Security.SNI = target.ServerHost
	}

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
