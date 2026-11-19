package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBackfillGiftCardOrdersIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Setting{}, &Order{}, &GiftCard{}))

	now := time.Now().UTC().Truncate(time.Second)
	usedCard := GiftCard{
		Code:           "GIFT-TEST-0001",
		Name:           "历史卡密",
		FaceValueCents: 1500,
		Status:         GiftCardUsed,
		UsedBy:         42,
		UsedAt:         &now,
	}
	unusedCard := GiftCard{
		Code:           "GIFT-TEST-0002",
		Name:           "未使用",
		FaceValueCents: 1000,
		Status:         GiftCardUnused,
	}
	require.NoError(t, db.Create(&usedCard).Error)
	require.NoError(t, db.Create(&unusedCard).Error)

	// 存量空 order_type 的套餐订单
	legacyPlanOrder := Order{
		OrderNo:     "LEG-001",
		OrderType:   "",
		UserID:      42,
		PlanID:      1,
		AmountCents: 2000,
		Status:      OrderPaid,
	}
	require.NoError(t, db.Create(&legacyPlanOrder).Error)

	require.NoError(t, backfillGiftCardOrders(db))
	// 幂等重跑
	require.NoError(t, backfillGiftCardOrders(db))

	var orders []Order
	require.NoError(t, db.Order("id ASC").Find(&orders).Error)
	require.Len(t, orders, 2)

	assert.Equal(t, OrderTypePlan, orders[0].OrderType)
	assert.Equal(t, OrderTypeRecharge, orders[1].OrderType)
	assert.Equal(t, uint64(42), orders[1].UserID)
	assert.Equal(t, int64(1500), orders[1].AmountCents)
	assert.Equal(t, PaymentMethodGiftCard, orders[1].PaymentMethod)
	assert.Equal(t, OrderPaid, orders[1].Status)
}
