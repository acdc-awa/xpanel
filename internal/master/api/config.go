package api

import (
	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// PublicConfig GET /api/v1/config —— 公开配置（人机验证与站点品牌展示前端初始化所需）。
// 仅下发非敏感项（site key 公开；secret key 永不下发）。
func (d *Deps) PublicConfig(c *gin.Context) {
	cfg := services.LoadCaptchaConfig(d.DB)
	site := d.Site.SiteGroup()
	util.OK(c, gin.H{
		"captcha_enable":     cfg.Enabled,
		"captcha_type":       cfg.Type,
		"turnstile_site_key": cfg.SiteKey,
		"app_name":           site[services.SettingAppName],
		"app_description":    site[services.SettingAppDesc],
		"logo":               site[services.SettingLogo],
		"favicon":            site[services.SettingFavicon],
		"stop_register":      site[services.SettingStopRegister] == "1",
		"tos_url":            site[services.SettingTOSURL],
		"currency":           site[services.SettingCurrency],
		"currency_symbol":    site[services.SettingCurrencySymbol],
		"subscribe_url":      site[services.SettingSubscribeURL],
		"subscribe_path":     site[services.SettingSubscribePath],
		"subscribe_port":     site[services.SettingSubscribePort],
	})
}
