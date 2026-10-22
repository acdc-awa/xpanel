package subscribe

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/protocols"
)

// mihomo 方言渲染：把 xray 形态的进阶参数翻译成 mihomo YAML 的对应字段。
// 取值来自 ProxyNodeDTO（入站投影时已把 xhttp 进阶键收进 Transport.Opts）。

// yamlQuote 输出单引号 YAML 标量（内部单引号翻倍）。
func yamlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// yamlValue 把标量格式化为 YAML 片段：bool/number 原样，字符串加引号。
func yamlValue(v any) string {
	switch t := v.(type) {
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case string:
		return yamlQuote(t)
	default:
		return yamlQuote(fmt.Sprint(t))
	}
}

// xhttpOptSpec 声明一个 mihomo xhttp-opts 键：xray 键名、mihomo 键名、值类型。
type xhttpOptSpec struct {
	xrayKey   string
	mihomoKey string
	boolean   bool
}

// xhttpOptSpecs 是 xray SplitHTTPConfig → mihomo xhttp-opts 的固定翻译表。
// 只列出 mihomo 有等价字段的键；xray 单端字段（noSSEHeader / scMaxBufferedPosts /
// scStreamUpServerSecs / serverMaxHeaderBytes / downloadSettings）刻意不输出。
var xhttpOptSpecs = []xhttpOptSpec{
	{"xPaddingBytes", "x-padding-bytes", false},
	{"xPaddingObfsMode", "x-padding-obfs-mode", true},
	{"xPaddingKey", "x-padding-key", false},
	{"xPaddingHeader", "x-padding-header", false},
	{"xPaddingPlacement", "x-padding-placement", false},
	{"xPaddingMethod", "x-padding-method", false},
	{"uplinkHTTPMethod", "uplink-http-method", false},
	{"sessionIDPlacement", "session-placement", false},
	{"sessionIDKey", "session-key", false},
	{"sessionIDTable", "session-table", false},
	{"sessionIDLength", "session-length", false},
	{"seqPlacement", "seq-placement", false},
	{"seqKey", "seq-key", false},
	{"uplinkDataPlacement", "uplink-data-placement", false},
	{"uplinkDataKey", "uplink-data-key", false},
	{"uplinkChunkSize", "uplink-chunk-size", false},
	{"scMaxEachPostBytes", "sc-max-each-post-bytes", false},
	{"scMinPostsIntervalMs", "sc-min-posts-interval-ms", false},
	{"noGRPCHeader", "no-grpc-header", true},
}

// xmuxKeyMap xray XmuxConfig → mihomo reuse-settings。
// 默认值不等价：xray 的 xmux 留空会套上面那组默认，mihomo 不写即完全不复用，
// 因此缺省时必须显式补出默认值（见 xrayXmuxDefaults）。
var xmuxKeyMap = []struct{ xray, mihomo string }{
	{"maxConcurrency", "max-concurrency"},
	{"maxConnections", "max-connections"},
	{"cMaxReuseTimes", "c-max-reuse-times"},
	{"hMaxRequestTimes", "h-max-request-times"},
	{"hMaxReusableSecs", "h-max-reusable-secs"},
	{"hKeepAlivePeriod", "h-keep-alive-period"},
}

// xrayXmuxDefaults 对应 infra/conf/transport_method.go 的默认值（Xmux 为零值时套用）。
var xrayXmuxDefaults = []struct{ key, val string }{
	{"max-connections", "3"},
	{"h-max-request-times", "600-900"},
	{"h-max-reusable-secs", "1800-3000"},
}

// xhttpScalarString 把 xhttp 参数标量转成字符串；数值按整数输出（避免 1e+06 这类科学计数法）。
func xhttpScalarString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// mergeXHTTPExtra 把 xhttpSettings.extra（xray 的第二套入口）展开进键值表。
// 外层键优先：与 xray 的 Build 行为一致（extra 的 host/path/mode 会被外层覆盖）。
func mergeXHTTPExtra(opts map[string]string) map[string]any {
	merged := make(map[string]any, len(opts))
	if raw, ok := opts["extra"]; ok && raw != "" {
		var extra map[string]any
		if err := json.Unmarshal([]byte(raw), &extra); err == nil {
			for k, v := range extra {
				switch k {
				case "host", "path", "mode":
					continue // xray 强制以外层为准
				}
				merged[k] = v
			}
		}
	}
	for k, v := range opts {
		if k == "extra" {
			continue
		}
		merged[k] = v
	}
	// headers 可能是对象（来自 extra）也可能是 JSON 字符串（来自 Opts）。
	if hv, ok := merged["headers"]; ok {
		delete(merged, "headers")
		switch h := hv.(type) {
		case map[string]any:
			if len(h) > 0 {
				merged["headers"] = h
			}
		case string:
			var parsed map[string]any
			if err := json.Unmarshal([]byte(h), &parsed); err == nil && len(parsed) > 0 {
				merged["headers"] = parsed
			}
		}
	}
	return merged
}

