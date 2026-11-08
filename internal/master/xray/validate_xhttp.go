package xray

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// 面板口径：Int32Range 只接受 0..2147483647（见 infra/conf/common.go 的 Int32Range）。
// xray 自身允许负数与倒序（倒序会交换），mihomo 两者都拒绝；面板统一收敛到更严的
// mihomo 口径，避免同一份 xmux 在 Clash 订阅里加载失败。
const xmuxRangeMax = 2147483647

var xmuxRangePattern = regexp.MustCompile(`^\d+(\s*-\s*\d+)?$`)

// validateXHTTPXmux 校验 streamSettings 里 xhttpSettings.xmux 的语义。
// xmux 只在入站上编辑，但会被两处消费：中转出站的 xray 配置与 Clash 订阅；
// xray 对 maxConnections/maxConcurrency 同时 >0 是硬拒（infra/conf/transport_method.go
// 的 Build），而 mihomo 不校验、只退化成软上限。这里按 xray 的硬约束在保存期拦截，
// 避免错误延迟到节点下发或客户端加载时才暴露。
func validateXHTTPXmux(streamSettings string) error {
	if strings.TrimSpace(streamSettings) == "" {
		return nil
	}
	stream, ok := decodeJSONObject(streamSettings)
	if !ok {
		return nil // JSON 非法由调用方的 JSON 校验报，避免重复/误导
	}
	return validateXHTTPXmuxStream(stream, 0)
}

func validateXHTTPXmuxStream(stream map[string]any, depth int) error {
	if stream == nil || depth > 4 {
		return nil
	}
	network, _ := stream["network"].(string)
	if network != "xhttp" {
		return nil
	}
	xh, ok := stream["xhttpSettings"].(map[string]any)
	if !ok {
		return nil
	}
	if err := validateXmuxObject(xh["xmux"]); err != nil {
		return err
	}
	// extra 是 xray 的第二套入口：Build() 存在 extra 时整体替换配置，其中的 xmux
	// 会覆盖顶层；两种存储形态（对象 / JSON 字符串）都要校验。
	switch extra := xh["extra"].(type) {
	case map[string]any:
		if err := validateXmuxObject(extra["xmux"]); err != nil {
			return err
		}
	case string:
		if strings.TrimSpace(extra) != "" {
			if parsed, ok := decodeJSONObject(extra); ok {
				if err := validateXmuxObject(parsed["xmux"]); err != nil {
					return err
				}
			}
		}
	}
	// downloadSettings 是独立的 StreamConfig，自带 network/xhttpSettings/xmux。
	if ds, ok := xh["downloadSettings"].(map[string]any); ok {
		return validateXHTTPXmuxStream(ds, depth+1)
	}
	return nil
}

func validateXmuxObject(raw any) error {
	if raw == nil {
		return nil
	}
	xmux, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("xhttpSettings.xmux 必须是对象")
	}

	var connUpper, concUpper int64
	for _, key := range []string{
		"maxConnections", "maxConcurrency", "cMaxReuseTimes", "hMaxRequestTimes", "hMaxReusableSecs",
	} {
		v, ok := xmux[key]
		if !ok || v == nil {
			continue
		}
		to, err := parseXmuxRange(key, v)
		if err != nil {
			return err
		}
		switch key {
		case "maxConnections":
			connUpper = to
		case "maxConcurrency":
			concUpper = to
		}
	}
	if connUpper > 0 && concUpper > 0 {
		return fmt.Errorf("xhttpSettings.xmux：maxConnections 与 maxConcurrency 不能同时大于 0")
	}

	if v, ok := xmux["hKeepAlivePeriod"]; ok && v != nil {
		n, ok := v.(json.Number)
		if !ok {
			return fmt.Errorf("xhttpSettings.xmux.hKeepAlivePeriod 必须是整数（-1 关闭空闲保活）")
		}
		iv, err := n.Int64()
		if err != nil || iv < -1 || iv > xmuxRangeMax {
			return fmt.Errorf("xhttpSettings.xmux.hKeepAlivePeriod 必须是 -1 或 0-%d 的整数", xmuxRangeMax)
		}
	}
	return nil
}

// parseXmuxRange 解析 Int32Range 并返回上限（To）。面板口径：非负、min<=max、<= MaxInt32。
func parseXmuxRange(key string, v any) (int64, error) {
	switch t := v.(type) {
	case json.Number:
		n, err := t.Int64()
		if err != nil || n < 0 || n > xmuxRangeMax {
			return 0, fmt.Errorf("xhttpSettings.xmux.%s 必须是 0-%d 的整数", key, xmuxRangeMax)
		}
		return n, nil
	case string:
		s := strings.TrimSpace(t)
		if !xmuxRangePattern.MatchString(s) {
			return 0, fmt.Errorf("xhttpSettings.xmux.%s 区间写法应为 \"最小-最大\" 或整数（0-%d）", key, xmuxRangeMax)
		}
		parts := strings.SplitN(s, "-", 2)
		from, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil || from < 0 || from > xmuxRangeMax {
			return 0, fmt.Errorf("xhttpSettings.xmux.%s 最小值必须是 0-%d 的整数", key, xmuxRangeMax)
		}
		if len(parts) == 1 {
			return from, nil
		}
		to, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || to < 0 || to > xmuxRangeMax {
			return 0, fmt.Errorf("xhttpSettings.xmux.%s 最大值必须是 0-%d 的整数", key, xmuxRangeMax)
		}
		if to < from {
			return 0, fmt.Errorf("xhttpSettings.xmux.%s 最大值不能小于最小值", key)
		}
		return to, nil
	default:
		return 0, fmt.Errorf("xhttpSettings.xmux.%s 必须是整数或 \"最小-最大\" 字符串", key)
	}
}

// decodeJSONObject 用 UseNumber 解析，保留数字原始字面量：xray 的 int32 字段拒绝
// 3.0 / 1e3 这类写法，json.Number 能原样识别，避免校验放行后 xray 反序列化失败。
func decodeJSONObject(s string) (map[string]any, bool) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, false
	}
	return m, true
}
