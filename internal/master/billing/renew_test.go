package billing

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/store/gormstore"
	"github.com/acdc-awa/xpanel/internal/models"
)

func renewTestDB(t *testing.T) *gorm.DB {
	dsn := "file:mem_renew_" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open memory sqlite: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func mkRenewPlan(t *testing.T, db *gorm.DB, renewable bool) models.Plan {
	pl := models.Plan{Name: "自动续套餐", PriceCents: 100, TrafficGB: 10, DurationDays: 30, Purchasable: true, Renewable: renewable}
	if err := db.Create(&pl).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return pl
}

func mkRenewUser(t *testing.T, db *gorm.DB, name string, planID uint64, expire *time.Time, balance int64, expireSwitch, exhaustSwitch bool) uint64 {
	u := models.User{
		Username: name, Email: name + "@x.com", UUID: "uuid-" + name, SubscribeToken: "tok-" + name,
		BalanceCents: balance, PlanID: planID, ExpireAt: expire,
		AutoRenewExpire: expireSwitch, AutoRenewExhaust: exhaustSwitch,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u.ID
}

// TestAutoRenewService 覆盖双触发路径、幂等、余额不足、停续门控与开关未开。
func TestAutoRenewService(t *testing.T) {
	db := renewTestDB(t)
	svc := NewAutoRenewService(db, NewOrderService(gormstore.NewBillingStore(db)))
	ctx := context.Background()
	plan := mkRenewPlan(t, db, true)
	day := 24 * time.Hour

	expireOf := func(uid uint64) *time.Time {
		var u models.User
		if err := db.First(&u, uid).Error; err != nil {
			t.Fatalf("load user %d: %v", uid, err)
		}
		return u.ExpireAt
	}
	balanceOf := func(uid uint64) int64 {
		var u models.User
		db.First(&u, uid)
		return u.BalanceCents
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

	// ① 到期窗口内（剩 30 分钟）+ 开关开 + 余额足 → 扣费续期，自当前时刻重算
	uid1 := mkRenewUser(t, db, "window", plan.ID, ptrTime(time.Now().Add(30*time.Minute)), 10_000, true, false)
	svc.RunOnce(ctx)
	approx(expireOf(uid1), time.Now().Add(30*day), "到期窗口应自动续期重算")
	if got := balanceOf(uid1); got != 10_000-100 {
		t.Fatalf("到期续费应扣款 100 分，实际余额 %d", got)
	}

	// ② 未进窗口（剩 2 小时）不触发
	uid2 := mkRenewUser(t, db, "notwindow", plan.ID, ptrTime(time.Now().Add(2*time.Hour)), 10_000, true, false)
	svc.RunOnce(ctx)
	approx(expireOf(uid2), time.Now().Add(2*time.Hour), "未进窗口不应续期")

	// ③ 窗口内但开关未开不触发
	uid3 := mkRenewUser(t, db, "switchoff", plan.ID, ptrTime(time.Now().Add(30*time.Minute)), 10_000, false, false)
	svc.RunOnce(ctx)
	approx(expireOf(uid3), time.Now().Add(30*time.Minute), "开关未开不应续期")

	// ④ 余额不足：不扣费、expire 不变、开关保持（充值后下轮自动补续）
	uid4 := mkRenewUser(t, db, "nobalance", plan.ID, ptrTime(time.Now().Add(10*time.Minute)), 0, true, false)
	svc.RunOnce(ctx)
	if got := balanceOf(uid4); got != 0 {
		t.Fatalf("余额不足不应扣款，实际 %d", got)
	}
	var u4 models.User
	db.First(&u4, uid4)
	if !u4.AutoRenewExpire {
		t.Fatalf("余额不足不应关闭自动续费开关")
	}

	// ⑤ 流量耗尽触发：本周期用量 >= 额度 → 自动续购（流量周期重置即退出耗尽条件）
	uid5 := mkRenewUser(t, db, "exhausted", plan.ID, ptrTime(time.Now().Add(10*day)), 10_000, false, true)
	db.Model(&models.User{}).Where("id = ?", uid5).Update("plan_traffic_bytes", 1024) // 额度 1KB 便于触发
	// 周期起点=用户创建时刻（BeforeCreate），上报周期落在起点之后才计入当期用量
	if err := db.Create(&models.TrafficLog{UserID: uid5, InboundID: 1, UpBytes: 2048, DownBytes: 0, PeriodStart: time.Now(), PeriodEnd: time.Now()}).Error; err != nil {
		t.Fatalf("create traffic log: %v", err)
	}
	svc.RunOnce(ctx)
	approx(expireOf(uid5), time.Now().Add(30*day), "流量耗尽应自动续期重算")
	var u5 models.User
	db.First(&u5, uid5)
	if u5.TrafficCycleStart.Before(time.Now().Add(-time.Minute)) {
		t.Fatalf("耗尽续费应重置流量周期起点")
	}

	// ⑥ 幂等：再次扫描，已续期用户不重复扣费
	svc.RunOnce(ctx)
	if got := balanceOf(uid1); got != 10_000-100 {
		t.Fatalf("二次扫描不应重复扣款（到期路径），实际余额 %d", got)
	}
	if got := balanceOf(uid5); got != 10_000-100 {
		t.Fatalf("二次扫描不应重复扣款（耗尽路径），实际余额 %d", got)
	}

	// ⑦ 套餐停续（renewable=false）→ 跳过不扣费
	if err := db.Model(&models.Plan{}).Where("id = ?", plan.ID).Update("renewable", false).Error; err != nil {
		t.Fatalf("disable renewable: %v", err)
	}
	uid7 := mkRenewUser(t, db, "norenew", plan.ID, ptrTime(time.Now().Add(30*time.Minute)), 10_000, true, false)
	svc.RunOnce(ctx)
	if got := balanceOf(uid7); got != 10_000 {
		t.Fatalf("停续套餐不应自动扣费，实际余额 %d", got)
	}

	// ⑧ 自动续费审计落库
	var auditCount int64
	db.Model(&models.AuditLog{}).Where("operator_type = ? AND action = ?", "system", "auto_renew").Count(&auditCount)
	if auditCount < 2 {
		t.Fatalf("期望至少 2 条自动续费审计（①⑤），实际 %d", auditCount)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