// renderXHTTPOpts 渲染 mihomo `xhttp-opts: { ... }` 的流式内容。
// 基础三键来自 DTO 的类型化字段，进阶键来自 Opts；reuse-settings 始终显式输出，
// 不依赖 mihomo 的「不写即不复用」默认。
func renderXHTTPOpts(t *contracts.TransportOptions) string {
	if t == nil {
		return ""
	}
	parts := make([]string, 0, 20)
	if t.Mode != "" {
		parts = append(parts, fmt.Sprintf("mode: %s", yamlQuote(t.Mode)))
	}
	path := t.Path
	if path == "" {
		path = "/"
	}
	parts = append(parts, fmt.Sprintf("path: %s", yamlQuote(path)))
	if t.Host != "" {
		parts = append(parts, fmt.Sprintf("host: %s", yamlQuote(t.Host)))
	}

	merged := mergeXHTTPExtra(t.Opts)

	// headers：mihomo 的 xhttp-opts.headers 是 map<string,string>
	if raw, ok := merged["headers"].(map[string]any); ok && len(raw) > 0 {
		keys := make([]string, 0, len(raw))
		for k := range raw {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fields := make([]string, 0, len(keys))
		for _, k := range keys {
			fields = append(fields, fmt.Sprintf("%s: %s", yamlQuote(k), yamlQuote(fmt.Sprint(raw[k]))))
		}
		parts = append(parts, fmt.Sprintf("headers: { %s }", strings.Join(fields, ", ")))
	}

	// 其余标量键
	for _, spec := range xhttpOptSpecs {
		raw, ok := merged[spec.xrayKey]
		if !ok {
			continue
		}
		if spec.boolean {
			switch b := raw.(type) {
			case bool:
				parts = append(parts, fmt.Sprintf("%s: %t", spec.mihomoKey, b))
			case string:
				// Opts 里的值来自 scalarToString，布尔会被序列化成 "true"/"false"
				if v, err := strconv.ParseBool(strings.TrimSpace(b)); err == nil {
					parts = append(parts, fmt.Sprintf("%s: %t", spec.mihomoKey, v))
				}
			}
			continue
		}
		s := xhttpScalarString(raw)
		if s == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", spec.mihomoKey, yamlQuote(s)))
	}

	// reuse-settings（xmux）：缺省补 xray 的默认值
	if reuse := renderReuseSettings(merged["xmux"]); reuse != "" {
		parts = append(parts, fmt.Sprintf("reuse-settings: { %s }", reuse))
	} else {
		defaults := make([]string, 0, len(xrayXmuxDefaults))
		for _, d := range xrayXmuxDefaults {
			defaults = append(defaults, fmt.Sprintf("%s: %s", d.key, yamlQuote(d.val)))
		}
		parts = append(parts, fmt.Sprintf("reuse-settings: { %s }", strings.Join(defaults, ", ")))
	}

	return strings.Join(parts, ", ")
}

// renderReuseSettings 把 xray xmux 对象翻译成 mihomo reuse-settings 内容；
// 空对象/缺失返回空串（由调用方补默认值）。
func renderReuseSettings(v any) string {
	if raw, ok := v.(string); ok {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return ""
		}
		v = parsed
	}
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(xmuxKeyMap))
	for _, km := range xmuxKeyMap {
		raw, ok := m[km.xray]
		if !ok {
			continue
		}
		if km.mihomo == "h-keep-alive-period" {
			// mihomo 该字段是 int
			switch n := raw.(type) {
			case float64:
				parts = append(parts, fmt.Sprintf("%s: %d", km.mihomo, int64(n)))
			case string:
				if n = strings.TrimSpace(n); n != "" {
					parts = append(parts, fmt.Sprintf("%s: %s", km.mihomo, n))
				}
			}
			continue
		}
		s := xhttpScalarString(raw)
		if s == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", km.mihomo, yamlQuote(s)))
	}
	return strings.Join(parts, ", ")
}

// hy2Opts 从 DTO 读取 Hysteria 2 客户端扩展参数。
func hy2Opts(dto *contracts.ProxyNodeDTO) map[string]string {
	if dto == nil || dto.Transport == nil {
		return nil
	}
	return dto.Transport.Opts
}

// hy2ObfsYAML 渲染 obfs 相关片段（salamander / gecko）。
func hy2ObfsYAML(opts map[string]string) []string {
	if opts == nil {
		return nil
	}
	obfs := opts[protocols.Hy2OptObfs]
	if obfs == "" {
		return nil
	}
	out := []string{fmt.Sprintf("obfs: %s", obfs)}
	if pw := opts[protocols.Hy2OptObfsPassword]; pw != "" {
		out = append(out, fmt.Sprintf("obfs-password: %s", yamlQuote(pw)))
	}
	if min := opts[protocols.Hy2OptObfsMinPacket]; min != "" {
		out = append(out, fmt.Sprintf("obfs-min-packet-size: %s", min))
	}
	if max := opts[protocols.Hy2OptObfsMaxPacket]; max != "" {
		out = append(out, fmt.Sprintf("obfs-max-packet-size: %s", max))
	}
	return out
}
