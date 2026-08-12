// Package subscribe 生成 Clash YAML / Base64 订阅（按 UA 区分）。
// 依据《mihomo-订阅语法.md》与《知识状态清单》A 类实测结论。
package subscribe

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/zhx/xray-panel/internal/master/xray"
	"github.com/zhx/xray-panel/internal/models"
)

// ProxyItem 订阅中的单个节点。
type ProxyItem struct {
	Name    string
	Host    string
	Port    int
	UUID    string
	Network string
	TLSType string
	Flow    string // 用户在该入站上的 flow（UserInbound 覆盖，与生成侧 clients 注入同源）
	Reality *xray.RealitySettings
	TLS     *xray.TLSSettings // tls 分支 servername / skip-cert-verify 透传
	WS      *xray.WSSettings
	XHTTP   *xray.XHTTPSettings
}

// BuildClash 生成 Clash YAML（proxy-providers 兼容格式）。
func BuildClash(user *models.User, items []ProxyItem) string {
	var b strings.Builder
	names := make([]string, 0, len(items))
	b.WriteString("proxies:\n")
	for _, it := range items {
		// reality 提取失败（缺 SNI/公钥）的节点跳过，避免 nil 崩溃与 proxy-groups 悬空引用
		if it.TLSType == "reality" && it.Reality == nil {
			continue
		}
		names = append(names, it.Name)
		b.WriteString(fmt.Sprintf("  - name: %q\n", it.Name))
		b.WriteString("    type: vless\n")
		b.WriteString(fmt.Sprintf("    server: %s\n", it.Host))
		b.WriteString(fmt.Sprintf("    port: %d\n", it.Port))
		b.WriteString(fmt.Sprintf("    uuid: %s\n", it.UUID))
		b.WriteString(fmt.Sprintf("    network: %s\n", it.Network))
		b.WriteString("    udp: true\n")
		// TLS 层
		switch it.TLSType {
		case "reality":
			b.WriteString("    tls: true\n")
			if it.Network == "tcp" {
				// 与生成侧 buildClients 一致：TCP+REALITY 自动 vision，UserInbound 覆盖优先
				flow := it.Flow
				if flow == "" {
					flow = "xtls-rprx-vision"
				}
				b.WriteString(fmt.Sprintf("    flow: %s\n", flow))
			}
			b.WriteString(fmt.Sprintf("    servername: %s\n", it.Reality.ServerName))
			b.WriteString("    client-fingerprint: chrome\n")
			b.WriteString("    reality-opts:\n")
			b.WriteString(fmt.Sprintf("      public-key: %s\n", it.Reality.PublicKey))
			b.WriteString(fmt.Sprintf("      short-id: %s\n", it.Reality.ShortID))
		case "tls":
			b.WriteString("    tls: true\n")
			// TCP+TLS+Vision 场景（UserInbound.Flow 已配置时订阅必须带 flow，否则握手不匹配）
			if it.Network == "tcp" && it.Flow != "" {
				b.WriteString(fmt.Sprintf("    flow: %s\n", it.Flow))
			}
			if it.TLS != nil && it.TLS.ServerName != "" {
				b.WriteString(fmt.Sprintf("    servername: %s\n", it.TLS.ServerName))
			}
			if it.TLS != nil && it.TLS.AllowInsecure {
				b.WriteString("    skip-cert-verify: true\n")
			}
		default:
			b.WriteString("    tls: false\n")
		}
		// 传输层参数（与 TLS 类型无关）
		switch it.Network {
		case "ws":
			if it.WS != nil {
				b.WriteString(fmt.Sprintf("    ws-opts:\n      path: %s\n", it.WS.Path))
				if it.WS.Host != "" {
					b.WriteString(fmt.Sprintf("      headers:\n        Host: %s\n", it.WS.Host))
				}
			}
		case "xhttp":
			// mihomo 默认仅 h2，显式声明 alpn（服务端可能 h3/http1.1）
			b.WriteString("    alpn: [h2]\n")
			if it.XHTTP != nil {
				b.WriteString("    xhttp-opts:\n")
				b.WriteString(fmt.Sprintf("      mode: %s\n", it.XHTTP.Mode))
				b.WriteString(fmt.Sprintf("      path: %s\n", it.XHTTP.Path))
				if it.XHTTP.Host != "" {
					b.WriteString(fmt.Sprintf("      host: %s\n", it.XHTTP.Host))
				}
			}
		}
	}

	b.WriteString("proxy-groups:\n")
	b.WriteString("  - name: \"🚀 节点选择\"\n    type: select\n    proxies:\n")
	for _, n := range names {
		b.WriteString(fmt.Sprintf("      - %q\n", n))
	}
	b.WriteString("  - name: \"♻️ 自动选择\"\n    type: url-test\n    url: http://cp.cloudflare.com/generate_204\n    interval: 300\n    proxies:\n")
	for _, n := range names {
		b.WriteString(fmt.Sprintf("      - %q\n", n))
	}
	b.WriteString("rules:\n  - MATCH,🚀 节点选择\n")
	return b.String()
}

