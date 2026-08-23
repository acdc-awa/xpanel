package contracts

import "context"

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

// Export 按 UA 选择导出器并导出订阅内容。
func (r *ExporterRegistry) Export(ctx context.Context, ua string, user UserSummaryDTO, nodes []ProxyNodeDTO, opts ExportOptions) (string, string, error) {
	e := r.Match(ua)
	if e == nil {
		return "", "", ErrNoExporterMatched
	}
	return e.Export(ctx, user, nodes, opts)
}

// ErrNoExporterMatched 没有匹配到订阅导出器。
var ErrNoExporterMatched = &ExporterNotFoundError{}

// ExporterNotFoundError 导出器未匹配错误。
type ExporterNotFoundError struct{}

func (e *ExporterNotFoundError) Error() string { return "no subscription exporter matched" }
