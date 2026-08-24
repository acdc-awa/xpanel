// Package subscribe 生成 Clash YAML / Base64 订阅（按 UA 区分）。
// 节点组装走协议插件（protocols），导出器原生消费 contracts.ProxyNodeDTO；
// 依据《mihomo-订阅语法.md》与《知识状态清单》A 类实测结论。
package subscribe

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/protocols"
	"github.com/acdc-awa/xpanel/internal/models"
)

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

var filterRegex = regexp.MustCompile(`(?i)(?:-\s*)?\$FILTER_PROXIES\(([^)]+)\)\$?`)
var filterBraceRegex = regexp.MustCompile(`(?i)(?:-\s*)?\{filter_proxies\(([^)]+)\)\}`)

func escapeSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// formatClashNode 生成单行紧凑 Flow 映射的 VLESS 节点 YAML（遵循 mihomo/Clash 官方规范，移除冗余字段）。
// 导出器为哑渲染器：flow 等协议语义已由插件决议进 DTO，此处只做格式映射；
// 新协议的 Clash 分支在此按 dto.Protocol 扩展。
func formatClashNode(dto *contracts.ProxyNodeDTO) string {
	if dto.Protocol != "vless" || dto.Auth == nil || dto.Transport == nil || dto.Security == nil {
		return ""
	}
	sec := dto.Security.Type
	if sec == "reality" && dto.Security.Reality == nil {
		return ""
	}
	parts := make([]string, 0, 16)
	parts = append(parts, fmt.Sprintf("name: '%s'", escapeSingleQuote(dto.Name)))
	parts = append(parts, "type: vless")
	parts = append(parts, fmt.Sprintf("server: %s", dto.ServerHost))
	parts = append(parts, fmt.Sprintf("port: %d", dto.ServerPort))
	parts = append(parts, fmt.Sprintf("uuid: %s", dto.Auth.UUID))
	parts = append(parts, "udp: true")

	// flow: 仅 TCP 传输可用且有效（插件已决议，含自动 vision 与 none 抑制）
	network := dto.Transport.Network
	if network == "tcp" && dto.Auth.Flow != "" {
		parts = append(parts, fmt.Sprintf("flow: %s", dto.Auth.Flow))
	}

	switch sec {
	case "reality":
		parts = append(parts, "tls: true")
		if dto.Security.SNI != "" {
			parts = append(parts, fmt.Sprintf("servername: %s", dto.Security.SNI))
		}
		parts = append(parts, fmt.Sprintf("reality-opts: { public-key: %s, short-id: %s }", dto.Security.Reality.PublicKey, dto.Security.Reality.ShortID))
		parts = append(parts, "client-fingerprint: chrome")
	case "tls":
		parts = append(parts, "tls: true")
		if dto.Security.AllowInsecure {
			parts = append(parts, "skip-cert-verify: true")
		}
		if dto.Security.SNI != "" {
			parts = append(parts, fmt.Sprintf("servername: %s", dto.Security.SNI))
		}
		parts = append(parts, "client-fingerprint: chrome")
	default:
		parts = append(parts, "tls: false")
	}

	if network == "xhttp" {
		parts = append(parts, "network: xhttp")
		parts = append(parts, "alpn: [h2]")
		if dto.Transport.Mode != "" {
			hostField := ""
			if dto.Transport.Host != "" {
				hostField = fmt.Sprintf(", host: %s", dto.Transport.Host)
			}
			parts = append(parts, fmt.Sprintf("xhttp-opts: { mode: %s, path: %s%s }", dto.Transport.Mode, dto.Transport.Path, hostField))
		}
	} else {
		parts = append(parts, "network: tcp")
	}

	return fmt.Sprintf("    - { %s }", strings.Join(parts, ", "))
}

