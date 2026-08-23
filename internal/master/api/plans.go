package api

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc/xray-panel/internal/models"
	"github.com/acdc/xray-panel/internal/pkg/util"
)

type planView struct {
	ID                uint64    `json:"id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	PriceCents        int64     `json:"price_cents"`
	TrafficGB         int64     `json:"traffic_gb"`
	DurationDays      int       `json:"duration_days"`
	DeviceLimit       int       `json:"device_limit"`        // 0=不限
	PermissionGroupID uint64    `json:"permission_group_id"` // 0=未绑定；购买后按权限组动态授权入站
	Enabled           bool      `json:"enabled"`
	CreatedAt         time.Time `json:"created_at"`
}

func toPlanView(p *models.Plan) planView {
	return planView{
		ID: p.ID, Name: p.Name, Description: p.Description, PriceCents: p.PriceCents, TrafficGB: p.TrafficGB,
		DurationDays: p.DurationDays,
		DeviceLimit: p.DeviceLimit,
		PermissionGroupID: p.PermissionGroupID,
		Enabled: p.Enabled, CreatedAt: p.CreatedAt,
	}
}

// AdminPlans GET /api/v1/admin/plans
func (d *Deps) AdminPlans(c *gin.Context) {
	var list []models.Plan
	if err := d.DB.Order("id ASC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	items := make([]planView, 0, len(list))
	for i := range list {
		items = append(items, toPlanView(&list[i]))
	}
	util.OK(c, gin.H{"items": items})
}

// AdminCreatePlan POST /api/v1/admin/plans
func (d *Deps) AdminCreatePlan(c *gin.Context) {
	var req struct {
		Name              string `json:"name" binding:"required,max=64"`
		Description       string `json:"description"`
		PriceCents        int64  `json:"price_cents" binding:"required,min=0"`
		TrafficGB         int64  `json:"traffic_gb" binding:"required,min=1"`
		DurationDays      int    `json:"duration_days" binding:"required,min=1"`
		DeviceLimit       int    `json:"device_limit"`
		PermissionGroupID uint64 `json:"permission_group_id"` // 0=不绑定
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if req.PermissionGroupID != 0 {
		var cnt int64
		d.DB.Model(&models.PermissionGroup{}).Where("id = ?", req.PermissionGroupID).Count(&cnt)
		if cnt == 0 {
			util.BadRequest(c, "权限组不存在")
			return
		}
	}
	plan := models.Plan{
		Name: req.Name, Description: req.Description, PriceCents: req.PriceCents, TrafficGB: req.TrafficGB,
		DurationDays: req.DurationDays,
		DeviceLimit: req.DeviceLimit,
		PermissionGroupID: req.PermissionGroupID, Enabled: true,
	}
	if err := d.DB.Create(&plan).Error; err != nil {
		util.ServerError(c, "创建失败")
		return
	}
	d.TriggerUserChange()
	util.OK(c, gin.H{"plan": toPlanView(&plan)})
}

// AdminUpdatePlan PUT /api/v1/admin/plans/:id
func (d *Deps) AdminUpdatePlan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var plan models.Plan
	if err := d.DB.First(&plan, id).Error; err != nil {
		util.Fail(c, 404, "套餐不存在")
		return
	}
	var req struct {
		Name              *string `json:"name"`
		Description       *string `json:"description"`
		PriceCents        *int64  `json:"price_cents"`
		TrafficGB         *int64  `json:"traffic_gb"`
		DurationDays      *int    `json:"duration_days"`
		DeviceLimit       *int    `json:"device_limit"`
		PermissionGroupID *uint64 `json:"permission_group_id"` // 显式 0 解绑
		Enabled           *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if req.PermissionGroupID != nil && *req.PermissionGroupID != 0 {
		var cnt int64
		d.DB.Model(&models.PermissionGroup{}).Where("id = ?", *req.PermissionGroupID).Count(&cnt)
		if cnt == 0 {
			util.BadRequest(c, "权限组不存在")
			return
		}
	}
	// ISSUE-13：更新接口语义校验（创建接口由 binding 兜底，更新指针字段无 binding）。
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 64 {
			util.BadRequest(c, "套餐名称需为 1-64 字符")
			return
		}
	}
	if req.PriceCents != nil && *req.PriceCents < 0 {
		util.BadRequest(c, "价格不能为负数")
		return
	}
	if req.TrafficGB != nil && *req.TrafficGB < 1 {
		util.BadRequest(c, "套餐流量至少为 1GB")
		return
	}
	if req.DurationDays != nil && *req.DurationDays < 1 {
		util.BadRequest(c, "套餐时长至少为 1 天")
		return
	}
	if req.DeviceLimit != nil && *req.DeviceLimit < 0 {
		util.BadRequest(c, "设备限制不能为负数（0=不限）")
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.PriceCents != nil {
		updates["price_cents"] = *req.PriceCents
	}
	if req.TrafficGB != nil {
		updates["traffic_gb"] = *req.TrafficGB
	}
	if req.DurationDays != nil {
		updates["duration_days"] = *req.DurationDays
	}
	if req.DeviceLimit != nil {
		updates["device_limit"] = *req.DeviceLimit
	}
	if req.PermissionGroupID != nil {
		updates["permission_group_id"] = *req.PermissionGroupID
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(updates) > 0 {
		if err := d.DB.Model(&plan).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新失败")
			return
		}
		d.TriggerUserChange()
	}
	d.DB.First(&plan, id)
	util.OK(c, gin.H{"plan": toPlanView(&plan)})
}

// AdminDeletePlan DELETE /api/v1/admin/plans/:id
// 2026-08-14 U6：有用户/订单引用时拒绝删除（防悬挂 plan_id 与流量限额失效）。
func (d *Deps) AdminDeletePlan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var userCnt int64
	d.DB.Model(&models.User{}).Where("plan_id = ?", id).Count(&userCnt)
	if userCnt > 0 {
		util.BadRequest(c, "该套餐有 "+strconv.FormatInt(userCnt, 10)+" 个用户持有，无法删除（可先停用套餐）")
		return
	}
	var orderCnt int64
	d.DB.Model(&models.Order{}).Where("plan_id = ?", id).Count(&orderCnt)
	if orderCnt > 0 {
		util.BadRequest(c, "该套餐存在订单记录，无法删除（可先停用套餐）")
		return
	}
	if err := d.DB.Delete(&models.Plan{}, id).Error; err != nil {
		util.ServerError(c, "删除失败")
		return
	}
	util.OK(c, gin.H{"deleted": id})
}

// PublicPlans GET /api/v1/plans —— 用户端上架套餐列表。
func (d *Deps) PublicPlans(c *gin.Context) {
	var list []models.Plan
	if err := d.DB.Where("enabled = ?", true).Order("price_cents ASC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	items := make([]planView, 0, len(list))
	for i := range list {
		items = append(items, toPlanView(&list[i]))
	}
	util.OK(c, gin.H{"items": items})
}
