package xray

import (
	"encoding/json"
	_ "embed"
	"sync"
)

//go:embed config.template.json
var defaultTemplateJSON []byte

// defaultTmpl 内嵌默认模板解析结果（进程内只读不可变——模板定死，不提供在线编辑入口）。
var (
	defaultTmplOnce sync.Once
	defaultTmpl     map[string]any
)

// LoadTemplate 返回内嵌默认模板的深拷贝（Generate 会原地改写嵌套段，调用方必须持有独立副本）。
// 返回值为 map[string]any 以便 Generate 逐段读取和修改。
func LoadTemplate() map[string]any {
	defaultTmplOnce.Do(func() {
		var tmpl map[string]any
		if err := json.Unmarshal(defaultTemplateJSON, &tmpl); err != nil {
			panic("xray: 内嵌默认模板 JSON 无效: " + err.Error())
		}
		defaultTmpl = tmpl
	})
	return cloneMap(defaultTmpl)
}

// cloneMap 深拷贝模板（含嵌套 map/slice）。
// 必须深拷贝：Generate 会原地改写嵌套段（如向出站 settings 注入 domainStrategy、
// 改写 vnext users），浅拷贝会污染共享的默认模板，导致跨服务器配置串扰与并发数据竞争。
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
