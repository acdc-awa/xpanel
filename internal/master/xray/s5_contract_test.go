package xray_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/protocols"
	"github.com/acdc-awa/xpanel/internal/master/xray"
)

// S5 契约测试：生成的服务端入站字段必须全部能在字段声明里解释。
// 出现未声明字段 = 界面无法解释 / 改协议时容易漏改，直接失败。
func TestGeneratedInboundFieldsAreDeclared(t *testing.T) {
	for _, sc := range s1Scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			raw, err := xray.Generate(sc.inbounds, sc.outbounds, sc.rules, sc.users, sc.ctx, "", "")
			if err != nil {
				t.Fatalf("Generate 失败: %v", err)
			}
			var root struct {
				Inbounds []map[string]any `json:"inbounds"`
			}
			if err := json.Unmarshal(raw, &root); err != nil {
				t.Fatalf("解析生成配置失败: %v", err)
			}
			for _, in := range root.Inbounds {
				tag, _ := in["tag"].(string)
				if tag == "api" {
					continue // 内核内部入站，不属于面板协议声明
				}
				proto, _ := in["protocol"].(string)
				fs, ok := protocols.FieldSet(proto)
				if !ok {
					t.Fatalf("入站 %s 的协议 %q 缺少字段声明", tag, proto)
				}
				for layer, prefix := range map[string]string{
					"settings":       "settings",
					"streamSettings": "stream",
					"sniffing":       "sniffing",
				} {
					m, ok := in[layer].(map[string]any)
					if !ok {
						continue
					}
					var undeclared []string
					walkLeaves(prefix, m, func(path string) {
						if !declaredOrContainer(fs, path) {
							undeclared = append(undeclared, path)
						}
					})
					if len(undeclared) > 0 {
						t.Errorf("入站 %s (%s) 出现未声明字段: %s", tag, proto, strings.Join(undeclared, ", "))
					}
				}
			}
		})
	}
}

// walkLeaves 遍历 map 叶子；数组不展开（字段声明按数组整体登记）。
func walkLeaves(prefix string, v any, fn func(path string)) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	for k, child := range m {
		path := prefix + "." + k
		if _, isMap := child.(map[string]any); isMap {
			walkLeaves(path, child, fn)
			continue
		}
		fn(path)
	}
}

// declaredOrContainer 路径被声明，或位于已声明对象/映射字段之下。
func declaredOrContainer(fs contracts.ProtocolFieldSet, path string) bool {
	for _, f := range fs.Fields {
		if f.Path == path {
			return true
		}
		if f.Shape == contracts.ShapeObject || f.Shape == contracts.ShapeMap {
			if strings.HasPrefix(path, f.Path+".") {
				return true
			}
		}
	}
	return false
}

// 反向验证：检查器必须能识别未声明字段，否则上面的契约测试只是假绿。
func TestDeclaredOrContainer_RejectsUndeclared(t *testing.T) {
	fs, ok := protocols.FieldSet("vless")
	if !ok {
		t.Fatal("vless 缺少字段声明")
	}
	if !declaredOrContainer(fs, "stream.xhttpSettings.sessionID") {
		t.Error("已声明的字段被误判为未声明")
	}
	if declaredOrContainer(fs, "stream.xhttpSettings.notARealField") {
		t.Error("未声明字段未被识别（契约测试会假绿）")
	}
}
