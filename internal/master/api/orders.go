package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/billing"
	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

type orderView struct {
	ID             uint64     `json:"id"`
	OrderNo        string     `json:"order_no"`
	OrderType      string     `json:"order_type"`
	UserID         uint64     `json:"user_id"`
	Email          string     `json:"email"`
	PlanID         uint64     `json:"plan_id"`
	PlanName       string     `json:"plan_name"`
	AmountCents    int64      `json:"amount_cents"`
	PaymentMethod  string     `json:"payment_method"`
	PaymentChannel string     `json:"payment_channel,omitempty"`
	PayPayload     string     `json:"pay_payload,omitempty"`
	Status         string     `json:"status"`
	CancelReason   string     `json:"cancel_reason,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	PaidAt         *time.Time `json:"paid_at"`
	CancelledAt    *time.Time `json:"cancelled_at,omitempty"`
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
		orderType := o.OrderType
		if orderType == "" {
			orderType = models.OrderTypePlan
		}
		planName := o.PlanName
		if planName == "" {
			if orderType == models.OrderTypeRecharge {
				planName = "账户充值"
			} else {
				planName = planByID[o.PlanID]
			}
		}
		items = append(items, orderView{
			ID: o.ID, OrderNo: o.OrderNo, OrderType: orderType, UserID: o.UserID, Email: emailByID[o.UserID],
			PlanID: o.PlanID, PlanName: planName, AmountCents: o.AmountCents,
			PaymentMethod: o.PaymentMethod, PaymentChannel: o.PaymentChannel, PayPayload: o.PayPayload,
			Status: o.Status, CancelReason: o.CancelReason,
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
	if ot := strings.TrimSpace(c.Query("order_type")); ot != "" {
		q = q.Where("order_type = ?", ot)
	}
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

func (d *Deps) enabledChannelsForPurpose(purpose string) []contracts.PaymentChannelInfo {
	providers := d.Payments.List()
	out := make([]contracts.PaymentChannelInfo, 0, len(providers))
	for _, p := range providers {
		if !p.Enabled() {
			continue
		}
		info := p.Info()
		switch purpose {
		case "recharge":
			if !info.SupportsRecharge {
				continue
			}
		case "recharge_checkout":
			// 待支付充值订单已有固定金额，仅允许需要指定金额的在线渠道，禁止卡密等免金额同步渠道
			if !info.SupportsRecharge || !info.RequiresAmount {
				continue
			}
		case "plan":
			// 当前阶段收敛为「先充值进余额，再余额购套餐」，套餐收银台仅开放内置余额与卡密渠道
			if !info.SupportsPlanPay || (info.ID != models.PaymentMethodBalance && info.ID != models.PaymentMethodGiftCard) {
				continue
			}
		}
		out = append(out, info)
	}
	return out
}

// UserPaymentChannels GET /api/v1/user/payment-channels —— 获取当前启用的支付/充值渠道列表。
func (d *Deps) UserPaymentChannels(c *gin.Context) {
	purpose := strings.TrimSpace(c.Query("purpose"))
	util.OK(c, gin.H{"items": d.enabledChannelsForPurpose(purpose)})
}

type createRechargeReq struct {
	Channel        string `json:"channel" binding:"required"`
	AmountCents    int64  `json:"amount_cents"`
	Code           string `json:"code"`
	ReplacePending bool   `json:"replace_pending"`
}

// CreateUserRecharge POST /api/v1/user/recharges —— 统一充值入口。
// 卡密渠道直接原子核销并生成已支付充值订单；在线支付渠道创建待支付充值订单并进收银台。
func (d *Deps) CreateUserRecharge(c *gin.Context) {
	var req createRechargeReq
	if !util.BindJSON(c, &req) {
		return
	}
	uid := middleware.CurrentUser(c)
	channel := strings.TrimSpace(req.Channel)

	provider, ok := d.Payments.Get(channel)
	if !ok {
		util.BadRequest(c, "不支持的充值渠道")
		return
	}
	info := provider.Info()
	if !provider.Enabled() || !info.SupportsRecharge {
		util.BadRequest(c, "该充值渠道暂未启用")
		return
	}

	// 同步免金额渠道（如卡密兑换）：直接核销并生成已支付充值订单
	if info.FlowType == contracts.PaymentFlowSync && !info.RequiresAmount {
		if channel == models.PaymentMethodGiftCard {
			card, order, newBalance, err := d.GiftCard.RedeemWithOrder(uid, req.Code)
			if err != nil {
				util.BadRequest(c, err.Error())
				return
			}
			views := d.toOrderViews([]models.Order{*order})
			util.OK(c, gin.H{
				"order":            views[0],
				"paid":             true,
				"face_value_cents": card.FaceValueCents,
				"balance_cents":    newBalance,
			})
			return
		}
		util.BadRequest(c, "不支持的同步充值渠道")
		return
	}

	// 在线支付渠道：按指定金额创建待支付充值订单
	order, err := d.Order.CreateRechargeOrder(uid, channel, req.AmountCents, req.ReplacePending)
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
		case errors.Is(err, billing.ErrInvalidRechargeAmount),
			errors.Is(err, billing.ErrUnsupportedChannel),
			errors.Is(err, billing.ErrChannelDisabled):
			util.BadRequest(c, err.Error())
		default:
			util.ServerError(c, "创建充值订单失败")
		}
		return
	}
	views := d.toOrderViews([]models.Order{*order})
	util.OK(c, gin.H{
		"order": views[0],
		"paid":  false,
	})
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
	purpose := "plan"
	if views[0].OrderType == models.OrderTypeRecharge {
		purpose = "recharge_checkout"
	}
	channels := d.enabledChannelsForPurpose(purpose)
	availableMethods := make([]string, 0, len(channels))
	for _, ch := range channels {
		availableMethods = append(availableMethods, ch.ID)
	}
	util.OK(c, gin.H{
		"order":             views[0],
		"balance_cents":     user.BalanceCents,
		"available_methods": availableMethods,
		"channels":          channels,
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
	method := strings.TrimSpace(req.Method)

	order, err := d.Order.GetOrder(uid, orderNo)
	if err != nil {
		handleOrderPayError(c, err)
		return
	}
	// 前置状态门控：已支付幂等返回（不重复调插件/不核销卡密），已取消/超时直接拒绝
	if order.Status == models.OrderPaid {
		views := d.toOrderViews([]models.Order{*order})
		util.OK(c, gin.H{"order": views[0], "paid": true})
		return
	}
	if order.Status != models.OrderPending {
		util.BadRequest(c, billing.ErrOrderNotPending.Error())
		return
	}

	// 充值订单支付分支：走标准 PaymentProvider 插件（仅允许需要指定金额的在线渠道）
	if order.OrderType == models.OrderTypeRecharge {
		if method == models.PaymentMethodBalance {
			util.BadRequest(c, "充值订单不支持余额支付")
			return
		}
		provider, ok := d.Payments.Get(method)
		if !ok || !provider.Enabled() || !provider.Info().SupportsRecharge {
			util.BadRequest(c, "不支持的充值渠道")
			return
		}
		if !provider.Info().RequiresAmount {
			util.BadRequest(c, "该渠道仅支持通过充值入口直接兑换，不能用于支付待支付充值订单")
			return
		}
		res, err := provider.Initiate(c.Request.Context(), contracts.PaymentInitiateRequest{
			Order:  order,
			UserID: uid,
			Code:   strings.TrimSpace(req.Code),
		})
		if err != nil {
			util.BadRequest(c, err.Error())
			return
		}
		if res.Paid {
			settled, err := d.Order.CompleteRechargeOrder(orderNo, method, res.TradeNo, order.AmountCents)
			if err != nil {
				handleOrderPayError(c, err)
				return
			}
			views := d.toOrderViews([]models.Order{*settled})
			util.OK(c, gin.H{"order": views[0], "paid": true, "flow": res.Flow})
			return
		}
		payloadBytes, _ := json.Marshal(res)
		_ = d.Order.UpdateOrderPaymentInfo(order.ID, method, method, res.TradeNo, string(payloadBytes))
		updated, err := d.Order.GetOrder(uid, orderNo)
		if err == nil {
			order = updated
		}
		views := d.toOrderViews([]models.Order{*order})
		util.OK(c, gin.H{
			"order":   views[0],
			"paid":    false,
			"flow":    res.Flow,
			"pay_url": res.PayURL,
			"qr_code": res.QRCode,
			"html":    res.HTML,
		})
		return
	}

	switch method {
	case models.PaymentMethodBalance:
		paidOrder, err := d.Order.PayOrderWithBalance(uid, orderNo)
		if err != nil {
			handleOrderPayError(c, err)
			return
		}
		views := d.toOrderViews([]models.Order{*paidOrder})
		util.OK(c, gin.H{"order": views[0], "paid": true})
	case models.PaymentMethodGiftCard:
		code := strings.TrimSpace(req.Code)
		if code == "" {
			util.BadRequest(c, "请输入卡密")
			return
		}
		// 先核销卡密充入余额（同时生成充值订单），再尝试用余额完成套餐订单扣款
		if _, _, err := d.GiftCard.Redeem(uid, code); err != nil {
			util.BadRequest(c, err.Error())
			return
		}
		paidOrder, err := d.Order.PayOrderWithBalance(uid, orderNo)
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
		views := d.toOrderViews([]models.Order{*paidOrder})
		util.OK(c, gin.H{"order": views[0], "paid": true})
	default:
		util.BadRequest(c, "不支持的支付方式")
	}
}

// PaymentCallback ANY /api/v1/payments/callback/:channel —— 第三方支付异步回调统一入口。
func (d *Deps) PaymentCallback(c *gin.Context) {
	channel := strings.TrimSpace(c.Param("channel"))
	provider, ok := d.Payments.Get(channel)
	if !ok || !provider.Enabled() {
		c.String(http.StatusNotFound, "unknown channel")
		return
	}
	cb, err := provider.VerifyCallback(c.Request.Context(), c.Request)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}
	if cb != nil && cb.Paid && cb.OrderNo != "" {
		if _, err := d.Order.CompleteRechargeOrder(cb.OrderNo, channel, cb.TradeNo, cb.AmountCents); err != nil {
			switch {
			case errors.Is(err, billing.ErrAmountMismatch):
				log.Printf("payment callback rejected (amount mismatch): channel=%s order_no=%s trade_no=%s paid_cents=%d",
					channel, cb.OrderNo, cb.TradeNo, cb.AmountCents)
				c.String(http.StatusBadRequest, "amount mismatch")
			case errors.Is(err, billing.ErrOrderNotPending), errors.Is(err, billing.ErrOrderNotFound):
				log.Printf("payment callback rejected (order not pending): channel=%s order_no=%s trade_no=%s err=%v",
					channel, cb.OrderNo, cb.TradeNo, err)
				c.String(http.StatusBadRequest, "order not pending")
			default:
				log.Printf("payment callback settle failed: channel=%s order_no=%s trade_no=%s err=%v",
					channel, cb.OrderNo, cb.TradeNo, err)
				c.String(http.StatusInternalServerError, "settle failed")
			}
			return
		}
	}
	status := http.StatusOK
	body := "success"
	if cb != nil {
		if cb.AckStatus > 0 {
			status = cb.AckStatus
		}
		if cb.AckBody != "" {
			body = cb.AckBody
		}
	}
	c.String(status, body)
}

func handleOrderPayError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, billing.ErrOrderNotFound):
		c.JSON(http.StatusNotFound, gin.H{"code": http.StatusNotFound, "message": err.Error()})
	case errors.Is(err, billing.ErrInsufficientBalance),
		errors.Is(err, billing.ErrOrderNotPending),
		errors.Is(err, billing.ErrOrderExpired),
		errors.Is(err, billing.ErrPlanNotFound),
		errors.Is(err, billing.ErrAmountMismatch),
		errors.Is(err, billing.ErrUnsupportedOrderPay):
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
