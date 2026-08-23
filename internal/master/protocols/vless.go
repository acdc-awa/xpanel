package protocols

import (
	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel-node/pkg/protocol"
)

// VLESSPlugin VLESS 协议插件（tcp/xhttp + reality/tls/none）。
// 收拢历史上散在 xray.buildClients、services.protoUsersFor、subscribe.BuildProxyItem
// 三处的 VLESS 知识，生成侧与订阅侧严格同源。
type VLESSPlugin struct{}

func (VLESSPlugin) Protocol() string { return "vless" }

// ServerClients 生成 xray VLESS 入站 clients（id/email + 可选 flow/limit）。
func (VLESSPlugin) ServerClients(users []protocol.User, _ *contracts.InboundSpec) []any {
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
	return clients
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

// ValidateInbound 协议级校验（JSON 可解析性与 REALITY 密钥预检由 xray.ValidateInbound 承担）。
func (VLESSPlugin) ValidateInbound(_ *contracts.InboundSpec) error { return nil }

// BuildClientNode 组装 VLESS 订阅节点 DTO，消费分享覆写（ShareSecurity/SNI/Host/Path/AllowInsecure）。
// reality 入站缺 SNI/公钥时返回 nil（与历史上 FormatProxiesYAML 跳过坏节点同源）。
func (VLESSPlugin) BuildClientNode(in *contracts.ClientNodeInput) *contracts.ProxyNodeDTO {
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
	case "reality":
		r := spec.Reality
		if r == nil || r.ServerName == "" {
			return nil
		}
		dto.Security.SNI = r.ServerName
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
