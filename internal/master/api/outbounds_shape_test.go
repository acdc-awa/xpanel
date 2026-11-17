package api

import "testing"

// 基础出口的 streamSettings 白名单：唯一允许的是 freedom 的域名解析策略
// （xray v26.9.8 起 settings.domainStrategy 废弃，落点只能是 streamSettings.sockopt.domainStrategy）。
func TestNormalizeOutboundShape_FreedomSockopt(t *testing.T) {
	const okSettings = `{"finalRules":[{"action":"allow"}]}`

	cases := []struct {
		name    string
		proto   string
		stream  string
		wantErr bool
	}{
		{"freedom 只带解析策略", "freedom", `{"sockopt":{"domainStrategy":"ForceIPv4"}}`, false},
		{"freedom 小写取值", "freedom", `{"sockopt":{"domainStrategy":"useipv4"}}`, false},
		{"freedom 无 streamSettings", "freedom", "", false},
		{"freedom 夹带其它 sockopt 键", "freedom", `{"sockopt":{"domainStrategy":"ForceIPv4","mark":255}}`, true},
		{"freedom 夹带传输层", "freedom", `{"network":"tcp"}`, true},
		{"freedom 未知策略", "freedom", `{"sockopt":{"domainStrategy":"UseIPv5"}}`, true},
		{"freedom streamSettings 非法 JSON", "freedom", `{`, true},
		{"blackhole 仍不接受 streamSettings", "blackhole", `{"sockopt":{"domainStrategy":"ForceIPv4"}}`, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := outboundShape{Protocol: c.proto, SettingsJSON: okSettings, StreamJSON: c.stream}
			_, msg := normalizeOutboundShape(in)
			if c.wantErr && msg == "" {
				t.Fatalf("应被拒绝，但通过了")
			}
			if !c.wantErr && msg != "" {
				t.Fatalf("应通过，但被拒绝: %s", msg)
			}
		})
	}
}
