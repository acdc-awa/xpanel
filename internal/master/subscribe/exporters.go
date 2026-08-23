package subscribe

import (
	"context"
	"strings"

	"github.com/acdc/xray-panel/internal/contracts"
	"github.com/acdc/xray-panel/internal/master/xray"
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

// ProxyItemsToDTOs 将内部 ProxyItem 列表转为订阅导出统一使用的 ProxyNodeDTO 列表。
func ProxyItemsToDTOs(items []ProxyItem) []contracts.ProxyNodeDTO {
	dtos := make([]contracts.ProxyNodeDTO, 0, len(items))
	for i := range items {
		dtos = append(dtos, items[i].ToDTO())
	}
	return dtos
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

func (clashExporter) Export(ctx context.Context, user contracts.UserSummaryDTO, nodes []contracts.ProxyNodeDTO, opts contracts.ExportOptions) (string, string, error) {
	items := dtoToProxyItems(nodes)
	u := &models.User{UUID: user.UUID}
	return BuildClashWithTemplate(u, items, opts.Template, opts.PanelHost), "application/yaml; charset=utf-8", nil
}

// base64Exporter Base64 vless:// 订阅导出器（兜底）。
type base64Exporter struct{}

func (base64Exporter) FormatKey() string { return "base64" }

func (base64Exporter) MatchUserAgent(ua string) bool { return true }

func (base64Exporter) Export(ctx context.Context, user contracts.UserSummaryDTO, nodes []contracts.ProxyNodeDTO, opts contracts.ExportOptions) (string, string, error) {
	items := dtoToProxyItems(nodes)
	u := &models.User{UUID: user.UUID}
	return BuildBase64(u, items), "text/plain; charset=utf-8", nil
}

// dtoToProxyItems 将导出统一 DTO 转换回内部 ProxyItem，复用现有 Clash/Base64 生成逻辑。
func dtoToProxyItems(dtos []contracts.ProxyNodeDTO) []ProxyItem {
	items := make([]ProxyItem, 0, len(dtos))
	for i := range dtos {
		items = append(items, dtoToProxyItem(&dtos[i]))
	}
	return items
}

func dtoToProxyItem(dto *contracts.ProxyNodeDTO) ProxyItem {
	it := ProxyItem{
		Name:    dto.Name,
		Host:    dto.ServerHost,
		Port:    dto.ServerPort,
		TLSType: proxySecurityType(dto),
	}
	if dto.Auth != nil {
		it.UUID = dto.Auth.UUID
		it.Flow = dto.Auth.Flow
	}
	if dto.Transport != nil {
		it.Network = dto.Transport.Network
		if dto.Transport.Network == "xhttp" {
			it.XHTTP = &xray.XHTTPSettings{
				Mode: dto.Transport.Mode,
				Path: dto.Transport.Path,
				Host: dto.Transport.Host,
			}
		}
	}
	applySecurityToProxyItem(dto, &it)
	for _, f := range dto.Features {
		if f == "no-auto-flow" {
			it.NoAutoFlow = true
		}
	}
	return it
}

// proxySecurityType 返回安全类型（nil-safe）。
func proxySecurityType(dto *contracts.ProxyNodeDTO) string {
	if dto.Security == nil {
		return ""
	}
	return dto.Security.Type
}

// applySecurityToProxyItem 将 SecurityOptions 同步到内部 ProxyItem 的 TLS/REALITY 字段。
func applySecurityToProxyItem(dto *contracts.ProxyNodeDTO, it *ProxyItem) {
	if dto.Security == nil {
		return
	}
	switch dto.Security.Type {
	case "reality":
		it.TLSType = "reality"
		it.Reality = &xray.RealitySettings{
			ServerName: dto.Security.SNI,
		}
		if dto.Security.Reality != nil {
			it.Reality.PublicKey = dto.Security.Reality.PublicKey
			it.Reality.ShortID = dto.Security.Reality.ShortID
		}
	case "tls":
		it.TLSType = "tls"
		it.TLS = &xray.TLSSettings{
			ServerName:    dto.Security.SNI,
			AllowInsecure: dto.Security.AllowInsecure,
		}
	default:
		it.TLSType = "none"
	}
}
