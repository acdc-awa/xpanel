package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// subTemplateMaxBytes 模板正文大小上限（与权限组模板使用场景一致，防误传超大内容）。
const subTemplateMaxBytes = 64 * 1024

type subTemplateForm struct {
	Name    string `json:"name" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// AdminListSubTemplates GET /api/v1/admin/sub-templates —— 命名订阅模板库列表。
func (d *Deps) AdminListSubTemplates(c *gin.Context) {
	var list []models.SubTemplate
	if err := d.DB.Order("id DESC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询模板库失败")
		return
	}
	util.OK(c, list)
}

// AdminCreateSubTemplate POST /api/v1/admin/sub-templates —— 保存模板到模板库。
func (d *Deps) AdminCreateSubTemplate(c *gin.Context) {
	var form subTemplateForm
	if err := c.ShouldBindJSON(&form); err != nil {
		util.BadRequest(c, "请填写模板名与内容")
		return
	}
	if len(form.Content) > subTemplateMaxBytes {
		util.BadRequest(c, "模板内容过大")
		return
	}

	tpl := models.SubTemplate{
		Name:    strings.TrimSpace(form.Name),
		Content: form.Content,
	}
	if err := d.DB.Create(&tpl).Error; err != nil {
		util.ServerError(c, "保存模板失败")
		return
	}
	// 审计由中间件统一落库（envelope body 含模板名）
	util.OK(c, tpl)
}

// AdminUpdateSubTemplate PUT /api/v1/admin/sub-templates/:id —— 更新模板库条目。
func (d *Deps) AdminUpdateSubTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的模板 ID")
		return
	}
	var tpl models.SubTemplate
	if err := d.DB.First(&tpl, id).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "模板不存在")
		return
	}

	var form subTemplateForm
	if err := c.ShouldBindJSON(&form); err != nil {
		util.BadRequest(c, "请填写模板名与内容")
		return
	}
	if len(form.Content) > subTemplateMaxBytes {
		util.BadRequest(c, "模板内容过大")
		return
	}

	tpl.Name = strings.TrimSpace(form.Name)
	tpl.Content = form.Content
	if err := d.DB.Save(&tpl).Error; err != nil {
		util.ServerError(c, "更新模板失败")
		return
	}
	util.OK(c, tpl)
}

// AdminDeleteSubTemplate DELETE /api/v1/admin/sub-templates/:id —— 删除模板库条目
// （仅删库存条目，各权限组已应用的 clash_template 不受影响）。
func (d *Deps) AdminDeleteSubTemplate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的模板 ID")
		return
	}
	if err := d.DB.Delete(&models.SubTemplate{}, id).Error; err != nil {
		util.ServerError(c, "删除模板失败")
		return
	}
	util.OK(c, gin.H{"deleted": true})
}
