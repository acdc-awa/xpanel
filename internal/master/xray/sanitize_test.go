package xray

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeStreamSettings(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want func(t *testing.T, out string)
	}{
		{
			name: "empty passthrough",
			in:   "",
			want: func(t *testing.T, out string) {
				if out != "" {
					t.Errorf("empty should stay empty, got %q", out)
				}
			},
		},
		{
			name: "invalid JSON passthrough",
			in:   `{not json`,
			want: func(t *testing.T, out string) {
				if out != `{not json` {
					t.Errorf("invalid JSON should be returned as-is")
				}
			},
		},
		{
			name: "delete externalProxy",
			in:   `{"network":"tcp","externalProxy":{"tag":"proxy"}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				if _, ok := m["externalProxy"]; ok {
					t.Error("externalProxy should be deleted")
				}
			},
		},
		{
			name: "delete client-only fields (fingerprint/publicKey/spiderX)",
			in:   `{"network":"tcp","security":"reality","fingerprint":"chrome","realitySettings":{"dest":"a.com:443","privateKey":"k","publicKey":"pub","spiderX":"/","serverNames":["a.com"],"shortIds":["ab"]}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				if _, ok := m["fingerprint"]; ok {
					t.Error("顶层 fingerprint（客户端字段）应删除")
				}
				r := m["realitySettings"].(map[string]any)
				if _, ok := r["publicKey"]; ok {
					t.Error("realitySettings.publicKey（客户端字段）应删除")
				}
				if _, ok := r["spiderX"]; ok {
					t.Error("realitySettings.spiderX（客户端字段）应删除")
				}
				if r["dest"] != "a.com:443" || r["privateKey"] != "k" {
					t.Error("服务端字段 dest/privateKey 应保留")
				}
			},
		},
		{
			name: "delete tlsSettings.settings",
			in:   `{"security":"tls","tlsSettings":{"serverName":"x.com","settings":{"allowInsecure":true}}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				tls := m["tlsSettings"].(map[string]any)
				if _, ok := tls["settings"]; ok {
					t.Error("tlsSettings.settings should be deleted")
				}
				if tls["serverName"] != "x.com" {
					t.Error("tlsSettings.serverName should be preserved")
				}
			},
		},
		{
			name: "delete tlsSettings.allowInsecure (v26.6.27 removed)",
			in:   `{"security":"tls","tlsSettings":{"serverName":"x.com","allowInsecure":true,"certificates":[{"certificateFile":"/c.pem","keyFile":"/k.pem"}]}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				tls := m["tlsSettings"].(map[string]any)
				if _, ok := tls["allowInsecure"]; ok {
					t.Error("tlsSettings.allowInsecure should be deleted (xray 拒绝加载)")
				}
				if tls["serverName"] != "x.com" {
					t.Error("tlsSettings.serverName should be preserved")
				}
			},
		},
		{
			name: "keep top-level allowInsecure",
			in:   `{"security":"tls","allowInsecure":true,"tlsSettings":{"serverName":"x.com"}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				if _, ok := m["allowInsecure"]; !ok {
					t.Error("顶层 allowInsecure xray 静默忽略，不应删（仅删 tlsSettings 内）")
				}
			},
		},
		{
			name: "delete realitySettings.settings",
			in:   `{"security":"reality","realitySettings":{"serverName":"apple.com","settings":{"publicKey":"pk"}}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				r := m["realitySettings"].(map[string]any)
				if _, ok := r["settings"]; ok {
					t.Error("realitySettings.settings should be deleted")
				}
			},
		},
		{
			name: "drop finalmask on reality",
			in:   `{"security":"reality","finalmask":{"rand":[{"length":64}]}}`,
			want: func(t *testing.T, out string) {
				if strings.Contains(out, "finalmask") {
					t.Error("finalmask should be deleted when security=reality")
				}
			},
		},
		{
			name: "keep finalmask on non-reality",
			in:   `{"security":"tls","finalmask":{"rand":[{"length":64}]}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				if _, ok := m["finalmask"]; !ok {
					t.Error("finalmask should be kept when security!=reality")
				}
			},
		},
		{
			name: "drop empty rand packets",
			in:   `{"finalmask":{"rand":[{},{"length":64}]}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				fm := m["finalmask"].(map[string]any)
				rand := fm["rand"].([]any)
				if len(rand) != 1 {
					t.Errorf("empty rand should be dropped, got %d items", len(rand))
				}
			},
		},
		{
			name: "lift xhttp session keys",
			in:   `{"xhttpSettings":{"mode":"stream-up","sessionPlacement":"middle","sessionKey":"mykey","path":"/x"}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				x := m["xhttpSettings"].(map[string]any)
				if _, ok := x["sessionPlacement"]; ok {
					t.Error("sessionPlacement should be lifted to sessionIDPlacement")
				}
				if _, ok := x["sessionKey"]; ok {
					t.Error("sessionKey should be lifted to sessionID")
				}
				if v, _ := x["sessionIDPlacement"].(string); v != "middle" {
					t.Errorf("sessionIDPlacement=%q, want middle", v)
				}
				if v, _ := x["sessionID"].(string); v != "mykey" {
					t.Errorf("sessionID=%q, want mykey", v)
				}
				if x["path"] != "/x" {
					t.Error("unrelated keys should be preserved")
				}
			},
		},
		{
			name: "full sanitize - all rules together",
			in: `{
				"network":"tcp",
				"security":"reality",
				"externalProxy":{"tag":"x"},
				"realitySettings":{"serverName":"a.com","settings":{"foo":1}},
				"finalmask":{"rand":[{},{"length":32}]},
				"xhttpSettings":{"sessionPlacement":"front"}
			}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(out), &m); err != nil {
					t.Fatalf("output should be valid JSON: %v", err)
				}
				// externalProxy removed
				if _, ok := m["externalProxy"]; ok {
					t.Error("externalProxy should be deleted")
				}
				// realitySettings.settings removed
				r := m["realitySettings"].(map[string]any)
				if _, ok := r["settings"]; ok {
					t.Error("realitySettings.settings should be deleted")
				}
				// finalmask removed (reality)
				if _, ok := m["finalmask"]; ok {
					t.Error("finalmask should be deleted when security=reality")
				}
				// xhttp keys lifted
				x := m["xhttpSettings"].(map[string]any)
				if _, ok := x["sessionPlacement"]; ok {
					t.Error("sessionPlacement should be lifted")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeStreamSettings(tt.in)
			tt.want(t, got)
		})
	}
}

