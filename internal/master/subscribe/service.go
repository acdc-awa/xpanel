// Package subscribe 生成 Clash YAML / Base64 订阅（按 UA 区分）。
// 依据《mihomo-订阅语法.md》与《知识状态清单》A 类实测结论。
package subscribe

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
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
	Flow    string // 用户在该入站上的 flow（UserInbound 覆盖 → 入站级 Flow，与生成侧 clients 注入同源）
	// NoAutoFlow 禁用 reality+tcp 的自动 vision 兜底（对应入站 Flow = "none"，
	// 服务端 clients 不注入 flow，订阅必须保持一致否则握手不匹配）。
	NoAutoFlow bool
	Reality *xray.RealitySettings
	TLS     *xray.TLSSettings // tls 分支 servername / skip-cert-verify 透传
	WS      *xray.WSSettings
	XHTTP   *xray.XHTTPSettings
}

// BuiltinDefaultClashTemplate 系统内置基础默认模板
const BuiltinDefaultClashTemplate = `mixed-port: 7890
allow-lan: true
mode: rule
log-level: info
ipv6: false

dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  nameserver:
    - 223.5.5.5
    - 119.29.29.29

proxies:
$PROXIES$

proxy-groups:
  - { name: 节点选择, type: select, proxies: [DIRECT, $ALL_PROXIES$] }
  - { name: 自动选择, type: url-test, url: http://cp.cloudflare.com/generate_204, interval: 300, proxies: [$ALL_PROXIES$] }

rules:
  - MATCH,节点选择
`