// BuildBase64 生成 vless:// 分享链接列表的 Base64（兜底，非 Clash 客户端）。
func BuildBase64(user *models.User, items []ProxyItem) string {
	links := make([]string, 0, len(items))
	for _, it := range items {
		q := url.Values{}
		q.Set("encryption", "none")
		switch it.TLSType {
		case "reality":
			if it.Reality == nil {
				continue
			}
			q.Set("security", "reality")
			q.Set("sni", it.Reality.ServerName)
			q.Set("fp", "chrome")
			q.Set("pbk", it.Reality.PublicKey)
			q.Set("sid", it.Reality.ShortID)
			if it.Network == "tcp" {
				flow := it.Flow
				if flow == "" {
					flow = "xtls-rprx-vision"
				}
				q.Set("flow", flow)
			}
		case "tls":
			q.Set("security", "tls")
			if it.TLS != nil {
				if it.TLS.ServerName != "" {
					q.Set("sni", it.TLS.ServerName)
				}
				if it.TLS.AllowInsecure {
					q.Set("allowInsecure", "1")
				}
			}
			// TCP+TLS+Vision：仅当用户已配置 flow（vision 不适用 ws/xhttp）
			if it.Network == "tcp" && it.Flow != "" {
				q.Set("flow", it.Flow)
			}
		default:
			q.Set("security", "none")
		}
		// 传输层参数（与 TLS 类型无关，独立输出）
		switch it.Network {
		case "ws":
			q.Set("type", "ws")
			if it.WS != nil {
				q.Set("path", url.QueryEscape(it.WS.Path))
				if it.WS.Host != "" {
					q.Set("host", it.WS.Host)
				}
			}
		case "xhttp":
			q.Set("type", "xhttp")
			if it.XHTTP != nil {
				q.Set("mode", it.XHTTP.Mode)
				q.Set("path", it.XHTTP.Path)
				if it.XHTTP.Host != "" {
					q.Set("host", it.XHTTP.Host)
				}
			}
		default:
			if it.Network == "tcp" && it.TLSType != "reality" {
				q.Set("type", "tcp")
			}
		}
		frag := url.QueryEscape(it.Name)
		link := fmt.Sprintf("vless://%s@%s:%d?%s#%s", it.UUID, it.Host, it.Port, q.Encode(), frag)
		links = append(links, link)
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))
}

// Hash 内容哈希（ETag 用）。
func Hash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// UserInfoHeader 生成 subscription-userinfo 头。
func UserInfoHeader(up, down, total, expire int64) string {
	if expire > 0 {
		return fmt.Sprintf("upload=%d; download=%d; total=%d; expire=%d", up, down, total, expire)
	}
	return fmt.Sprintf("upload=%d; download=%d; total=%d", up, down, total)
}

// NodeName 生成节点名（服务器+入站）。
func NodeName(server *models.Server, inb *models.Inbound) string {
	return fmt.Sprintf("%s | %s", server.Name, inb.Tag)
}

var _ = time.Now
