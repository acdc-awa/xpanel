package api

import (
	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/services"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// PublicConfig GET /api/v1/config —— 公开配置（人机验证等前端初始化所需）。
// 仅下发非敏感项（site key 公开；secret key 永不下发）。
func (d *Deps) PublicConfig(c *gin.Context) {
	cfg := services.LoadCaptchaConfig(d.DB)
	util.OK(c, gin.H{
		"captcha_enable":    cfg.Enabled,
		"captcha_type":      cfg.Type,
		"turnstile_site_key": cfg.SiteKey,
	})
}
