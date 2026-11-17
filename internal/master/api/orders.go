package api

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

type orderView struct {
	ID            uint64     `json:"id"`
	OrderNo       string     `json:"order_no"`
	UserID        uint64     `json:"user_id"`
	Email         string     `json:"email"`
	PlanID        uint64     `json:"plan_id"`
	PlanName      string     `json:"plan_name"`
	AmountCents   int64      `json:"amount_cents"`
	PaymentMethod string     `json:"payment_method"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	PaidAt        *time.Time `json:"paid_at"`
}

func (d *Deps) toOrderViews(orders []models.Order) []orderView {
	items := make([]orderView, 0, len(orders))
	if len(orders) == 0 {
		return items
	}
	userIDs := make([]uint64, 0, len(orders))
	userSeen := make(map[uint64]bool, len(orders))
	planIDs := make([]uint64, 0, len(orders))
	planSeen := make(map[uint64]bool, len(orders))
	for i := range orders {
		o := &orders[i]
		if o.UserID > 0 && !userSeen[o.UserID] {
			userSeen[o.UserID] = true
			userIDs = append(userIDs, o.UserID)
		}
		if o.PlanName == "" && o.PlanID > 0 && !planSeen[o.PlanID] {
			planSeen[o.PlanID] = true
			planIDs = append(planIDs, o.PlanID)
		}
	}

	emailByID := make(map[uint64]string, len(userIDs))
	if len(userIDs) > 0 {
		var users []models.User
		if err := d.DB.Select("id, email").Where("id IN ?", userIDs).Find(&users).Error; err == nil {
			for _, u := range users {
				emailByID[u.ID] = u.Email
			}
		}
	}

	// 套餐名优先用订单快照（2026-10-25）：套餐可归档，归档行在常规查询里不可见，
	// 实时 JOIN 会拿不到名称。旧订单若快照为空，仍批量 Unscoped 回落查询。
	planByID := make(map[uint64]string, len(planIDs))
	if len(planIDs) > 0 {
		var plans []models.Plan
		if err := d.DB.Unscoped().Select("id, name").Where("id IN ?", planIDs).Find(&plans).Error; err == nil {
			for _, p := range plans {
				planByID[p.ID] = p.Name
			}
		}
	}

	for i := range orders {
		o := &orders[i]
		planName := o.PlanName
		if planName == "" {
			planName = planByID[o.PlanID]
		}
		items = append(items, orderView{
			ID: o.ID, OrderNo: o.OrderNo, UserID: o.UserID, Email: emailByID[o.UserID],
			PlanID: o.PlanID, PlanName: planName, AmountCents: o.AmountCents,
			PaymentMethod: o.PaymentMethod, Status: o.Status, CreatedAt: o.CreatedAt, PaidAt: o.PaidAt,
		})
	}
	return items
}

// UserOrders GET /api/v1/user/orders —— 我的订单（余额直付记录，只读）。
func (d *Deps) UserOrders(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	list, err := d.Order.ListByUser(uid)
	if err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	util.OK(c, gin.H{"items": d.toOrderViews(list)})
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
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		// 用子查询匹配邮箱，避免把命中的用户 ID 全量拉回再拼 IN：
		// 关键词命中大量用户时会生成超长占位符列表（SQLite 变量上限）且多一次往返。
		pat := "%" + kw + "%"
		q = q.Where(
			"(order_no LIKE ? OR user_id IN (SELECT id FROM users WHERE email LIKE ? AND deleted_at IS NULL))",
			pat, pat,
		)
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
	util.OK(c, gin.H{"total": total, "page": page, "size": size, "items": d.toOrderViews(list)})
}
