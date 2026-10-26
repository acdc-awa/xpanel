package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// subTemplateMaxBytes 模板正文大小上限（与权限组模板使用场景一致，防误传超大内容）。
const subTemplateMaxBytes = 64 * 1024

// subTemplateMaxNameLen 模板名上限（binding:required 只挡空串，空格名/超长名在此兜底）。
const subTemplateMaxNameLen = 64

// validateSubTemplateName TrimSpace 后复查：空名/超长名统一 400。
func validateSubTemplateName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > subTemplateMaxNameLen {
		return "模板名不能为空且不超过 64 字符"
	}
	return ""
}

type subTemplateForm struct {
	Name    string `json:"name" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// subTemplateView 模板库列表视图：附加「被哪些权限组绑定」，供删除确认与影响面提示。
type subTemplateView struct {
	models.SubTemplate
	UsageCount int      `json:"usage_count"`
	GroupNames []string `json:"group_names"`
}

// AdminListSubTemplates GET /api/v1/admin/sub-templates —— 命名订阅模板库列表。
func (d *Deps) AdminListSubTemplates(c *gin.Context) {
	var list []models.SubTemplate
	if err := d.DB.Order("id DESC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询模板库失败")
		return
	}

	// 绑定关系一次查全（权限组表很小；避免逐条模板 N+1）。
	var groups []models.PermissionGroup
	_ = d.DB.Select("id, name, sub_template_id").
		Where("sub_template_id IS NOT NULL").
		Order("id ASC").
		Find(&groups).Error
	usage := make(map[uint64][]string, len(groups))
	for _, g := range groups {
		if g.SubTemplateID != nil {
			usage[*g.SubTemplateID] = append(usage[*g.SubTemplateID], g.Name)
		}
	}

	items := make([]subTemplateView, 0, len(list))
	for _, tpl := range list {
		names := usage[tpl.ID]
		items = append(items, subTemplateView{
			SubTemplate: tpl,
			UsageCount:  len(names),
			GroupNames:  names,
		})
	}
	util.OK(c, items)
}

// AdminCreateSubTemplate POST /api/v1/admin/sub-templates —— 保存模板到模板库。
func (d *Deps) AdminCreateSubTemplate(c *gin.Context) {
	var form subTemplateForm
	if err := c.ShouldBindJSON(&form); err != nil {
		util.BadRequest(c, "请填写模板名与内容")
		return
	}
	if msg := validateSubTemplateName(form.Name); msg != "" {
		util.BadRequest(c, msg)
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
	if msg := validateSubTemplateName(form.Name); msg != "" {
		util.BadRequest(c, msg)
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

// AdminDeleteSubTemplate DELETE /api/v1/admin/sub-templates/:id —— 删除模板库条目。
// 被权限组绑定时允许删除：事务内先把模板当前内容落进各组的 clash_template，再清空
// sub_template_id（转为自定义/空键），最后删模板——删除瞬间订阅输出不变。
func (d *Deps) AdminDeleteSubTemplate(c *gin.Context) {
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

	var detached int64
	err = d.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.PermissionGroup{}).
			Where("sub_template_id = ?", id).
			Updates(map[string]any{
				"clash_template":  tpl.Content,
				"sub_template_id": nil,
			})
		if res.Error != nil {
			return res.Error
		}
		detached = res.RowsAffected
		return tx.Delete(&models.SubTemplate{}, id).Error
	})
	if err != nil {
		util.ServerError(c, "删除模板失败")
		return
	}
	util.OK(c, gin.H{"deleted": true, "detached_group_count": detached})
}
