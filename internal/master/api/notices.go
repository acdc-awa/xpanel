package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/middleware"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

type noticeForm struct {
	Title     string `json:"title" binding:"required"`
	Content   string `json:"content" binding:"required"`
	IsPinned  bool   `json:"is_pinned"`
	IsPopup   bool   `json:"is_popup"`
	Status    *int   `json:"status"`
	SortOrder int    `json:"sort_order"`
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
	if err := q.Order("is_pinned DESC, sort_order DESC, id DESC").Find(&list).Error; err != nil {
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

	notice := models.Notice{
		Title:     strings.TrimSpace(form.Title),
		Content:   form.Content,
		IsPinned:  form.IsPinned,
		IsPopup:   form.IsPopup,
		Status:    status,
		SortOrder: form.SortOrder,
	}

	if err := d.DB.Create(&notice).Error; err != nil {
		util.ServerError(c, "创建公告失败")
		return
	}

	uid := middleware.CurrentUser(c)
	d.Audit.Log("admin", uid, "notice.create", "创建公告 #"+strconv.FormatUint(notice.ID, 10)+"「"+notice.Title+"」", util.ClientIPFromContext(c))
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
	notice.IsPinned = form.IsPinned
	notice.IsPopup = form.IsPopup
	if form.Status != nil {
		notice.Status = *form.Status
	}
	notice.SortOrder = form.SortOrder

	if err := d.DB.Save(&notice).Error; err != nil {
		util.ServerError(c, "更新公告失败")
		return
	}

	uid := middleware.CurrentUser(c)
	d.Audit.Log("admin", uid, "notice.update", "更新公告 #"+strconv.FormatUint(notice.ID, 10)+"「"+notice.Title+"」", util.ClientIPFromContext(c))
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

	uid := middleware.CurrentUser(c)
	d.Audit.Log("admin", uid, "notice.delete", "删除公告 #"+strconv.FormatUint(id, 10)+"「"+notice.Title+"」", util.ClientIPFromContext(c))
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
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
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

	uid := middleware.CurrentUser(c)
	d.Audit.Log("admin", uid, "notice.toggle", "快捷切换公告 #"+strconv.FormatUint(id, 10)+" 字段 "+req.Field, util.ClientIPFromContext(c))
	d.DB.First(&notice, id)
	util.OK(c, notice)
}

// UserListNotices GET /api/v1/user/notices —— 用户端获取已启用的公告列表。
func (d *Deps) UserListNotices(c *gin.Context) {
	var list []models.Notice
	if err := d.DB.Where("status = ?", models.StatusActive).
		Order("is_pinned DESC, sort_order DESC, id DESC").
		Find(&list).Error; err != nil {
		util.ServerError(c, "获取公告失败")
		return
	}
	util.OK(c, list)
}

// UserGetNotice GET /api/v1/user/notices/:id —— 用户端获取单条公告详情。
func (d *Deps) UserGetNotice(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的公告 ID")
		return
	}

	var notice models.Notice
	if err := d.DB.Where("status = ?", models.StatusActive).First(&notice, id).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "公告不存在或已被隐藏")
		return
	}
	util.OK(c, notice)
}
