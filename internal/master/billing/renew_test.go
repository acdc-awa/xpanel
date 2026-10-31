package billing

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/contracts"
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

func mkRenewPlan(t *testing.T, db *gorm.DB, name string, renewable bool, groupID uint64) models.Plan {
	pl := models.Plan{
		Name: name, PriceCents: 100, TrafficGB: 10, DurationDays: 30,
		Purchasable: true, Renewable: renewable, PermissionGroupID: groupID,
	}
	if err := db.Create(&pl).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	return pl
}

// mkRenewUser 建一个持有套餐的用户，并写入与套餐一致的快照三列（用于断言退订后归零）。
func mkRenewUser(t *testing.T, db *gorm.DB, name string, planID uint64, expire *time.Time, balance int64, expireSwitch, exhaustSwitch bool) uint64 {
	u := models.User{
		Username: name, Email: name + "@x.com", UUID: "uuid-" + name, SubscribeToken: "tok-" + name,
		BalanceCents: balance, PlanID: planID, ExpireAt: expire,
		AutoRenewExpire: expireSwitch, AutoRenewExhaust: exhaustSwitch,
		PlanTrafficBytes: 10 * 1024 * 1024 * 1024,
		PlanDeviceLimit:  3,
		PlanGroupID:      7,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user %s: %v", name, err)
	}
	return u.ID
}

// mkTrafficLog 按用户当前账期写一条计费明细（与线上 same-bucket/cycle_id 打标一致）。
func mkTrafficLog(t *testing.T, db *gorm.DB, userID uint64, billed int64) {
	t.Helper()
	var u models.User
	if err := db.First(&u, userID).Error; err != nil {
		t.Fatalf("load user %d: %v", userID, err)
	}
	log := models.TrafficLog{
		UserID: userID, InboundID: 1,
		UpBytes: billed, BilledUp: billed,
		PeriodStart: models.TrafficCycleAlign(time.Now()),
		PeriodEnd:   time.Now(),
		CycleID:     u.TrafficCycleID,
	}
	if err := db.Create(&log).Error; err != nil {
		t.Fatalf("create traffic log: %v", err)
	}
}

func loadRenewUser(t *testing.T, db *gorm.DB, id uint64) models.User {
	t.Helper()
	var u models.User
	if err := db.First(&u, id).Error; err != nil {
		t.Fatalf("load user %d: %v", id, err)
	}
	return u
}

// cycleUsed 读生产口径的当前账期已用量（与 UserBilled/配额判定同源谓词）。
func cycleUsed(t *testing.T, db *gorm.DB, userID uint64) int64 {
	t.Helper()
	var used int64
	if err := db.Raw(`
		SELECT COALESCE(SUM(`+models.CycleUsageSQL+`), 0) AS used
		FROM users u
		LEFT JOIN traffic_logs l ON l.user_id = u.id
		WHERE u.id = ?`, userID).Scan(&used).Error; err != nil {
		t.Fatalf("sum cycle usage user=%d: %v", userID, err)
	}
	return used
}

func auditCount(t *testing.T, db *gorm.DB, action string) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&models.AuditLog{}).
		Where("operator_type = ? AND action = ?", "system", action).Count(&n).Error; err != nil {
		t.Fatalf("count audit %s: %v", action, err)
	}
	return n
}

func assertCleared(t *testing.T, u models.User, label string) {
	t.Helper()
	if u.PlanID != 0 {
		t.Fatalf("%s: 退订后 plan_id 应为 0，实际 %d", label, u.PlanID)
	}
	if u.PlanTrafficBytes != 0 || u.PlanDeviceLimit != 0 || u.PlanGroupID != 0 {
		t.Fatalf("%s: 套餐快照未清空: traffic=%d device=%d group=%d",
			label, u.PlanTrafficBytes, u.PlanDeviceLimit, u.PlanGroupID)
	}
	if u.PermissionGroupID != 0 {
		t.Fatalf("%s: 手动权限组未清空: %d", label, u.PermissionGroupID)
	}
}

