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

// TestPayWithBalanceDurationSemantics 时长计算规则（2026-09-04 拍板，二改统一）：
// 购买一律作废现有剩余时长，自购买时刻重新起算——续费（同套餐）与切换（不同套餐）同规则，
// 已过期 / 无套餐同样自当前时刻起算；流量周期同步重置。
func TestPayWithBalanceDurationSemantics(t *testing.T) {
	db := orderTestDB(t)
	svc := NewOrderService(gormstore.NewBillingStore(db))

	mkPlan := func(name string, days int) models.Plan {
		pl := models.Plan{Name: name, PriceCents: 1, TrafficGB: 10, DurationDays: days, Purchasable: true, Renewable: true}
		if err := db.Create(&pl).Error; err != nil {
			t.Fatalf("create plan: %v", err)
		}
		return pl
	}
	planA := mkPlan("套餐A", 30)
	planB := mkPlan("套餐B", 30)

	mkUser := func(name string, planID uint64, expire *time.Time) uint64 {
		u := models.User{Username: name, Email: name + "@x.com", UUID: "uuid-" + name, SubscribeToken: "tok-" + name, BalanceCents: 1_000_000, PlanID: planID, ExpireAt: expire}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
		return u.ID
	}

	approx := func(got *time.Time, want time.Time, label string) {
		t.Helper()
		if got == nil {
			t.Fatalf("%s: expire_at 为空", label)
		}
		if d := got.Sub(want); d > 5*time.Second || d < -5*time.Second {
			t.Fatalf("%s: 期望 ≈ %v，实际 %v", label, want.Format(time.RFC3339), got.Format(time.RFC3339))
		}
	}

	// ① 切换套餐：剩余 20 天作废，自购买时刻重新起算 30 天（而非顺延为 50 天）
	future := time.Now().Add(20 * 24 * time.Hour)
	uid := mkUser("switch", planA.ID, &future)
	if _, err := svc.PayWithBalance(uid, planB.ID); err != nil {
		t.Fatalf("切换购买应通过: %v", err)
	}
	var u models.User
	if err := db.First(&u, uid).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	approx(u.ExpireAt, time.Now().Add(30*24*time.Hour), "切换套餐应作废剩余时长重新计时")

	// ② 同套餐续费：同样作废剩余 20 天，自购买时刻重新起算 30 天（而非顺延为 50 天）
	future2 := time.Now().Add(20 * 24 * time.Hour)
	uid = mkUser("renew", planA.ID, &future2)
	if _, err := svc.PayWithBalance(uid, planA.ID); err != nil {
		t.Fatalf("续费应通过: %v", err)
	}
	var u2 models.User // 复用 u 会让 First 把 dest 非零字段叠加进 WHERE（GORM 结构体条件机制），必须用新变量
	if err := db.First(&u2, uid).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	approx(u2.ExpireAt, time.Now().Add(30*24*time.Hour), "续费应作废剩余时长重新计时")

	// ③ 已过期用户买不同套餐：自当前时刻起算
	past := time.Now().Add(-24 * time.Hour)
	uid = mkUser("expired", planA.ID, &past)
	if _, err := svc.PayWithBalance(uid, planB.ID); err != nil {
		t.Fatalf("过期用户购买应通过: %v", err)
	}
	var u3 models.User
	if err := db.First(&u3, uid).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	approx(u3.ExpireAt, time.Now().Add(30*24*time.Hour), "已过期购买应自当前时刻起算")

	// ④ 无套餐用户购买：自当前时刻起算
	uid = mkUser("noplans", 0, nil)
	if _, err := svc.PayWithBalance(uid, planB.ID); err != nil {
		t.Fatalf("无套餐购买应通过: %v", err)
	}
	var u4 models.User
	if err := db.First(&u4, uid).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	approx(u4.ExpireAt, time.Now().Add(30*24*time.Hour), "无套餐购买应自当前时刻起算")
}
