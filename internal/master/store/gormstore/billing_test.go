package gormstore

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
)

func setupTestStore(t *testing.T) *BillingStore {
	t.Helper()
	dsn := fmt.Sprintf("file:mem_gormstore_%s_%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open memory sqlite: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewBillingStore(db)
}

// TestBillingStore_GiftCardListAndDelete 覆盖列表过滤/分页与删除路径
// （服务层测试已覆盖兑换/支付事务，这里直测仓储的查询语义）。
func TestBillingStore_GiftCardListAndDelete(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	mk := func(code, name, status string) {
		card := models.GiftCard{Code: code, Name: name, FaceValueCents: 1000, Status: status, CreatedAt: time.Now()}
		if err := store.CreateGiftCard(ctx, &card); err != nil {
			t.Fatalf("CreateGiftCard: %v", err)
		}
	}
	mk("CODE-A1", "周年庆批次", "unused")
	mk("CODE-A2", "周年庆批次", "used")
	mk("CODE-B1", "日常批次", "unused")

	// 状态过滤
	list, total, err := store.ListGiftCards(ctx, contracts.GiftCardQuery{Status: "unused", Page: 1, Size: 20})
	if err != nil || total != 2 || len(list) != 2 {
		t.Fatalf("状态过滤: total=%d len=%d err=%v", total, len(list), err)
	}
	// 关键词过滤（命中 name）
	_, total, err = store.ListGiftCards(ctx, contracts.GiftCardQuery{Search: "周年庆", Page: 1, Size: 20})
	if err != nil || total != 2 {
		t.Fatalf("名称关键词: total=%d err=%v", total, err)
	}
	// 关键词过滤（命中 code）
	_, total, err = store.ListGiftCards(ctx, contracts.GiftCardQuery{Search: "CODE-B", Page: 1, Size: 20})
	if err != nil || total != 1 {
		t.Fatalf("code 关键词: total=%d err=%v", total, err)
	}
	// 分页
	list, total, err = store.ListGiftCards(ctx, contracts.GiftCardQuery{Page: 2, Size: 2})
	if err != nil || total != 3 || len(list) != 1 {
		t.Fatalf("分页: total=%d len=%d err=%v", total, len(list), err)
	}

	// Get + Delete
	card, err := store.LockGiftCardByCode(ctx, "CODE-A1")
	if err != nil {
		t.Fatalf("LockGiftCardByCode: %v", err)
	}
	if err := store.DeleteGiftCard(ctx, card.ID); err != nil {
		t.Fatalf("DeleteGiftCard: %v", err)
	}
	if _, err := store.GetGiftCard(ctx, card.ID); err == nil {
		t.Fatal("删除后 GetGiftCard 应报错")
	}
}

// TestBillingStore_GiftCardListOrder 覆盖默认排序：未使用且未过期在前，
// 过期/已使用/已作废随后，组内均按 id 倒序。
func TestBillingStore_GiftCardListOrder(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(24 * time.Hour)
	mk := func(code, status string, expires *time.Time) {
		card := models.GiftCard{Code: code, Name: "批次", FaceValueCents: 1000, Status: status, ExpiresAt: expires}
		if err := store.CreateGiftCard(ctx, &card); err != nil {
			t.Fatalf("CreateGiftCard: %v", err)
		}
	}
	// 依次创建（id 递增）：已使用 / 未使用已过期 / 已作废 / 未使用未过期 / 未使用永久
	mk("C1-USED", "used", nil)
	mk("C2-EXPIRED", "unused", &past)
	mk("C3-DISABLED", "disabled", nil)
	mk("C4-ACTIVE", "unused", &future)
	mk("C5-ACTIVE", "unused", nil)

	list, total, err := store.ListGiftCards(ctx, contracts.GiftCardQuery{Page: 1, Size: 20})
	if err != nil || total != 5 {
		t.Fatalf("total=%d len=%d err=%v", total, len(list), err)
	}
	want := []string{"C5-ACTIVE", "C4-ACTIVE", "C3-DISABLED", "C2-EXPIRED", "C1-USED"}
	got := make([]string, 0, len(list))
	for _, c := range list {
		got = append(got, c.Code)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("顺序不符:\n got  %v\n want %v", got, want)
		}
	}
}

// TestBillingStore_ListOrdersByUser 订单列表倒序 + limit。
func TestBillingStore_ListOrdersByUser(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		o := models.Order{
			OrderNo: fmt.Sprintf("NO-%d", i), UserID: 7, PlanID: 1,
			AmountCents: 100, PaymentMethod: models.PaymentMethodBalance, Status: models.OrderPaid,
		}
		if err := store.CreateOrder(ctx, &o); err != nil {
			t.Fatalf("CreateOrder: %v", err)
		}
	}
	// 其他用户的订单不应混入
	if err := store.CreateOrder(ctx, &models.Order{OrderNo: "NO-OTHER", UserID: 8, Status: models.OrderPaid}); err != nil {
		t.Fatalf("CreateOrder other: %v", err)
	}

	list, err := store.ListOrdersByUser(ctx, 7, 2)
	if err != nil || len(list) != 2 {
		t.Fatalf("limit=2 应返回 2 条, got %d err=%v", len(list), err)
	}
	if list[0].OrderNo != "NO-2" {
		t.Fatalf("应按 id 倒序（最新在前）, got %s", list[0].OrderNo)
	}
	for _, o := range list {
		if o.UserID != 7 {
			t.Fatalf("混入其他用户订单: %+v", o)
		}
	}
}
