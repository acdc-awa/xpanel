package api

import (
	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/pkg/util"
)

// AdminSettings GET /api/v1/admin/settings —— 站点设置读取（当前：web_base）。
func (d *Deps) AdminSettings(c *gin.Context) {
	webBase := ""
	if d.Site != nil {
		webBase = d.Site.WebBase()
	}
	util.OK(c, gin.H{"web_base": webBase})
}

// AdminUpdateSettings PUT /api/v1/admin/settings —— 站点设置保存（web_base）。
func (d *Deps) AdminUpdateSettings(c *gin.Context) {
	var req struct {
		WebBase string `json:"web_base"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if d.Site == nil {
		util.ServerError(c, "站点服务未初始化")
		return
	}
	if err := d.Site.SetWebBase(req.WebBase); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	util.OK(c, gin.H{"web_base": d.Site.WebBase()})
}
