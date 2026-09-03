package api

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
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
	SortOrder         int       `json:"sort_order"`          // 商城展示排序（越小越靠前）
	IsFeatured        bool      `json:"is_featured"`         // 商城「热门推荐」标记
	Purchasable       bool      `json:"purchasable"`         // 可新购（商店展示并允许非持有者购买）
	Renewable         bool      `json:"renewable"`           // 可续费（持有者余额直付顺延）
	CreatedAt         time.Time `json:"created_at"`
}

func toPlanView(p *models.Plan) planView {
	return planView{
		ID: p.ID, Name: p.Name, Description: p.Description, PriceCents: p.PriceCents, TrafficGB: p.TrafficGB,
		DurationDays:      p.DurationDays,
		DeviceLimit:       p.DeviceLimit,
		PermissionGroupID: p.PermissionGroupID,
		SortOrder:         p.SortOrder, IsFeatured: p.IsFeatured,
		Purchasable: p.Purchasable, Renewable: p.Renewable, CreatedAt: p.CreatedAt,
	}
}

// publicPlanView 公开套餐视图：隐藏 permission_group_id（内部权限组 ID 不下发，防枚举/探测）。
type publicPlanView struct {
	ID           uint64    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	PriceCents   int64     `json:"price_cents"`
	TrafficGB    int64     `json:"traffic_gb"`
	DurationDays int       `json:"duration_days"`
	DeviceLimit  int       `json:"device_limit"` // 0=不限
	IsFeatured   bool      `json:"is_featured"`  // 商城「热门推荐」标记（前端据此挂徽标）
	Purchasable  bool      `json:"purchasable"`
	Renewable    bool      `json:"renewable"`
	CreatedAt    time.Time `json:"created_at"`
}

func toPublicPlanView(p *models.Plan) publicPlanView {
	return publicPlanView{
		ID: p.ID, Name: p.Name, Description: p.Description, PriceCents: p.PriceCents, TrafficGB: p.TrafficGB,
		DurationDays: p.DurationDays,
		DeviceLimit:  p.DeviceLimit,
		IsFeatured:   p.IsFeatured,
		Purchasable:  p.Purchasable, Renewable: p.Renewable, CreatedAt: p.CreatedAt,
	}
}