// FormatNodesYAML 将节点 DTO 列表格式化为 YAML 节点块（每项单行紧凑映射），并返回有效节点名称列表。
func FormatNodesYAML(nodes []contracts.ProxyNodeDTO) (string, []string) {
	var b strings.Builder
	names := make([]string, 0, len(nodes))
	for i := range nodes {
		line := formatClashNode(&nodes[i])
		if line == "" {
			continue
		}
		names = append(names, nodes[i].Name)
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String(), names
}

// BuildClashWithTemplate 根据自定义模板与占位符生成 Clash YAML。
// 若 template 为空，则回退至内置标准模板。
func BuildClashWithTemplate(nodes []contracts.ProxyNodeDTO, template string, panelHost ...string) string {
	raw := strings.TrimSpace(template)
	if raw == "" {
		raw = BuiltinDefaultClashTemplate
	}

	host := "localhost"
	if len(panelHost) > 0 && panelHost[0] != "" {
		host = panelHost[0]
	}

	proxiesYAML, names := FormatNodesYAML(nodes)

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
func BuildClash(nodes []contracts.ProxyNodeDTO) string {
	proxiesYAML, names := FormatNodesYAML(nodes)
	var b strings.Builder
	b.WriteString("proxies:\n")
	b.WriteString(proxiesYAML)
	b.WriteString("proxy-groups:\n")
	b.WriteString("  - name: \"节点选择\"\n    type: select\n    proxies:\n")
	if len(names) > 0 {
		for _, n := range names {
			b.WriteString(fmt.Sprintf("      - %q\n", n))
		}
	} else {
		b.WriteString("      - DIRECT\n")
	}
	b.WriteString("  - name: \"自动选择\"\n    type: url-test\n    url: http://cp.cloudflare.com/generate_204\n    interval: 300\n    proxies:\n")
	if len(names) > 0 {
		for _, n := range names {
			b.WriteString(fmt.Sprintf("      - %q\n", n))
		}
	} else {
		b.WriteString("      - DIRECT\n")
	}
	b.WriteString("rules:\n  - MATCH,节点选择\n")
	return b.String()
}

// BuildBase64 生成 vless:// 分享链接列表的 Base64（兜底，非 Clash 客户端）。
// 哑渲染器：flow 等协议语义已由插件决议进 DTO；reality 缺参数的节点跳过。
func BuildBase64(nodes []contracts.ProxyNodeDTO) string {
	links := make([]string, 0, len(nodes))
	for i := range nodes {
		dto := &nodes[i]
		if dto.Protocol != "vless" || dto.Auth == nil || dto.Transport == nil || dto.Security == nil {
			continue
		}
		q := url.Values{}
		q.Set("encryption", "none")
		sec := dto.Security.Type
		network := dto.Transport.Network
		switch sec {
		case "reality":
			if dto.Security.Reality == nil {
				continue
			}
			q.Set("security", "reality")
			q.Set("sni", dto.Security.SNI)
			q.Set("fp", "chrome")
			q.Set("pbk", dto.Security.Reality.PublicKey)
			q.Set("sid", dto.Security.Reality.ShortID)
			if network == "tcp" && dto.Auth.Flow != "" {
				q.Set("flow", dto.Auth.Flow)
			}
		case "tls":
			q.Set("security", "tls")
			if dto.Security.SNI != "" {
				q.Set("sni", dto.Security.SNI)
			}
			if dto.Security.AllowInsecure {
				q.Set("allowInsecure", "1")
			}
			// TCP+TLS+Vision：仅当用户已配置 flow（vision 不适用 ws/xhttp）
			if network == "tcp" && dto.Auth.Flow != "" {
				q.Set("flow", dto.Auth.Flow)
			}
		default:
			q.Set("security", "none")
		}
		// 传输层参数（与 TLS 类型无关，独立输出）
		if network == "xhttp" {
			q.Set("type", "xhttp")
			if dto.Transport.Mode != "" {
				q.Set("mode", dto.Transport.Mode)
				q.Set("path", dto.Transport.Path)
				if dto.Transport.Host != "" {
					q.Set("host", dto.Transport.Host)
				}
			}
		} else if network == "tcp" && sec != "reality" {
			q.Set("type", "tcp")
		}
		frag := url.QueryEscape(dto.Name)
		link := fmt.Sprintf("vless://%s@%s:%d?%s#%s", dto.Auth.UUID, dto.ServerHost, dto.ServerPort, q.Encode(), frag)
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

// NodeName 生成节点名（服务器名 + 倍率 + 入站标签，如 '香港01 x1 | IEPL'）。
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

// ShareAddrOf 计算订阅对外地址与端口（订阅专用，与 xray 物理监听解耦）。
// ShareAddrOf 解析入站「节点自有地址」：策略仅 node（服务器 Host）与 custom（ShareAddr）；
// 订阅对外地址与物理监听解耦，监听地址（Listen）不参与对外分享（四层转发场景由转发端点覆写）。
// 这是订阅管道的第一层；经 L4 中转的订阅会被转发端点覆写（见 ResolveAPSubscription），
// 直连入站且接入点未覆写时本值是最终地址。
func ShareAddrOf(srv *models.Server, inb *models.Inbound) (string, int) {
	port := inb.Port
	if inb.SharePort > 0 {
		port = inb.SharePort
	}
	switch inb.ShareAddrStrategy {
	case "custom":
		if inb.ShareAddr != "" {
			return inb.ShareAddr, port
		}
	}
	return srv.Host, port
}

// BuildNodeDTO 将 Inbound 模型与 Server 模型转换为订阅导出用的 ProxyNodeDTO。
// 协议知识（传输/安全/flow/凭证派生）由协议插件承担；未注册协议或参数不足返回 nil，
// 调用方记日志跳过（替代历史上 switch 静默 continue）。
// layer 非空表示入站挂对外接入层：对外 host/port 由层决议（内部实现——直连 TLS / 反代 / CDN——
// 对订阅不可见），安全层取层定义（share_security 显式覆写优先），SNI 缺省回落层 Host；
// 未挂层（layer=nil）沿用 ShareAddrStrategy 直连端点语义。
func BuildNodeDTO(srv *models.Server, inb *models.Inbound, userUUID string, layer *models.AccessLayer) *contracts.ProxyNodeDTO {
	plugin := protocols.Find(inb.Protocol)
	if plugin == nil {
		log.Printf("[subscribe] 入站 %s 协议 %q 未注册导出插件，跳过", inb.Tag, inb.Protocol)
		return nil
	}
	host, port := ShareAddrOf(srv, inb)
	sec := inb.ShareSecurity
	sni := inb.ShareSNI
	if layer != nil {
		host = layer.Host
		port = layer.Port
		if sec == "" || sec == "auto" {
			sec = layer.Security
		}
		if sni == "" {
			sni = layer.Host
		}
	}
	dto := plugin.BuildClientNode(&contracts.ClientNodeInput{
		Name: NodeName(srv, inb),
		Host: host,
		Port: port,
		Spec: contracts.DecodeInbound(inb),
		Share: contracts.ShareOverride{
			Security:      sec,
			SNI:           sni,
			Host:          inb.ShareHost,
			Path:          inb.SharePath,
			AllowInsecure: inb.ShareAllowInsecure,
		},
		InboundFlow: inb.Flow,
		UserUUID:    userUUID,
	})
	if dto == nil {
		log.Printf("[subscribe] 入站 %s 协议参数不足以产出订阅节点（如 reality 缺 SNI/公钥），跳过", inb.Tag)
	}
	return dto
}

// ResolveAPSubscription 沿订阅管道解析用户接入点（AP）产出的订阅节点（/sub 实时订阅与画布预览同源）：
//  1. 入站自有地址：BuildNodeDTO 按入站 ShareAddrStrategy（custom/listen/回退）解析节点 IP/端口；
//     挂对外接入层（layer_id）时，对外 host/port/security 由层决议（见 BuildNodeDTO）；
//  2. 管道覆写：AP 指向 L4 转发规则时，订阅消费者实际连接转发端点，host/port 覆写为
//     转发机 Host + L4 监听端口（入站分享地址描述的是目标入站自身，在此语义不适用）；
//     层语义同样不沿四层链路传递（L4 转发目标=入站内部端口，对外 TLS/SNI 无意义）；
//  3. AP 消费层：命名 + 可选 CustomHost/CustomPort 覆写（最高优先）。
//
// 目标缺失 / 权限组过滤由调用方负责；此处仅解析，解析失败返回 nil。
func ResolveAPSubscription(ap *models.UserAccessPoint, srvMap map[uint64]models.Server, inbMap map[uint64]models.Inbound, l4Map map[uint64]models.L4PortRule, layerMap map[uint64]models.AccessLayer, userUUID string) *contracts.ProxyNodeDTO {
	var (
		targetInb models.Inbound
		targetSrv models.Server
		ok        bool
		viaL4     bool
		l4Host    string
		l4Port    int
	)
	switch ap.TargetType {
	case "inbound":
		if ap.TargetInboundID == nil {
			return nil
		}
		targetInb, ok = inbMap[*ap.TargetInboundID]
		if !ok {
			return nil
		}
		targetSrv, ok = srvMap[targetInb.ServerID]
		if !ok {
			return nil
		}
	case "l4_rule":
		if ap.TargetL4RuleID == nil {
			return nil
		}
		l4Rule, ok := l4Map[*ap.TargetL4RuleID]
		if !ok {
			return nil
		}
		l4Srv, ok := srvMap[l4Rule.ServerID]
		if !ok {
			return nil
		}
		targetInb, ok = inbMap[l4Rule.TargetInboundID]
		if !ok {
			return nil
		}
		targetSrv, ok = srvMap[targetInb.ServerID]
		if !ok {
			return nil
		}
		viaL4, l4Host, l4Port = true, l4Srv.Host, l4Rule.ListenPort
	default:
		return nil
	}

	// 挂层解析：目标入站 layer_id → 对外接入层（L4 链不消费层语义，见函数注释）
	var layer *models.AccessLayer
	if targetInb.LayerID != nil {
		if l, ok := layerMap[*targetInb.LayerID]; ok {
			layer = &l
		}
	}
	if viaL4 {
		layer = nil
	}
	dto := BuildNodeDTO(&targetSrv, &targetInb, userUUID, layer)
	if dto == nil {
		return nil
	}
	if viaL4 {
		dto.ServerHost = l4Host
		dto.ServerPort = l4Port
	}
	dto.Name = ap.Name
	if ap.CustomHost != "" {
		dto.ServerHost = ap.CustomHost
	}
	if ap.CustomPort > 0 {
		dto.ServerPort = ap.CustomPort
	}
	return dto
}
