package protocols

import (
	"encoding/json"
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

// NormalizeServer 服务端规范化：
//  1. xray 拒绝入站级 encryption（运行时恒为 none），库内存量写入也必须剥掉；
//  2. decryption 是 vlessenc 安全层的服务端载体：合法则原样保留，否则补 "none"。
func (VLESSPlugin) NormalizeServer(spec *contracts.InboundSpec) (*contracts.ServerOverrides, error) {
	settings := make(map[string]any, len(spec.Settings)+1)
	for k, v := range spec.Settings {
		settings[k] = v
	}
	delete(settings, "encryption")
	dec, _ := settings["decryption"].(string)
	if VlessEncEnabled(dec) {
		if err := ValidateVlessEncDecryption(dec); err != nil {
			return nil, fmt.Errorf("VLESS Encryption decryption 非法: %w", err)
		}
	} else {
		settings["decryption"] = "none"
	}
	return &contracts.ServerOverrides{Settings: settings}, nil
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

// ResolveRelayFlow 机间中继凭据的 flow：在 ResolveFlow 之上，tcp 且
// tls / vlessenc 同样自动 vision（中继链路必须带流控，普通用户不自动注入）。
func (VLESSPlugin) ResolveRelayFlow(spec *contracts.InboundSpec, inboundFlow string) string {
	flow := resolveVLESSFlow(spec.Network, spec.Security, inboundFlow)
	dec, _ := spec.Settings["decryption"].(string)
	if flow == "" && spec.Network == "tcp" &&
		(spec.Security == "reality" || spec.Security == "tls" || VlessEncEnabled(dec)) {
		flow = "xtls-rprx-vision"
	}
	return flow
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

// projectBase 协议语义投影（P2 订阅 / P4 互联共用）：传输层 + 安全层。
// 只依赖入站规范模型与「生效的安全类型」，不关心角色、地址、凭证来源；
// 角色差异（名字/地址/端口、凭证、抹平下发）由调用方在此之上覆写。
func (p VLESSPlugin) projectBase(spec *contracts.InboundSpec, sec string) *contracts.ProxyNodeDTO {
	dto := &contracts.ProxyNodeDTO{
		ID:        spec.ID,
		Protocol:  "vless",
		Transport: &contracts.TransportOptions{Network: spec.Network},
		Security:  &contracts.SecurityOptions{Type: sec},
	}
	// 指纹为客户端角色字段（streamSettings.fingerprint），仅 TLS 类安全层消费，空兜底 chrome。
	fp := spec.Fingerprint
	if fp == "" {
		fp = "chrome"
	}
	switch sec {
	case "tls":
		if spec.TLS != nil {
			dto.Security.SNI = spec.TLS.ServerName
			dto.Security.AllowInsecure = spec.TLS.AllowInsecure
		}
		dto.Security.Fingerprint = fp
	case "reality":
		if r := spec.Reality; r != nil {
			dto.Security.SNI = r.ServerName
			dto.Security.Reality = contracts.NewRealityOptions(r.PublicKey, r.ShortID)
		}
		dto.Security.Fingerprint = fp
	}

	if spec.Network == "xhttp" {
		if spec.XHTTP != nil {
			dto.Transport.Mode = spec.XHTTP.Mode
			dto.Transport.Path = spec.XHTTP.Path
			dto.Transport.Host = spec.XHTTP.Host
			if len(spec.XHTTP.Extra) > 0 {
				dto.Transport.Opts = make(map[string]string, len(spec.XHTTP.Extra))
				for k, v := range spec.XHTTP.Extra {
					dto.Transport.Opts[k] = v
				}
			}
		}
	}
	return dto
}

func (p VLESSPlugin) projectSubscription(in *contracts.ClientProjectionInput) *contracts.ProxyNodeDTO {
	spec := in.Spec
	if spec == nil {
		return nil
	}

	// 对外安全层：接入层决议的值优先；不挂层时就是入站自身的传输安全层。
	sec := spec.Security
	if in.EntrySecurity != "" {
		sec = in.EntrySecurity
	}

	dto := p.projectBase(spec, sec)
	dto.Name = in.Name
	dto.ServerHost = in.Host
	dto.ServerPort = in.Port
	// vlessenc：服务端 settings.decryption 派生客户端 encryption；派生失败说明密钥非法，
	// 与其下发一个必然不通的节点，不如按参数不足跳过（调用方会记日志）。
	encryption := "none"
	if dec, _ := spec.Settings["decryption"].(string); VlessEncEnabled(dec) {
		enc, err := VlessEncClientFromDecryption(dec)
		if err != nil {
			return nil
		}
		encryption = enc
	}
	dto.Auth = &contracts.ClientCredentialDTO{UUID: in.UserUUID, Encryption: encryption}
	// flow 与服务端注入同源（订阅侧用覆写后的安全类型判定 reality）
	dto.Auth.Flow = resolveVLESSFlow(spec.Network, sec, in.InboundFlow)

	switch sec {
	case "tls":
		if in.Share.SNI != "" {
			dto.Security.SNI = in.Share.SNI
		}
		if in.Share.AllowInsecure {
			dto.Security.AllowInsecure = true
		}
	case "reality":
		// reality 入站缺 SNI/公钥时返回 nil（与历史上 FormatProxiesYAML 跳过坏节点同源）
		if r := spec.Reality; r == nil || r.ServerName == "" {
			return nil
		}
	}

	// xhttp 的 mode / path / host 全部来自入站 streamSettings（与反代同源）：
	// 不再提供客户端侧覆写，避免出现「反代按 A 路径、xray 只认 B 路径」这类必坏组合。
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

	dto := p.projectBase(spec, spec.Security)
	dto.Name = inb.Tag
	dto.ServerHost = target.ServerHost
	dto.ServerPort = inb.Port

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
	if spec.Network == "tcp" && (spec.Security == "reality" || spec.Security == "tls" || VlessEncEnabled(dec)) {
		flow = "xtls-rprx-vision"
	}

	peerUUID := in.OutboundUUID
	if peerUUID == "" {
		peerUUID = inb.InternalUUID
	}

	dto.Auth = &contracts.ClientCredentialDTO{
		UUID:       peerUUID,
		Flow:       flow,
		Encryption: encryption,
		Extra:      make(map[string]any),
	}
	if target.CertPin != "" {
		dto.Auth.Extra["CertPin"] = target.CertPin
	}

	switch spec.Security {
	case "reality":
		// 目标 reality 缺 serverName/公钥时历史上不写 realitySettings（保持原状，避免掩盖坏目标）
		if r := spec.Reality; r == nil || r.ServerName == "" {
			dto.Security.Reality = nil
			dto.Security.SNI = ""
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
		Role:          contracts.RoleSubscription,
		Name:          in.Name,
		Host:          in.Host,
		Port:          in.Port,
		Spec:          in.Spec,
		Share:         in.Share,
		EntrySecurity: in.EntrySecurity,
		InboundFlow:   in.InboundFlow,
		UserUUID:      in.UserUUID,
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
		// 进阶参数：链接通道只能通过 extra=<xray SplitHTTPConfig JSON> 承载
		// （mihomo 的转换器解析它并翻译成 xhttp-opts；YAML 导出走另一条路径）。
		if extra := xhttpExtraJSON(dto.Transport); extra != "" {
			q.Set("extra", extra)
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
	if enc := q.Get("encryption"); enc != "" {
		dto.Auth.Encryption = enc
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

// xhttpExtraJSON 把 DTO 里的 xhttp 进阶参数还原成 xray SplitHTTPConfig 形态的 JSON，
// 供订阅链接的 extra= 参数使用（mihomo 的链接转换器会读它并翻译成 xhttp-opts）。
// host/path/mode 由 query 单独承载，不重复写入；数值型字段还原为数字。
func xhttpExtraJSON(t *contracts.TransportOptions) string {
	if t == nil || len(t.Opts) == 0 {
		return ""
	}
	merged := make(map[string]any, len(t.Opts))
	if raw := t.Opts["extra"]; raw != "" {
		var m map[string]any
		if json.Unmarshal([]byte(raw), &m) == nil {
			for k, v := range m {
				merged[k] = v
			}
		}
	}
	for k, raw := range t.Opts {
		if k == "extra" {
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err == nil {
			merged[k] = v
		} else {
			merged[k] = raw
		}
	}
	delete(merged, "host")
	delete(merged, "path")
	delete(merged, "mode")
	if len(merged) == 0 {
		return ""
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return ""
	}
	return string(b)
}
