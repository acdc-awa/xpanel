package services

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 本文件是流量计费审计（F1/F2/F3）的回归测试：批次去重与账期归属。
// 背景见 docs/research/流量计费审计报告.md，实现在 services/traffic.go + models/traffic_cycle.go。

func cycleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}, &models.User{},
		&models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{}, &models.TrafficBatch{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func cycleTestUser(t *testing.T, db *gorm.DB, name string, cycleID uint64, cycleStart time.Time) models.User {
	t.Helper()
	u := models.User{Username: name, Email: name + "@t.com", UUID: "uuid-" + name,
		SubscribeToken: "tok-" + name, Status: models.StatusActive}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.User{}).Where("id = ?", u.ID).Updates(map[string]any{
		"traffic_cycle_start": cycleStart,
		"traffic_cycle_id":    cycleID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return u
}

// TestSaveBatchDedupChargesOnce 审计 F2 回归：同一 BatchID 重复投递只计一次。
// 旧实现按 (user,inbound,period) upsert 累加，重发会翻倍计量（实测一次消费 300 计成 600）。
func TestSaveBatchDedupChargesOnce(t *testing.T) {
	db := cycleTestDB(t)
	u := cycleTestUser(t, db, "dup", 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	svc := &TrafficService{DB: db}

	batch := protocol.TrafficReportPayload{
		Period:  "2026-09-20T00:00:00Z",
		BatchID: "batch-aaa",
		Entries: []protocol.TrafficEntry{{UserID: u.ID, UpBytes: 100, DownBytes: 200}},
	}
	for i := 0; i < 3; i++ { // 投递三次（重发两次）
		if _, err := svc.Save(batch, 1); err != nil {
			t.Fatalf("Save #%d: %v", i+1, err)
		}
	}
	up, down, err := svc.UserBilled(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if up != 100 || down != 200 {
		t.Fatalf("同一批次重复投递后计费 = %d/%d, want 100/200（只计一次）", up, down)
	}

	// 不同批次号、同一小时 → 必须累加（去重不得误伤同小时的合法多次上报）
	batch.BatchID = "batch-bbb"
	if _, err := svc.Save(batch, 1); err != nil {
		t.Fatalf("Save(新批次): %v", err)
	}
	if up, down, _ = svc.UserBilled(u.ID); up != 200 || down != 400 {
		t.Fatalf("新批次应累加 = %d/%d, want 200/400", up, down)
	}

	// 去重记录落库（供运维核对），且按服务器维度隔离
	var n int64
	if err := db.Model(&models.TrafficBatch{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("去重记录 = %d 条, want 2（重发不新增记录）", n)
	}
}

// TestSaveBatchDedupScopedByServer 去重键含服务器：不同服务器用同一批次号不互相吞掉。
func TestSaveBatchDedupScopedByServer(t *testing.T) {
	db := cycleTestDB(t)
	u := cycleTestUser(t, db, "srv", 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	svc := &TrafficService{DB: db}
	p := protocol.TrafficReportPayload{
		Period:  "2026-09-20T00:00:00Z",
		BatchID: "same-id",
		Entries: []protocol.TrafficEntry{{UserID: u.ID, UpBytes: 10}},
	}
	if _, err := svc.Save(p, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(p, 2); err != nil { // 另一台服务器
		t.Fatal(err)
	}
	if up, _, _ := svc.UserBilled(u.ID); up != 20 {
		t.Fatalf("不同服务器的同名批次应各自入账，实际 %d, want 20", up)
	}
}

// TestSaveWithoutBatchIDStillWorks 旧 agent 不带批次号：不做去重（维持旧行为），
// 同一帧投递两次仍累加——这是有意的兼容边界，不是缺陷。
func TestSaveWithoutBatchIDStillWorks(t *testing.T) {
	db := cycleTestDB(t)
	u := cycleTestUser(t, db, "nobatch", 1, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	svc := &TrafficService{DB: db}
	p := protocol.TrafficReportPayload{
		Period:  "2026-09-20T00:00:00Z",
		Entries: []protocol.TrafficEntry{{UserID: u.ID, UpBytes: 100}},
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.Save(p, 1); err != nil {
			t.Fatal(err)
		}
	}
	if up, _, _ := svc.UserBilled(u.ID); up != 200 {
		t.Fatalf("无批次号时不参与去重（旧行为），实际 %d, want 200", up)
	}
}

// TestSaveCycleAttribution 审计 F3 回归：归属按账期 ID，不按上报时刻。
//
// 场景：用户在 10:30 续费（账期 1 → 2）。续费前已产生但续费后才送达的增量带 cycle_id=1，
// 必须留在旧账期（旧套餐已付费，不该占新套餐额度）；续费后产生的带 cycle_id=2 才进新周期。
// 旧实现按小时桶归属，这批迟到增量会落进新周期（实测新周期用量 300，正确值 0）。
func TestSaveCycleAttribution(t *testing.T) {
	db := cycleTestDB(t)
	aligned := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	u := cycleTestUser(t, db, "cyc", 2, aligned) // 续费后：账期 2、起点 10:00
	svc := &TrafficService{DB: db}

	// 续费前产生、续费后抵达的旧账期增量（桶 = 周期起点桶，旧实现下会算进新周期）
	if _, err := svc.Save(protocol.TrafficReportPayload{
		Period:  "2026-09-20T10:20:00Z",
		BatchID: "old-cycle",
		Entries: []protocol.TrafficEntry{{UserID: u.ID, UpBytes: 300, CycleID: 1}},
	}, 1); err != nil {
		t.Fatal(err)
	}
	up, _, err := svc.UserBilled(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if up != 0 {
		t.Fatalf("旧账期迟到增量不得计入新周期，实际 %d, want 0", up)
	}

	// 续费后产生的增量（新账期）→ 计入
	if _, err := svc.Save(protocol.TrafficReportPayload{
		Period:  "2026-09-20T10:40:00Z",
		BatchID: "new-cycle",
		Entries: []protocol.TrafficEntry{{UserID: u.ID, UpBytes: 500, CycleID: 2}},
	}, 1); err != nil {
		t.Fatal(err)
	}
	if up, _, _ = svc.UserBilled(u.ID); up != 500 {
		t.Fatalf("新账期增量应计入，实际 %d, want 500", up)
	}

	// 旧账期流水完整留痕（审计 §5「重置后的可追溯性」诉求：不再被清零抹掉）
	var oldRow models.TrafficLog
	if err := db.Where("user_id = ? AND cycle_id = ?", u.ID, 1).First(&oldRow).Error; err != nil {
		t.Fatalf("旧账期流水应保留: %v", err)
	}
	if oldRow.UpBytes != 300 || oldRow.BilledUp != 300 {
		t.Fatalf("旧账期流水被改写: up=%d billed=%d, want 300/300", oldRow.UpBytes, oldRow.BilledUp)
	}
	// 同小时桶内两个账期各占一行（唯一索引含 cycle_id），互不合并
	var n int64
	if err := db.Model(&models.TrafficLog{}).Where("user_id = ?", u.ID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("同小时跨账期应有 2 行，实际 %d", n)
	}
}

// TestSaveCycleClampedToCurrent 节点上报大于当前账期的 ID（异常/伪造）→ 收敛到当前账期，
// 避免流量被挂到不存在的账期上而永久不计费。
func TestSaveCycleClampedToCurrent(t *testing.T) {
	db := cycleTestDB(t)
	u := cycleTestUser(t, db, "clamp", 5, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	svc := &TrafficService{DB: db}
	if _, err := svc.Save(protocol.TrafficReportPayload{
		Period:  "2026-09-20T00:00:00Z",
		BatchID: "future",
		Entries: []protocol.TrafficEntry{{UserID: u.ID, UpBytes: 42, CycleID: 99}},
	}, 1); err != nil {
		t.Fatal(err)
	}
	var row models.TrafficLog
	if err := db.Where("user_id = ?", u.ID).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.CycleID != 5 {
		t.Fatalf("未来账期应收敛到当前账期 5，实际 %d", row.CycleID)
	}
	if up, _, _ := svc.UserBilled(u.ID); up != 42 {
		t.Fatalf("收敛后应正常计费，实际 %d, want 42", up)
	}
}

// TestUserBilledLegacyRowsFallback 无账期标记的存量行（cycle_id=0）按时间轴回退归属，
// 引入账期不丢历史用量；带标记的行一律按账期，即使时间早于周期起点。
func TestUserBilledLegacyRowsFallback(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.TrafficLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cycleStart := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	u := cycleTestUser(t, db, "legacy", 3, cycleStart)

	rows := []models.TrafficLog{
		{UserID: u.ID, UpBytes: 100, BilledUp: 100, PeriodStart: cycleStart, CycleID: 0},                 // 存量行、周期内 → 计入
		{UserID: u.ID, UpBytes: 999, BilledUp: 999, PeriodStart: cycleStart.Add(-time.Hour), CycleID: 0}, // 存量行、周期前 → 排除
		{UserID: u.ID, UpBytes: 7, BilledUp: 7, PeriodStart: cycleStart.Add(-time.Hour), CycleID: 3},     // 带标记，即使时间更早也算当前账期
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := &TrafficService{DB: db}
	up, _, err := svc.UserBilled(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if up != 107 {
		t.Fatalf("用量 = %d, want 107（存量行按时间轴 + 带标记行按账期）", up)
	}
}

// TestFindViolatorsCycleSemantics 配额判定与展示同源：旧账期迟到增量不得把新周期推到超额。
func TestFindViolatorsCycleSemantics(t *testing.T) {
	db := cycleTestDB(t)
	aligned := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	u := cycleTestUser(t, db, "viol", 2, aligned)
	if err := db.Model(&models.User{}).Where("id = ?", u.ID).
		Update("plan_traffic_bytes", int64(1000)).Error; err != nil {
		t.Fatal(err)
	}
	svc := &TrafficService{DB: db}

	// 旧账期迟到 5000 字节（远超额度）：不该判超额
	if _, err := svc.Save(protocol.TrafficReportPayload{
		Period:  "2026-09-20T10:10:00Z",
		BatchID: "viol-old",
		Entries: []protocol.TrafficEntry{{UserID: u.ID, UpBytes: 5000, CycleID: 1}},
	}, 1); err != nil {
		t.Fatal(err)
	}
	violators, err := svc.FindViolators([]uint64{u.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(violators) != 0 {
		t.Fatalf("旧账期流量不得触发新周期超额，实际 %v", violators)
	}

	// 新账期用满额度 → 判超额
	if _, err := svc.Save(protocol.TrafficReportPayload{
		Period:  "2026-09-20T10:50:00Z",
		BatchID: "viol-new",
		Entries: []protocol.TrafficEntry{{UserID: u.ID, UpBytes: 1000, CycleID: 2}},
	}, 1); err != nil {
		t.Fatal(err)
	}
	violators, err = svc.FindViolators([]uint64{u.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(violators) != 1 || violators[0] != u.ID {
		t.Fatalf("新账期用满额度应判超额，实际 %v", violators)
	}
}

// TestSaveLateOldCycleBatchInLaterHour 旧账期批次落在「切换之后的小时」时仍归旧账期。
//
// 这是节点补发积压的真实形态：Period 取**发送时刻**（agent 侧 `time.Now()`），而条目的
// CycleID 是采集时刻打的标，因此离线积压的旧账期批次会在切换之后才送达、落进更晚的小时桶。
// 该小时不在 loadCycleStartProtection 的保护范围内，且与当前账期的行同处一桶——压缩必须
// 保留 cycle_id 才能不把旧账期消费并进新账期（见 TestCompactTrafficHourKeepsCycleAttribution）。
func TestSaveLateOldCycleBatchInLaterHour(t *testing.T) {
	db := cycleTestDB(t)
	aligned := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	u := cycleTestUser(t, db, "late", 2, aligned) // 账期 2、起点 10:00
	svc := &TrafficService{DB: db}

	// 旧账期（1）的积压批次：12:30 才送达，比切换晚两个多小时
	if _, err := svc.Save(protocol.TrafficReportPayload{
		Period:  "2026-09-20T12:30:00Z",
		BatchID: "late-old-cycle",
		Entries: []protocol.TrafficEntry{{UserID: u.ID, UpBytes: 5000, CycleID: 1}},
	}, 1); err != nil {
		t.Fatal(err)
	}
	if up, _, err := svc.UserBilled(u.ID); err != nil {
		t.Fatal(err)
	} else if up != 0 {
		t.Fatalf("切换后送达的旧账期增量不得计入新周期，实际 %d, want 0", up)
	}

	// 同一小时桶内的新账期增量
	if _, err := svc.Save(protocol.TrafficReportPayload{
		Period:  "2026-09-20T12:40:00Z",
		BatchID: "late-new-cycle",
		Entries: []protocol.TrafficEntry{{UserID: u.ID, UpBytes: 100, CycleID: 2}},
	}, 1); err != nil {
		t.Fatal(err)
	}
	if up, _, _ := svc.UserBilled(u.ID); up != 100 {
		t.Fatalf("新账期增量应计入，实际 %d, want 100", up)
	}

	// 两行必须同处一个整点桶且账期标记各自保留——压缩按 (user, inbound, cycle) 分组的前提
	var rows []models.TrafficLog
	if err := db.Where("user_id = ?", u.ID).Order("cycle_id ASC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("应有两条流水，实际 %d", len(rows))
	}
	h := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	if !rows[0].PeriodStart.Equal(h) || !rows[1].PeriodStart.Equal(h) {
		t.Fatalf("两条流水应落在同一小时桶 %s，实际 %s / %s", h, rows[0].PeriodStart, rows[1].PeriodStart)
	}
	if rows[0].CycleID != 1 || rows[1].CycleID != 2 {
		t.Fatalf("账期标记 = %d/%d, want 1/2", rows[0].CycleID, rows[1].CycleID)
	}
}