// BuiltinAdvancedClashTemplate 系统推荐多地区与分流高级模板（依据 docs/example.yaml 标准生产配置）
const BuiltinAdvancedClashTemplate = `mixed-port: 7890
allow-lan: true
ipv6: true
bind-address: '*'
mode: rule
log-level: info
unified-delay: true
tcp-concurrent: true
geodata-mode: true
geo-auto-update: true
geo-update-interval: 24
geox-url:
    geoip: 'https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.dat'
    geosite: 'https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat'
dns:
    enable: true
    ipv6: true
    prefer-h3: false
    use-hosts: true
    use-system-hosts: true
    respect-rules: true
    enhanced-mode: fake-ip
    fake-ip-range: 198.18.0.1/16
    default-nameserver: [223.5.5.5, 119.29.29.29]
    proxy-server-nameserver: [223.5.5.5, 119.29.29.29]
    direct-nameserver: [223.5.5.5, 119.29.29.29]
    direct-nameserver-follow-policy: true
    fake-ip-filter: ['geosite:private', 'geosite:cn', +.lan, +.local, +.localhost, +.home.arpa, '*.msftncsi.com', '*.msftconnecttest.com', '+.stun.*', '+.stun.*.*', lens.l.google.com]
    nameserver-policy: { +.lan: system, +.local: system, +.home.arpa: system, 'geosite:cn': [223.5.5.5, 119.29.29.29], 'geosite:geolocation-!cn': ['tcp://1.1.1.1#节点选择', 'tcp://8.8.8.8#节点选择'] }
    nameserver: ['tcp://1.1.1.1#节点选择', 'tcp://8.8.8.8#节点选择']
proxies:
$PROXIES$
proxy-groups:
    - { name: 节点选择, type: select, proxies: [DIRECT, $ALL_PROXIES$] }
    - { name: 自动选择, type: url-test, url: http://cp.cloudflare.com/generate_204, interval: 300, proxies: [$ALL_PROXIES$] }
    - { name: 香港节点, type: select, proxies: [$FILTER_PROXIES(HK|香港)$] }
    - { name: 日本节点, type: select, proxies: [$FILTER_PROXIES(JP|日本)$] }
    - { name: 美国节点, type: select, proxies: [$FILTER_PROXIES(US|美国)$] }
    - { name: 台湾节点, type: select, proxies: [$FILTER_PROXIES(TW|台湾)$] }
    - { name: 新加坡节点, type: select, proxies: [$FILTER_PROXIES(SG|新加坡)$] }
    - { name: Anthropic, type: select, proxies: [节点选择, $FILTER_PROXIES(家宽|台湾|日本|香港)$] }
    - { name: Google, type: select, proxies: [节点选择, $ALL_PROXIES$] }
    - { name: OpenAI, type: select, proxies: [节点选择, $ALL_PROXIES$] }
rule-providers:
    google: { type: http, behavior: classical, format: yaml, url: 'https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Google/Google.yaml', path: ./ruleset/ios-rule-script/google.yaml, interval: 86400, proxy: 节点选择 }
    anthropic: { type: http, behavior: classical, format: yaml, url: 'https://raw.githubusercontent.com/jinxinkai/clash-ai-rules/refs/heads/master/Anthropic.yaml', path: ./ruleset/clash-ai-rules/anthropic.yaml, interval: 86400, proxy: 节点选择, size-limit: 0 }
    gemini: { type: http, behavior: classical, format: yaml, url: 'https://raw.githubusercontent.com/jinxinkai/clash-ai-rules/refs/heads/master/Gemini.yaml', path: ./ruleset/clash-ai-rules/gemini.yaml, interval: 86400, proxy: 节点选择, size-limit: 0 }
    openai: { type: http, behavior: classical, format: yaml, url: 'https://raw.githubusercontent.com/jinxinkai/clash-ai-rules/refs/heads/master/OpenAI.yaml', path: ./ruleset/clash-ai-rules/openai.yaml, interval: 86400, proxy: 节点选择, size-limit: 0 }
    tiktok: { type: http, behavior: classical, format: yaml, url: 'https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/TikTok/TikTok.yaml', path: ./ruleset/ios-rule-script/tiktok.yaml, interval: 86400, proxy: 节点选择 }
rules:
    - 'DOMAIN,$PANEL_HOST$,DIRECT'
    - 'GEOIP,private,DIRECT,no-resolve'
    - 'GEOSITE,private,DIRECT'
    - 'DOMAIN,localhost,DIRECT'
    - 'DOMAIN-SUFFIX,lan,DIRECT'
    - 'DOMAIN-SUFFIX,local,DIRECT'
    - 'DOMAIN-SUFFIX,home.arpa,DIRECT'
    - 'IP-CIDR,0.0.0.0/8,DIRECT,no-resolve'
    - 'IP-CIDR,10.0.0.0/8,DIRECT,no-resolve'
    - 'IP-CIDR,100.64.0.0/10,DIRECT,no-resolve'
    - 'IP-CIDR,127.0.0.0/8,DIRECT,no-resolve'
    - 'IP-CIDR,169.254.0.0/16,DIRECT,no-resolve'
    - 'IP-CIDR,172.16.0.0/12,DIRECT,no-resolve'
    - 'IP-CIDR,192.168.0.0/16,DIRECT,no-resolve'
    - 'IP-CIDR6,::1/128,DIRECT,no-resolve'
    - 'IP-CIDR6,fc00::/7,DIRECT,no-resolve'
    - 'IP-CIDR6,fe80::/10,DIRECT,no-resolve'
    - 'IP-CIDR6,ff00::/8,DIRECT,no-resolve'
    - 'DOMAIN-SUFFIX,daily-cloudcode-pa.googleapis.com,Google'
    - 'DOMAIN-SUFFIX,daily-cloudcode-pa.sandbox.googleapis.com,Google'
    - 'DOMAIN-SUFFIX,googleapis.com,Google'
    - 'DOMAIN-SUFFIX,www.googleapis.com,Google'
    - 'DOMAIN-SUFFIX,play.googleapis.com,Google'
    - 'DOMAIN-SUFFIX,oauth2.googleapis.com,Google'
    - 'DOMAIN-SUFFIX,antigravity-unleash.goog,Google'
    - 'DOMAIN-SUFFIX,lh3.googleusercontent.com,Google'
    - 'RULE-SET,anthropic,Anthropic'
    - 'RULE-SET,openai,OpenAI'
    - 'RULE-SET,gemini,Google'
    - 'RULE-SET,google,Google'
    - 'DOMAIN-SUFFIX,cdn.bootcdn.net,DIRECT'
    - 'GEOSITE,cn,DIRECT'
    - 'GEOIP,CN,DIRECT'
    - 'MATCH,节点选择'
`

var filterRegex = regexp.MustCompile(`(?i)(?:-\s*)?\$FILTER_PROXIES\(([^)]+)\)\$?`)
var filterBraceRegex = regexp.MustCompile(`(?i)(?:-\s*)?\{filter_proxies\(([^)]+)\)\}`)

func escapeSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// FormatSingleProxyItem 生成单行紧凑 Flow 映射的 VLESS 节点 YAML（遵循 mihomo/Clash 官方规范，移除冗余字段）。
func FormatSingleProxyItem(it *ProxyItem) string {
	parts := make([]string, 0, 16)
	parts = append(parts, fmt.Sprintf("name: '%s'", escapeSingleQuote(it.Name)))
	parts = append(parts, "type: vless")
	parts = append(parts, fmt.Sprintf("server: %s", it.Host))
	parts = append(parts, fmt.Sprintf("port: %d", it.Port))
	parts = append(parts, fmt.Sprintf("uuid: %s", it.UUID))
	parts = append(parts, "udp: true")

	// flow: 仅 TCP 传输可用且有效
	flow := it.Flow
	if it.Network == "tcp" && it.TLSType == "reality" && flow == "" && !it.NoAutoFlow {
		flow = "xtls-rprx-vision"
	}
	if it.Network == "tcp" && flow != "" {
		parts = append(parts, fmt.Sprintf("flow: %s", flow))
	}

	switch it.TLSType {
	case "reality":
		if it.Reality == nil {
			return ""
		}
		parts = append(parts, "tls: true")
		if it.Reality.ServerName != "" {
			parts = append(parts, fmt.Sprintf("servername: %s", it.Reality.ServerName))
		}
		parts = append(parts, fmt.Sprintf("reality-opts: { public-key: %s, short-id: %s }", it.Reality.PublicKey, it.Reality.ShortID))
		parts = append(parts, "client-fingerprint: chrome")
	case "tls":
		parts = append(parts, "tls: true")
		if it.TLS != nil && it.TLS.AllowInsecure {
			parts = append(parts, "skip-cert-verify: true")
		}
		if it.TLS != nil && it.TLS.ServerName != "" {
			parts = append(parts, fmt.Sprintf("servername: %s", it.TLS.ServerName))
		}
		parts = append(parts, "client-fingerprint: chrome")
	default:
		parts = append(parts, "tls: false")
	}

	switch it.Network {
	case "ws":
		parts = append(parts, "network: ws")
		if it.WS != nil {
			if it.WS.Host != "" {
				parts = append(parts, fmt.Sprintf("ws-opts: { path: %s, headers: { Host: %s } }", it.WS.Path, it.WS.Host))
			} else {
				parts = append(parts, fmt.Sprintf("ws-opts: { path: %s }", it.WS.Path))
			}
		}
	case "xhttp":
		parts = append(parts, "network: xhttp")
		parts = append(parts, "alpn: [h2]")
		if it.XHTTP != nil {
			hostField := ""
			if it.XHTTP.Host != "" {
				hostField = fmt.Sprintf(", host: %s", it.XHTTP.Host)
			}
			parts = append(parts, fmt.Sprintf("xhttp-opts: { mode: %s, path: %s%s }", it.XHTTP.Mode, it.XHTTP.Path, hostField))
		}
	default:
		parts = append(parts, "network: tcp")
	}

	return fmt.Sprintf("    - { %s }", strings.Join(parts, ", "))
}

