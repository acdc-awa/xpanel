package util

import (
	"net/http"
	"testing"
)

// trust 在测试内设置可信代理，结束后清空（不信任任何转发头）。
func trust(t *testing.T, entries ...string) {
	t.Helper()
	if err := SetTrustedProxies(entries); err != nil {
		t.Fatalf("SetTrustedProxies(%v): %v", entries, err)
	}
	t.Cleanup(func() { _ = SetTrustedProxies(nil) })
}

func newIPRequest(remoteAddr string, headers map[string]string) *http.Request {
	req, _ := http.NewRequest("GET", "http://example.com/api/v1/test", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.RemoteAddr = remoteAddr
	return req
}

func TestGetRealIP(t *testing.T) {
	tests := []struct {
		name       string
		trusted    []string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			// 直连对端不在可信集合：转发头全部忽略，防公网伪造
			name:    "untrusted peer ignores forwarded headers",
			trusted: []string{"127.0.0.1/32"},
			headers: map[string]string{
				"CF-Connecting-IP": "104.28.19.45",
				"X-Real-IP":        "172.68.1.1",
				"X-Forwarded-For":  "203.0.113.9, 10.0.0.1",
			},
			remoteAddr: "198.51.100.7:54321",
			expected:   "198.51.100.7",
		},
		{
			name:       "empty trust list ignores forwarded headers",
			trusted:    nil,
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.9"},
			remoteAddr: "127.0.0.1:54321",
			expected:   "127.0.0.1",
		},
		{
			// 可信反代写入/追加的 XFF：从右向左第一个不可信地址即客户端
			name:       "trusted peer takes rightmost untrusted XFF entry",
			trusted:    []string{"127.0.0.1/32", "::1/128"},
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.22, 127.0.0.1"},
			remoteAddr: "127.0.0.1:54321",
			expected:   "198.51.100.22",
		},
		{
			// 客户端在左侧塞入伪造条目也不影响：右侧真实跳数优先
			name:       "spoofed leftmost entries are ignored",
			trusted:    []string{"127.0.0.1/32"},
			headers:    map[string]string{"X-Forwarded-For": "1.2.3.4, 203.0.113.195, 127.0.0.1"},
			remoteAddr: "127.0.0.1:54321",
			expected:   "203.0.113.195",
		},
		{
			// 多级可信代理（如 CF 回源 + Caddy）：整条链可信时取最左
			name:       "all-trusted chain returns leftmost",
			trusted:    []string{"127.0.0.1/32", "10.0.0.0/8"},
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.195, 10.1.2.3, 127.0.0.1"},
			remoteAddr: "127.0.0.1:54321",
			expected:   "203.0.113.195",
		},
		{
			name:       "trusted peer falls back to X-Real-IP when XFF absent",
			trusted:    []string{"127.0.0.1/32"},
			headers:    map[string]string{"X-Real-IP": "198.51.100.22"},
			remoteAddr: "127.0.0.1:54321",
			expected:   "198.51.100.22",
		},
		{
			// CF-Connecting-IP 不再被信任：没有 XFF/X-Real-IP 时回落到对端
			name:       "CF-Connecting-IP is not trusted",
			trusted:    []string{"127.0.0.1/32"},
			headers:    map[string]string{"CF-Connecting-IP": "104.28.19.45"},
			remoteAddr: "127.0.0.1:54321",
			expected:   "127.0.0.1",
		},
		{
			name:       "direct connection RemoteAddr fallback",
			trusted:    []string{"127.0.0.1/32"},
			headers:    map[string]string{},
			remoteAddr: "192.0.2.88:12345",
			expected:   "192.0.2.88",
		},
		{
			name:       "IPv6 direct connection",
			trusted:    []string{"::1/128"},
			headers:    map[string]string{},
			remoteAddr: "[2001:db8::1]:8080",
			expected:   "2001:db8::1",
		},
		{
			name:       "IPv6 trusted peer with IPv6 XFF",
			trusted:    []string{"::1/128"},
			headers:    map[string]string{"X-Forwarded-For": "2001:db8::9, ::1"},
			remoteAddr: "[::1]:8080",
			expected:   "2001:db8::9",
		},
		{
			// 裸 IP（无端口）也按对端处理
			name:       "bare remote addr without port",
			trusted:    []string{"127.0.0.1/32"},
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.9"},
			remoteAddr: "127.0.0.1",
			expected:   "203.0.113.9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trust(t, tt.trusted...)
			got := GetRealIP(newIPRequest(tt.remoteAddr, tt.headers))
			if got != tt.expected {
				t.Errorf("GetRealIP() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSetTrustedProxiesValidation(t *testing.T) {
	if err := SetTrustedProxies([]string{"127.0.0.1/32", "::1/128", "10.0.0.5"}); err != nil {
		t.Fatalf("合法 IP/CIDR 应通过: %v", err)
	}
	if err := SetTrustedProxies([]string{"not-an-ip"}); err == nil {
		t.Fatal("非法地址应返回错误，避免配置写错后静默降级")
	}
	_ = SetTrustedProxies(nil)
}

func TestGetRealIPNilRequest(t *testing.T) {
	if got := GetRealIP(nil); got != "" {
		t.Fatalf("GetRealIP(nil) = %q, want empty", got)
	}
}
