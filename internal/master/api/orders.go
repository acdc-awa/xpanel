package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/billing"
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
	CancelReason  string     `json:"cancel_reason,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	PaidAt        *time.Time `json:"paid_at"`
	CancelledAt   *time.Time `json:"cancelled_at,omitempty"`
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
			PaymentMethod: o.PaymentMethod, Status: o.Status, CancelReason: o.CancelReason,
			ExpiresAt: o.ExpiresAt, CreatedAt: o.CreatedAt, PaidAt: o.PaidAt, CancelledAt: o.CancelledAt,
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

type createOrderReq struct {
	PlanID         uint64 `json:"plan_id" binding:"required"`
	ReplacePending bool   `json:"replace_pending"`
}

// CreateUserOrder POST /api/v1/user/orders —— 创建待支付订单（0元套餐自动完成）。
func (d *Deps) CreateUserOrder(c *gin.Context) {
	var req createOrderReq
	if !util.BindJSON(c, &req) {
		return
	}
	uid := middleware.CurrentUser(c)
	order, err := d.Order.CreateOrder(uid, req.PlanID, req.ReplacePending)
	if err != nil {
		switch {
		case errors.Is(err, billing.ErrPendingOrderExists):
			pendingNo := ""
			if order != nil {
				pendingNo = order.OrderNo
			}
			c.JSON(http.StatusConflict, gin.H{
				"code":             http.StatusConflict,
				"message":          err.Error(),
				"pending_order_no": pendingNo,
			})
		case errors.Is(err, billing.ErrPlanNotFound),
			errors.Is(err, billing.ErrPlanNotPurchasable),
			errors.Is(err, billing.ErrPlanRenewalStopped):
			util.BadRequest(c, err.Error())
		default:
			util.ServerError(c, "创建订单失败")
		}
		return
	}
	views := d.toOrderViews([]models.Order{*order})
	util.OK(c, gin.H{"order": views[0]})
}

// GetUserOrder GET /api/v1/user/orders/:orderNo —— 收银台拉取订单详情与可用支付方式。
func (d *Deps) GetUserOrder(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	orderNo := strings.TrimSpace(c.Param("orderNo"))
	order, err := d.Order.GetOrder(uid, orderNo)
	if err != nil {
		if errors.Is(err, billing.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": http.StatusNotFound, "message": err.Error()})
			return
		}
		util.ServerError(c, "查询订单失败")
		return
	}
	var user models.User
	_ = d.DB.Select("balance_cents").Where("id = ?", uid).First(&user).Error
	views := d.toOrderViews([]models.Order{*order})
	util.OK(c, gin.H{
		"order":             views[0],
		"balance_cents":     user.BalanceCents,
		"available_methods": []string{"balance", "gift_card"},
	})
}

type payOrderReq struct {
	Method string `json:"method" binding:"required"`
	Code   string `json:"code"`
}

// PayUserOrder POST /api/v1/user/orders/:orderNo/pay —— 收银台支付订单。
func (d *Deps) PayUserOrder(c *gin.Context) {
	var req payOrderReq
	if !util.BindJSON(c, &req) {
		return
	}
	uid := middleware.CurrentUser(c)
	orderNo := strings.TrimSpace(c.Param("orderNo"))

	switch req.Method {
	case "balance":
		order, err := d.Order.PayOrderWithBalance(uid, orderNo)
		if err != nil {
			handleOrderPayError(c, err)
			return
		}
		views := d.toOrderViews([]models.Order{*order})
		util.OK(c, gin.H{"order": views[0], "paid": true})
	case "gift_card":
		code := strings.TrimSpace(req.Code)
		if code == "" {
			util.BadRequest(c, "请输入卡密")
			return
		}
		// 先核销卡密充入余额，再尝试用余额完成订单扣款
		if _, _, err := d.GiftCard.Redeem(uid, code); err != nil {
			util.BadRequest(c, err.Error())
			return
		}
		order, err := d.Order.PayOrderWithBalance(uid, orderNo)
		if err != nil {
			if errors.Is(err, billing.ErrInsufficientBalance) {
				var user models.User
				_ = d.DB.Select("balance_cents").Where("id = ?", uid).First(&user).Error
				c.JSON(http.StatusPaymentRequired, gin.H{
					"code":          http.StatusPaymentRequired,
					"message":       "卡密已充入余额，但当前余额仍不足以支付该订单",
					"balance_cents": user.BalanceCents,
					"paid":          false,
				})
				return
			}
			handleOrderPayError(c, err)
			return
		}
		views := d.toOrderViews([]models.Order{*order})
		util.OK(c, gin.H{"order": views[0], "paid": true})
	default:
		util.BadRequest(c, "不支持的支付方式")
	}
}

func handleOrderPayError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, billing.ErrOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": http.StatusNotFound, "message": err.Error()})
	case errors.Is(err, billing.ErrInsufficientBalance),
		errors.Is(err, billing.ErrOrderNotPending),
		errors.Is(err, billing.ErrOrderExpired),
		errors.Is(err, billing.ErrPlanNotFound):
		util.BadRequest(c, err.Error())
	default:
		util.ServerError(c, "支付失败")
	}
}

// CancelUserOrder POST /api/v1/user/orders/:orderNo/cancel —— 手动取消待支付订单。
func (d *Deps) CancelUserOrder(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	orderNo := strings.TrimSpace(c.Param("orderNo"))
	order, err := d.Order.CancelOrder(uid, orderNo)
	if err != nil {
		if errors.Is(err, billing.ErrOrderNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": http.StatusNotFound, "message": err.Error()})
			return
		}
		if errors.Is(err, billing.ErrOrderNotPending) {
			util.BadRequest(c, err.Error())
			return
		}
		util.ServerError(c, "取消订单失败")
		return
	}
	views := d.toOrderViews([]models.Order{*order})
	util.OK(c, gin.H{"order": views[0]})
}
