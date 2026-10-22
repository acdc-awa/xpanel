package protocols

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/contracts"
)

// Hysteria 2 客户端扩展参数在 ProxyNodeDTO.Transport.Opts 里的键名。
// 订阅导出（mihomo YAML / 链接）与机间出站渲染共用一套键，避免两端各写一份字面量。
const (
	Hy2OptObfs          = "obfs"         // salamander | gecko
	Hy2OptObfsPassword  = "obfsPassword" // 混淆密码
	Hy2OptObfsMinPacket = "obfsMinPacketSize"
	Hy2OptObfsMaxPacket = "obfsMaxPacketSize"
	Hy2OptBwUp          = "up"    // xray 原样带宽串（如 "100 mbps"）
	Hy2OptBwDown        = "down"  // 同上
	Hy2OptPorts         = "ports" // 端口跳跃目标端口集合（如 "20000-30000"）
	Hy2OptHopInterval   = "hopInterval"
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

// projectBase 协议语义投影（P2 订阅 / P4 互联共用）：Hysteria 2 固定
// transport=hysteria / security=tls，安全参数只取入站自身配置。
func (p HysteriaPlugin) projectBase(spec *contracts.InboundSpec) *contracts.ProxyNodeDTO {
	dto := &contracts.ProxyNodeDTO{
		ID:        spec.ID,
		Protocol:  "hysteria",
		Transport: &contracts.TransportOptions{Network: "hysteria"},
		Security:  &contracts.SecurityOptions{Type: "tls"},
	}
	if spec.TLS != nil {
		dto.Security.SNI = spec.TLS.ServerName
		dto.Security.AllowInsecure = spec.TLS.AllowInsecure
	}
	hydrateHysteriaOpts(dto, spec)
	// Hysteria 2 走 QUIC，不存在 TLS ClientHello 指纹模拟，Fingerprint 对本协议没有意义。
	// 不写入 DTO：mihomo 的 hysteria2.fingerprint 是证书 pin，填浏览器名会让整份配置加载失败。
	return dto
}

// projectSubscription 用户订阅端点投影：生成中性 ProxyNodeDTO。
func (p HysteriaPlugin) projectSubscription(in *contracts.ClientProjectionInput) *contracts.ProxyNodeDTO {
	spec := in.Spec
	if spec == nil {
		return nil
	}

	dto := p.projectBase(spec)
	dto.Name = in.Name
	dto.ServerHost = in.Host
	dto.ServerPort = in.Port
	dto.Auth = &contracts.ClientCredentialDTO{
		Password: in.UserUUID,
		UUID:     in.UserUUID,
	}

	sni := in.Host
	if spec.TLS != nil && spec.TLS.ServerName != "" {
		sni = spec.TLS.ServerName
	}
	if in.Share.SNI != "" {
		sni = in.Share.SNI
	}
	dto.Security.SNI = sni
	dto.Security.AllowInsecure = in.Share.AllowInsecure || dto.Security.AllowInsecure

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

	dto := p.projectBase(spec)
	dto.Name = inb.Tag
	dto.ServerHost = target.ServerHost
	dto.ServerPort = inb.Port
	dto.Auth = &contracts.ClientCredentialDTO{
		UUID:     peerAuth,
		Password: peerAuth,
		Extra:    make(map[string]any),
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
	// 客户端扩展参数：obfs 与带宽必须与服务端 finalmask 同源，否则连不上/限速不生效。
	var opts map[string]string
	if dto.Transport != nil {
		opts = dto.Transport.Opts
	}
	if opts != nil {
		if obfs := opts[Hy2OptObfs]; obfs != "" {
			q.Set("obfs", obfs)
			if pw := opts[Hy2OptObfsPassword]; pw != "" {
				q.Set("obfs-password", pw)
			}
		}
		if up := Hy2BandwidthToMihomo(opts[Hy2OptBwUp]); up != "" {
			q.Set("up", up)
		}
		if down := Hy2BandwidthToMihomo(opts[Hy2OptBwDown]); down != "" {
			q.Set("down", down)
		}
	}
	// 端口跳跃在链接里的写法是把端口区域整体放进 authority（mihomo 转换器据此拆出 ports）。
	hostPort := fmt.Sprintf("%s:%d", dto.ServerHost, dto.ServerPort)
	if opts != nil {
		if ports := opts[Hy2OptPorts]; ports != "" {
			hostPort = fmt.Sprintf("%s:%s", dto.ServerHost, ports)
		}
	}
	u := url.URL{
		Scheme:   "hysteria2",
		User:     url.User(password),
		Host:     hostPort,
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
	// 端口跳跃：链接把端口集合整体放在 authority（如 host:20000-30000），
	// 此时 u.Port() 为空，需要从 Host 尾部取出集合再取首端口。
	ports := ""
	if portStr == "" {
		if idx := strings.LastIndex(u.Host, ":"); idx >= 0 && idx+1 < len(u.Host) {
			ports = u.Host[idx+1:]
			first := ports
			if j := strings.IndexAny(ports, ",-"); j > 0 {
				first = ports[:j]
			}
			portStr = first
		}
	}
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
	if q.Get("obfs") != "" {
		if dto.Transport.Opts == nil {
			dto.Transport.Opts = make(map[string]string)
		}
		dto.Transport.Opts[Hy2OptObfs] = q.Get("obfs")
		if pw := q.Get("obfs-password"); pw != "" {
			dto.Transport.Opts[Hy2OptObfsPassword] = pw
		}
	}
	if up := Hy2BandwidthFromMihomo(q.Get("up")); up != "" {
		if dto.Transport.Opts == nil {
			dto.Transport.Opts = make(map[string]string)
		}
		dto.Transport.Opts[Hy2OptBwUp] = up
	}
	if down := Hy2BandwidthFromMihomo(q.Get("down")); down != "" {
		if dto.Transport.Opts == nil {
			dto.Transport.Opts = make(map[string]string)
		}
		dto.Transport.Opts[Hy2OptBwDown] = down
	}
	if ports != "" && strings.ContainsAny(ports, ",-") {
		if dto.Transport.Opts == nil {
			dto.Transport.Opts = make(map[string]string)
		}
		dto.Transport.Opts[Hy2OptPorts] = ports
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
	if fm := hysteriaFinalMask(dto); fm != nil {
		item["streamSettings"].(map[string]any)["finalmask"] = fm
	}
	return item, nil
}

// ─── Hysteria 2 客户端扩展参数（finalmask）──────────────────────────────────

// hmScalar 把任意 JSON 标量转成字符串（PortList / Int32Range 既可能是字符串也可能是数字）。
func hmScalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return ""
	}
}

// hmPacketSize 解析 salamander 的 packetSize（Int32Range：对象或字符串）。
// 返回 (min, max)；max<=0 表示未启用 gecko 变体。
func hmPacketSize(v any) (int64, int64) {
	switch t := v.(type) {
	case map[string]any:
		return int64Of(t["from"]), int64Of(t["to"])
	case string:
		parts := strings.SplitN(strings.TrimSpace(t), "-", 2)
		if len(parts) == 1 {
			n := int64Of(parts[0])
			return n, n
		}
		return int64Of(parts[0]), int64Of(parts[1])
	default:
		return 0, 0
	}
}

func int64Of(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		return n
	default:
		return 0
	}
}

// hydrateHysteriaOpts 把服务端 finalmask 里「客户端必须同源」的参数投影到 DTO.Opts：
// salamander/gecko 混淆、端口跳跃、带宽。Hysteria 2 的 obfs 不是可选装饰——服务端开了
// 而客户端不带，连接直接不通，所以订阅与机间出站都必须消费这组值。
func hydrateHysteriaOpts(dto *contracts.ProxyNodeDTO, spec *contracts.InboundSpec) {
	if dto == nil || dto.Transport == nil || spec == nil || spec.Stream == nil {
		return
	}
	fm, _ := spec.Stream["finalmask"].(map[string]any)
	if fm == nil {
		return
	}
	puts := func(k, v string) {
		if v == "" {
			return
		}
		if dto.Transport.Opts == nil {
			dto.Transport.Opts = make(map[string]string)
		}
		dto.Transport.Opts[k] = v
	}

	if udp, ok := fm["udp"].([]any); ok {
		for _, item := range udp {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := m["type"].(string)
			settings, _ := m["settings"].(map[string]any)
			switch strings.ToLower(typ) {
			case "salamander":
				pw := hmScalar(settings["password"])
				if pw == "" {
					continue
				}
				min, max := hmPacketSize(settings["packetSize"])
				if max > 0 {
					// xray 用 salamander + packetSize 表达 gecko；mihomo 是独立的 obfs 类型
					puts(Hy2OptObfs, "gecko")
					puts(Hy2OptObfsMinPacket, strconv.FormatInt(min, 10))
					puts(Hy2OptObfsMaxPacket, strconv.FormatInt(max, 10))
				} else {
					puts(Hy2OptObfs, "salamander")
				}
				puts(Hy2OptObfsPassword, pw)
			case "udphop":
				// 只有目标端口跳跃能映射到 mihomo 的 ports；
				// intervalLocal 只跳源端口，mihomo 无等价表达，宁可不输出。
				mode := strings.ToLower(hmScalar(settings["mode"]))
				if !strings.Contains(mode, "intervalremote") && !strings.Contains(mode, "perconnremote") {
					continue
				}
				puts(Hy2OptPorts, hmScalar(settings["remotePorts"]))
				puts(Hy2OptHopInterval, hmScalar(settings["interval"]))
			}
		}
	}

	if qp, ok := fm["quicParams"].(map[string]any); ok {
		// 方向相对论：服务端的上传是客户端的下载，服务端的下载是客户端的上传。
		// DTO.Opts 统一用客户端语义（mihomo 的 up/down），所以这里必须交叉映射：
		// 服务端 brutalUp → 客户端 down，服务端 brutalDown → 客户端 up。
		puts(Hy2OptBwDown, hmScalar(qp["brutalUp"]))
		puts(Hy2OptBwUp, hmScalar(qp["brutalDown"]))
	}
}

// hysteriaFinalMask 由 DTO.Opts 反向生成 xray 的 streamSettings.finalmask（机间出站用）。
// 只输出与连通性相关的 obfs / 端口跳跃；带宽属于本地发送策略，不下发给对端。
func hysteriaFinalMask(dto *contracts.ProxyNodeDTO) map[string]any {
	if dto == nil || dto.Transport == nil {
		return nil
	}
	opts := dto.Transport.Opts
	if len(opts) == 0 {
		return nil
	}
	udp := make([]any, 0, 2)
	switch opts[Hy2OptObfs] {
	case "salamander":
		udp = append(udp, map[string]any{
			"type":     "salamander",
			"settings": map[string]any{"password": opts[Hy2OptObfsPassword]},
		})
	case "gecko":
		settings := map[string]any{"password": opts[Hy2OptObfsPassword]}
		min := int64Of(opts[Hy2OptObfsMinPacket])
		max := int64Of(opts[Hy2OptObfsMaxPacket])
		if min > 0 && max > 0 {
			// xray 的 Int32Range 只接受字符串 "a-b" 或整数，写成 {from,to} 对象会拒绝加载。
			settings["packetSize"] = strconv.FormatInt(min, 10) + "-" + strconv.FormatInt(max, 10)
		}
		udp = append(udp, map[string]any{"type": "salamander", "settings": settings})
	}
	if ports := opts[Hy2OptPorts]; ports != "" {
		settings := map[string]any{
			"mode":        "intervalRemote",
			"remotePorts": ports,
		}
		if interval := opts[Hy2OptHopInterval]; interval != "" {
			settings["interval"] = interval
		}
		udp = append(udp, map[string]any{"type": "udphop", "settings": settings})
	}
	if len(udp) == 0 {
		return nil
	}
	return map[string]any{"udp": udp}
}

// Hy2BandwidthToMihomo 把 xray 的带宽写法翻译成 mihomo 认可的写法。
//
// xray Bandwidth.Bps(): 无单位按字节、k/m/g 按 1024，最终统一 /8；
// mihomo StringToBps(): 无单位按 Mbps、k/m/g 按 1000，同样 /8。
//   - 带单位（如 "100 mbps"）→ "<N> <UNIT>bps"，数字不变（两端进制差约 4.9%，见字段语义模型）；
//   - 纯数字（xray 语义 = N/8 字节/秒）→ "<N/8> Bps"，保持严格等价。
func Hy2BandwidthToMihomo(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return ""
	}
	idx := len(s)
	for i, c := range s {
		if (c < '0' || c > '9') && c != '.' {
			idx = i
			break
		}
	}
	numStr, unit := strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx:])
	n, err := strconv.ParseFloat(numStr, 64)
	if err != nil || n <= 0 {
		return ""
	}
	switch unit {
	case "", "b", "bps":
		// xray 的字节写法也会统一 /8
		return fmt.Sprintf("%d Bps", int64(n/8))
	}
	var upper string
	switch unit {
	case "k", "kb", "kbps":
		upper = "K"
	case "m", "mb", "mbps":
		upper = "M"
	case "g", "gb", "gbps":
		upper = "G"
	case "t", "tb", "tbps":
		upper = "T"
	default:
		return ""
	}
	return fmt.Sprintf("%s %sbps", numStr, upper)
}