// xray 的 cipherSuites 是 ":" 分隔字符串；历史前端按数组提交会整份配置反序列化失败。
func TestSanitizeCipherSuites(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want any // nil = 字段应被删除
	}{
		{
			name: "array joined with colon",
			in:   `{"security":"tls","tlsSettings":{"cipherSuites":["TLS_AES_128_GCM_SHA256","TLS_AES_256_GCM_SHA384"]}}`,
			want: "TLS_AES_128_GCM_SHA256:TLS_AES_256_GCM_SHA384",
		},
		{
			name: "string passthrough",
			in:   `{"security":"tls","tlsSettings":{"cipherSuites":"TLS_AES_128_GCM_SHA256"}}`,
			want: "TLS_AES_128_GCM_SHA256",
		},
		{
			name: "empty array dropped",
			in:   `{"security":"tls","tlsSettings":{"cipherSuites":[]}}`,
			want: nil,
		},
		{
			name: "blank string dropped",
			in:   `{"security":"tls","tlsSettings":{"cipherSuites":"  "}}`,
			want: nil,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]any
			if err := json.Unmarshal([]byte(sanitizeStreamSettings(tt.in)), &m); err != nil {
				t.Fatalf("output should be valid JSON: %v", err)
			}
			tls, _ := m["tlsSettings"].(map[string]any)
			got, ok := tls["cipherSuites"]
			if tt.want == nil {
				if ok {
					t.Fatalf("cipherSuites should be dropped, got %v", got)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("cipherSuites = %v, want %v", got, tt.want)
			}
		})
	}
}

// 入站生成时剥离客户端专用的 TLS 校验字段与 usage=verify 证书（DB 原文保留）。
func TestStripInboundClientOnlyTLS(t *testing.T) {
	stream := map[string]any{
		"security": "tls",
		"tlsSettings": map[string]any{
			"disableSystemRoot":    true,
			"pinnedPeerCertSha256": "abc",
			"verifyPeerCertByName": "a.com",
			"certificates": []any{
				map[string]any{"usage": "verify", "certificateFile": "/ca.crt"},
				map[string]any{"usage": "encipherment", "certificateFile": "/srv.crt", "keyFile": "/srv.key"},
			},
		},
	}
	stripInboundClientOnlyTLS(stream)
	tls := stream["tlsSettings"].(map[string]any)
	for _, k := range []string{"disableSystemRoot", "pinnedPeerCertSha256", "verifyPeerCertByName"} {
		if _, ok := tls[k]; ok {
			t.Fatalf("%s should be stripped", k)
		}
	}
	certs := tls["certificates"].([]any)
	if len(certs) != 1 {
		t.Fatalf("expected 1 cert after strip, got %d", len(certs))
	}
	if certs[0].(map[string]any)["usage"] != "encipherment" {
		t.Fatalf("wrong cert kept: %v", certs[0])
	}

	// 只有 verify 项时整个 certificates 删除。
	onlyVerify := map[string]any{
		"tlsSettings": map[string]any{
			"certificates": []any{map[string]any{"usage": "verify", "certificateFile": "/ca.crt"}},
		},
	}
	stripInboundClientOnlyTLS(onlyVerify)
	if _, ok := onlyVerify["tlsSettings"].(map[string]any)["certificates"]; ok {
		t.Fatal("certificates should be deleted when only verify entries exist")
	}
}
