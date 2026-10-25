package protocols

import "github.com/acdc-awa/xpanel/internal/contracts"

// tunnel（四层直通；xray 协议名 tunnel，dokodemo-door 是其别名）字段声明。
// 该协议没有客户端投影，目标自 C2 起住在派生 freedom 出站的 redirect 上，入站只监听。
func tunnelFieldSet() contracts.ProtocolFieldSet {
	fields := []contracts.FieldDecl{
		field("endpoint.listen", "监听地址", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("endpoint.port", "监听端口", contracts.ShapeScalar, contracts.CardOne, contracts.RoleServerOnly, contracts.OriginAdmin),

		enumField("settings.network", "转发协议", contracts.RoleServerOnly, contracts.OriginAdmin,
			[]string{"tcp", "udp", "tcp,udp"}, "tcp,udp"),
		field("settings.followRedirect", "跟随透明重定向（固定 false）", contracts.ShapeScalar, contracts.CardOne, contracts.RoleServerOnly, contracts.OriginSystem),
		field("settings.address", "入站目标地址（已废弃）", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginSystem),
		field("settings.port", "入站目标端口（已废弃）", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginSystem),

		field("stream.acceptProxyProtocol", "接受 PROXY Protocol", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("stream.tcpSettings.acceptProxyProtocol", "TCP 层接受 PROXY Protocol", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginDerived),
		field("stream.proxy_protocol", "透传 PROXY Protocol（面板私有键）", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),

		enumField("target.mode", "目标方式", contracts.RoleServerOnly, contracts.OriginAdmin, []string{"linked", "external"}, "linked"),
		field("target.inboundId", "连线目标入站", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("target.address", "手动目标地址", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("target.port", "手动目标端口", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginAdmin),
		field("target.outboundRedirect", "派生出的 freedom redirect", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleServerOnly, contracts.OriginDerived),
	}

	rules := []contracts.Rule{
		rule(contracts.RuleRequires, "target.inboundId", "连线模式必须选择目标入站", condEq("target.mode", "linked")),
		rule(contracts.RuleForbids, "target.address", "连线模式的目标地址由目标入站决定", condEq("target.mode", "linked")),
		rule(contracts.RuleForbids, "target.port", "连线模式的目标端口由目标入站决定", condEq("target.mode", "linked")),
		rule(contracts.RuleRequires, "target.address", "手动模式必须填写目标地址", condEq("target.mode", "external")),
		rule(contracts.RuleRequires, "target.port", "手动模式必须填写目标端口", condEq("target.mode", "external")),
		rule(contracts.RuleForbids, "settings.address",
			"目标由派生 freedom 出站的 redirect 承担，入站不再写 address", condNonEmpty("settings.address")),
		rule(contracts.RuleForbids, "settings.port",
			"目标由派生 freedom 出站的 redirect 承担，入站不再写 port", condNonEmpty("settings.port")),
	}

	return contracts.ProtocolFieldSet{
		Protocol: "tunnel",
		Label:    "四层直通",
		Fields:   fields,
		Rules:    rules,
		Dialects: nil, // 直通不产生客户端投影（不进入订阅）
	}
}
