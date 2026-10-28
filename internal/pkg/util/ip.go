package util

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// trustedSet 可信反向代理网段（app.trusted_proxies 的解析结果）。
// 只有直连对端（RemoteAddr）落在这些网段内时，转发头才被采信；否则一律使用
// RemoteAddr——这样公网客户端自报 X-Forwarded-For / CF-Connecting-IP 也无法伪造 IP。
type trustedSet struct {
	nets []*net.IPNet
}

var trustedProxies atomic.Pointer[trustedSet]

// SetTrustedProxies 配置可信反代网段（IP 或 CIDR 均可）。
// 空列表 = 不信任任何转发头，只认 TCP 对端地址（最严格）。
// 无效条目返回错误（启动期由 main 作为致命错误处理，避免配置写错后静默降级）。
func SetTrustedProxies(entries []string) error {
	nets := make([]*net.IPNet, 0, len(entries))
	for _, raw := range entries {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(raw); err == nil {
			nets = append(nets, n)
			continue
		}
		ip := net.ParseIP(raw)
		if ip == nil {
			return fmt.Errorf("无效的可信代理地址 %q（需 IP 或 CIDR，如 127.0.0.1/32、::1/128）", raw)
		}
		if v4 := ip.To4(); v4 != nil {
			nets = append(nets, &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)})
		} else {
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)})
		}
	}
	trustedProxies.Store(&trustedSet{nets: nets})
	return nil
}

func (s *trustedSet) contains(ip net.IP) bool {
	if s == nil || ip == nil {
		return false
	}
	for _, n := range s.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// remoteIP 解析 RemoteAddr（host:port 或裸 IP）为 net.IP。
func remoteIP(remoteAddr string) net.IP {
	raw := strings.TrimSpace(remoteAddr)
	if host, _, err := net.SplitHostPort(raw); err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(raw)
}

// GetRealIP 提取客户端 IP（用于限流、审计与人机验证）：
//   - 直连对端不在 app.trusted_proxies 内 → 只认 RemoteAddr，忽略所有转发头；
//   - 直连对端是可信反代 → X-Forwarded-For 从右向左取第一个「不可信」地址
//     （反代必须覆盖/追加该头，例如 Caddy `header_up X-Forwarded-For {remote_host}`）；
//   - 整条链都是可信代理时取最左有效地址；XFF 缺失时回退 X-Real-IP。
//
// 不再信任 CF-Connecting-IP：通用反代会原样透传客户端自带的该头，只有让反代
// 把真实客户端写进 X-Forwarded-For 才是可验证的口径（Cloudflare 场景见部署文档）。
func GetRealIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	peer := remoteIP(r.RemoteAddr)
	if peer == nil {
		// 非标准 RemoteAddr（测试/自定义监听器）：原样返回，避免丢弃可诊断信息。
		return strings.TrimSpace(r.RemoteAddr)
	}
	set := trustedProxies.Load()
	if !set.contains(peer) {
		return peer.String()
	}

	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		leftmost := ""
		for i := len(parts) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(parts[i]))
			if ip == nil {
				continue
			}
			leftmost = ip.String()
			if !set.contains(ip) {
				return ip.String()
			}
		}
		if leftmost != "" {
			return leftmost
		}
	}

	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		if ip := net.ParseIP(real); ip != nil {
			return ip.String()
		}
	}
	return peer.String()
}

// ClientIPFromContext 从 Gin 上下文中提取真实客户端 IP。
func ClientIPFromContext(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return GetRealIP(c.Request)
}