// AdminPlans GET /api/v1/admin/plans
func (d *Deps) AdminPlans(c *gin.Context) {
	var list []models.Plan
	// 与商城同序（sort_order 优先），管理端所见即用户端展示顺序
	if err := d.DB.Order("sort_order ASC, id ASC").Find(&list).Error; err != nil {
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
		SortOrder         int    `json:"sort_order"`          // 展示排序（越小越靠前）
		IsFeatured        bool   `json:"is_featured"`         // 「热门推荐」标记
		// 销售两属性：缺省 true（新套餐默认全开）；GORM default:false 下 true 非零值必显式写入，
		// false 落 DB 默认——布尔零值陷阱两向均安全
		Purchasable *bool `json:"purchasable"`
		Renewable   *bool `json:"renewable"`
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
	purchasable, renewable := true, true
	if req.Purchasable != nil {
		purchasable = *req.Purchasable
	}
	if req.Renewable != nil {
		renewable = *req.Renewable
	}
	plan := models.Plan{
		Name: req.Name, Description: req.Description, PriceCents: req.PriceCents, TrafficGB: req.TrafficGB,
		DurationDays:      req.DurationDays,
		DeviceLimit:       req.DeviceLimit,
		PermissionGroupID: req.PermissionGroupID, SortOrder: req.SortOrder, IsFeatured: req.IsFeatured,
		Purchasable: purchasable, Renewable: renewable,
	}
	if err := d.DB.Create(&plan).Error; err != nil {
		util.ServerError(c, "创建失败")
		return
	}
	// 新套餐无订阅者，无需触发节点同步（快照化后套餐变更不再自动联动存量用户）
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
		SortOrder         *int    `json:"sort_order"`
		IsFeatured        *bool   `json:"is_featured"`
		Purchasable       *bool   `json:"purchasable"` // 可新购（商店展示并允许非持有者购买）
		Renewable         *bool   `json:"renewable"`   // 可续费（持有者余额直付顺延）
		// SyncUsers「同步存量用户」（2026-09-01 快照化）：默认 false——套餐编辑只影响新购/续费
		// （存量用户按快照用到自己到期为止）；勾选后把新的额度/设备限制/权限组快照批量覆盖到
		// 该套餐全部用户，并触发热更同步（立即踢除超量用户）。
		SyncUsers *bool `json:"sync_users"`
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
	if req.SortOrder != nil && *req.SortOrder < 0 {
		util.BadRequest(c, "排序值不能为负数")
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
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}
	if req.IsFeatured != nil {
		updates["is_featured"] = *req.IsFeatured
	}
	if req.Purchasable != nil {
		updates["purchasable"] = *req.Purchasable
	}
	if req.Renewable != nil {
		updates["renewable"] = *req.Renewable
	}
	if len(updates) > 0 {
		if err := d.DB.Model(&plan).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新失败")
			return
		}
		// 快照化后套餐编辑默认零影响存量用户（判定链只读用户行快照）；
		// 显式勾选 sync_users 才批量重快照并触发同步（立即对存量用户生效/踢除超量）。
		if req.SyncUsers != nil && *req.SyncUsers {
			snapUpdates := models.PlanSnapshotColumns(&plan)
			if err := d.DB.Model(&models.User{}).Where("plan_id = ?", plan.ID).Updates(snapUpdates).Error; err != nil {
				util.ServerError(c, "同步存量用户快照失败")
				return
			}
			d.TriggerUserChange()
		}
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
		util.BadRequest(c, "该套餐有 "+strconv.FormatInt(userCnt, 10)+" 个用户持有，无法删除（可先关闭「可新购/可续费」下架）")
		return
	}
	var orderCnt int64
	d.DB.Model(&models.Order{}).Where("plan_id = ?", id).Count(&orderCnt)
	if orderCnt > 0 {
		util.BadRequest(c, "该套餐存在订单记录，无法删除（可先关闭「可新购/可续费」下架）")
		return
	}
	if err := d.DB.Delete(&models.Plan{}, id).Error; err != nil {
		util.ServerError(c, "删除失败")
		return
	}
	util.OK(c, gin.H{"deleted": id})
}

// PublicPlans GET /api/v1/plans —— 商店套餐列表（身份感知过滤，2026-09-03 销售两属性）：
// 非持有者/匿名只看到 purchasable 的套餐；持有者额外看到自己当前套餐（renewable 才可见，
// 余额直付同套餐即续费顺延）。不满足门控的套餐直接不下发（「隐藏」语义），
// 支付接口按同一矩阵兜底。仍隐藏内部 permission_group_id。
func (d *Deps) PublicPlans(c *gin.Context) {
	// AuthOptional 已通过校验的登录身份（匿名时为 0）
	var callerPlanID uint64
	if uid := middleware.CurrentUser(c); uid > 0 {
		var u models.User
		if err := d.DB.Select("id, plan_id").First(&u, uid).Error; err == nil {
			callerPlanID = u.PlanID
		}
	}

	var list []models.Plan
	// 展示顺序：sort_order 优先（管理端可控），同序值回退价格升序、再按 id 稳定
	if err := d.DB.Order("sort_order ASC, price_cents ASC, id ASC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	items := make([]publicPlanView, 0, len(list))
	for i := range list {
		p := &list[i]
		if callerPlanID > 0 && p.ID == callerPlanID {
			if !p.Renewable {
				continue // 自己当前套餐但已停止续费 → 隐藏
			}
		} else if !p.Purchasable {
			continue // 非持有者只可见可新购套餐
		}
		items = append(items, toPublicPlanView(p))
	}
	util.OK(c, gin.H{"items": items})
}
