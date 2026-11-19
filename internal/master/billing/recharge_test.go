package billing

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/store/gormstore"
	"github.com/acdc-awa/xpanel/internal/models"
)

type mockOnlineProvider struct {
	id string
}

func (m *mockOnlineProvider) Info() contracts.PaymentChannelInfo {
	return contracts.PaymentChannelInfo{
		ID:               m.id,
		Name:             "MockPay",
		FlowType:         contracts.PaymentFlowRedirect,
		SupportsRecharge: true,
		SupportsPlanPay:  false,
		RequiresAmount:   true,
		MinAmountCents:   100,
		Enabled:          true,
	}
}
func (m *mockOnlineProvider) ConfigSchema() []contracts.PaymentFieldSchema { return nil }
func (m *mockOnlineProvider) Enabled() bool                              { return true }
func (m *mockOnlineProvider) Initiate(_ context.Context, req contracts.PaymentInitiateRequest) (*contracts.PaymentInitiateResult, error) {
	return &contracts.PaymentInitiateResult{
		Paid:    false,
		Flow:    contracts.PaymentFlowRedirect,
		TradeNo: "MOCK-TRADE-1",
		PayURL:  "https://pay.example.com/" + req.Order.OrderNo,
	}, nil
}
func (m *mockOnlineProvider) VerifyCallback(_ context.Context, _ *http.Request) (*contracts.PaymentCallbackResult, error) {
	return nil, nil
}
func (m *mockOnlineProvider) QueryStatus(_ context.Context, order *models.Order) (bool, string, error) {
	return order.Status == models.OrderPaid, order.TradeNo, nil
}

func setupRechargeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Plan{}, &models.Order{}, &models.GiftCard{}, &models.BalanceLog{}, &models.TrafficLog{}))
	return db
}

func TestGiftCardRedeemCreatesUnifiedOrder(t *testing.T) {
	db := setupRechargeTestDB(t)
	store := gormstore.NewBillingStore(db)
	gcSvc := NewGiftCardService(store)
	orderSvc := NewOrderService(store)

	user := models.User{Email: "u1@example.com", UUID: "u1-uuid", SubscribeToken: "u1-tok", Status: models.StatusActive}
	require.NoError(t, db.Create(&user).Error)

	cards, err := gcSvc.BatchGenerate(1, 1, "测试卡密", 2500, nil)
	require.NoError(t, err)
	require.Len(t, cards, 1)

	card, order, newBal, err := gcSvc.RedeemWithOrder(user.ID, cards[0].Code)
	require.NoError(t, err)
	assert.Equal(t, models.GiftCardUsed, card.Status)
	assert.Equal(t, int64(2500), newBal)
	require.NotNil(t, order)
	assert.Equal(t, models.OrderTypeRecharge, order.OrderType)
	assert.Equal(t, models.OrderPaid, order.Status)
	assert.Equal(t, int64(2500), order.AmountCents)
	assert.Equal(t, models.PaymentMethodGiftCard, order.PaymentMethod)

	list, err := orderSvc.ListByUser(user.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, order.OrderNo, list[0].OrderNo)
}

func TestRechargeAndPlanPendingOrderIsolation(t *testing.T) {
	db := setupRechargeTestDB(t)
	store := gormstore.NewBillingStore(db)
	orderSvc := NewOrderService(store)
	orderSvc.Payments.Register(&mockOnlineProvider{id: "mockpay"})

	user := models.User{Email: "u2@example.com", UUID: "u2-uuid", SubscribeToken: "u2-tok", Status: models.StatusActive}
	require.NoError(t, db.Create(&user).Error)

	plan := models.Plan{Name: "月付套餐", PriceCents: 3000, TrafficGB: 100, DurationDays: 30, Purchasable: true}
	require.NoError(t, db.Create(&plan).Error)

	// 1. 先创建待支付套餐订单
	planOrder, err := orderSvc.CreateOrder(user.ID, plan.ID, false)
	require.NoError(t, err)
	assert.Equal(t, models.OrderTypePlan, planOrder.OrderType)
	assert.Equal(t, models.OrderPending, planOrder.Status)

	// 2. 不冲突：同一用户可同时创建待支付充值订单
	rechargeOrder, err := orderSvc.CreateRechargeOrder(user.ID, "mockpay", 5000, false)
	require.NoError(t, err)
	assert.Equal(t, models.OrderTypeRecharge, rechargeOrder.OrderType)
	assert.Equal(t, models.OrderPending, rechargeOrder.Status)
	assert.Equal(t, int64(5000), rechargeOrder.AmountCents)

	// 2.5 金额不匹配或非正数时必须拒绝结算
	_, err = orderSvc.CompleteRechargeOrder(rechargeOrder.OrderNo, "mockpay", "EXT-BAD", 0)
	assert.ErrorIs(t, err, ErrAmountMismatch)
	_, err = orderSvc.CompleteRechargeOrder(rechargeOrder.OrderNo, "mockpay", "EXT-BAD", 100)
	assert.ErrorIs(t, err, ErrAmountMismatch)

	// 3. 结算充值订单并验证幂等性
	settled, err := orderSvc.CompleteRechargeOrder(rechargeOrder.OrderNo, "mockpay", "EXT-999", 5000)
	require.NoError(t, err)
	assert.Equal(t, models.OrderPaid, settled.Status)

	settledAgain, err := orderSvc.CompleteRechargeOrder(rechargeOrder.OrderNo, "mockpay", "EXT-999", 5000)
	require.NoError(t, err)
	assert.Equal(t, settled.ID, settledAgain.ID)

	var refreshed models.User
	require.NoError(t, db.First(&refreshed, user.ID).Error)
	assert.Equal(t, int64(5000), refreshed.BalanceCents)

	// 4. 余额充足后可继续支付原套餐订单
	paidPlan, err := orderSvc.PayOrderWithBalance(user.ID, planOrder.OrderNo)
	require.NoError(t, err)
	assert.Equal(t, models.OrderPaid, paidPlan.Status)
	assert.WithinDuration(t, time.Now(), *paidPlan.PaidAt, 5*time.Second)

	require.NoError(t, db.First(&refreshed, user.ID).Error)
	assert.Equal(t, int64(2000), refreshed.BalanceCents)
}

func TestPaymentRegistryIsolationAndGiftCardGuard(t *testing.T) {
	db1 := setupRechargeTestDB(t)
	db2 := setupRechargeTestDB(t)
	svc1 := NewOrderService(gormstore.NewBillingStore(db1))
	svc2 := NewOrderService(gormstore.NewBillingStore(db2))

	svc1.Payments.Register(&mockOnlineProvider{id: "only-in-1"})
	_, ok1 := svc1.Payments.Get("only-in-1")
	_, ok2 := svc2.Payments.Get("only-in-1")
	assert.True(t, ok1)
	assert.False(t, ok2, "实例级注册表不应跨服务实例泄露")

	// GiftCardProvider.Initiate 必须拒绝待支付充值单（防双重入账/金额放大）
	gcSvc := NewGiftCardService(gormstore.NewBillingStore(db1))
	RegisterBuiltinPaymentProviders(svc1.Payments, svc1, gcSvc)
	p, ok := svc1.Payments.Get(models.PaymentMethodGiftCard)
	require.True(t, ok)
	_, err := p.Initiate(context.Background(), contracts.PaymentInitiateRequest{
		Order:  &models.Order{OrderNo: "RC-1", OrderType: models.OrderTypeRecharge, AmountCents: 10000},
		UserID: 1,
		Code:   "ANY-CODE",
	})
	assert.Error(t, err)
}
