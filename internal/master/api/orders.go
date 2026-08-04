package api

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/middleware"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

type orderView struct {
	ID          uint64     `json:"id"`
	OrderNo     string     `json:"order_no"`
	UserID      uint64     `json:"user_id"`
	Username    string     `json:"username"`
	PlanID      uint64     `json:"plan_id"`
	PlanName    string     `json:"plan_name"`
	AmountCents int64      `json:"amount_cents"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	PaidAt      *time.Time `json:"paid_at"`
}

func (d *Deps) toOrderView(o *models.Order) orderView {
	v := orderView{
		ID: o.ID, OrderNo: o.OrderNo, UserID: o.UserID, PlanID: o.PlanID,
		AmountCents: o.AmountCents, Status: o.Status, CreatedAt: o.CreatedAt, PaidAt: o.PaidAt,
	}
	var user models.User
	if err := d.DB.First(&user, o.UserID).Error; err == nil {
		v.Username = user.Username
	}
	var plan models.Plan
	if err := d.DB.First(&plan, o.PlanID).Error; err == nil {
		v.PlanName = plan.Name
	}
	return v
}

// UserCreateOrder POST /api/v1/user/orders —— 用户下单。
func (d *Deps) UserCreateOrder(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req struct {
		PlanID uint64 `json:"plan_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
		return
	}
	order, err := d.Order.Create(uid, req.PlanID)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	d.Audit.Log("user", uid, "order.create", "下单套餐 #"+strconv.FormatUint(req.PlanID, 10), c.ClientIP())
	util.OK(c, gin.H{"order": d.toOrderView(order)})
}

// UserOrders GET /api/v1/user/orders —— 我的订单。
func (d *Deps) UserOrders(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	list, err := d.Order.ListByUser(uid)
	if err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	items := make([]orderView, 0, len(list))
	for i := range list {
		items = append(items, d.toOrderView(&list[i]))
	}
	util.OK(c, gin.H{"items": items})
}

// AdminOrders GET /api/v1/admin/orders —— 订单列表（分页）。
func (d *Deps) AdminOrders(c *gin.Context) {
	page := atoiDefault(c.Query("page"), 1)
	size := atoiDefault(c.Query("size"), 20)
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	q := d.DB.Model(&models.Order{})
	if st := c.Query("status"); st != "" {
		q = q.Where("status = ?", st)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	var list []models.Order
	if err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	items := make([]orderView, 0, len(list))
	for i := range list {
		items = append(items, d.toOrderView(&list[i]))
	}
	util.OK(c, gin.H{"total": total, "page": page, "size": size, "items": items})
}

// AdminConfirmOrder POST /api/v1/admin/orders/:id/confirm —— 确认收款，套餐生效。
func (d *Deps) AdminConfirmOrder(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	adminID := middleware.CurrentUser(c)
	if err := d.Order.Confirm(id, adminID); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	if d.Hub != nil {
		d.Hub.SyncUsersToAll()
	}
	d.Audit.Log("admin", adminID, "order.confirm", "确认订单 #"+strconv.FormatUint(id, 10), c.ClientIP())
	util.OK(c, gin.H{"confirmed": id})
}

// AdminCancelOrder POST /api/v1/admin/orders/:id/cancel
func (d *Deps) AdminCancelOrder(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	adminID := middleware.CurrentUser(c)
	if err := d.Order.Cancel(id, adminID); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	d.Audit.Log("admin", adminID, "order.cancel", "取消订单 #"+strconv.FormatUint(id, 10), c.ClientIP())
	util.OK(c, gin.H{"cancelled": id})
}