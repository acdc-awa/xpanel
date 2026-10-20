package protocols

import "github.com/acdc-awa/xpanel/internal/contracts"

// VLESS 字段声明。Path 对应入站存储的三层 JSON（settings / stream / sniffing）。
func vlessFieldSet() contracts.ProtocolFieldSet {
	fields := []contracts.FieldDecl{
		// L2 / L5 协议专有
		field("settings.decryption", "VLESS Encryption 服务端密钥", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("settings.encryption", "VLESS Encryption 客户端派生", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleClientOnly, contracts.OriginDerived),
		field("settings.fallbacks", "回落目标", contracts.ShapeArray, contracts.CardMany, contracts.RoleBoth, contracts.OriginAdmin),
		field("settings.clients", "认证用户（后端注入）", contracts.ShapeArray, contracts.CardMany, contracts.RoleServerOnly, contracts.OriginSystem),
		contracts.FieldDecl{Path: "flow", Label: "流控", Shape: contracts.ShapeEnum, Cardinality: contracts.CardOptional,
			Role: contracts.RoleBoth, Origin: contracts.OriginAdmin, Enum: []string{"", "none", "xtls-rprx-vision"}, Default: ""},

		// L3 传输
		enumField("stream.network", "传输协议", contracts.RoleBoth, contracts.OriginAdmin, []string{"tcp", "xhttp"}, "tcp"),
		field("stream.tcpSettings.acceptProxyProtocol", "接受 PROXY Protocol", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),

		enumField("stream.xhttpSettings.mode", "XHTTP 模式", contracts.RoleBoth, contracts.OriginAdmin,
			[]string{"auto", "packet-up", "stream-up", "stream-one"}, "auto"),
		field("stream.xhttpSettings.path", "XHTTP 路径", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.xhttpSettings.host", "XHTTP Host", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.xhttpSettings.xPaddingBytes", "XHTTP 填充区间", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.xhttpSettings.sessionIDPlacement", "会话 ID 位置", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.xhttpSettings.sessionIDLength", "会话 ID 长度", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.xhttpSettings.sessionID", "会话 ID（旧键提升）", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginDerived),
		field("stream.xhttpSettings.xmux", "XHTTP 多路复用", contracts.ShapeObject, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.xhttpSettings.extra", "XHTTP 扩展参数", contracts.ShapeMap, contracts.CardOptional, contracts.RoleClientOnly, contracts.OriginAdmin),
		field("stream.finalmask", "finalmask（本协议禁用）", contracts.ShapeObject, contracts.CardOptional, contracts.RoleBoth, contracts.OriginSystem),

		// L4 安全
		enumField("stream.security", "安全层", contracts.RoleBoth, contracts.OriginAdmin, []string{"none", "tls", "reality"}, "none"),
		field("stream.fingerprint", "客户端 uTLS 指纹", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleClientOnly, contracts.OriginAdmin),

		field("stream.tlsSettings.serverName", "TLS SNI", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.tlsSettings.certificates", "TLS 证书", contracts.ShapeArray, contracts.CardMany, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("stream.tlsSettings.allowInsecure", "跳过证书校验", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleClientOnly, contracts.OriginAdmin),
		field("stream.tlsSettings.pinnedPeerCertSha256", "证书指纹固定", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleClientOnly, contracts.OriginAdmin),
		field("stream.tlsSettings.disableSystemRoot", "禁用系统根证书", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleClientOnly, contracts.OriginAdmin),
		field("stream.tlsSettings.verifyPeerCertByName", "按名称校验证书", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleClientOnly, contracts.OriginAdmin),
		field("stream.tlsSettings.rejectUnknownSni", "拒绝未知 SNI", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),

		field("stream.realitySettings.serverNames", "REALITY SNI 列表", contracts.ShapeArray, contracts.CardMany, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("stream.realitySettings.shortIds", "REALITY ShortId 列表", contracts.ShapeArray, contracts.CardMany, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("stream.realitySettings.privateKey", "REALITY 私钥", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("stream.realitySettings.dest", "REALITY 目标", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("stream.realitySettings.publicKey", "REALITY 公钥（客户端）", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleClientOnly, contracts.OriginDerived),
		field("stream.realitySettings.serverName", "REALITY SNI（客户端）", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleClientOnly, contracts.OriginDerived),
		field("stream.realitySettings.shortId", "REALITY ShortId（客户端）", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleClientOnly, contracts.OriginDerived),
		field("stream.realitySettings.spiderX", "REALITY SpiderX", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleClientOnly, contracts.OriginDerived),

		// L7 嗅探
		field("sniffing.enabled", "启用嗅探", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("sniffing.destOverride", "覆盖目标域名", contracts.ShapeArray, contracts.CardMany, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("sniffing.routeOnly", "仅按域名路由", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),
	}
	fields = append(fields, sockoptFields()...)

	rules := []contracts.Rule{
		allowRule("flow", "VLESS flow 只接受 vision 或关闭", []string{"", "none", "xtls-rprx-vision"}),
		rule(contracts.RuleForbids, "settings.encryption",
			"xray 拒绝入站级 encryption（运行时恒为 none）；客户端 encryption 由 decryption 派生",
			condNonEmpty("settings.encryption")),
		rule(contracts.RuleRequires, "stream.realitySettings.privateKey", "REALITY 服务端必须提供私钥", condEq("stream.security", "reality")),
		rule(contracts.RuleRequires, "stream.realitySettings.dest", "REALITY 服务端必须提供 dest", condEq("stream.security", "reality")),
		rule(contracts.RuleRequires, "stream.realitySettings.serverNames", "REALITY 服务端至少一个 SNI", condEq("stream.security", "reality")),
		rule(contracts.RuleRequires, "stream.realitySettings.shortIds", "REALITY 服务端至少一个 ShortId", condEq("stream.security", "reality")),
		rule(contracts.RuleForbids, "stream.tlsSettings", "安全层是二选一：REALITY 与 TLS 不能同时配置", condEq("stream.security", "reality")),
		rule(contracts.RuleForbids, "stream.realitySettings", "security=none 时不得携带 REALITY 参数", condEq("stream.security", "none")),
		rule(contracts.RuleForbids, "stream.tlsSettings", "security=none 时不得携带 TLS 参数", condEq("stream.security", "none")),
		rule(contracts.RuleForbids, "stream.finalmask", "REALITY 与 finalmask 组合会触发 xray panic（XTLS/Xray-core#6453）", condEq("stream.security", "reality")),
		rule(contracts.RuleRequires, "stream.xhttpSettings.mode", "XHTTP 传输必须声明模式（缺省 auto）", condEq("stream.network", "xhttp")),
		allowRule("flow", "XHTTP 不支持 vision 流控", []string{"", "none"}, condEq("stream.network", "xhttp")),
	}

	dialects := []contracts.DialectMapping{
		{Path: "settings.decryption", Xray: "settings.decryption", Note: "仅服务端配置与 vlessenc 链接携带"},
		{Path: "settings.encryption", Xray: "settings.vnext[].users[].encryption", Link: "encryption", Note: "入站禁止，出站必须补 none"},
		{Path: "flow", Xray: "settings.clients[].flow / settings.vnext[].users[].flow", Link: "flow"},
		{Path: "stream.network", Xray: "streamSettings.network", Mihomo: "type", Link: "type"},
		{Path: "stream.security", Xray: "streamSettings.security", Mihomo: "type（安全段）", Link: "security"},
		{Path: "stream.fingerprint", Xray: "streamSettings.tlsSettings.fingerprint", Mihomo: "client-fingerprint", Link: "fp",
			Note: "mihomo 的 fingerprint 是证书 pin，uTLS 指纹必须用 client-fingerprint"},
		{Path: "stream.tlsSettings.serverName", Xray: "streamSettings.tlsSettings.serverName", Mihomo: "servername", Link: "sni"},
		{Path: "stream.tlsSettings.pinnedPeerCertSha256", Xray: "streamSettings.tlsSettings.pinnedPeerCertSha256", Mihomo: "fingerprint", Link: "pcs",
			Note: "同名不同义：mihomo YAML 的 fingerprint 是 pin"},
		{Path: "stream.tlsSettings.allowInsecure", Mihomo: "skip-cert-verify", Link: "allowInsecure",
			Note: "xray v26.6.27 起已移除 allowInsecure，不写入服务端配置"},
		{Path: "stream.realitySettings.serverNames", Xray: "streamSettings.realitySettings.serverNames", Mihomo: "servername", Link: "sni", Transform: "first",
			Note: "服务端数组、客户端标量（基数陷阱）"},
		{Path: "stream.realitySettings.shortIds", Xray: "streamSettings.realitySettings.shortIds", Mihomo: "reality-opts.short-id", Link: "sid", Transform: "first",
			Note: "服务端数组、客户端标量（基数陷阱）"},
		{Path: "stream.realitySettings.privateKey", Xray: "streamSettings.realitySettings.privateKey", Note: "服务端私有，永不导出"},
		{Path: "stream.realitySettings.dest", Xray: "streamSettings.realitySettings.dest", Note: "服务端私有，永不导出"},
		{Path: "stream.realitySettings.publicKey", Xray: "outbound streamSettings.realitySettings.password", Mihomo: "reality-opts.public-key", Link: "pbk",
			Note: "xray 客户端 password 优先于 publicKey"},
		{Path: "stream.realitySettings.serverName", Xray: "outbound streamSettings.realitySettings.serverName", Mihomo: "servername", Link: "sni", Transform: "first"},
		{Path: "stream.realitySettings.shortId", Xray: "outbound streamSettings.realitySettings.shortId", Mihomo: "reality-opts.short-id", Link: "sid", Transform: "first"},
		{Path: "stream.realitySettings.spiderX", Xray: "outbound streamSettings.realitySettings.spiderX", Transform: "json"},
		{Path: "stream.xhttpSettings.mode", Xray: "streamSettings.xhttpSettings.mode", Mihomo: "xhttp-opts.mode", Link: "mode"},
		{Path: "stream.xhttpSettings.path", Xray: "streamSettings.xhttpSettings.path", Mihomo: "xhttp-opts.path", Link: "path"},
		{Path: "stream.xhttpSettings.host", Xray: "streamSettings.xhttpSettings.host", Mihomo: "xhttp-opts.host", Link: "host"},
		{Path: "stream.xhttpSettings.xPaddingBytes", Mihomo: "xhttp-opts.x-padding-bytes", Transform: "range",
			Note: "xray 是区间对象，mihomo/链接是字符串"},
		{Path: "stream.xhttpSettings.sessionIDPlacement", Mihomo: "xhttp-opts.session-placement",
			Note: "四处改名：sessionPlacement/sessionIDPlacement/session-placement"},
		{Path: "stream.xhttpSettings.sessionIDLength", Mihomo: "xhttp-opts.session-length", Transform: "range"},
		{Path: "stream.xhttpSettings.xmux", Mihomo: "xhttp-opts.reuse-settings",
			Note: "改名且默认值相反：xray 默认开复用，mihomo 不填即不复用"},
		{Path: "stream.xhttpSettings.extra", Mihomo: "xhttp-opts（展开）", Note: "mihomo 无独立 extra 字段，必须展开"},
		{Path: "stream.sockopt.mark", Xray: "streamSettings.sockopt.mark", Note: "mihomo 客户端无对应字段"},
		{Path: "stream.sockopt.tcpFastOpen", Xray: "streamSettings.sockopt.tcpFastOpen"},
		{Path: "stream.sockopt.acceptProxyProtocol", Xray: "streamSettings.sockopt.acceptProxyProtocol"},
	}

	return contracts.ProtocolFieldSet{
		Protocol: "vless",
		Label:    "VLESS",
		Fields:   fields,
		Rules:    rules,
		Dialects: dialects,
	}
}
