package protocols

import (
	"sort"

	"github.com/acdc-awa/xpanel/internal/contracts"
)

// 字段注册表入口（S3）：协议知识（字段清单、交叉规则、方言映射）按协议聚合在这里。
// 未注册协议返回 false；调用方应 fail-closed（不猜测字段）。

var fieldSets = map[string]contracts.ProtocolFieldSet{
	"vless":         vlessFieldSet(),
	"hysteria":      hysteriaFieldSet(),
	"dokodemo-door": dokodemoFieldSet(),
}

// FieldSet 按协议名取字段声明。
func FieldSet(proto string) (contracts.ProtocolFieldSet, bool) {
	fs, ok := fieldSets[proto]
	return fs, ok
}

// AllFieldSets 按协议名排序返回全部声明（稳定输出，便于 API/测试）。
func AllFieldSets() []contracts.ProtocolFieldSet {
	names := make([]string, 0, len(fieldSets))
	for name := range fieldSets {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]contracts.ProtocolFieldSet, 0, len(names))
	for _, name := range names {
		out = append(out, fieldSets[name])
	}
	return out
}

func field(path, label string, shape contracts.FieldShape, card contracts.Cardinality, role contracts.FieldRole, origin contracts.FieldOrigin) contracts.FieldDecl {
	return contracts.FieldDecl{Path: path, Label: label, Shape: shape, Cardinality: card, Role: role, Origin: origin}
}

func enumField(path, label string, role contracts.FieldRole, origin contracts.FieldOrigin, values []string, def any) contracts.FieldDecl {
	return contracts.FieldDecl{
		Path: path, Label: label, Shape: contracts.ShapeEnum, Cardinality: contracts.CardOne,
		Role: role, Origin: origin, Enum: values, Default: def,
	}
}

func condEq(path string, value any) contracts.Condition {
	return contracts.Condition{Path: path, Op: contracts.CondEq, Value: value}
}

func condIn(path string, values ...string) contracts.Condition {
	return contracts.Condition{Path: path, Op: contracts.CondIn, Values: values}
}

func condNonEmpty(path string) contracts.Condition {
	return contracts.Condition{Path: path, Op: contracts.CondNonEmpty}
}

func rule(op contracts.RuleOp, target, reason string, when ...contracts.Condition) contracts.Rule {
	return contracts.Rule{When: when, Op: op, Target: target, Reason: reason}
}

func allowRule(target, reason string, values []string, when ...contracts.Condition) contracts.Rule {
	return contracts.Rule{When: when, Op: contracts.RuleAllows, Target: target, Values: values, Reason: reason}
}

// sockoptFields 两种流式协议共用的 socket 调优字段（role=both）。
func sockoptFields() []contracts.FieldDecl {
	return []contracts.FieldDecl{
		field("stream.sockopt.mark", "Socket 标记", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.sockopt.tcpFastOpen", "TCP Fast Open", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		enumField("stream.sockopt.tproxy", "透明代理模式", contracts.RoleBoth, contracts.OriginAdmin,
			[]string{"", "off", "tproxy", "redirect"}, ""),
		field("stream.sockopt.acceptProxyProtocol", "接受 PROXY Protocol", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.sockopt.domainStrategy", "域名解析策略", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.sockopt.tcpCongestion", "TCP 拥塞控制", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.sockopt.tcpKeepAliveIdle", "TCP KeepAlive 空闲（秒）", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.sockopt.tcpKeepAliveInterval", "TCP KeepAlive 间隔（秒）", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.sockopt.tcpUserTimeout", "TCP User Timeout", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.sockopt.tcpMaxSeg", "TCP Max Segment", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.sockopt.tcpWindowClamp", "TCP Window Clamp", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.sockopt.tcpMptcp", "TCP MPTCP", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.sockopt.interface", "出站网卡", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
		field("stream.sockopt.v6only", "仅 IPv6", contracts.ShapeScalar, contracts.CardOptional, contracts.RoleBoth, contracts.OriginAdmin),
	}
}
