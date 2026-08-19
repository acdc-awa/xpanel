package api

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/services"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// AdminSettings GET /api/v1/admin/settings —— 站点设置读取（分组式，17 号 P0 ①）。
// 返回三组：site（站点）/ captcha（人机验证，管理端专属，含 secret）/ web_base（访问路径）。
func (d *Deps) AdminSettings(c *gin.Context) {
	site := map[string]string{}
	webBase := ""
	if d.Site != nil {
		site = d.Site.SiteGroup()
		webBase = d.Site.WebBase()
	}
	util.OK(c, gin.H{
		"site":     site,
		"captcha":  services.CaptchaSettings(d.DB),
		"web_base": webBase,
	})
}

// AdminUpdateSettings PUT /api/v1/admin/settings —— 站点设置保存（分组式，整体覆盖）。
// 任意分组可省略（不更新）；stop_register / captcha_enable 规范化为 1/0。
func (d *Deps) AdminUpdateSettings(c *gin.Context) {
	var req struct {
		Site    *map[string]string `json:"site"`
		Captcha *map[string]string `json:"captcha"`
		WebBase *string            `json:"web_base"`
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
	if req.Site != nil {
		if err := d.Site.SetSiteGroup(*req.Site); err != nil {
			util.BadRequest(c, err.Error())
			return
		}
		changed = true
		if d.SubServer != nil {
			siteMap := *req.Site
			if portStr, ok := siteMap[services.SettingSubscribePort]; ok {
				port, _ := strconv.Atoi(strings.TrimSpace(portStr))
				_ = d.SubServer.Reload(port)
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
	if req.WebBase != nil {
		if err := d.Site.SetWebBase(*req.WebBase); err != nil {
			util.BadRequest(c, err.Error())
			return
		}
		changed = true
	}
	if !changed {
		util.BadRequest(c, "没有需要保存的内容")
		return
	}
	util.OK(c, gin.H{
		"site":     d.Site.SiteGroup(),
		"captcha":  services.CaptchaSettings(d.DB),
		"web_base": d.Site.WebBase(),
	})
}
