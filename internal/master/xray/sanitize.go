package xray

import (
	"encoding/json"
)

// sanitizeStreamSettings 规范化入站 streamSettings：删除面板专用字段、修复旧版键名、
// 拦截已知的 xray panic 组合。输出前调用，确保 xray-core 收到的配置是干净的。
func sanitizeStreamSettings(raw string) string {
	if raw == "" {
		return raw
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(raw), &stream); err != nil {
		return raw // 解析失败则原样保留（让 xray -test 报错）
	}

	// 面板专用字段，xray 不识别
	delete(stream, "externalProxy")

	// tlsSettings / realitySettings 下的 "settings" 是面板嵌套存储，不进 xray 配置
	deleteNestedKey(stream, "tlsSettings", "settings")
	deleteNestedKey(stream, "realitySettings", "settings")

	// REALITY + finalmask 组合会导致 xray panic（XTLS/Xray-core#6453）
	if security, _ := stream["security"].(string); security == "reality" {
		delete(stream, "finalmask")
	}

	// 清理无效的 finalmask 数据
	dropEmptyRandPackets(stream["finalmask"])

	// xray-core #6258：session 旧键名提升
	liftXhttpSessionKeys(stream)

	result, err := json.Marshal(stream)
	if err != nil {
		return raw
	}
	return string(result)
}

// deleteNestedKey 删除 m[key] 下的 subKey（如果 m[key] 是一个 map）。
func deleteNestedKey(m map[string]any, key, subKey string) {
	if inner, ok := m[key].(map[string]any); ok {
		delete(inner, subKey)
	}
}

// dropEmptyRandPackets 删除 finalmask 下空的 rand 包条目。
func dropEmptyRandPackets(v any) {
	fm, ok := v.(map[string]any)
	if !ok {
		return
	}
	rand, ok := fm["rand"].([]any)
	if !ok {
		return
	}
	filtered := make([]any, 0, len(rand))
	for _, item := range rand {
		if m, ok := item.(map[string]any); ok {
			if len(m) == 0 {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		delete(fm, "rand")
	} else {
		fm["rand"] = filtered
	}
}

// liftXhttpSessionKeys 将 xhttpSettings 下的旧键名提升为新键名（xray-core #6258）。
func liftXhttpSessionKeys(stream map[string]any) {
	xhttp, ok := stream["xhttpSettings"].(map[string]any)
	if !ok {
		return
	}
	if v, ok := xhttp["sessionPlacement"]; ok {
		delete(xhttp, "sessionPlacement")
		xhttp["sessionIDPlacement"] = v
	}
	if v, ok := xhttp["sessionKey"]; ok {
		delete(xhttp, "sessionKey")
		xhttp["sessionID"] = v
	}
}
