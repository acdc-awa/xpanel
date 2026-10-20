package protocols

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

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
		Name:          "vless",
		Label:         "VLESS",
		Networks:      []string{"tcp", "xhttp"},
		Securities:    []string{"reality", "tls", "none"},
		Credential:    contracts.CredentialUUID,
		SupportsUDP:   true,
		HotSync:       true,
		AccountType:   "vless",
		InboundTypes:  []string{"user"},
		ClientSchemes: []string{"vless"},
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
		dto.Security.Reality = contracts.NewRealityOptions(r.PublicKey, r.ShortID)
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
			dto.Security.Reality = contracts.NewRealityOptions(r.PublicKey, r.ShortID)
		}
	case "tls":
		serverName := target.ServerHost
		if t := spec.TLS; t != nil && t.ServerName != "" {
			serverName = t.ServerName
		}
		ApplyPeerTLSValidation(dto, in, serverName)
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

// FormatLink 格式化为标准 vless:// 订阅链接。
func (VLESSPlugin) FormatLink(dto *contracts.ProxyNodeDTO) (string, error) {
	if dto == nil || dto.Auth == nil || dto.Auth.UUID == "" {
		return "", fmt.Errorf("vless DTO 缺少 UUID")
	}
	q := url.Values{}
	enc := "none"
	if dto.Auth.Encryption != "" {
		enc = dto.Auth.Encryption
	}
	q.Set("encryption", enc)

	sec := "none"
	if dto.Security != nil && dto.Security.Type != "" {
		sec = dto.Security.Type
	}
	q.Set("security", sec)
	switch sec {
	case "reality":
		if dto.Security.Reality == nil || dto.Security.Reality.PublicKey == "" {
			return "", fmt.Errorf("reality 缺少公钥")
		}
		q.Set("pbk", dto.Security.Reality.PublicKey)
		if dto.Security.Reality.ShortID != "" {
			q.Set("sid", dto.Security.Reality.ShortID)
		}
		if dto.Security.SNI != "" {
			q.Set("sni", dto.Security.SNI)
		}
		fp := dto.Security.Fingerprint
		if fp == "" {
			fp = "chrome"
		}
		q.Set("fp", fp)
		// 方言翻译：DTO 的 SupportMLKEM 在链接里写成 query 参数（mihomo 的转换器识别它，
		// 见 common/convert/v.go → reality-opts.support-x25519mlkem768）。
		if dto.Security.Reality.SupportMLKEM {
			q.Set("support-x25519mlkem768", "1")
		}
	case "tls":
		if dto.Security.SNI != "" {
			q.Set("sni", dto.Security.SNI)
		}
		if dto.Security.Fingerprint != "" {
			q.Set("fp", dto.Security.Fingerprint)
		}
		if dto.Security.AllowInsecure {
			q.Set("allowInsecure", "1")
		}
	}

	if dto.Auth.Flow != "" {
		q.Set("flow", dto.Auth.Flow)
	}

	net := "tcp"
	if dto.Transport != nil && dto.Transport.Network != "" {
		net = dto.Transport.Network
	}
	q.Set("type", net)
	switch net {
	case "xhttp":
		if dto.Transport.Path != "" {
			q.Set("path", dto.Transport.Path)
		}
		if dto.Transport.Host != "" {
			q.Set("host", dto.Transport.Host)
		}
		if dto.Transport.Mode != "" {
			q.Set("mode", dto.Transport.Mode)
		}
	case "ws":
		if dto.Transport.Path != "" {
			q.Set("path", dto.Transport.Path)
		}
		if dto.Transport.Host != "" {
			q.Set("host", dto.Transport.Host)
		}
	case "grpc":
		if dto.Transport.Opts != nil && dto.Transport.Opts["serviceName"] != "" {
			q.Set("serviceName", dto.Transport.Opts["serviceName"])
		}
	}

	frag := url.QueryEscape(dto.Name)
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", dto.Auth.UUID, dto.ServerHost, dto.ServerPort, q.Encode(), frag), nil
}

