package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/api"
	"github.com/acdc-awa/xpanel/internal/master/billing"
	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/master/store/gormstore"
	"github.com/acdc-awa/xpanel/internal/models"
)

type stubOnlineProvider struct {
	id              string
	supportsPlanPay bool
	initiateCalls   int
	callbackResult  *contracts.PaymentCallbackResult
}

func (p *stubOnlineProvider) Info() contracts.PaymentChannelInfo {
	return contracts.PaymentChannelInfo{
		ID:               p.id,
		Name:             "StubPay",
		FlowType:         contracts.PaymentFlowRedirect,
		SupportsRecharge: true,
		SupportsPlanPay:  p.supportsPlanPay,
		RequiresAmount:   true,
		MinAmountCents:   100,
		Enabled:          true,
	}
}
func (p *stubOnlineProvider) ConfigSchema() []contracts.PaymentFieldSchema { return nil }
func (p *stubOnlineProvider) Enabled() bool                              { return true }
func (p *stubOnlineProvider) Initiate(_ context.Context, req contracts.PaymentInitiateRequest) (*contracts.PaymentInitiateResult, error) {
	p.initiateCalls++
	return &contracts.PaymentInitiateResult{
		Paid:    false,
		Flow:    contracts.PaymentFlowRedirect,
		TradeNo: "STUB-T-1",
		PayURL:  "https://pay.example.com/" + req.Order.OrderNo,
	}, nil
}
func (p *stubOnlineProvider) VerifyCallback(_ context.Context, _ *http.Request) (*contracts.PaymentCallbackResult, error) {
	return p.callbackResult, nil
}
func (p *stubOnlineProvider) QueryStatus(_ context.Context, order *models.Order) (bool, string, error) {
	return order.Status == models.OrderPaid, order.TradeNo, nil
}

func setupOrderAPITest(t *testing.T, online *stubOnlineProvider) (*gorm.DB, *api.Deps, models.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Plan{}, &models.Order{}, &models.GiftCard{}, &models.BalanceLog{}, &models.TrafficLog{}, &models.AuditLog{}))

	store := gormstore.NewBillingStore(db)
	orderSvc := billing.NewOrderService(store)
	giftCardSvc := billing.NewGiftCardService(store)
	billing.RegisterBuiltinPaymentProviders(orderSvc.Payments, orderSvc, giftCardSvc)
	if online != nil {
		orderSvc.Payments.Register(online)
	}

	user := models.User{Email: "pay@example.com", UUID: "pay-uuid", SubscribeToken: "pay-tok", Status: models.StatusActive}
	require.NoError(t, db.Create(&user).Error)

	deps := &api.Deps{
		DB:       db,
		Audit:    &services.AuditService{DB: db},
		Order:    orderSvc,
		GiftCard: giftCardSvc,
		Payments: orderSvc.Payments,
	}
	return db, deps, user
}

func callUserJSON(handler gin.HandlerFunc, uid uint64, method, path string, params gin.Params, body any) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = params
	c.Set(middleware.CtxClaimsKey, &contracts.JWTClaims{UserID: uid})
	handler(c)
	return w
}

