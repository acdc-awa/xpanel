package api

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/middleware"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

type orderView struct {
	ID            uint64     `json:"id"`
	OrderNo       string     `json:"order_no"`
	UserID        uint64     `json:"user_id"`
	Username      string     `json:"username"`
	PlanID        uint64     `json:"plan_id"`
	PlanName      string     `json:"plan_name"`
	AmountCents   int64      `json:"amount_cents"`
	PaymentMethod string     `json:"payment_method"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	PaidAt        *time.Time `json:"paid_at"`
}

func (d *Deps) toOrderView(o *models.Order) orderView {
	v := orderView{
		ID: o.ID, OrderNo: o.OrderNo, UserID: o.UserID, PlanID: o.PlanID,
		AmountCents: o.AmountCents, PaymentMethod: o.PaymentMethod, Status: o.Status, CreatedAt: o.CreatedAt, PaidAt: o.PaidAt,
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

// UserOrders GET /api/v1/user/orders —— 我的订单（余额直付记录，只读）。
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

// AdminOrders GET /api/v1/admin/orders —— 订单列表（分页，只读支付记录）。
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
