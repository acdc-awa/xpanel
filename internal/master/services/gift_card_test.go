package services

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/models"
)

func setupTestDB(t *testing.T) *gorm.DB {
	dsn := fmt.Sprintf("file:mem_%s_%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open memory sqlite: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestGiftCard_BatchGenerateAndRedeem(t *testing.T) {
	db := setupTestDB(t)
	svc := &GiftCardService{DB: db}

	// 1. 创建测试用户
	user := models.User{
		Username:       "testuser",
		Email:          "testuser@test.local",
		UUID:           "11111111-2222-3333-4444-555555555555",
		SubscribeToken: "subtoken111111111111111111111111111111111111111111111111111111111111",
		BalanceCents:   0,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	// 2. 批量生成 3 张 50 元 (5000分) 卡密
	adminID := uint64(1)
	cards, err := svc.BatchGenerate(adminID, 3, "测试批次50元", 5000, nil)
	if err != nil {
		t.Fatalf("BatchGenerate failed: %v", err)
	}
	if len(cards) != 3 {
		t.Fatalf("expected 3 cards, got %d", len(cards))
	}

	// 3. 用户兑换第 1 张
	card1 := cards[0]
	redeemed, newBal, err := svc.Redeem(user.ID, card1.Code)
	if err != nil {
		t.Fatalf("Redeem failed: %v", err)
	}
	if redeemed.Status != models.GiftCardUsed {
		t.Errorf("expected card status used, got %s", redeemed.Status)
	}
	if newBal != 5000 {
		t.Errorf("expected balance 5000, got %d", newBal)
	}

	// 4. 重复兑换同一张卡密应报错
	_, _, err = svc.Redeem(user.ID, card1.Code)
	if err == nil {
		t.Errorf("expected error when re-redeeming used card, got nil")
	}

	// 5. 兑换已过期的卡密应报错
	past := time.Now().Add(-1 * time.Hour)
	expiredCards, err := svc.BatchGenerate(adminID, 1, "过期卡", 1000, &past)
	if err != nil {
		t.Fatalf("BatchGenerate expired failed: %v", err)
	}
	_, _, err = svc.Redeem(user.ID, expiredCards[0].Code)
	if err == nil {
		t.Errorf("expected error when redeeming expired card, got nil")
	}

	// 6. 验证流水账本
	logs, total, err := svc.ListBalanceLogs(user.ID, 1, 20)
	if err != nil {
		t.Fatalf("ListBalanceLogs failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 log, got %d", total)
	}
	if logs[0].AmountCents != 5000 || logs[0].BalanceAfter != 5000 {
		t.Errorf("unexpected log record: %+v", logs[0])
	}
}

func TestOrder_PayWithBalance(t *testing.T) {
	db := setupTestDB(t)
	cardSvc := &GiftCardService{DB: db}
	orderSvc := &OrderService{DB: db}

	// 1. 创建套餐 (2500分 = 25元, 30天)
	plan := models.Plan{
		Name:         "月付25元",
		PriceCents:   2500,
		TrafficGB:    100,
		DurationDays: 30,
		Enabled:      true,
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan failed: %v", err)
	}

	// 2. 创建初始余额为 5000分 (50元) 的用户
	user := models.User{
		Username:       "buyer",
		Email:          "buyer@test.local",
		UUID:           "22222222-3333-4444-5555-666666666666",
		SubscribeToken: "subtoken222222222222222222222222222222222222222222222222222222222222",
		BalanceCents:   5000,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	// 3. 余额直付购买套餐
	order, err := orderSvc.PayWithBalance(user.ID, plan.ID)
	if err != nil {
		t.Fatalf("PayWithBalance failed: %v", err)
	}
	if order.Status != models.OrderPaid {
		t.Errorf("expected order status paid, got %s", order.Status)
	}
	if order.PaymentMethod != models.PaymentMethodBalance {
		t.Errorf("expected payment method balance, got %s", order.PaymentMethod)
	}

	// 4. 验证用户扣款与套餐生效
	var updatedUser models.User
	db.First(&updatedUser, user.ID)
	if updatedUser.BalanceCents != 2500 {
		t.Errorf("expected remaining balance 2500, got %d", updatedUser.BalanceCents)
	}
	if updatedUser.PlanID != plan.ID {
		t.Errorf("expected plan_id %d, got %d", plan.ID, updatedUser.PlanID)
	}
	if updatedUser.ExpireAt == nil {
		t.Errorf("expected expire_at to be set, got nil")
	}

	// 5. 验证扣款流水
	logs, total, err := cardSvc.ListBalanceLogs(user.ID, 1, 20)
	if err != nil {
		t.Fatalf("ListBalanceLogs failed: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 balance log, got %d", total)
	}
	if logs[0].AmountCents != -2500 || logs[0].BalanceAfter != 2500 {
		t.Errorf("unexpected deduction log: %+v", logs[0])
	}

	// 6. 幂等：窗口内重复支付同套餐 → 复用同一订单，不重复扣款
	dup, err := orderSvc.PayWithBalance(user.ID, plan.ID)
	if err != nil {
		t.Fatalf("idempotent re-purchase failed: %v", err)
	}
	if dup == nil || dup.ID != order.ID {
		t.Errorf("expected same order reused (original %d), got %+v", order.ID, dup)
	}
	var afterIdem models.User
	db.First(&afterIdem, user.ID)
	if afterIdem.BalanceCents != 2500 {
		t.Errorf("expected balance still 2500 after idempotent call, got %d", afterIdem.BalanceCents)
	}

	// 7. 余额不足测试（模拟幂等窗口过期后再购买）
	// 窗口过期 → 真实再次购买（扣 2500 -> 0）
	db.Model(&models.Order{}).Where("id = ?", order.ID).Update("created_at", time.Now().Add(-time.Minute))
	_, err = orderSvc.PayWithBalance(user.ID, plan.ID)
	if err != nil {
		t.Fatalf("purchase after window should succeed: %v", err)
	}
	// 把全部订单移出幂等窗口（模拟时间流逝），余额 0 < 2500 应该失败
	db.Model(&models.Order{}).Where("user_id = ? AND plan_id = ?", user.ID, plan.ID).
		Update("created_at", time.Now().Add(-time.Minute))
	_, err = orderSvc.PayWithBalance(user.ID, plan.ID)
	if err == nil {
		t.Errorf("expected error for insufficient balance, got nil")
	}
}

func TestGiftCard_AdminAdjustBalance(t *testing.T) {
	db := setupTestDB(t)
	svc := &GiftCardService{DB: db}

	user := models.User{
		Username:       "adjuser",
		Email:          "adjuser@test.local",
		UUID:           "33333333-4444-5555-6666-777777777777",
		SubscribeToken: "subtoken333333333333333333333333333333333333333333333333333333333333",
		BalanceCents:   1000,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	// 管理员手动 +3000分 (¥30.00)
	newBal, err := svc.AdminAdjustBalance(1, user.ID, 3000, "活动赠送")
	if err != nil {
		t.Fatalf("AdminAdjustBalance failed: %v", err)
	}
	if newBal != 4000 {
		t.Errorf("expected 4000, got %d", newBal)
	}

	// 扣成负数应拦截
	_, err = svc.AdminAdjustBalance(1, user.ID, -5000, "扣款超额")
	if err == nil {
		t.Errorf("expected error when balance becomes negative, got nil")
	}
}