// TestPayPendingRechargeOrderRejectsGiftCard 回归：待支付充值订单禁止使用卡密支付，防任意金额放大与双重入账。
func TestPayPendingRechargeOrderRejectsGiftCard(t *testing.T) {
	online := &stubOnlineProvider{id: "stubpay"}
	db, deps, user := setupOrderAPITest(t, online)

	cards, err := deps.GiftCard.BatchGenerate(1, 1, "1元卡", 100, nil)
	require.NoError(t, err)

	// 创建 10000 分的待支付充值订单
	rechargeOrder, err := deps.Order.CreateRechargeOrder(user.ID, "stubpay", 10000, false)
	require.NoError(t, err)

	// 1. GetUserOrder 收银台返回渠道不应包含 gift_card
	wGet := callUserJSON(deps.GetUserOrder, user.ID, http.MethodGet, "/user/orders/"+rechargeOrder.OrderNo,
		gin.Params{{Key: "orderNo", Value: rechargeOrder.OrderNo}}, nil)
	require.Equal(t, http.StatusOK, wGet.Code)
	var getResp struct {
		Data struct {
			AvailableMethods []string `json:"available_methods"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(wGet.Body.Bytes(), &getResp))
	assert.Contains(t, getResp.Data.AvailableMethods, "stubpay")
	assert.NotContains(t, getResp.Data.AvailableMethods, models.PaymentMethodGiftCard)

	// 2. 直接调 PayUserOrder 用 gift_card 支付必须被拒绝，且卡密未被核销、余额仍为 0、订单仍为 pending
	wPay := callUserJSON(deps.PayUserOrder, user.ID, http.MethodPost, "/user/orders/"+rechargeOrder.OrderNo+"/pay",
		gin.Params{{Key: "orderNo", Value: rechargeOrder.OrderNo}},
		map[string]any{"method": models.PaymentMethodGiftCard, "code": cards[0].Code})
	assert.Equal(t, http.StatusBadRequest, wPay.Code, wPay.Body.String())

	var gotUser models.User
	require.NoError(t, db.First(&gotUser, user.ID).Error)
	assert.Equal(t, int64(0), gotUser.BalanceCents)

	var gotCard models.GiftCard
	require.NoError(t, db.First(&gotCard, cards[0].ID).Error)
	assert.Equal(t, models.GiftCardUnused, gotCard.Status)

	var gotOrder models.Order
	require.NoError(t, db.First(&gotOrder, rechargeOrder.ID).Error)
	assert.Equal(t, models.OrderPending, gotOrder.Status)
}

// TestPayUserOrderNonPendingStatusGuard 回归：非 pending 订单支付门控（已支付幂等返回 200，已取消返回 400），且绝不重复调 Initiate。
func TestPayUserOrderNonPendingStatusGuard(t *testing.T) {
	online := &stubOnlineProvider{id: "stubpay"}
	_, deps, user := setupOrderAPITest(t, online)

	// 1. 已支付充值单：重复调 PayUserOrder 幂等返回 200 且不调 Initiate
	paidOrder, err := deps.Order.CreateRechargeOrder(user.ID, "stubpay", 2000, false)
	require.NoError(t, err)
	_, err = deps.Order.CompleteRechargeOrder(paidOrder.OrderNo, "stubpay", "T-PAID", 2000)
	require.NoError(t, err)

	wPaid := callUserJSON(deps.PayUserOrder, user.ID, http.MethodPost, "/user/orders/"+paidOrder.OrderNo+"/pay",
		gin.Params{{Key: "orderNo", Value: paidOrder.OrderNo}},
		map[string]any{"method": "stubpay"})
	assert.Equal(t, http.StatusOK, wPaid.Code, wPaid.Body.String())
	assert.Equal(t, 0, online.initiateCalls)

	// 2. 已取消充值单：调 PayUserOrder 返回 400 且不调 Initiate
	cancelledOrder, err := deps.Order.CreateRechargeOrder(user.ID, "stubpay", 3000, false)
	require.NoError(t, err)
	_, err = deps.Order.CancelOrder(user.ID, cancelledOrder.OrderNo)
	require.NoError(t, err)

	wCancel := callUserJSON(deps.PayUserOrder, user.ID, http.MethodPost, "/user/orders/"+cancelledOrder.OrderNo+"/pay",
		gin.Params{{Key: "orderNo", Value: cancelledOrder.OrderNo}},
		map[string]any{"method": "stubpay"})
	assert.Equal(t, http.StatusBadRequest, wCancel.Code, wCancel.Body.String())
	assert.Equal(t, 0, online.initiateCalls)
}

// TestPaymentCallbackAmountAndStatusValidation 回归：回调金额校验与非 pending 拒绝策略。
func TestPaymentCallbackAmountAndStatusValidation(t *testing.T) {
	online := &stubOnlineProvider{id: "stubpay"}
	db, deps, user := setupOrderAPITest(t, online)

	order, err := deps.Order.CreateRechargeOrder(user.ID, "stubpay", 5000, false)
	require.NoError(t, err)

	callCB := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/payments/callback/stubpay", nil)
		c.Params = gin.Params{{Key: "channel", Value: "stubpay"}}
		deps.PaymentCallback(c)
		return w
	}

	// 1. 金额不一致 → 400 且不入账
	online.callbackResult = &contracts.PaymentCallbackResult{
		OrderNo: order.OrderNo, TradeNo: "T-BAD", AmountCents: 100, Paid: true,
	}
	wMismatch := callCB()
	assert.Equal(t, http.StatusBadRequest, wMismatch.Code)
	var u models.User
	require.NoError(t, db.First(&u, user.ID).Error)
	assert.Equal(t, int64(0), u.BalanceCents)

	// 2. 金额一致 → 200 入账，重复回调幂等 200 不重复入账
	online.callbackResult = &contracts.PaymentCallbackResult{
		OrderNo: order.OrderNo, TradeNo: "T-OK", AmountCents: 5000, Paid: true,
	}
	assert.Equal(t, http.StatusOK, callCB().Code)
	assert.Equal(t, http.StatusOK, callCB().Code)
	require.NoError(t, db.First(&u, user.ID).Error)
	assert.Equal(t, int64(5000), u.BalanceCents)

	// 3. 已取消订单收到迟到回调 → 400 且不入账
	cancelled, err := deps.Order.CreateRechargeOrder(user.ID, "stubpay", 4000, false)
	require.NoError(t, err)
	_, err = deps.Order.CancelOrder(user.ID, cancelled.OrderNo)
	require.NoError(t, err)

	online.callbackResult = &contracts.PaymentCallbackResult{
		OrderNo: cancelled.OrderNo, TradeNo: "T-LATE", AmountCents: 4000, Paid: true,
	}
	assert.Equal(t, http.StatusBadRequest, callCB().Code)
	require.NoError(t, db.First(&u, user.ID).Error)
	assert.Equal(t, int64(5000), u.BalanceCents)
}

// TestPlanCheckoutHidesThirdPartyProviders 回归：套餐收银台现阶段只暴露余额与卡密渠道。
func TestPlanCheckoutHidesThirdPartyProviders(t *testing.T) {
	online := &stubOnlineProvider{id: "stubpay", supportsPlanPay: true}
	db, deps, user := setupOrderAPITest(t, online)

	plan := models.Plan{Name: "月付", PriceCents: 1000, TrafficGB: 10, DurationDays: 30, Purchasable: true}
	require.NoError(t, db.Create(&plan).Error)

	planOrder, err := deps.Order.CreateOrder(user.ID, plan.ID, false)
	require.NoError(t, err)

	w := callUserJSON(deps.GetUserOrder, user.ID, http.MethodGet, "/user/orders/"+planOrder.OrderNo,
		gin.Params{{Key: "orderNo", Value: planOrder.OrderNo}}, nil)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data struct {
			AvailableMethods []string `json:"available_methods"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.ElementsMatch(t, []string{models.PaymentMethodBalance, models.PaymentMethodGiftCard}, resp.Data.AvailableMethods)
}

func TestPayUserOrder_WritesAuditLog(t *testing.T) {
	db, deps, user := setupOrderAPITest(t, nil)
	require.NoError(t, db.Model(&models.User{}).Where("id = ?", user.ID).Update("balance_cents", 5000).Error)

	plan := models.Plan{Name: "月付", PriceCents: 1000, TrafficGB: 10, DurationDays: 30, Purchasable: true}
	require.NoError(t, db.Create(&plan).Error)

	planOrder, err := deps.Order.CreateOrder(user.ID, plan.ID, false)
	require.NoError(t, err)

	w := callUserJSON(deps.PayUserOrder, user.ID, http.MethodPost, "/user/orders/"+planOrder.OrderNo+"/pay",
		gin.Params{{Key: "orderNo", Value: planOrder.OrderNo}}, map[string]any{"method": models.PaymentMethodBalance})
	require.Equal(t, http.StatusOK, w.Code)

	var logs []models.AuditLog
	require.NoError(t, db.Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "user", logs[0].OperatorType)
	assert.Equal(t, user.ID, logs[0].OperatorID)
	assert.Equal(t, "order.pay_balance", logs[0].Action)
	assert.Equal(t, models.AuditCategoryBilling, logs[0].Category)
	assert.Equal(t, "order", logs[0].TargetType)
	assert.Equal(t, planOrder.ID, logs[0].TargetID)
	assert.Equal(t, planOrder.OrderNo, logs[0].TargetName)
	assert.Contains(t, logs[0].Detail, "月付")
	assert.Contains(t, logs[0].Detail, planOrder.OrderNo)
}