// Hy2BandwidthFromMihomo 把链接/mihomo 的带宽写法换算回 xray 的写法（ParseLink 用）。
// 纯数字与 b 结尾按 Mbps（mihomo 语义），B 结尾按字节/秒；最终乘 8 交给 xray 的 /8。
func Hy2BandwidthFromMihomo(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return ""
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		if n <= 0 {
			return ""
		}
		return strconv.FormatInt(int64(n*1_000_000/8)*8, 10)
	}
	idx := len(s)
	for i, c := range s {
		if (c < '0' || c > '9') && c != '.' {
			idx = i
			break
		}
	}
	numStr, unit := strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx:])
	n, err := strconv.ParseFloat(numStr, 64)
	if err != nil || n <= 0 {
		return ""
	}
	unit = strings.TrimSuffix(unit, "ps")
	bits := strings.HasSuffix(unit, "b")
	unit = strings.TrimSuffix(unit, "b")
	mult := float64(1)
	switch unit {
	case "":
		mult = 1_000_000 // 无单位在 mihomo 里是 Mbps
	case "k":
		mult = 1_000
	case "m":
		mult = 1_000_000
	case "g":
		mult = 1_000_000_000
	case "t":
		mult = 1_000_000_000_000
	default:
		return ""
	}
	bps := n * mult
	if bits {
		bps /= 8
	}
	return strconv.FormatInt(int64(bps)*8, 10)
}
