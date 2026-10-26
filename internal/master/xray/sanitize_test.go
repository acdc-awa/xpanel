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
					t.Error("sessionKey should be lifted to sessionIDKey")
				}
				if v, _ := x["sessionIDPlacement"].(string); v != "middle" {
					t.Errorf("sessionIDPlacement=%q, want middle", v)
				}
				if v, _ := x["sessionIDKey"].(string); v != "mykey" {
					t.Errorf("sessionIDKey=%q, want mykey", v)
				}
				if x["path"] != "/x" {
					t.Error("unrelated keys should be preserved")
				}
			},
		},
		{
			name: "force hysteria alpn to h3",
			in:   `{"network":"hysteria","security":"tls","tlsSettings":{"serverName":"hy.example.com","alpn":["h2","http/1.1"],"minVersion":"1.2","maxVersion":"1.2","cipherSuites":"TLS_AES_128_GCM_SHA256"}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				tls := m["tlsSettings"].(map[string]any)
				alpn, _ := tls["alpn"].([]any)
				if len(alpn) != 1 || alpn[0] != "h3" {
					t.Errorf("hysteria alpn 必须强制为 [h3]，got %v", tls["alpn"])
				}
				for _, k := range []string{"minVersion", "maxVersion", "cipherSuites"} {
					if _, ok := tls[k]; ok {
						t.Errorf("hysteria 的 %s 对 QUIC 无意义且可能致命，应删除", k)
					}
				}
				if tls["serverName"] != "hy.example.com" {
					t.Error("其它 tlsSettings 字段应保留")
				}
			},
		},
		{
			name: "force hysteria alpn with method alias",
			in:   `{"method":"hysteria","security":"tls","tlsSettings":{"alpn":["h2"]}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				tls := m["tlsSettings"].(map[string]any)
				alpn, _ := tls["alpn"].([]any)
				if len(alpn) != 1 || alpn[0] != "h3" {
					t.Errorf("method=hysteria 时 alpn 也必须强制为 [h3]，got %v", tls["alpn"])
				}
			},
		},
		{
			name: "keep non-hysteria alpn",
			in:   `{"network":"tcp","security":"tls","tlsSettings":{"alpn":["h2","http/1.1"],"minVersion":"1.2","maxVersion":"1.3","cipherSuites":"TLS_AES_128_GCM_SHA256"}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				tls := m["tlsSettings"].(map[string]any)
				alpn, _ := tls["alpn"].([]any)
				if len(alpn) != 2 || alpn[0] != "h2" {
					t.Errorf("非 hysteria 传输的 alpn 不应被改动，got %v", tls["alpn"])
				}
				if tls["minVersion"] != "1.2" || tls["maxVersion"] != "1.3" {
					t.Error("非 hysteria 传输的 TLS 版本范围不应被删除")
				}
				if tls["cipherSuites"] != "TLS_AES_128_GCM_SHA256" {
					t.Error("非 hysteria 传输的 cipherSuites 不应被删除")
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

// 入站生成时剥离 udphop（仅客户端实现）；出站侧必须保留，否则端口跳跃失效。
func TestStripInboundClientOnlyMasks(t *testing.T) {
	stream := map[string]any{
		"security": "tls",
		"finalmask": map[string]any{
			"udp": []any{
				map[string]any{"type": "salamander", "settings": map[string]any{"password": "pw"}},
				map[string]any{"type": "udphop", "settings": map[string]any{"mode": "intervalRemote", "remotePorts": "20000-30000"}},
			},
		},
	}
	stripInboundClientOnlyMasks(stream)
	udp := stream["finalmask"].(map[string]any)["udp"].([]any)
	if len(udp) != 1 || udp[0].(map[string]any)["type"] != "salamander" {
		t.Fatalf("udphop 应被剥离、salamander 保留，got %v", udp)
	}

	// 只有 udphop 时删掉整个 udp 键。
	onlyHop := map[string]any{
		"finalmask": map[string]any{
			"udp": []any{map[string]any{"type": "udphop", "settings": map[string]any{"mode": "intervalRemote"}}},
		},
	}
	stripInboundClientOnlyMasks(onlyHop)
	if _, ok := onlyHop["finalmask"].(map[string]any)["udp"]; ok {
		t.Fatal("空 udp 应删除")
	}

	// 反向验证：通用 sanitizeStreamSettings 是入站/出站共用的，不能在这里剥离。
	raw := `{"network":"hysteria","security":"tls","finalmask":{"udp":[{"type":"udphop","settings":{"mode":"intervalRemote","remotePorts":"20000-30000"}}]}}`
	var m map[string]any
	if err := json.Unmarshal([]byte(sanitizeStreamSettings(raw)), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["finalmask"].(map[string]any)["udp"]; !ok {
		t.Fatal("sanitizeStreamSettings 共用于出站，不得剥离 udphop")
	}
}

// 入站生成时剥离 xhttpSettings 里只被客户端 dialer 消费的字段；
// 服务端实际消费的键必须保留。
func TestStripInboundClientOnlyXHTTP(t *testing.T) {
	stream := map[string]any{
		"network":  "xhttp",
		"security": "tls",
		"xhttpSettings": map[string]any{
			"path":                 "/stream",
			"mode":                 "auto",
			"host":                 "example.com",
			"xmux":                 map[string]any{"maxConnections": "3"},
			"downloadSettings":     map[string]any{"network": "xhttp"},
			"noGRPCHeader":         true,
			"uplinkHTTPMethod":     "PUT",
			"uplinkChunkSize":      "4096-8192",
			"scMinPostsIntervalMs": "10-20",
			"scMaxEachPostBytes":   "1000000",
			"scMaxBufferedPosts":   30,
			"scStreamUpServerSecs": "20-80",
			"serverMaxHeaderBytes": 4096,
			"sessionIDPlacement":   "header",
			"uplinkDataPlacement":  "body",
		},
	}
	stripInboundClientOnlyXHTTP(stream)
	x := stream["xhttpSettings"].(map[string]any)
	for _, k := range []string{"xmux", "downloadSettings", "noGRPCHeader", "uplinkHTTPMethod", "uplinkChunkSize", "scMinPostsIntervalMs"} {
		if _, ok := x[k]; ok {
			t.Errorf("client-only xhttp key %q should be stripped", k)
		}
	}
	for _, k := range []string{"path", "mode", "host", "scMaxEachPostBytes", "scMaxBufferedPosts", "scStreamUpServerSecs", "serverMaxHeaderBytes", "sessionIDPlacement", "uplinkDataPlacement"} {
		if _, ok := x[k]; !ok {
			t.Errorf("server-consumed xhttp key %q should be kept", k)
		}
	}
}

// 入站生成时剥离 REALITY 的客户端侧字段；服务端字段必须保留。
func TestStripInboundClientOnlyReality(t *testing.T) {
	stream := map[string]any{
		"realitySettings": map[string]any{
			"privateKey":    "sk",
			"dest":          "example.com:443",
			"serverNames":   []any{"example.com"},
			"shortIds":      []any{"abcd"},
			"publicKey":     "pk",
			"password":      "pk",
			"spiderX":       "/",
			"serverName":    "example.com",
			"shortId":       "abcd",
			"fingerprint":   "chrome",
			"mldsa65Verify": "verify",
		},
	}
	stripInboundClientOnlyReality(stream)
	r := stream["realitySettings"].(map[string]any)
	for _, k := range []string{"publicKey", "password", "spiderX", "serverName", "shortId", "fingerprint", "mldsa65Verify"} {
		if _, ok := r[k]; ok {
			t.Errorf("client-only reality key %q should be stripped", k)
		}
	}
	for _, k := range []string{"privateKey", "dest", "serverNames", "shortIds"} {
		if _, ok := r[k]; !ok {
			t.Errorf("server-consumed reality key %q should be kept", k)
		}
	}
}

// TLS 的客户端角色字段（uTLS 指纹 / ECH 客户端配置 / 系统根池开关）不得进入服务端配置。
func TestStripInboundClientOnlyTLS_ClientOnlyExtras(t *testing.T) {
	stream := map[string]any{
		"tlsSettings": map[string]any{
			"fingerprint":      "chrome",
			"echConfigList":    "AEX...",
			"useSystemCA":      true,
			"alpn":             []any{"h2"},
			"rejectUnknownSni": true,
			"minVersion":       "1.2",
		},
	}
	stripInboundClientOnlyTLS(stream)
	tls := stream["tlsSettings"].(map[string]any)
	for _, k := range []string{"fingerprint", "echConfigList", "useSystemCA"} {
		if _, ok := tls[k]; ok {
			t.Errorf("client-only tls key %q should be stripped", k)
		}
	}
	for _, k := range []string{"alpn", "rejectUnknownSni", "minVersion"} {
		if _, ok := tls[k]; !ok {
			t.Errorf("server-consumed tls key %q should be kept", k)
		}
	}
}

// 出站是客户端角色：清洗时不得删除 fingerprint/publicKey/spiderX/hysteria auth。
func TestSanitizeOutboundStreamMap_KeepsClientFields(t *testing.T) {
	raw := `{
		"network":"hysteria",
		"security":"tls",
		"tlsSettings":{
			"serverName":"example.com",
			"allowInsecure":true,
			"fingerprint":"chrome",
			"pinnedPeerCertSha256":"abc",
			"disableSystemRoot":true,
			"settings":{"panel":1},
			"certificates":[{"certificateFile":"/ca.pem","usage":"verify"}]
		},
		"hysteriaSettings":{"version":2,"auth":"secret"},
		"externalProxy":{"tag":"x"},
		"fingerprint":"top-level",
		"finalmask":{"udp":[{"type":"udphop","ports":"1000-2000"}]}
	}`
	out := SanitizeOutboundStreamSettings(raw)
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("sanitized output is not JSON: %v (%s)", err, out)
	}
	if _, ok := m["externalProxy"]; ok {
		t.Error("externalProxy should be deleted")
	}
	if _, ok := m["fingerprint"]; ok {
		t.Error("top-level fingerprint should be deleted")
	}
	tls := m["tlsSettings"].(map[string]any)
	if _, ok := tls["allowInsecure"]; ok {
		t.Error("tlsSettings.allowInsecure was removed by xray and must be stripped")
	}
	if _, ok := tls["settings"]; ok {
		t.Error("panel-nested tlsSettings.settings should be deleted")
	}
	for _, k := range []string{"fingerprint", "pinnedPeerCertSha256", "disableSystemRoot", "certificates"} {
		if _, ok := tls[k]; !ok {
			t.Errorf("client TLS key %q should be kept", k)
		}
	}
	hy := m["hysteriaSettings"].(map[string]any)
	if got, _ := hy["auth"].(string); got != "secret" {
		t.Errorf("hysteria auth should be kept, got %q", got)
	}
	fm := m["finalmask"].(map[string]any)
	if _, ok := fm["udp"]; !ok {
		t.Error("hysteria finalmask udp (client capability) should be kept")
	}
}

// 出站 REALITY 必须保留客户端字段；REALITY + finalmask 仍按 panic 防护剔除。
func TestSanitizeOutboundStreamMap_Reality(t *testing.T) {
	raw := `{
		"network":"tcp",
		"security":"reality",
		"realitySettings":{
			"serverName":"example.com",
			"publicKey":"pk",
			"password":"pw",
			"shortId":"0123",
			"fingerprint":"chrome",
			"spiderX":"/",
			"settings":{"panel":1}
		},
		"finalmask":{"tcp":[{"type":"noise"}]}
	}`
	out := SanitizeOutboundStreamSettings(raw)
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("sanitized output is not JSON: %v", err)
	}
	r := m["realitySettings"].(map[string]any)
	for _, k := range []string{"publicKey", "password", "spiderX", "fingerprint", "shortId", "serverName"} {
		if _, ok := r[k]; !ok {
			t.Errorf("client reality key %q should be kept", k)
		}
	}
	if _, ok := r["settings"]; ok {
		t.Error("panel-nested realitySettings.settings should be deleted")
	}
	if _, ok := m["finalmask"]; ok {
		t.Error("finalmask must be dropped for REALITY")
	}
}

// 出站 stream 的通用归一：cipherSuites 数组转字符串、xhttp 旧 session 键提升。
func TestSanitizeOutboundStreamMap_Normalize(t *testing.T) {
	raw := `{
		"network":"xhttp",
		"security":"tls",
		"tlsSettings":{"cipherSuites":["TLS_AES_128_GCM_SHA256","TLS_AES_256_GCM_SHA384"]},
		"xhttpSettings":{"sessionPlacement":"path","sessionKey":"sid","mode":"auto"}
	}`
	out := SanitizeOutboundStreamSettings(raw)
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("sanitized output is not JSON: %v", err)
	}
	tls := m["tlsSettings"].(map[string]any)
	if got, _ := tls["cipherSuites"].(string); got != "TLS_AES_128_GCM_SHA256:TLS_AES_256_GCM_SHA384" {
		t.Errorf("cipherSuites should be normalized to colon string, got %#v", tls["cipherSuites"])
	}
	x := m["xhttpSettings"].(map[string]any)
	if _, ok := x["sessionPlacement"]; ok {
		t.Error("old sessionPlacement should be lifted")
	}
	if got, _ := x["sessionIDPlacement"].(string); got != "path" {
		t.Errorf("sessionIDPlacement = %q, want path", got)
	}
	if _, ok := x["sessionKey"]; ok {
		t.Error("old sessionKey should be lifted")
	}
	if got, _ := x["sessionIDKey"].(string); got != "sid" {
		t.Errorf("sessionIDKey = %q, want sid", got)
	}
}