// ParseLink 解析 vless:// 订阅链接为中性 ProxyNodeDTO。
func (VLESSPlugin) ParseLink(u *url.URL) (*contracts.ProxyNodeDTO, error) {
	if u == nil {
		return nil, fmt.Errorf("url 不能为空")
	}
	if strings.ToLower(u.Scheme) != "vless" {
		return nil, fmt.Errorf("协议 scheme 不匹配: %s, 期望 vless", u.Scheme)
	}

	uuid := u.User.Username()
	if uuid == "" {
		return nil, fmt.Errorf("vless 链接缺少 UUID")
	}
	host := u.Hostname()
	portStr := u.Port()
	if host == "" || portStr == "" {
		return nil, fmt.Errorf("vless 链接缺少主机或端口: %s", u.Host)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("非法端口: %s", portStr)
	}

	q := u.Query()
	net := q.Get("type")
	if net == "" {
		net = q.Get("network")
	}
	if net == "" {
		net = "tcp"
	}
	net = strings.ToLower(net)

	sec := strings.ToLower(q.Get("security"))
	if sec == "" {
		sec = "none"
	}

	name, _ := url.QueryUnescape(u.Fragment)
	if name == "" {
		name = fmt.Sprintf("vless-%s-%d", host, port)
	}

	dto := &contracts.ProxyNodeDTO{
		Name:       name,
		ServerHost: host,
		ServerPort: port,
		Protocol:   "vless",
		Transport:  &contracts.TransportOptions{Network: net},
		Security:   &contracts.SecurityOptions{Type: sec},
		Auth: &contracts.ClientCredentialDTO{
			UUID:       uuid,
			Flow:       q.Get("flow"),
			Encryption: "none",
			Extra:      make(map[string]any),
		},
	}

	// 提取 SNI 与 Fingerprint
	sni := q.Get("sni")
	if sni == "" {
		sni = q.Get("serverName")
	}
	dto.Security.SNI = sni

	fp := q.Get("fp")
	if fp == "" {
		fp = q.Get("fingerprint")
	}
	dto.Security.Fingerprint = fp

	switch sec {
	case "reality":
		pbk := q.Get("pbk")
		if pbk == "" {
			pbk = q.Get("publicKey")
		}
		sid := q.Get("sid")
		if sid == "" {
			sid = q.Get("shortId")
		}
		dto.Security.Reality = contracts.NewRealityOptions(pbk, sid)
		// 链接未声明时按面板默认（开启）处理：旧内核不敏感，开启无副作用。
		if v := q.Get("support-x25519mlkem768"); v != "" {
			if enabled, err := strconv.ParseBool(v); err == nil {
				dto.Security.Reality.SupportMLKEM = enabled
			}
		}
	case "tls":
		if q.Get("allowInsecure") == "1" || q.Get("insecure") == "1" {
			dto.Security.AllowInsecure = true
		}
	}

	// 传输层参数
	switch net {
	case "xhttp":
		dto.Transport.Path = q.Get("path")
		dto.Transport.Host = q.Get("host")
		dto.Transport.Mode = q.Get("mode")
		if dto.Transport.Mode == "" {
			dto.Transport.Mode = "auto"
		}
		if dto.Transport.Path == "" {
			dto.Transport.Path = "/"
		}
	case "ws":
		dto.Transport.Path = q.Get("path")
		dto.Transport.Host = q.Get("host")
		if dto.Transport.Path == "" {
			dto.Transport.Path = "/"
		}
	case "grpc":
		svc := q.Get("serviceName")
		if svc != "" {
			dto.Transport.Opts = map[string]string{"serviceName": svc}
		}
	}

	return dto, nil
}

// RenderOutbound 将中性 ProxyNodeDTO 渲染为 Xray 客户端出站结构 map。
func (VLESSPlugin) RenderOutbound(dto *contracts.ProxyNodeDTO) (map[string]any, error) {
	if dto == nil || dto.Auth == nil {
		return nil, fmt.Errorf("DTO 不能为空")
	}
	encryption := dto.Auth.Encryption
	if encryption == "" {
		encryption = "none"
	}
	user := map[string]any{
		"id":         dto.Auth.UUID,
		"encryption": encryption,
	}
	if dto.Auth.Flow != "" {
		user["flow"] = dto.Auth.Flow
	}
	item := map[string]any{
		"protocol": "vless",
		"settings": map[string]any{
			"vnext": []any{map[string]any{
				"address": dto.ServerHost,
				"port":    dto.ServerPort,
				"users":   []any{user},
			}},
		},
	}

	net := "tcp"
	if dto.Transport != nil && dto.Transport.Network != "" {
		net = dto.Transport.Network
	}
	sec := "none"
	if dto.Security != nil && dto.Security.Type != "" {
		sec = dto.Security.Type
	}
	ss := map[string]any{
		"network":  net,
		"security": sec,
	}

	switch net {
	case "xhttp":
		mode := "auto"
		path := "/"
		if dto.Transport != nil {
			if dto.Transport.Mode != "" {
				mode = dto.Transport.Mode
			}
			if dto.Transport.Path != "" {
				path = dto.Transport.Path
			}
		}
		xh := map[string]any{"mode": mode, "path": path}
		if dto.Transport != nil && dto.Transport.Host != "" {
			xh["host"] = dto.Transport.Host
		}
		ss["xhttpSettings"] = xh
	case "ws":
		path := "/"
		if dto.Transport != nil && dto.Transport.Path != "" {
			path = dto.Transport.Path
		}
		ws := map[string]any{"path": path}
		if dto.Transport != nil && dto.Transport.Host != "" {
			ws["host"] = dto.Transport.Host
		}
		ss["wsSettings"] = ws
	case "grpc":
		svc := "grpc"
		if dto.Transport != nil && dto.Transport.Opts != nil && dto.Transport.Opts["serviceName"] != "" {
			svc = dto.Transport.Opts["serviceName"]
		}
		ss["grpcSettings"] = map[string]any{"serviceName": svc}
	}

	switch sec {
	case "reality":
		if dto.Security != nil && dto.Security.Reality != nil {
			fp := dto.Security.Fingerprint
			if fp == "" {
				fp = "chrome"
			}
			ss["realitySettings"] = map[string]any{
				"serverName":  dto.Security.SNI,
				"password":    dto.Security.Reality.PublicKey,
				"shortId":     dto.Security.Reality.ShortID,
				"fingerprint": fp,
				"spiderX":     "/",
			}
		}
	case "tls":
		if dto.Security != nil {
			tlsSettings := map[string]any{"serverName": dto.Security.SNI}
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
			ss["tlsSettings"] = tlsSettings
		}
	}

	item["streamSettings"] = ss
	return item, nil
}
