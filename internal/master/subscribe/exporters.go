package subscribe

import (
	"context"
	"strings"

	"github.com/acdc/xray-panel/internal/contracts"
	"github.com/acdc/xray-panel/internal/models"
)

// DefaultRegistry 返回系统内置的订阅导出器注册表。
// 默认顺序：Clash -> Base64（Base64 作为通用兜底）。
func DefaultRegistry() *contracts.ExporterRegistry {
	r := contracts.NewExporterRegistry()
	r.Register(clashExporter{})
	r.Register(base64Exporter{})
	return r
}

// UserToSummaryDTO 将 models.User 转为订阅导出器使用的用户摘要 DTO。
func UserToSummaryDTO(u *models.User) contracts.UserSummaryDTO {
	return contracts.UserSummaryDTO{
		ID:         u.ID,
		Username:   u.Username,
		Email:      u.Email,
		UUID:       u.UUID,
		Status:     u.Status,
		PlanID:     u.PlanID,
		ExpireTime: u.ExpireAt,
	}
}

// clashExporter Clash / Mihomo 订阅导出器。
type clashExporter struct{}

func (clashExporter) FormatKey() string { return "clash" }

func (clashExporter) MatchUserAgent(ua string) bool {
	return strings.Contains(ua, "clash") || strings.Contains(ua, "mihomo") ||
		strings.Contains(ua, "stash") || strings.Contains(ua, "verge")
}

func (clashExporter) Export(_ context.Context, _ contracts.UserSummaryDTO, nodes []contracts.ProxyNodeDTO, opts contracts.ExportOptions) (string, string, error) {
	return BuildClashWithTemplate(nodes, opts.Template, opts.PanelHost), "application/yaml; charset=utf-8", nil
}

// base64Exporter Base64 vless:// 订阅导出器（兜底）。
type base64Exporter struct{}

func (base64Exporter) FormatKey() string { return "base64" }

func (base64Exporter) MatchUserAgent(ua string) bool { return true }

func (base64Exporter) Export(_ context.Context, _ contracts.UserSummaryDTO, nodes []contracts.ProxyNodeDTO, _ contracts.ExportOptions) (string, string, error) {
	return BuildBase64(nodes), "text/plain; charset=utf-8", nil
}