// TestAutoRenewService 覆盖到期处置（续费/退订矩阵）与流量耗尽续费。
func TestAutoRenewService(t *testing.T) {
	db := renewTestDB(t)
	orderSvc := NewOrderService(gormstore.NewBillingStore(db))

	// 事件断言：退订必须批量上报一次，供主控同步节点用户列表。
	bus := contracts.NewEventBus()
	var clearedIDs []uint64
	bus.Subscribe(contracts.EventSubscriptionCleared, func(_ context.Context, ev contracts.DomainEvent) error {
		if e, ok := ev.(contracts.SubscriptionClearedEvent); ok {
			clearedIDs = append(clearedIDs, e.UserIDs...)
		}
		return nil
	})
	orderSvc.Events = bus

	svc := NewAutoRenewService(db, orderSvc)
	ctx := context.Background()
	plan := mkRenewPlan(t, db, "自动续套餐", true, 7)
	noRenewPlan := mkRenewPlan(t, db, "停续套餐", false, 7)
	day := 24 * time.Hour
	now := time.Now()

	approx := func(got *time.Time, want time.Time, label string) {
		t.Helper()
		if got == nil {
			t.Fatalf("%s: expire_at 为空", label)
		}
		if d := got.Sub(want); d > 5*time.Second || d < -5*time.Second {
			t.Fatalf("%s: 期望 ≈ %v，实际 %v", label, want.Format(time.RFC3339), got.Format(time.RFC3339))
		}
	}

	// ① 已到期 + 开关开 + 余额足 → 扣费续期，自当前时刻重算（不是顺延）
	uid1 := mkRenewUser(t, db, "renew", plan.ID, ptrTime(now.Add(-time.Minute)), 10_000, true, false)
	before1 := loadRenewUser(t, db, uid1)
	svc.RunExpiryOnce(ctx)
	after1 := loadRenewUser(t, db, uid1)
	approx(after1.ExpireAt, time.Now().Add(30*day), "到期自动续费应重算")
	if after1.BalanceCents != 9_900 {
		t.Fatalf("到期续费应扣款 100 分，实际余额 %d", after1.BalanceCents)
	}
	if after1.PlanID != plan.ID || after1.PlanGroupID != 7 {
		t.Fatalf("续费后套餐与快照应保持: plan=%d group=%d", after1.PlanID, after1.PlanGroupID)
	}
	if after1.TrafficCycleID != before1.TrafficCycleID+1 {
		t.Fatalf("续费后账期应递增: %d → %d", before1.TrafficCycleID, after1.TrafficCycleID)
	}

	// ② 未到期 → 不处置（哪怕开关开、余额足）
	uid2 := mkRenewUser(t, db, "notexpired", plan.ID, ptrTime(now.Add(2*time.Hour)), 10_000, true, false)
	svc.RunExpiryOnce(ctx)
	after2 := loadRenewUser(t, db, uid2)
	if after2.BalanceCents != 10_000 || after2.PlanID != plan.ID {
		t.Fatalf("未到期不应续费/退订: balance=%d plan=%d", after2.BalanceCents, after2.PlanID)
	}

	// ③ 已到期 + 开关关 → 退订（含手动分配的权限组一并清空、当前账期用量归零）
	uid3 := mkRenewUser(t, db, "switchoff", plan.ID, ptrTime(now.Add(-time.Minute)), 10_000, false, false)
	if err := db.Model(&models.User{}).Where("id = ?", uid3).Update("permission_group_id", 11).Error; err != nil {
		t.Fatalf("set permission group: %v", err)
	}
	mkTrafficLog(t, db, uid3, 4096)
	if got := cycleUsed(t, db, uid3); got != 4096 {
		t.Fatalf("退订前当前账期用量应为 4096，实际 %d", got)
	}
	before3 := loadRenewUser(t, db, uid3)
	svc.RunExpiryOnce(ctx)
	after3 := loadRenewUser(t, db, uid3)
	assertCleared(t, after3, "开关关到期")
	if after3.BalanceCents != 10_000 {
		t.Fatalf("退订不应扣款，实际余额 %d", after3.BalanceCents)
	}
	if after3.ExpireAt == nil || !after3.ExpireAt.Equal(*before3.ExpireAt) {
		t.Fatalf("退订应保留到期时间作二重拦截，实际 %v", after3.ExpireAt)
	}
	// 用量归零走账期递增（与购买/管理员重置同语义）：当前账期 0，旧账期明细仍留痕。
	if after3.TrafficCycleID != before3.TrafficCycleID+1 {
		t.Fatalf("退订后账期应递增: %d → %d", before3.TrafficCycleID, after3.TrafficCycleID)
	}
	if got := cycleUsed(t, db, uid3); got != 0 {
		t.Fatalf("退订后当前账期用量应归零，实际 %d", got)
	}
	if !after3.TrafficCycleStart.Equal(models.TrafficCycleAlign(after3.TrafficCycleStart)) {
		t.Fatalf("退订后流量周期起点未对齐整点: %v", after3.TrafficCycleStart)
	}
	var logCount int64
	if err := db.Model(&models.TrafficLog{}).Where("user_id = ?", uid3).Count(&logCount).Error; err != nil {
		t.Fatalf("count traffic logs: %v", err)
	}
	if logCount != 1 {
		t.Fatalf("旧账期明细应保留（仅当前账期归零），实际 %d 条", logCount)
	}

	// ④ 已到期 + 开关开 + 余额不足 → 不扣款 + 退订
	uid4 := mkRenewUser(t, db, "nobalance", plan.ID, ptrTime(now.Add(-time.Minute)), 0, true, false)
	svc.RunExpiryOnce(ctx)
	after4 := loadRenewUser(t, db, uid4)
	assertCleared(t, after4, "余额不足到期")
	if after4.BalanceCents != 0 {
		t.Fatalf("余额不足不应扣款，实际 %d", after4.BalanceCents)
	}

	// ⑤ 已到期 + 开关开 + 套餐已停止续费 → 不扣款 + 退订
	uid5 := mkRenewUser(t, db, "norenew", noRenewPlan.ID, ptrTime(now.Add(-time.Minute)), 10_000, true, false)
	svc.RunExpiryOnce(ctx)
	after5 := loadRenewUser(t, db, uid5)
	assertCleared(t, after5, "停续套餐到期")
	if after5.BalanceCents != 10_000 {
		t.Fatalf("停续套餐不应扣款，实际 %d", after5.BalanceCents)
	}

	// ⑥ 流量耗尽（未到期）+ 耗尽开关开 → 自动续购，账期重置
	uid6 := mkRenewUser(t, db, "exhausted", plan.ID, ptrTime(now.Add(10*day)), 10_000, false, true)
	if err := db.Model(&models.User{}).Where("id = ?", uid6).Update("plan_traffic_bytes", 1024).Error; err != nil {
		t.Fatalf("set quota: %v", err)
	}
	before6 := loadRenewUser(t, db, uid6)
	mkTrafficLog(t, db, uid6, 2048)
	svc.RunExhaustOnce(ctx)
	after6 := loadRenewUser(t, db, uid6)
	approx(after6.ExpireAt, time.Now().Add(30*day), "流量耗尽应自动续期重算")
	if after6.BalanceCents != 9_900 {
		t.Fatalf("流量耗尽续费应扣款 100 分，实际余额 %d", after6.BalanceCents)
	}
	if after6.TrafficCycleID != before6.TrafficCycleID+1 {
		t.Fatalf("流量耗尽续费后账期应递增: %d → %d", before6.TrafficCycleID, after6.TrafficCycleID)
	}
	// 周期起点对齐整点（写入侧不变量）
	if !after6.TrafficCycleStart.Equal(models.TrafficCycleAlign(after6.TrafficCycleStart)) {
		t.Fatalf("流量周期起点未对齐整点: %v", after6.TrafficCycleStart)
	}

	// ⑦ 流量耗尽但开关关 → 保留套餐（不清空、不扣款）
	uid7 := mkRenewUser(t, db, "exhaust-off", plan.ID, ptrTime(now.Add(10*day)), 10_000, false, false)
	if err := db.Model(&models.User{}).Where("id = ?", uid7).Update("plan_traffic_bytes", 1024).Error; err != nil {
		t.Fatalf("set quota: %v", err)
	}
	mkTrafficLog(t, db, uid7, 2048)
	svc.RunExhaustOnce(ctx)
	after7 := loadRenewUser(t, db, uid7)
	if after7.PlanID != plan.ID || after7.BalanceCents != 10_000 {
		t.Fatalf("耗尽开关关应保留套餐且不扣款: plan=%d balance=%d", after7.PlanID, after7.BalanceCents)
	}

	// ⑧ 幂等：整轮再跑一次，不重复扣款、不重复退订
	renewAuditBefore := auditCount(t, db, "auto_renew")
	clearAuditBefore := auditCount(t, db, "subscription_expired")
	svc.RunOnce(ctx)
	if got := auditCount(t, db, "auto_renew"); got != renewAuditBefore {
		t.Fatalf("二次扫描不应重复续费审计: %d → %d", renewAuditBefore, got)
	}
	if got := auditCount(t, db, "subscription_expired"); got != clearAuditBefore {
		t.Fatalf("二次扫描不应重复退订审计: %d → %d", clearAuditBefore, got)
	}
	if after := loadRenewUser(t, db, uid1); after.BalanceCents != 9_900 {
		t.Fatalf("二次扫描不应重复扣款（到期路径），实际余额 %d", after.BalanceCents)
	}
	if after := loadRenewUser(t, db, uid6); after.BalanceCents != 9_900 {
		t.Fatalf("二次扫描不应重复扣款（耗尽路径），实际余额 %d", after.BalanceCents)
	}

	// ⑨ 审计落库：续费 2 条（①⑥），退订 3 条（③④⑤）
	if got := auditCount(t, db, "auto_renew"); got != 2 {
		t.Fatalf("期望 2 条自动续费审计，实际 %d", got)
	}
	if got := auditCount(t, db, "subscription_expired"); got != 3 {
		t.Fatalf("期望 3 条到期退订审计，实际 %d", got)
	}

	// ⑩ 退订事件：只上报被清空的 3 个用户（批量去重后）
	seen := map[uint64]bool{}
	for _, id := range clearedIDs {
		seen[id] = true
	}
	if len(clearedIDs) != 3 || !seen[uid3] || !seen[uid4] || !seen[uid5] {
		t.Fatalf("退订事件应收齐 3 个用户，实际 %v", clearedIDs)
	}
}

