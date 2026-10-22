package protocols

import "github.com/acdc-awa/xpanel/internal/contracts"

// Hysteria 2 字段声明：传输/安全/版本由面板强制，管理员只配置少数参数。
func hysteriaFieldSet() contracts.ProtocolFieldSet {
	fields := []contracts.FieldDecl{
		field("settings.version", "协议版本（固定 2）", contracts.ShapeScalar, contracts.CardOne, contracts.RoleServerOnly, contracts.OriginSystem),
		field("settings.users", "认证用户（后端注入）", contracts.ShapeArray, contracts.CardMany, contracts.RoleServerOnly, contracts.OriginSystem),
		field("settings.clients", "clients 键（本协议禁用）", contracts.ShapeArray, contracts.CardMany, contracts.RoleServerOnly, contracts.OriginSystem),

		field("stream.network", "传输协议（固定 hysteria）", contracts.ShapeScalar, contracts.CardOne, contracts.RoleBoth, contracts.OriginSystem),
		field("stream.security", "安全层（固定 tls）", contracts.ShapeScalar, contracts.CardOne, contracts.RoleBoth, contracts.OriginSystem),

		field("stream.hysteriaSettings.version", "Hysteria 版本（固定 2）", contracts.ShapeScalar, contracts.CardOne, contracts.RoleServerOnly, contracts.OriginSystem),
		field("stream.hysteriaSettings.udpIdleTimeout", "UDP 空闲超时（秒）", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("stream.hysteriaSettings.auth", "共享密码（仅客户端）", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleClientOnly, contracts.OriginDerived),
		field("stream.hysteriaSettings.masquerade", "伪装站点（404/file/proxy/string）", contracts.ShapeObject, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),

		field("stream.tlsSettings.serverName", "TLS SNI", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.tlsSettings.alpn", "TLS ALPN（Hysteria 2 必须含 h3）", contracts.ShapeArray, contracts.CardMany, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.tlsSettings.certificates", "TLS 证书", contracts.ShapeArray, contracts.CardMany, contracts.RoleServerOnly, contracts.OriginAdmin),

		field("stream.finalmask.udp", "UDP 混淆（obfs）", contracts.ShapeArray, contracts.CardMany, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.finalmask.quicParams", "QUIC 参数（带宽/拥塞窗口）", contracts.ShapeObject, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.realitySettings", "REALITY（本协议禁用）", contracts.ShapeObject, contracts.CardOptional, contracts.RoleBoth, contracts.OriginSystem),

		field("sniffing.enabled", "启用嗅探", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),
	}
	fields = append(fields, sockoptFields()...)

	rules := []contracts.Rule{
		allowRule("stream.network", "Hysteria 2 只能使用 hysteria 传输", []string{"hysteria"}),
		allowRule("stream.security", "Hysteria 2 只能使用 TLS", []string{"tls"}),
		rule(contracts.RuleForbids, "stream.hysteriaSettings.auth",
			"服务端抛弃共享密码：认证全部由 settings.users[] 承担", condNonEmpty("stream.hysteriaSettings.auth")),
		rule(contracts.RuleForbids, "stream.realitySettings", "Hysteria 2 不支持 REALITY"),
		rule(contracts.RuleForbids, "settings.clients", "Hysteria 2 的用户数组键是 users，不是 clients", condNonEmpty("settings.clients")),
	}

	dialects := []contracts.DialectMapping{
		{Path: "settings.version", Xray: "settings.version"},
		{Path: "stream.hysteriaSettings.udpIdleTimeout", Xray: "streamSettings.hysteriaSettings.udpIdleTimeout"},
		{Path: "stream.hysteriaSettings.auth", Xray: "outbound streamSettings.hysteriaSettings.auth", Mihomo: "password", Link: "userinfo",
			Note: "仅出站/客户端；服务端 users[].auth 才是计费凭据"},
		{Path: "stream.hysteriaSettings.masquerade", Xray: "streamSettings.hysteriaSettings.masquerade",
			Note: "纯服务端能力：未通过 Hysteria 认证的普通 HTTP 请求返回 404/file/proxy/string；mihomo 无对应字段"},
		{Path: "stream.tlsSettings.serverName", Xray: "streamSettings.tlsSettings.serverName", Mihomo: "sni", Link: "sni"},
		{Path: "stream.tlsSettings.alpn", Xray: "streamSettings.tlsSettings.alpn", Mihomo: "alpn", Link: "alpn",
			Note: "服务端必须含 h3（留空内核补 h3）；写成 h2,http/1.1 会 tls: no application protocol"},
		{Path: "stream.finalmask.udp", Mihomo: "obfs / obfs-password / obfs-min-packet-size / obfs-max-packet-size / ports / hop-interval", Transform: "json",
			Note: "容器位置不同：xray 在 finalmask.udp[]{type:\"salamander\",settings:{password,packetSize}}，mihomo 是顶层；带 packetSize 时 mihomo 侧 obfs 必须是 gecko。端口跳跃同在 udp 数组：udphop（mode 含 intervalRemote/perconnRemote）↔ mihomo 顶层 ports + hop-interval（秒，≥5，默认 30）。udphop 仅客户端实现，面板存入 DB 供订阅投影，生成服务端配置时会被 sanitize 剥离（服务端带它监听直接失败）"},
		{Path: "stream.finalmask.quicParams", Mihomo: "down / up（方向交叉）", Transform: "json",
			Note: "方向相对论：服务端 brutalUp 是服务端上行=客户端下行 → mihomo down；服务端 brutalDown → mihomo up。带宽字段名不同（brutalUp/brutalDown），单位也不同（xray 无单位按 bps、1024 进制，mihomo 无单位按 Mbps、1000 进制），两端都填写才会启用 Brutal，否则退回 BBR"},
	}

	return contracts.ProtocolFieldSet{
		Protocol: "hysteria",
		Label:    "Hysteria 2",
		Fields:   fields,
		Rules:    rules,
		Dialects: dialects,
	}
}
