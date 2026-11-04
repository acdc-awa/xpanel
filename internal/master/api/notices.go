package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// noticeForm 中所有非标题/正文字段都是指针：缺省即「不改」。
// 编辑弹窗不再包含置顶/弹窗开关（改由列表卡片直接切换），漏传不能把它们清成 false。
type noticeForm struct {
	Title     string `json:"title" binding:"required"`
	Content   string `json:"content" binding:"required"`
	IsPinned  *bool  `json:"is_pinned"`
	IsPopup   *bool  `json:"is_popup"`
	Status    *int   `json:"status"`
	SortOrder *int   `json:"sort_order"`
}

// AdminListNotices GET /api/v1/admin/notices —— 管理端公告列表。
func (d *Deps) AdminListNotices(c *gin.Context) {
	q := d.DB.Model(&models.Notice{})
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("title LIKE ? OR content LIKE ?", like, like)
	}
	if st := c.Query("status"); st != "" {
		if v, err := strconv.Atoi(st); err == nil {
			q = q.Where("status = ?", v)
		}
	}

	var list []models.Notice
	if err := q.Order("is_pinned DESC, sort_order ASC, id ASC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询公告失败")
		return
	}
	util.OK(c, list)
}

// AdminCreateNotice POST /api/v1/admin/notices —— 管理端创建公告。
func (d *Deps) AdminCreateNotice(c *gin.Context) {
	var form noticeForm
	if err := c.ShouldBindJSON(&form); err != nil {
		util.BadRequest(c, "请填写标题与内容")
		return
	}

	status := models.StatusActive
	if form.Status != nil {
		status = *form.Status
	}

	// 排序由列表拖拽维护；未显式指定时追加到末尾。
	sortOrder := nextSortOrder(d.DB, &models.Notice{}, nil)
	if form.SortOrder != nil {
		sortOrder = *form.SortOrder
	}

	notice := models.Notice{
		Title:     strings.TrimSpace(form.Title),
		Content:   form.Content,
		IsPinned:  form.IsPinned != nil && *form.IsPinned,
		IsPopup:   form.IsPopup != nil && *form.IsPopup,
		Status:    status,
		SortOrder: sortOrder,
	}

	if err := d.DB.Create(&notice).Error; err != nil {
		util.ServerError(c, "创建公告失败")
		return
	}

	// Status 列带 gorm default:1：零值（草稿）在 Create 时会被数据库默认值顶成已发布，
	// 必须显式回写一次。这与 GORM 的零值省略规则有关，不是业务判断。
	if status != models.StatusActive {
		if err := d.DB.Model(&notice).UpdateColumn("status", status).Error; err != nil {
			util.ServerError(c, "创建公告失败")
			return
		}
		notice.Status = status
	}

	// 审计由中间件统一落库（envelope body 含标题，不再手动打点）
	util.OK(c, notice)
}

// AdminUpdateNotice PUT /api/v1/admin/notices/:id —— 管理端更新公告。
func (d *Deps) AdminUpdateNotice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的公告 ID")
		return
	}

	var notice models.Notice
	if err := d.DB.First(&notice, id).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "公告不存在")
		return
	}

	var form noticeForm
	if err := c.ShouldBindJSON(&form); err != nil {
		util.BadRequest(c, "请填写标题与内容")
		return
	}

	notice.Title = strings.TrimSpace(form.Title)
	notice.Content = form.Content
	if form.IsPinned != nil {
		notice.IsPinned = *form.IsPinned
	}
	if form.IsPopup != nil {
		notice.IsPopup = *form.IsPopup
	}
	if form.Status != nil {
		notice.Status = *form.Status
	}
	if form.SortOrder != nil {
		notice.SortOrder = *form.SortOrder
	}

	if err := d.DB.Save(&notice).Error; err != nil {
		util.ServerError(c, "更新公告失败")
		return
	}

	util.OK(c, notice)
}

// AdminDeleteNotice DELETE /api/v1/admin/notices/:id —— 管理端删除公告。
func (d *Deps) AdminDeleteNotice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的公告 ID")
		return
	}

	var notice models.Notice
	if err := d.DB.First(&notice, id).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "公告不存在")
		return
	}

	if err := d.DB.Delete(&notice).Error; err != nil {
		util.ServerError(c, "删除公告失败")
		return
	}

	util.OK(c, gin.H{"deleted": true})
}

// AdminToggleNotice POST /api/v1/admin/notices/:id/toggle —— 快捷切换状态。
func (d *Deps) AdminToggleNotice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的公告 ID")
		return
	}

	var notice models.Notice
	if err := d.DB.First(&notice, id).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "公告不存在")
		return
	}

	var req struct {
		Field string `json:"field" binding:"required"` // status, is_pinned, is_popup
		Value any    `json:"value"`
	}
	if !util.BindJSON(c, &req) {
		return
	}

	updates := map[string]any{}
	switch req.Field {
	case "status":
		if notice.Status == models.StatusActive {
			updates["status"] = models.StatusDisabled
		} else {
			updates["status"] = models.StatusActive
		}
	case "is_pinned":
		updates["is_pinned"] = !notice.IsPinned
	case "is_popup":
		updates["is_popup"] = !notice.IsPopup
	default:
		util.BadRequest(c, "不支持的切换字段")
		return
	}

	if err := d.DB.Model(&notice).Updates(updates).Error; err != nil {
		util.ServerError(c, "切换状态失败")
		return
	}

	d.DB.First(&notice, id)
	util.OK(c, notice)
}

// AdminReorderNotices PUT /api/v1/admin/notices/order
// body: {"ids":[...]} —— 全量顺序（数组下标即新 sort_order）。
// 置顶公告始终排在最前，故排序值只需保证组内相对顺序。
func (d *Deps) AdminReorderNotices(c *gin.Context) {
	reorderAll(c, d.DB, &models.Notice{}, nil, "排序列表已过期（公告数量已变化），请刷新后重试")
}

// UserListNotices GET /api/v1/user/notices —— 用户端获取已启用的公告列表。
func (d *Deps) UserListNotices(c *gin.Context) {
	var list []models.Notice
	if err := d.DB.Where("status = ?", models.StatusActive).
		Order("is_pinned DESC, sort_order ASC, id ASC").
		Find(&list).Error; err != nil {
		util.ServerError(c, "获取公告失败")
		return
	}
	util.OK(c, list)
}
