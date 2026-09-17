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

// cycleUsed 复刻计费口径（与 services.TrafficService.UserBilled 同语义）：
// 周期内 SUM(billed_up/billed_down)。
func (s *BillingStore) cycleUsed(ctx context.Context, userID uint64, cycleStart time.Time) (int64, int64, error) {
	var row struct{ Up, Down int64 }
	q := s.with(ctx).Model(&models.TrafficLog{}).Where("user_id = ?", userID)
	if !cycleStart.IsZero() {
		q = q.Where("period_start >= ?", cycleStart)
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