// FormatProxiesYAML 将 ProxyItem 列表格式化为 YAML 节点块（每项单行紧凑映射），并返回有效节点名称列表。
func FormatProxiesYAML(items []ProxyItem) (string, []string) {
	var b strings.Builder
	names := make([]string, 0, len(items))
	for _, it := range items {
		// reality 提取失败（缺 SNI/公钥）的节点跳过，避免 nil 崩溃与 proxy-groups 悬空引用
		if it.TLSType == "reality" && it.Reality == nil {
			continue
		}
		line := FormatSingleProxyItem(&it)
		if line == "" {
			continue
		}
		names = append(names, it.Name)
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String(), names
}

// BuildClashWithTemplate 根据自定义模板与占位符生成 Clash YAML。
// 若 template 为空，则回退至内置标准模板。
func BuildClashWithTemplate(user *models.User, items []ProxyItem, template string, panelHost ...string) string {
	raw := strings.TrimSpace(template)
	if raw == "" {
		raw = BuiltinDefaultClashTemplate
	}

	host := "localhost"
	if len(panelHost) > 0 && panelHost[0] != "" {
		host = panelHost[0]
	}

	proxiesYAML, names := FormatProxiesYAML(items)

	// 1. 替换面板域名占位符
	processed := raw
	processed = strings.ReplaceAll(processed, "$PANEL_HOST$", host)
	processed = strings.ReplaceAll(processed, "$PANEL_DOMAIN$", host)
	processed = strings.ReplaceAll(processed, "$SUB_DOMAIN$", host)

	// 2. 替换 $PROXIES$ 节点池占位符
	if strings.Contains(processed, "$PROXIES$") {
		processed = strings.ReplaceAll(processed, "$PROXIES$", strings.TrimRight(proxiesYAML, "\n"))
	} else if strings.Contains(processed, "{proxies}") {
		processed = strings.ReplaceAll(processed, "{proxies}", strings.TrimRight(proxiesYAML, "\n"))
	} else if strings.Contains(processed, "{all_proxies}") && strings.Contains(processed, "proxies:") {
		processed = strings.Replace(processed, "{all_proxies}", strings.TrimRight(proxiesYAML, "\n"), 1)
	}

	// 3. 按行处理策略组内的 $ALL_PROXIES$ 和 $FILTER_PROXIES(...)$
	lines := strings.Split(processed, "\n")
	resultLines := make([]string, 0, len(lines)+len(names)*2)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 检查是否为行内数组（包含 '[' 与 ']'）
		if strings.Contains(line, "[") && strings.Contains(line, "]") &&
			(strings.Contains(line, "$ALL_PROXIES$") || strings.Contains(line, "$FILTER_PROXIES(") ||
				strings.Contains(line, "{all_proxies}") || strings.Contains(line, "{filter_proxies(")) {
			resultLines = append(resultLines, replaceInlineArrayPlaceholders(line, names))
			continue
		}

		// 多行列表模式：检查 $ALL_PROXIES$ / {all_proxies}
		if strings.Contains(trimmed, "$ALL_PROXIES$") || strings.Contains(trimmed, "{all_proxies}") {
			indent := getLeadingWhitespace(line)
			if indent == "" {
				indent = "      "
			}
			if len(names) > 0 {
				for _, n := range names {
					resultLines = append(resultLines, fmt.Sprintf("%s- '%s'", indent, escapeSingleQuote(n)))
				}
			} else {
				resultLines = append(resultLines, fmt.Sprintf("%s- DIRECT", indent))
			}
			continue
		}

		// 多行列表模式：检查 $FILTER_PROXIES(pattern)$
		if m := filterRegex.FindStringSubmatch(trimmed); len(m) > 1 {
			pattern := strings.TrimSpace(m[1])
			indent := getLeadingWhitespace(line)
			if indent == "" {
				indent = "      "
			}
			matchedNames := filterNodeNames(names, pattern)
			if len(matchedNames) > 0 {
				for _, n := range matchedNames {
					resultLines = append(resultLines, fmt.Sprintf("%s- '%s'", indent, escapeSingleQuote(n)))
				}
			} else {
				resultLines = append(resultLines, fmt.Sprintf("%s- DIRECT", indent))
			}
			continue
		}

		// 多行列表模式：检查 {filter_proxies(pattern)}
		if m := filterBraceRegex.FindStringSubmatch(trimmed); len(m) > 1 {
			pattern := strings.TrimSpace(m[1])
			indent := getLeadingWhitespace(line)
			if indent == "" {
				indent = "      "
			}
			matchedNames := filterNodeNames(names, pattern)
			if len(matchedNames) > 0 {
				for _, n := range matchedNames {
					resultLines = append(resultLines, fmt.Sprintf("%s- '%s'", indent, escapeSingleQuote(n)))
				}
			} else {
				resultLines = append(resultLines, fmt.Sprintf("%s- DIRECT", indent))
			}
			continue
		}

		resultLines = append(resultLines, line)
	}

	return strings.Join(resultLines, "\n")
}

// replaceInlineArrayPlaceholders 替换行内数组 `[item1, $ALL_PROXIES$, ...]` 中的占位符。
func replaceInlineArrayPlaceholders(line string, names []string) string {
	// 替换 $ALL_PROXIES$ / {all_proxies}
	if strings.Contains(line, "$ALL_PROXIES$") || strings.Contains(line, "{all_proxies}") {
		quotedNames := make([]string, 0, len(names))
		for _, n := range names {
			quotedNames = append(quotedNames, fmt.Sprintf("'%s'", escapeSingleQuote(n)))
		}
		replacement := strings.Join(quotedNames, ", ")
		if replacement == "" {
			replacement = "DIRECT"
		}
		line = strings.ReplaceAll(line, "$ALL_PROXIES$", replacement)
		line = strings.ReplaceAll(line, "{all_proxies}", replacement)
	}

	// 替换 $FILTER_PROXIES(pattern)$
	line = filterRegex.ReplaceAllStringFunc(line, func(match string) string {
		m := filterRegex.FindStringSubmatch(match)
		if len(m) <= 1 {
			return match
		}
		pattern := strings.TrimSpace(m[1])
		matched := filterNodeNames(names, pattern)
		if len(matched) == 0 {
			return ""
		}
		quotedMatched := make([]string, 0, len(matched))
		for _, n := range matched {
			quotedMatched = append(quotedMatched, fmt.Sprintf("'%s'", escapeSingleQuote(n)))
		}
		return strings.Join(quotedMatched, ", ")
	})

	// 替换 {filter_proxies(pattern)}
	line = filterBraceRegex.ReplaceAllStringFunc(line, func(match string) string {
		m := filterBraceRegex.FindStringSubmatch(match)
		if len(m) <= 1 {
			return match
		}
		pattern := strings.TrimSpace(m[1])
		matched := filterNodeNames(names, pattern)
		if len(matched) == 0 {
			return ""
		}
		quotedMatched := make([]string, 0, len(matched))
		for _, n := range matched {
			quotedMatched = append(quotedMatched, fmt.Sprintf("'%s'", escapeSingleQuote(n)))
		}
		return strings.Join(quotedMatched, ", ")
	})

	// 清理多余空项与逗号
	line = cleanInlineArray(line)
	return line
}

