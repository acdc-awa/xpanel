package gormstore

import (
	"context"
	"testing"
	"time"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 购买/续费切周期：起点对齐整点，并清零该整点桶内切换前已累计的计费字节。
// 两者缺一不可——只对齐会把「整点到购买时刻」的旧用量带进新周期（购买后已用量不为 0）；
// 只清零而不对齐则桶仍早于起点，整桶继续被计费口径排除（原缺陷：新用户首小时计费恒为 0）。
func TestUpdateSubscriptionAlignsCycleAndClearsBoundaryBucket(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	db := store.db

	user := models.User{
		Username: "cycle@panel.local", Email: "cycle@panel.local",
		UUID: "uuid-cycle", PasswordHash: "x", Status: models.StatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	plan := models.Plan{Name: "P", DurationDays: 30}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	// 购买时刻落在小时中间（用户 23 的实际形态），该小时桶内已有切换前的用量
	buyAt := time.Date(2026, 9, 17, 6, 1, 21, 896970490, time.UTC)
	bucket := models.TrafficCycleAlign(buyAt)
	seed := models.TrafficLog{
		UserID: user.ID, InboundID: 12,
		UpBytes: 423431915, DownBytes: 1071081579,
		BilledUp: 4234321, BilledDown: 10710821,
		PeriodStart: bucket,
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatalf("seed traffic: %v", err)
	}

	if err := store.Transaction(ctx, func(tx contracts.BillingStore) error {
		return tx.UpdateSubscription(ctx, user.ID, &plan, buyAt.AddDate(0, 0, 30), buyAt)
	}); err != nil {
		t.Fatalf("UpdateSubscription: %v", err)
	}

	var got models.User
	if err := db.First(&got, user.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if !got.TrafficCycleStart.Equal(bucket) {
		t.Fatalf("周期起点 = %v, want 对齐后的 %v", got.TrafficCycleStart, bucket)
	}

	var row models.TrafficLog
	if err := db.First(&row, seed.ID).Error; err != nil {
		t.Fatalf("reload traffic: %v", err)
	}
	if row.BilledUp != 0 || row.BilledDown != 0 {
		t.Fatalf("边界桶计费字节未清零: billed=%d/%d, want 0/0", row.BilledUp, row.BilledDown)
	}
	// 原始字节列不动：仪表盘/每日汇总/节点流量口径不受倍率与周期切换影响
	if row.UpBytes != seed.UpBytes || row.DownBytes != seed.DownBytes {
		t.Fatalf("原始字节被改动: %d/%d, want %d/%d", row.UpBytes, row.DownBytes, seed.UpBytes, seed.DownBytes)
	}

	// 切换之后的上报继续 upsert 累加进同一行（唯一索引 user_id+inbound_id+period_start），
	// 该行 period_start 恰等于新周期起点，故新周期用量自切换时刻起精确起算。
	if err := db.Model(&models.TrafficLog{}).Where("id = ?", seed.ID).
		Updates(map[string]any{"billed_up": 1000, "billed_down": 2000}).Error; err != nil {
		t.Fatalf("simulate post-switch report: %v", err)
	}
	up, down, err := store.cycleUsed(ctx, user.ID, got.TrafficCycleStart)
	if err != nil {
		t.Fatalf("cycleUsed: %v", err)
	}
	if up != 1000 || down != 2000 {
		t.Fatalf("新周期用量 = %d/%d, want 1000/2000（只计切换后的字节）", up, down)
	}
}

// cycleUsed 复刻计费口径（与 services.TrafficService.UserBilled 同语义，审计 F3 后为账期口径）：
// 账期 ID 命中的行计入；cycle_id = 0 的存量行按 period_start >= 周期起点回退。
func (s *BillingStore) cycleUsed(ctx context.Context, userID uint64, cycleStart time.Time) (int64, int64, error) {
	var row struct{ Up, Down int64 }
	q := s.with(ctx).Model(&models.TrafficLog{}).Where("user_id = ?", userID)
	if !cycleStart.IsZero() {
		q = q.Where("cycle_id = (SELECT traffic_cycle_id FROM users WHERE id = ?) OR (cycle_id = 0 AND period_start >= ?)",
			userID, cycleStart)
	}
	err := q.Select("COALESCE(SUM(billed_up),0) AS up, COALESCE(SUM(billed_down),0) AS down").Scan(&row).Error
	return row.Up, row.Down, err
}

// 新建用户（注册路径）的周期起点也必须落在整点，否则该用户首个小时的用量同样被排除。
func TestUserBeforeCreateAlignsCycleStart(t *testing.T) {
	store := setupTestStore(t)
	u := models.User{
		Username: "reg@panel.local", Email: "reg@panel.local",
		UUID: "uuid-reg", PasswordHash: "x", Status: models.StatusActive,
	}
	if err := store.db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if u.TrafficCycleStart.IsZero() {
		t.Fatal("TrafficCycleStart 未被设置")
	}
	if !u.TrafficCycleStart.Equal(models.TrafficCycleAlign(u.TrafficCycleStart)) {
		t.Fatalf("TrafficCycleStart = %v, 未对齐整点", u.TrafficCycleStart)
	}
	if u.TrafficCycleStart.Minute() != 0 || u.TrafficCycleStart.Second() != 0 || u.TrafficCycleStart.Nanosecond() != 0 {
		t.Fatalf("TrafficCycleStart = %v, 含分/秒/纳秒", u.TrafficCycleStart)
	}
}

// TestUpdateSubscriptionBumpsCycleIDAndKeepsTaggedRows 审计 F3：切周期递增账期 ID，
// 且**不再**清零带账期标记的行（旧账期账本留痕）。
//
// 归属改由 cycle_id 决定后，新周期从 0 起算不再依赖「清零边界桶」；清零仅保留给
// cycle_id = 0 的存量行/旧 agent 行（它们只能按时间轴回退归属）。若连带标记的行一起清零，
// 旧账期的计费流水就被抹掉了（审计 §5「重置后的可追溯性」）。
func TestUpdateSubscriptionBumpsCycleIDAndKeepsTaggedRows(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	db := store.db

	user := models.User{
		Username: "cyc@panel.local", Email: "cyc@panel.local",
		UUID: "uuid-cyc", PasswordHash: "x", Status: models.StatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	plan := models.Plan{Name: "P", DurationDays: 30}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	buyAt := time.Date(2026, 9, 20, 10, 30, 0, 0, time.UTC)
	bucket := models.TrafficCycleAlign(buyAt)
	// 三行：带旧账期标记（应保留）、无标记（应清零）、周期起点之前的无标记行（不受影响）
	tagged := models.TrafficLog{UserID: user.ID, InboundID: 1, UpBytes: 300, BilledUp: 300,
		PeriodStart: bucket, CycleID: 1}
	legacy := models.TrafficLog{UserID: user.ID, InboundID: 2, UpBytes: 200, BilledUp: 200,
		PeriodStart: bucket, CycleID: 0}
	elsewhere := models.TrafficLog{UserID: user.ID, InboundID: 3, UpBytes: 400, BilledUp: 400,
		PeriodStart: bucket.Add(-2 * time.Hour), CycleID: 0}
	for _, r := range []*models.TrafficLog{&tagged, &legacy, &elsewhere} {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed traffic: %v", err)
		}
	}

	if err := store.Transaction(ctx, func(tx contracts.BillingStore) error {
		return tx.UpdateSubscription(ctx, user.ID, &plan, buyAt.AddDate(0, 0, 30), buyAt)
	}); err != nil {
		t.Fatalf("UpdateSubscription: %v", err)
	}

	var got models.User
	if err := db.First(&got, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.TrafficCycleID != 2 {
		t.Fatalf("账期 ID 应递增到 2，实际 %d", got.TrafficCycleID)
	}
	if !got.TrafficCycleStart.Equal(bucket) {
		t.Fatalf("周期起点应仍对齐整点 %v，实际 %v", bucket, got.TrafficCycleStart)
	}

	var gotTagged, gotLegacy models.TrafficLog
	if err := db.First(&gotTagged, tagged.ID).Error; err != nil {
		t.Fatal(err)
	}
	if gotTagged.BilledUp != 300 {
		t.Fatalf("带账期标记的行不得被清零（旧账期账本留痕），实际 billed=%d", gotTagged.BilledUp)
	}
	if err := db.First(&gotLegacy, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if gotLegacy.BilledUp != 0 {
		t.Fatalf("无账期标记的边界桶行应清零（按时间轴回退归属的保护），实际 billed=%d", gotLegacy.BilledUp)
	}

	// 新周期用量 = 0：旧账期行按 cycle_id 排除，无标记行已被清零
	up, _, err := store.cycleUsed(ctx, user.ID, got.TrafficCycleStart)
	if err != nil {
		t.Fatal(err)
	}
	if up != 0 {
		t.Fatalf("切换后新周期用量应为 0，实际 %d", up)
	}
}
