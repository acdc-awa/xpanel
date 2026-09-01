package contracts

// ExporterRegistry 订阅导出器注册表。
// 支持按 FormatKey 查找，也支持按 User-Agent 匹配选型。
type ExporterRegistry struct {
	exporters []SubscriptionExporter
}

// NewExporterRegistry 创建空注册表。
func NewExporterRegistry() *ExporterRegistry {
	return &ExporterRegistry{}
}

// Register 注册导出器；后注册的同名导出器会覆盖之前的同名项。
func (r *ExporterRegistry) Register(e SubscriptionExporter) {
	if e == nil {
		return
	}
	key := e.FormatKey()
	for i, old := range r.exporters {
		if old.FormatKey() == key {
			r.exporters[i] = e
			return
		}
	}
	r.exporters = append(r.exporters, e)
}

// Find 按 FormatKey 查找导出器；未找到返回 nil。
func (r *ExporterRegistry) Find(key string) SubscriptionExporter {
	for _, e := range r.exporters {
		if e.FormatKey() == key {
			return e
		}
	}
	return nil
}

// Match 按 User-Agent 匹配导出器；未匹配返回 nil。
// 注册顺序即优先级，先注册的优先被匹配。
func (r *ExporterRegistry) Match(ua string) SubscriptionExporter {
	for _, e := range r.exporters {
		if e.MatchUserAgent(ua) {
			return e
		}
	}
	return nil
}