// TestClearExpiredSubscriptionGuards 覆盖退订的行锁复核：未到期/无套餐/已退订都不应再写库。
func TestClearExpiredSubscriptionGuards(t *testing.T) {
	db := renewTestDB(t)
	svc := NewOrderService(gormstore.NewBillingStore(db))
	plan := mkRenewPlan(t, db, "守卫套餐", true, 0)
	now := time.Now()

	// 未到期 → 返回 false 且套餐保留
	future := mkRenewUser(t, db, "future", plan.ID, ptrTime(now.Add(time.Hour)), 0, false, false)
	if ok, err := svc.ClearExpiredSubscription(future); err != nil || ok {
		t.Fatalf("未到期不应退订: ok=%v err=%v", ok, err)
	}
	if u := loadRenewUser(t, db, future); u.PlanID != plan.ID {
		t.Fatalf("未到期套餐被误清: %d", u.PlanID)
	}

	// 已到期 → 清空一次；第二次返回 false（plan_id 已为 0）
	past := mkRenewUser(t, db, "past", plan.ID, ptrTime(now.Add(-time.Minute)), 0, false, false)
	if ok, err := svc.ClearExpiredSubscription(past); err != nil || !ok {
		t.Fatalf("已到期应退订: ok=%v err=%v", ok, err)
	}
	assertCleared(t, loadRenewUser(t, db, past), "已到期")
	if ok, err := svc.ClearExpiredSubscription(past); err != nil || ok {
		t.Fatalf("重复退订应无操作: ok=%v err=%v", ok, err)
	}

	// 无套餐 + 到期时间已过（历史数据）→ 不清空（无副作用但也不上报）
	var bare models.User
	if err := db.Create(&bare).Error; err != nil {
		t.Fatalf("create bare user: %v", err)
	}
	if ok, err := svc.ClearExpiredSubscription(bare.ID); err != nil || ok {
		t.Fatalf("无套餐不应触发退订: ok=%v err=%v", ok, err)
	}
}

