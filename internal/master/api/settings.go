package api

import (
	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// AdminSettings GET /api/v1/admin/settings —— 站点设置读取（分组式，17 号 P0 ①）。
// 返回三组：site（站点）/ captcha（人机验证，管理端专属，含 secret）/ agent（节点上报周期）。
// 2026-08-24 四端口拆分：web_base 与订阅端口退役（端口走 env/配置，web_base 由域名分流取代）。
func (d *Deps) AdminSettings(c *gin.Context) {
	site := map[string]string{}
	if d.Site != nil {
		site = d.Site.SiteGroup()
	}
	util.OK(c, gin.H{
		"site":    site,
		"captcha": services.CaptchaSettings(d.DB),
		"agent":   services.AgentSettingsGroup(d.DB),
	})
}

// AdminUpdateSettings PUT /api/v1/admin/settings —— 站点设置保存（分组式，整体覆盖）。
// 任意分组可省略（不更新）；stop_register / captcha_enable 规范化为 1/0。
// agent 组保存后即时下发所有在线节点（离线节点由重连时 ServeWS 下发兜底）。
func (d *Deps) AdminUpdateSettings(c *gin.Context) {
	var req struct {
		Site    *map[string]string `json:"site"`
		Captcha *map[string]string `json:"captcha"`
		Agent   *map[string]string `json:"agent"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if d.Site == nil {
		util.ServerError(c, "站点服务未初始化")
		return
	}
	changed := false
	agentChanged := false
	if req.Site != nil {
		if err := d.Site.SetSiteGroup(*req.Site); err != nil {
			util.BadRequest(c, err.Error())
			return
		}
		changed = true
		// 订阅入口路径 / 拒绝码变更 → 重建订阅服务（端口不变）
		if d.SubServer != nil {
			siteMap := *req.Site
			if _, ok := siteMap[services.SettingSubscribePath]; ok {
				_ = d.SubServer.Reload()
			} else if _, ok := siteMap[services.SettingSubDenyCode]; ok {
				_ = d.SubServer.Reload()
			}
		}
	}
	if req.Captcha != nil {
		if err := services.SaveCaptchaSettings(d.DB, *req.Captcha); err != nil {
			util.BadRequest(c, err.Error())
			return
		}
		changed = true
	}
	if req.Agent != nil {
		if err := services.SaveAgentSettingsGroup(d.DB, *req.Agent); err != nil {
			util.BadRequest(c, err.Error())
			return
		}
		changed = true
		agentChanged = true
	}
	if !changed {
		util.BadRequest(c, "没有需要保存的内容")
		return
	}
	if agentChanged && d.Hub != nil {
		d.Hub.BroadcastAgentSettings()
	}
	util.OK(c, gin.H{
		"site":    d.Site.SiteGroup(),
		"captcha": services.CaptchaSettings(d.DB),
		"agent":   services.AgentSettingsGroup(d.DB),
	})
}
