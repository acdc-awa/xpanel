package util

import (
	"net/http"
	"testing"
)

func TestGetRealIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expected   string
	}{
		{
			name: "Cloudflare CDN header has highest priority",
			headers: map[string]string{
				"CF-Connecting-IP": "104.28.19.45",
				"X-Real-IP":        "172.68.1.1",
				"X-Forwarded-For":  "172.68.1.1, 10.0.0.1",
			},
			remoteAddr: "127.0.0.1:54321",
			expected:   "104.28.19.45",
		},
		{
			name: "Nginx/Caddy X-Real-IP when no Cloudflare",
			headers: map[string]string{
				"X-Real-IP":       "198.51.100.22",
				"X-Forwarded-For": "198.51.100.22, 10.0.0.2",
			},
			remoteAddr: "127.0.0.1:54321",
			expected:   "198.51.100.22",
		},
		{
			name: "Multi-proxy X-Forwarded-For first client IP",
			headers: map[string]string{
				"X-Forwarded-For": "203.0.113.195, 198.51.100.1, 10.0.0.1",
			},
			remoteAddr: "127.0.0.1:54321",
			expected:   "203.0.113.195",
		},
		{
			name:       "Direct connection RemoteAddr fallback",
			headers:    map[string]string{},
			remoteAddr: "192.0.2.88:12345",
			expected:   "192.0.2.88",
		},
		{
			name:       "IPv6 direct connection",
			headers:    map[string]string{},
			remoteAddr: "[2001:db8::1]:8080",
			expected:   "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "http://example.com/api/v1/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			req.RemoteAddr = tt.remoteAddr

			got := GetRealIP(req)
			if got != tt.expected {
				t.Errorf("GetRealIP() = %v, want %v", got, tt.expected)
			}
		})
	}
}
