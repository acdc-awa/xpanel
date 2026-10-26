package billing

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/master/store/gormstore"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 下单即固化套餐快照（并入 v4，2026-10-25）：套餐之后被归档或改名，旧订单仍能读出
// 当时购买的套餐名与配置，不依赖实时 JOIN plans。
func TestPayWithBalanceSnapshotsPlanOnOrder(t *testing.T) {
	db := orderTestDB(t)
	svc := NewOrderService(gormstore.NewBillingStore(db))

	plan := models.Plan{
		Name: "快照套餐", PriceCents: 1200, TrafficGB: 200, DurationDays: 30,
		DeviceLimit: 3, PermissionGroupID: 7, Purchasable: true, Renewable: true,
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	user := models.User{
		Username: "snap@test.local", Email: "snap@test.local", UUID: "uuid-snap",
		SubscribeToken: "tok-snap", BalanceCents: 1_000_000,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if _, err := svc.PayWithBalance(user.ID, plan.ID); err != nil {
		t.Fatalf("pay: %v", err)
	}

	var order models.Order
	if err := db.Where("user_id = ?", user.ID).First(&order).Error; err != nil {
		t.Fatalf("load order: %v", err)
	}
	if order.PlanName != "快照套餐" {
		t.Fatalf("order.plan_name = %q, want 快照套餐", order.PlanName)
	}
	if !strings.Contains(order.PlanSnapshot, `"traffic_gb":200`) ||
		!strings.Contains(order.PlanSnapshot, `"duration_days":30`) ||
		!strings.Contains(order.PlanSnapshot, `"device_limit":3`) {
		t.Fatalf("plan_snapshot 不完整: %s", order.PlanSnapshot)
	}
	var snap struct {
		Name              string `json:"name"`
		PriceCents        int64  `json:"price_cents"`
		PermissionGroupID uint64 `json:"permission_group_id"`
	}
	if err := json.Unmarshal([]byte(order.PlanSnapshot), &snap); err != nil {
		t.Fatalf("snapshot json: %v", err)
	}
	if snap.Name != "快照套餐" || snap.PriceCents != 1200 || snap.PermissionGroupID != 7 {
		t.Fatalf("snapshot = %+v", snap)
	}

	// 归档套餐后订单字段仍然自持可读（不依赖 plans 行是否可见）。
	if err := db.Delete(&plan).Error; err != nil {
		t.Fatalf("archive plan: %v", err)
	}
	var after models.Order
	if err := db.First(&after, order.ID).Error; err != nil {
		t.Fatalf("load order after archive: %v", err)
	}
	if after.PlanName != "快照套餐" || after.PlanSnapshot != order.PlanSnapshot {
		t.Fatalf("归档后订单快照被改写: name=%q snapshot=%q", after.PlanName, after.PlanSnapshot)
	}
}
