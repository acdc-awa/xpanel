package util

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetRealIP 从 HTTP 请求中提取真实客户端 IP（全兼容 Cloudflare CDN、Caddy/Nginx 反代及直连模式）。
// 优先级：CF-Connecting-IP -> X-Real-IP -> X-Forwarded-For -> RemoteAddr
func GetRealIP(r *http.Request) string {
	if r == nil {
		return ""
	}

	// 1. 优先检查 Cloudflare CDN 专属请求头
	if cfIP := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cfIP != "" {
		if ip := net.ParseIP(cfIP); ip != nil {
			return ip.String()
		}
	}

	// 2. 检查 X-Real-IP（Nginx / Caddy 常用）
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		if ip := net.ParseIP(realIP); ip != nil {
			return ip.String()
		}
	}

	// 3. 检查 X-Forwarded-For（取首个有效客户端 IP）
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		for _, part := range strings.Split(xff, ",") {
			part = strings.TrimSpace(part)
			if ip := net.ParseIP(part); ip != nil {
				return ip.String()
			}
		}
	}

	// 4. 回退至底层 TCP 连接 RemoteAddr（直连模式）
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
		return host
	}

	raw := strings.TrimSpace(r.RemoteAddr)
	if ip := net.ParseIP(raw); ip != nil {
		return ip.String()
	}
	return raw
}

// ClientIPFromContext 从 Gin 上下文中提取真实客户端 IP。
func ClientIPFromContext(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return GetRealIP(c.Request)
}
