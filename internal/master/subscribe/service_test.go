package subscribe

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/zhx/xray-panel/internal/master/xray"
	"github.com/zhx/xray-panel/internal/models"
)

func testUser() *models.User {
	return &models.User{ID: 1, UUID: "11111111-1111-1111-1111-111111111111"}
}

// A-G 差距回归（03 号文档 §4）：
// A tcp+tls+vision 缺 flow · B xhttp 缺 alpn:[h2] · C tls 缺 servername ·
// D xhttp 缺 host · E 缺 skip-cert-verify 透传 · F url-test 用 cloudflare ·
// G BuildBase64 同步（tcp+tls+flow / xhttp host / ws+tls 无 flow）
func TestBuildClash_Gaps(t *testing.T) {
	items := []ProxyItem{
		{
			Name: "tcp-reality", Host: "r.example.com", Port: 443,
			UUID: "uuid-r", Network: "tcp", TLSType: "reality",
			Reality: &xray.RealitySettings{ServerName: "r.example.com", PublicKey: "pbk123", ShortID: "abcd"},
		},
		{
			Name: "tcp-tls-vision", Host: "t.example.com", Port: 443,
			UUID: "uuid-t", Network: "tcp", TLSType: "tls", Flow: "xtls-rprx-vision",
			TLS: &xray.TLSSettings{ServerName: "t.example.com", AllowInsecure: true},
		},
		{
			Name: "ws-tls", Host: "w.example.com", Port: 8443,
			UUID: "uuid-w", Network: "ws", TLSType: "tls",
			TLS: &xray.TLSSettings{ServerName: "w.example.com"},
			WS:  &xray.WSSettings{Path: "/ws", Host: "cdn.example.com"},
		},
		{
			Name: "xhttp-reality", Host: "x.example.com", Port: 443,
			UUID: "uuid-x", Network: "xhttp", TLSType: "reality",
			Reality: &xray.RealitySettings{ServerName: "x.example.com", PublicKey: "pbk456", ShortID: "ef01"},
			XHTTP:   &xray.XHTTPSettings{Mode: "auto", Path: "/xp", Host: "h.example.com"},
		},
		{
			Name: "broken-reality", Host: "b.example.com", Port: 443,
			UUID: "uuid-b", Network: "tcp", TLSType: "reality", // Reality=nil 应跳过不崩溃
		},
	}

	yaml := BuildClash(testUser(), items)

	for _, want := range []string{
		"flow: xtls-rprx-vision",        // reality+tcp 自动 vision
		"public-key: pbk123",            // reality 公钥
		"servername: t.example.com",     // C tls servername
		"skip-cert-verify: true",        // E allowInsecure 透传
		"ws-opts:",                      // ws 保留
		"alpn: [h2]",                    // B xhttp 显式 alpn
		"host: h.example.com",           // D xhttp host
		"cp.cloudflare.com/generate_204", // F url-test 地址
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("BuildClash 缺少 %q\n---\n%s", want, yaml)
		}
	}
	if strings.Contains(yaml, "broken-reality") {
		t.Error("Reality=nil 的节点应被跳过")
	}
	if strings.Contains(yaml, "flow:") && !strings.Contains(yaml, "tcp-reality") && !strings.Contains(yaml, "tcp-tls-vision") {
		t.Error("flow 不应出现在 ws 节点")
	}
	// ws-tls 节点（无 flow 覆盖）不应输出 flow
	wsIdx := strings.Index(yaml, `"ws-tls"`)
	if wsIdx >= 0 {
		rest := yaml[wsIdx:]
		if idx := strings.Index(rest, "\n  - name"); idx > 0 {
			rest = rest[:idx]
		}
		if strings.Contains(rest, "flow:") {
			t.Error("ws+tls 节点不应输出 flow")
		}
	}
}

func TestBuildBase64_Gaps(t *testing.T) {
	items := []ProxyItem{
		{
			Name: "tcp-tls-vision", Host: "t.example.com", Port: 443,
			UUID: "uuid-t", Network: "tcp", TLSType: "tls", Flow: "xtls-rprx-vision",
			TLS: &xray.TLSSettings{ServerName: "t.example.com", AllowInsecure: true},
		},
		{
			Name: "xhttp-host", Host: "x.example.com", Port: 443,
			UUID: "uuid-x", Network: "xhttp", TLSType: "reality",
			Reality: &xray.RealitySettings{ServerName: "x.example.com", PublicKey: "pbk456", ShortID: "ef01"},
			XHTTP:   &xray.XHTTPSettings{Mode: "auto", Path: "/xp", Host: "h.example.com"},
		},
		{
			Name: "ws-tls", Host: "w.example.com", Port: 8443,
			UUID: "uuid-w", Network: "ws", TLSType: "tls",
			WS: &xray.WSSettings{Path: "/ws", Host: "cdn.example.com"},
		},
	}
	b64 := BuildBase64(testUser(), items)
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("Base64 解码失败: %v", err)
	}
	links := strings.Split(string(raw), "\n")
	if len(links) != 3 {
		t.Fatalf("期望 3 条链接，实际 %d: %s", len(links), raw)
	}
	for _, want := range []string{
		"flow=xtls-rprx-vision", // A/G tcp+tls+vision 缺 flow
		"sni=t.example.com",     // C tls servername
		"allowInsecure=1",       // E
		"host=h.example.com",    // D/G xhttp host
		"type=ws",               // ws 保留
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("BuildBase64 缺少 %q\n---\n%s", want, raw)
		}
	}
	// ws+tls 不应有 flow
	for _, l := range links {
		if strings.Contains(l, "ws-tls") && strings.Contains(l, "flow=") {
			t.Errorf("ws+tls 链接不应带 flow: %s", l)
		}
	}
}

func TestBuildClash_SkipsBrokenReality(t *testing.T) {
	items := []ProxyItem{
		{Name: "ok", Host: "a.com", Port: 443, UUID: "u", Network: "tcp", TLSType: "reality",
			Reality: &xray.RealitySettings{ServerName: "a.com", PublicKey: "pk", ShortID: "sid"}},
		{Name: "broken", Host: "b.com", Port: 443, UUID: "u2", Network: "tcp", TLSType: "reality"},
	}
	yaml := BuildClash(testUser(), items)
	if strings.Contains(yaml, "broken") {
		t.Errorf("broken 节点不应输出: %s", yaml)
	}
	b64 := BuildBase64(testUser(), items)
	raw, _ := base64.StdEncoding.DecodeString(b64)
	if strings.Contains(string(raw), "u2@") {
		t.Errorf("broken 节点不应输出链接: %s", raw)
	}
}
