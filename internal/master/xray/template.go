package xray

import (
	"encoding/json"
	_ "embed"
	"sync"
)

//go:embed config.template.json
var defaultTemplateJSON []byte

var (
	templateMu    sync.RWMutex
	templateCache map[string]any // DB 覆盖后的模板（nil=用默认）
)

// LoadTemplate 返回当前生效的模板（DB 覆盖 > 嵌入默认）。
// 返回值为 map[string]any 以便 Generate 逐段读取和修改。
func LoadTemplate() map[string]any {
	templateMu.RLock()
	if templateCache != nil {
		templateMu.RUnlock()
		return cloneMap(templateCache)
	}
	templateMu.RUnlock()

	var tmpl map[string]any
	if err := json.Unmarshal(defaultTemplateJSON, &tmpl); err != nil {
		panic("xray: 内嵌默认模板 JSON 无效: " + err.Error())
	}
	return tmpl
}

// SetTemplate 设置 DB 自定义模板（管理员 WebUI 编辑后调用）。
// data 为 JSON 文本；若为空则回退到默认模板。
func SetTemplate(data []byte) error {
	templateMu.Lock()
	defer templateMu.Unlock()

	if len(data) == 0 {
		templateCache = nil
		return nil
	}

	var tmpl map[string]any
	if err := json.Unmarshal(data, &tmpl); err != nil {
		return err
	}
	templateCache = tmpl
	return nil
}

// cloneMap 深拷贝模板（含嵌套 map/slice）。
// 必须深拷贝：Generate 会原地改写嵌套段（如向出站 settings 注入 domainStrategy、
// 改写 vnext users），浅拷贝会污染共享缓存，导致跨服务器配置串扰与并发数据竞争。
func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = deepCopyValue(v)
	}
	return dst
}

// deepCopyValue 递归复制 JSON 派生值（map/slice/标量）。
func deepCopyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return cloneMap(t)
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = deepCopyValue(item)
		}
		return out
	default:
		return v // 标量（string/float64/bool/nil）不可变，直接复用
	}
}