// cleanInlineArray 清理行内数组 `[ ... ]` 因空替换可能产生的多余逗号。
func cleanInlineArray(s string) string {
	// [ , item ] -> [ item ]
	cleanStart := regexp.MustCompile(`\[\s*,+\s*`)
	s = cleanStart.ReplaceAllString(s, "[")
	// [ item, ] -> [ item ]
	cleanEnd := regexp.MustCompile(`\s*,+\s*\]`)
	s = cleanEnd.ReplaceAllString(s, "]")
	// item1, , item2 -> item1, item2
	cleanDouble := regexp.MustCompile(`,\s*,+`)
	for cleanDouble.MatchString(s) {
		s = cleanDouble.ReplaceAllString(s, ", ")
	}
	// proxies: [] -> proxies: [DIRECT]
	emptyArr := regexp.MustCompile(`proxies:\s*\[\s*\]`)
	s = emptyArr.ReplaceAllString(s, "proxies: [DIRECT]")
	return s
}

// filterNodeNames 按正则表达式或关键词过滤节点名。
func filterNodeNames(names []string, pattern string) []string {
	if pattern == "" {
		return names
	}
	re, err := regexp.Compile("(?i)" + pattern)
	matched := make([]string, 0, len(names))
	for _, n := range names {
		if err == nil {
			if re.MatchString(n) {
				matched = append(matched, n)
			}
		} else {
			if strings.Contains(strings.ToLower(n), strings.ToLower(pattern)) {
				matched = append(matched, n)
			}
		}
	}
	return matched
}

// getLeadingWhitespace 提取前导空白缩进。
func getLeadingWhitespace(s string) string {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return s[:i]
		}
	}
	return s
}

// BuildClash 生成 Clash YAML（proxy-providers 兼容格式，使用内置标准模板）。
func BuildClash(user *models.User, items []ProxyItem) string {
	proxiesYAML, names := FormatProxiesYAML(items)
	var b strings.Builder
	b.WriteString("proxies:\n")
	b.WriteString(proxiesYAML)
	b.WriteString("proxy-groups:\n")
	b.WriteString("  - name: \"🚀 节点选择\"\n    type: select\n    proxies:\n")
	if len(names) > 0 {
		for _, n := range names {
			b.WriteString(fmt.Sprintf("      - %q\n", n))
		}
	} else {
		b.WriteString("      - DIRECT\n")
	}
	b.WriteString("  - name: \"♻️ 自动选择\"\n    type: url-test\n    url: http://cp.cloudflare.com/generate_204\n    interval: 300\n    proxies:\n")
	if len(names) > 0 {
		for _, n := range names {
			b.WriteString(fmt.Sprintf("      - %q\n", n))
		}
	} else {
		b.WriteString("      - DIRECT\n")
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
				if flow == "" && !it.NoAutoFlow {
					flow = "xtls-rprx-vision"
				}
				if flow != "" {
					q.Set("flow", flow)
				}
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

// NodeName 生成节点名（服务器名 + 倍率 + 入站标签，如 '🇭🇰香港01 x1 | IEPL'）。
func NodeName(server *models.Server, inb *models.Inbound) string {
	ratioStr := ""
	if inb.Ratio > 0 {
		if inb.Ratio == float64(int64(inb.Ratio)) {
			ratioStr = fmt.Sprintf(" x%d", int64(inb.Ratio))
		} else {
			ratioStr = fmt.Sprintf(" x%g", inb.Ratio)
		}
	}
	if inb.Tag != "" {
		return fmt.Sprintf("%s%s | %s", server.Name, ratioStr, inb.Tag)
	}
	return fmt.Sprintf("%s%s", server.Name, ratioStr)
}

var _ = time.Now