// TestRenewWithBalanceRejectsPlanSwitch 自动续费在锁内复核持有关系：
// 用户已换套餐时不得按旧套餐扣款（否则会把旧套餐当新购卖出并覆盖刚生效的新套餐）。
func TestRenewWithBalanceRejectsPlanSwitch(t *testing.T) {
	db := renewTestDB(t)
	svc := NewOrderService(gormstore.NewBillingStore(db))
	oldPlan := mkRenewPlan(t, db, "旧套餐", true, 0)
	newPlan := mkRenewPlan(t, db, "新套餐", true, 0)
	uid := mkRenewUser(t, db, "switched", newPlan.ID, ptrTime(time.Now().Add(30*time.Hour)), 10_000, false, false)

	if _, err := svc.RenewWithBalance(uid, oldPlan.ID); !errors.Is(err, ErrPlanChanged) {
		t.Fatalf("换套餐后应拒绝按旧套餐续费，实际 err=%v", err)
	}
	u := loadRenewUser(t, db, uid)
	if u.PlanID != newPlan.ID || u.BalanceCents != 10_000 {
		t.Fatalf("不应扣款/改套餐: plan=%d balance=%d", u.PlanID, u.BalanceCents)
	}

	// 按当前持有套餐续费应正常生效
	if _, err := svc.RenewWithBalance(uid, newPlan.ID); err != nil {
		t.Fatalf("按当前套餐续费应成功: %v", err)
	}
	if u := loadRenewUser(t, db, uid); u.BalanceCents != 9_900 {
		t.Fatalf("续费应扣款 100 分，实际 %d", u.BalanceCents)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
