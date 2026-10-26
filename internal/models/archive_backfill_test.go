package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newBackfillTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func clearBackfillMarkers(t *testing.T, db *gorm.DB, keys ...string) {
	t.Helper()
	if err := db.Where("key IN ?", keys).Delete(&Setting{}).Error; err != nil {
		t.Fatalf("clear markers: %v", err)
	}
}

// v4 追加迁移（2026-10-25）：存量 traffic_logs 按 inbound_id → inbounds.server_id 回填；
// 入站已删的孤儿行保持 0（无法重建），且回填标记写入后不再重复扫描。
func TestBackfillTrafficServerIDs(t *testing.T) {
	db := newBackfillTestDB(t)
	clearBackfillMarkers(t, db, "traffic_server_id_backfill_version", "traffic_server_id_backfill_cursor")

	srv := Server{Name: "s1", Host: "1.1.1.1", NodeID: "n1", Secret: "s"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatalf("create server: %v", err)
	}
	inb := Inbound{ServerID: srv.ID, Tag: "in-1", Protocol: ProtocolVless, Type: InboundTypeUser, Port: 443}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	now := time.Now().UTC()
	attributable := TrafficLog{UserID: 1, InboundID: inb.ID, UpBytes: 10, DownBytes: 20, PeriodStart: now}
	orphan := TrafficLog{UserID: 1, InboundID: 999999, UpBytes: 1, DownBytes: 1, PeriodStart: now}
	if err := db.Create(&attributable).Error; err != nil {
		t.Fatalf("create attributable: %v", err)
	}
	if err := db.Create(&orphan).Error; err != nil {
		t.Fatalf("create orphan: %v", err)
	}

	if err := backfillTrafficServerIDs(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	var gotAttr, gotOrphan TrafficLog
	if err := db.First(&gotAttr, attributable.ID).Error; err != nil {
		t.Fatalf("read attributable: %v", err)
	}
	if gotAttr.ServerID != srv.ID {
		t.Fatalf("attributable server_id = %d, want %d", gotAttr.ServerID, srv.ID)
	}
	if err := db.First(&gotOrphan, orphan.ID).Error; err != nil {
		t.Fatalf("read orphan: %v", err)
	}
	if gotOrphan.ServerID != 0 {
		t.Fatalf("orphan server_id = %d, want 0（入站已删无法重建）", gotOrphan.ServerID)
	}
	if v := readSettingValue(db, "traffic_server_id_backfill_version"); v != "1" {
		t.Fatalf("done marker = %q, want 1", v)
	}

	// 标记生效后不再回填新行（留待后续显式迁移），再次执行应保持 0。
	late := TrafficLog{UserID: 2, InboundID: inb.ID, UpBytes: 5, DownBytes: 5, PeriodStart: now.Add(time.Hour)}
	if err := db.Create(&late).Error; err != nil {
		t.Fatalf("create late: %v", err)
	}
	if err := backfillTrafficServerIDs(db); err != nil {
		t.Fatalf("backfill again: %v", err)
	}
	var gotLate TrafficLog
	if err := db.First(&gotLate, late.ID).Error; err != nil {
		t.Fatalf("read late: %v", err)
	}
	if gotLate.ServerID != 0 {
		t.Fatalf("done marker 未生效：late server_id = %d", gotLate.ServerID)
	}

	// 清掉标记后（等价于迁移中断重跑）应把剩余行补齐。
	clearBackfillMarkers(t, db, "traffic_server_id_backfill_version", "traffic_server_id_backfill_cursor")
	if err := backfillTrafficServerIDs(db); err != nil {
		t.Fatalf("backfill rerun: %v", err)
	}
	if err := db.First(&gotLate, late.ID).Error; err != nil {
		t.Fatalf("read late after rerun: %v", err)
	}
	if gotLate.ServerID != srv.ID {
		t.Fatalf("rerun late server_id = %d, want %d", gotLate.ServerID, srv.ID)
	}
}

// v4 追加迁移（2026-10-25）：存量订单回填套餐名；归档套餐行仍在 plans 表，子查询同样能取到名称。
func TestBackfillOrderPlanNames(t *testing.T) {
	db := newBackfillTestDB(t)
	clearBackfillMarkers(t, db, "order_plan_name_backfill_version")

	plan := Plan{Name: "历史套餐", PriceCents: 100, TrafficGB: 10, DurationDays: 30}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	order := Order{OrderNo: "ORD-BACKFILL-1", UserID: 1, PlanID: plan.ID, AmountCents: 100}
	if err := db.Create(&order).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}
	// 归档套餐（软删除）后其名称仍应能回填。
	if err := db.Delete(&plan).Error; err != nil {
		t.Fatalf("archive plan: %v", err)
	}

	if err := backfillOrderPlanNames(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	var got Order
	if err := db.First(&got, order.ID).Error; err != nil {
		t.Fatalf("read order: %v", err)
	}
	if got.PlanName != "历史套餐" {
		t.Fatalf("plan_name = %q, want 历史套餐", got.PlanName)
	}

	// 幂等：改坏名称后重跑（标记已写）不应覆盖。
	if err := db.Model(&Order{}).Where("id = ?", order.ID).Update("plan_name", "manual").Error; err != nil {
		t.Fatalf("mutate name: %v", err)
	}
	if err := backfillOrderPlanNames(db); err != nil {
		t.Fatalf("backfill second: %v", err)
	}
	requireName := ""
	var names []string
	if err := db.Model(&Order{}).Where("id = ?", order.ID).Pluck("plan_name", &names).Error; err != nil {
		t.Fatalf("read name: %v", err)
	}
	if len(names) == 1 {
		requireName = names[0]
	}
	if requireName != "manual" {
		t.Fatalf("幂等失败：plan_name = %q, want manual（标记生效后不得覆盖）", requireName)
	}
}
