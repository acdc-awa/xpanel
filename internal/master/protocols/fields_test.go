package protocols

import (
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
)

// S3 契约锁：字段声明必须自洽（可被 S4 的 API 与前端直接消费），
// 并且关键方言映射（同名不同义、改名、容器不同）不能回退。

func TestFieldSets_StructuralIntegrity(t *testing.T) {
	for _, fs := range AllFieldSets() {
		t.Run(fs.Protocol, func(t *testing.T) {
			if fs.Protocol == "" || fs.Label == "" {
				t.Error("protocol/label 不能为空")
			}
			if len(fs.Fields) == 0 {
				t.Fatal("字段声明不能为空")
			}

			seen := map[string]bool{}
			for _, f := range fs.Fields {
				if f.Path == "" {
					t.Error("字段 path 不能为空")
				}
				if seen[f.Path] {
					t.Errorf("字段 path 重复: %s", f.Path)
				}
				seen[f.Path] = true

				if f.Origin == "" || f.Role == "" || f.Shape == "" || f.Cardinality == "" {
					t.Errorf("%s: shape/cardinality/role/origin 必须齐全: %+v", f.Path, f)
				}
				if f.Shape == contracts.ShapeEnum {
					if len(f.Enum) == 0 {
						t.Errorf("%s: enum 字段必须声明取值", f.Path)
					}
					if f.Default != nil {
						if s, ok := f.Default.(string); ok && !contains(f.Enum, s) {
							t.Errorf("%s: 默认值 %q 不在枚举内", f.Path, s)
						}
					}
				}
				if f.Origin == contracts.OriginSystem && f.Role == "" {
					t.Errorf("%s: system 字段必须说明角色", f.Path)
				}
			}

			for _, r := range fs.Rules {
				if len(r.When) == 0 {
					// 无条件的规则合法（如 "入站禁止 encryption"）
				}
				for _, c := range r.When {
					if !pathDeclared(fs, c.Path) {
						t.Errorf("规则条件引用了未声明字段: %s (target=%s)", c.Path, r.Target)
					}
					switch c.Op {
					case contracts.CondEq, contracts.CondNe:
						if c.Value == nil {
							t.Errorf("条件 %s 缺少 value", c.Path)
						}
					case contracts.CondIn:
						if len(c.Values) == 0 {
							t.Errorf("条件 %s 缺少 values", c.Path)
						}
					case contracts.CondEmpty, contracts.CondNonEmpty:
					default:
						t.Errorf("未知条件算子: %s", c.Op)
					}
				}
				if !pathDeclared(fs, r.Target) {
					t.Errorf("规则目标未声明: %s (%s)", r.Target, r.Reason)
				}
				switch r.Op {
				case contracts.RuleAllows:
					if len(r.Values) == 0 {
						t.Errorf("allows 规则必须声明取值: %s", r.Target)
					}
				case contracts.RuleRequires, contracts.RuleForbids:
				default:
					t.Errorf("未知规则算子: %s", r.Op)
				}
				if r.Reason == "" {
					t.Errorf("规则缺少 reason: %+v", r)
				}
			}

			for _, d := range fs.Dialects {
				if !pathDeclared(fs, d.Path) {
					t.Errorf("方言映射引用了未声明字段: %s", d.Path)
				}
				if d.Xray == "" && d.Mihomo == "" && d.Link == "" {
					t.Errorf("方言映射没有任何目标: %s", d.Path)
				}
			}
		})
	}
}

func TestFieldSets_CoverRegisteredProtocols(t *testing.T) {
	for _, proto := range RegisteredProtocols() {
		if _, ok := FieldSet(proto); !ok {
			t.Errorf("已注册协议 %q 缺少字段声明", proto)
		}
	}
	if _, ok := FieldSet("dokodemo-door"); !ok {
		t.Error("直通协议 dokodemo-door 缺少字段声明")
	}
	if _, ok := FieldSet("vmess"); ok {
		t.Error("未注册协议不应有字段声明")
	}
}

// TestFieldSets_DialectTraps 锁定文档 §2 的方言陷阱，防止回归成"同名直传"。
func TestFieldSets_DialectTraps(t *testing.T) {
	vless, _ := FieldSet("vless")
	hy, _ := FieldSet("hysteria")

	cases := []struct {
		set    contracts.ProtocolFieldSet
		path   string
		target string // mihomo 侧
		why    string
	}{
		{vless, "stream.fingerprint", "client-fingerprint", "mihomo fingerprint 是证书 pin，uTLS 必须用 client-fingerprint"},
		{vless, "stream.tlsSettings.pinnedPeerCertSha256", "fingerprint", "pin 在 mihomo 叫 fingerprint"},
		{vless, "stream.xhttpSettings.sessionIDPlacement", "xhttp-opts.session-placement", "session 键在 mihomo 是连字符命名"},
		{vless, "stream.xhttpSettings.xmux", "xhttp-opts.reuse-settings", "xmux 在 mihomo 改名且默认值相反"},
		{hy, "stream.finalmask.udp", "obfs / obfs-password", "hysteria obfs 容器位置不同"},
	}
	for _, tc := range cases {
		d, ok := findDialect(tc.set, tc.path)
		if !ok {
			t.Errorf("%s: 缺少方言映射 %s", tc.set.Protocol, tc.path)
			continue
		}
		if d.Mihomo != tc.target {
			t.Errorf("%s/%s: mihomo 映射 = %q, want %q（%s）", tc.set.Protocol, tc.path, d.Mihomo, tc.target, tc.why)
		}
	}
}

func TestFieldSets_RuleVocabulary(t *testing.T) {
	vless, _ := FieldSet("vless")
	// REALITY 必须要求私钥/dest/SNI/ShortId
	required := map[string]bool{}
	for _, r := range vless.Rules {
		if r.Op == contracts.RuleRequires && len(r.When) == 1 &&
			r.When[0].Path == "stream.security" && r.When[0].Value == "reality" {
			required[r.Target] = true
		}
	}
	for _, path := range []string{
		"stream.realitySettings.privateKey",
		"stream.realitySettings.dest",
		"stream.realitySettings.serverNames",
		"stream.realitySettings.shortIds",
	} {
		if !required[path] {
			t.Errorf("REALITY 规则缺少 requires: %s", path)
		}
	}

	hy, _ := FieldSet("hysteria")
	// 服务端共享密码必须被禁止
	found := false
	for _, r := range hy.Rules {
		if r.Op == contracts.RuleForbids && r.Target == "stream.hysteriaSettings.auth" {
			found = true
		}
	}
	if !found {
		t.Error("Hysteria 2 必须禁止服务端 hysteriaSettings.auth")
	}
}

func pathDeclared(fs contracts.ProtocolFieldSet, path string) bool {
	for _, f := range fs.Fields {
		if f.Path == path || strings.HasPrefix(f.Path, path+".") {
			return true
		}
	}
	return false
}

func findDialect(fs contracts.ProtocolFieldSet, path string) (contracts.DialectMapping, bool) {
	for _, d := range fs.Dialects {
		if d.Path == path {
			return d, true
		}
	}
	return contracts.DialectMapping{}, false
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
