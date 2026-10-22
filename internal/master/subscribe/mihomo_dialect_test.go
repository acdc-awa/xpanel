package subscribe

import (
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/protocols"
)

// TestFormatClashNode_XHTTPAdvanced xray 的 xhttp 进阶参数必须落到 mihomo 的
// xhttp-opts，且 reuse-settings 不能依赖 mihomo 的「不写即不复用」默认。
func TestFormatClashNode_XHTTPAdvanced(t *testing.T) {
	dto := &contracts.ProxyNodeDTO{
		Name: "x", ServerHost: "cdn.example.com", ServerPort: 443, Protocol: "vless",
		Transport: &contracts.TransportOptions{
			Network: "xhttp", Mode: "stream-up", Path: "/xp", Host: "h.example.com",
			Opts: map[string]string{
				"xPaddingBytes":      "100-200",
				"xPaddingObfsMode":   "true",
				"sessionIDPlacement": "header",
				"sessionIDKey":       "X-Sess",
				"sessionIDLength":    "8-16",
				"scMaxEachPostBytes": "1000000",
				"noGRPCHeader":       "false",
				"headers":            `{"X-A":"b"}`,
				"xmux":               `{"maxConnections":"5","hKeepAlivePeriod":30}`,
			},
		},
		Security: &contracts.SecurityOptions{Type: "tls", SNI: "h.example.com", Fingerprint: "chrome"},
		Auth:     &contracts.ClientCredentialDTO{UUID: "11111111-1111-1111-1111-111111111111"},
	}

	line := formatClashNode(dto)
	for _, want := range []string{
		"xhttp-opts: {",
		"mode: 'stream-up'",
		"path: '/xp'",
		"host: 'h.example.com'",
		"x-padding-bytes: '100-200'",
		"x-padding-obfs-mode: true",
		"session-placement: 'header'",
		"session-key: 'X-Sess'",
		"session-length: '8-16'",
		"sc-max-each-post-bytes: '1000000'",
		"no-grpc-header: false",
		"headers: { 'X-A': 'b' }",
		"reuse-settings: { max-connections: '5', h-keep-alive-period: 30 }",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("缺少 %q\n---\n%s", want, line)
		}
	}
	// 显式给了 xmux 就不能再补默认值，也不能混淆 maxConcurrency/maxConnections。
	if strings.Contains(line, "h-max-request-times") {
		t.Errorf("显式 xmux 时不应补默认值:\n%s", line)
	}
}

// TestFormatClashNode_XHTTPDefaultReuse 反向验证：xmux 缺省时补 xray 的默认复用参数，
// 否则 mihomo 完全不复用，行为与 xray 服务端不一致。
func TestFormatClashNode_XHTTPDefaultReuse(t *testing.T) {
	dto := &contracts.ProxyNodeDTO{
		Name: "x", ServerHost: "h", ServerPort: 443, Protocol: "vless",
		Transport: &contracts.TransportOptions{Network: "xhttp", Mode: "auto", Path: "/"},
		Security:  &contracts.SecurityOptions{Type: "tls", Fingerprint: "chrome"},
		Auth:      &contracts.ClientCredentialDTO{UUID: "u"},
	}
	line := formatClashNode(dto)
	for _, want := range []string{
		"reuse-settings: { max-connections: '3', h-max-request-times: '600-900', h-max-reusable-secs: '1800-3000' }",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("缺少默认复用参数 %q:\n%s", want, line)
		}
	}
}

// TestFormatClashNode_XHTTPExtraEntry xray 的 xhttpSettings.extra 是第二套入口，
// 导出前必须展开（mihomo YAML 没有 extra 字段）。
func TestFormatClashNode_XHTTPExtraEntry(t *testing.T) {
	dto := &contracts.ProxyNodeDTO{
		Name: "x", ServerHost: "h", ServerPort: 443, Protocol: "vless",
		Transport: &contracts.TransportOptions{
			Network: "xhttp", Mode: "packet-up", Path: "/",
			Opts: map[string]string{
				"extra":                `{"xPaddingBytes":"50-60","sessionIDPlacement":"query","scMaxEachPostBytes":1000000,"host":"ignored.example.com"}`,
				"scMinPostsIntervalMs": "20",
			},
		},
		Security: &contracts.SecurityOptions{Type: "none"},
		Auth:     &contracts.ClientCredentialDTO{UUID: "u"},
	}
	line := formatClashNode(dto)
	for _, want := range []string{"x-padding-bytes: '50-60'", "session-placement: 'query'", "sc-min-posts-interval-ms: '20'", "sc-max-each-post-bytes: '1000000'"} {
		if !strings.Contains(line, want) {
			t.Errorf("extra 未展开，缺少 %q:\n%s", want, line)
		}
	}
	if strings.Contains(line, "ignored.example.com") {
		t.Errorf("extra 里的 host 应以外层为准（这里外层 host 为空）:\n%s", line)
	}
}

// TestFormatClashNode_VlessEncryption vlessenc 的加密串必须写进 YAML，
// 否则 Clash 侧退化成明文 VLESS（链接通道一直有，YAML 曾漏）。
func TestFormatClashNode_VlessEncryption(t *testing.T) {
	dto := &contracts.ProxyNodeDTO{
		Name: "v", ServerHost: "h", ServerPort: 443, Protocol: "vless",
		Transport: &contracts.TransportOptions{Network: "tcp"},
		Security:  &contracts.SecurityOptions{Type: "none"},
		Auth: &contracts.ClientCredentialDTO{
			UUID:       "u",
			Encryption: "mlkem768x25519plus.native.0rtt.key",
		},
	}
	line := formatClashNode(dto)
	if !strings.Contains(line, "encryption: mlkem768x25519plus.native.0rtt.key") {
		t.Errorf("缺少 encryption:\n%s", line)
	}
}

// TestFormatClashNode_HysteriaExtras 服务端 finalmask 的 obfs/带宽/端口跳跃
// 必须同步到订阅，否则「服务端开混淆、客户端不带」直接断连。
func TestFormatClashNode_HysteriaExtras(t *testing.T) {
	dto := &contracts.ProxyNodeDTO{
		Name: "hy", ServerHost: "hy.example.com", ServerPort: 44321, Protocol: "hysteria",
		Transport: &contracts.TransportOptions{Network: "hysteria", Opts: map[string]string{
			protocols.Hy2OptObfs:         "salamander",
			protocols.Hy2OptObfsPassword: "obspw",
			protocols.Hy2OptBwUp:         "100 mbps",
			protocols.Hy2OptBwDown:       "50 mbps",
			protocols.Hy2OptPorts:        "20000-30000",
			protocols.Hy2OptHopInterval:  "10-20",
		}},
		Security: &contracts.SecurityOptions{Type: "tls", SNI: "hy.example.com"},
		Auth:     &contracts.ClientCredentialDTO{Password: "secret"},
	}
	line := formatClashNode(dto)
	for _, want := range []string{
		"obfs: salamander",
		"obfs-password: 'obspw'",
		"up: '100 Mbps'",
		"down: '50 Mbps'",
		"ports: '20000-30000'",
		"hop-interval: '10-20'",
		"alpn: [h3]",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("缺少 %q\n---\n%s", want, line)
		}
	}
	// hysteria2 的 fingerprint 是证书 pin，任何情况下都不许输出浏览器指纹名。
	if strings.Contains(line, "fingerprint") {
		t.Errorf("hysteria2 不应输出 fingerprint:\n%s", line)
	}
}
