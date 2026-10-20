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

		field("stream.tlsSettings.serverName", "TLS SNI", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.tlsSettings.certificates", "TLS 证书", contracts.ShapeArray, contracts.CardMany, contracts.RoleServerOnly, contracts.OriginAdmin),

		field("stream.finalmask.udp", "UDP 混淆（obfs）", contracts.ShapeArray, contracts.CardMany, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.finalmask.quicParams", "QUIC 参数（带宽/端口跳跃）", contracts.ShapeObject, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
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
		{Path: "stream.tlsSettings.serverName", Xray: "streamSettings.tlsSettings.serverName", Mihomo: "sni", Link: "sni"},
		{Path: "stream.finalmask.udp", Mihomo: "obfs / obfs-password", Transform: "json",
			Note: "容器位置不同：xray 在 finalmask，mihomo 是顶层字段"},
		{Path: "stream.finalmask.quicParams", Mihomo: "up / down / ports", Transform: "json",
			Note: "带宽与端口跳跃在 xray 侧位于 finalmask.quicParams"},
	}

	return contracts.ProtocolFieldSet{
		Protocol: "hysteria",
		Label:    "Hysteria 2",
		Fields:   fields,
		Rules:    rules,
		Dialects: dialects,
	}
}
