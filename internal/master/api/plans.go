package api

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

type planView struct {
	ID             uint64    `json:"id"`
	Name           string    `json:"name"`
	PriceCents     int64     `json:"price_cents"`
	TrafficGB      int64     `json:"traffic_gb"`
	DurationDays   int       `json:"duration_days"`
	SpeedLimitKbps int64     `json:"speed_limit_kbps"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
}

func toPlanView(p *models.Plan) planView {
	return planView{
		ID: p.ID, Name: p.Name, PriceCents: p.PriceCents, TrafficGB: p.TrafficGB,
		DurationDays: p.DurationDays, SpeedLimitKbps: p.SpeedLimitKbps,
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
		Name           string `json:"name" binding:"required,max=64"`
		PriceCents     int64  `json:"price_cents" binding:"required,min=0"`
		TrafficGB      int64  `json:"traffic_gb" binding:"required,min=1"`
		DurationDays   int    `json:"duration_days" binding:"required,min=1"`
		SpeedLimitKbps int64  `json:"speed_limit_kbps"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	plan := models.Plan{
		Name: req.Name, PriceCents: req.PriceCents, TrafficGB: req.TrafficGB,
		DurationDays: req.DurationDays, SpeedLimitKbps: req.SpeedLimitKbps, Enabled: true,
	}
	if err := d.DB.Create(&plan).Error; err != nil {
		util.ServerError(c, "创建失败")
		return
	}
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
		Name           *string `json:"name"`
		PriceCents     *int64  `json:"price_cents"`
		TrafficGB      *int64  `json:"traffic_gb"`
		DurationDays   *int    `json:"duration_days"`
		SpeedLimitKbps *int64  `json:"speed_limit_kbps"`
		Enabled        *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
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
	if req.SpeedLimitKbps != nil {
		updates["speed_limit_kbps"] = *req.SpeedLimitKbps
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(updates) > 0 {
		if err := d.DB.Model(&plan).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新失败")
			return
		}
	}
	d.DB.First(&plan, id)
	util.OK(c, gin.H{"plan": toPlanView(&plan)})
}

// AdminDeletePlan DELETE /api/v1/admin/plans/:id
func (d *Deps) AdminDeletePlan(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
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