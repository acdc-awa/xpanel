package billing

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/store/gormstore"
	"github.com/acdc-awa/xpanel/internal/models"
)

func orderTestDB(t *testing.T) *gorm.DB {
	dsn := fmt.Sprintf("file:mem_o_%s_%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open memory sqlite: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

// TestPayWithBalanceSaleGating 销售两属性兜底门控（2026-09-03）：
// 持有者（user.PlanID==planID）走续费语义查 renewable；非持有者新购查 purchasable。
// 商店层已按身份隐藏，此处验证手搓请求同样被拒。
func TestPayWithBalanceSaleGating(t *testing.T) {
	db := orderTestDB(t)
	svc := NewOrderService(gormstore.NewBillingStore(db))

	mkPlan := func(name string, purchasable, renewable bool) models.Plan {
		pl := models.Plan{Name: name, PriceCents: 1, TrafficGB: 10, DurationDays: 30, Purchasable: purchasable, Renewable: renewable}
		if err := db.Create(&pl).Error; err != nil {
			t.Fatalf("create plan: %v", err)
		}
		return pl
	}
	openNew := mkPlan("新购+续费", true, true)
	soldOut := mkPlan("停售保续费", false, true)
	noRenew := mkPlan("停续", true, false)
	retired := mkPlan("双关全闭", false, false)

	mkUser := func(name string, planID uint64) uint64 {
		u := models.User{Username: name, Email: name + "@x.com", UUID: "uuid-" + name, SubscribeToken: "tok-" + name, BalanceCents: 1_000_000, PlanID: planID}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
		return u.ID
	}

	pay := func(uid, planID uint64) error {
		_, err := svc.PayWithBalance(uid, planID)
		return err
	}
	reject := func(err error, want string) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("期望拒绝 %q，实际 %v", want, err)
		}
	}

	if err := pay(mkUser("u1", 0), openNew.ID); err != nil {
		t.Fatalf("非持有者新购应通过: %v", err)
	}
	reject(pay(mkUser("u2", 0), soldOut.ID), "已停售")
	reject(pay(mkUser("u3", 0), retired.ID), "已停售")

	if err := pay(mkUser("u4", openNew.ID), openNew.ID); err != nil {
		t.Fatalf("持有者可续费应通过: %v", err)
	}
	reject(pay(mkUser("u5", noRenew.ID), noRenew.ID), "已停止续费")

	// 持有 A 但买停售的 B → 非持有身份 → 拒绝
	reject(pay(mkUser("u6", openNew.ID), soldOut.ID), "已停售")

	// 成功支付后套餐快照落地（PlanID + 快照额度按当前套餐值）
	uid := mkUser("u7", 0)
	if err := pay(uid, openNew.ID); err != nil {
		t.Fatalf("新购应通过: %v", err)
	}
	var u models.User
	if err := db.First(&u, uid).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if u.PlanID != openNew.ID || u.PlanTrafficBytes != openNew.TrafficGB*1024*1024*1024 || u.PlanGroupID != openNew.PermissionGroupID {
		t.Fatalf("支付后快照未按套餐值落地: plan=%d traffic=%d group=%d", u.PlanID, u.PlanTrafficBytes, u.PlanGroupID)
	}
}
