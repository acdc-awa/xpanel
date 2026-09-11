package services

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
)

func TestResetPeriodKey(t *testing.T) {
	thursday := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC) // Thursday
	monday := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	sunday := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		now    time.Time
		policy string
		want   string
	}{
		{thursday, "daily", "2026-08-13"},
		{thursday, "weekly", "2026-08-10"},
		{monday, "weekly", "2026-08-10"},
		{sunday, "weekly", "2026-08-10"},
		{thursday, "monthly", "2026-08-01"},
		{thursday, "never", ""},
	}
	for _, c := range cases {
		if got := resetPeriodKey(c.now, c.policy); got != c.want {
			t.Errorf("resetPeriodKey(%v, %q) = %q, want %q", c.now, c.policy, got, c.want)
		}
	}
}

func TestRetentionPolicyDeletesOnlyExpired(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.TrafficLog{}, &models.NodeReport{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Date(2026, 8, 16, 5, 0, 0, 0, time.UTC)
	s := &TrafficService{DB: db, now: func() time.Time { return now }}

	old := now.AddDate(0, 0, -200)
	new := now.AddDate(0, 0, -1)
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 1, UpBytes: 1, PeriodStart: old}).Error; err != nil {
		t.Fatalf("old log: %v", err)
	}
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 1, UpBytes: 1, PeriodStart: new}).Error; err != nil {
		t.Fatalf("new log: %v", err)
	}
	if err := db.Create(&models.NodeReport{ServerID: 1, ReportedAt: old}).Error; err != nil {
		t.Fatalf("old report: %v", err)
	}
	if err := db.Create(&models.NodeReport{ServerID: 1, ReportedAt: new}).Error; err != nil {
		t.Fatalf("new report: %v", err)
	}
	if err := db.Create(&models.AuditLog{OperatorType: "user", OperatorID: 1, Action: "x", CreatedAt: old}).Error; err != nil {
		t.Fatalf("old audit: %v", err)
	}
	if err := db.Create(&models.AuditLog{OperatorType: "user", OperatorID: 1, Action: "x", CreatedAt: new}).Error; err != nil {
		t.Fatalf("new audit: %v", err)
	}

	s.runRetention()

	var logs, reports, audits int64
	db.Model(&models.TrafficLog{}).Count(&logs)
	db.Model(&models.NodeReport{}).Count(&reports)
	db.Model(&models.AuditLog{}).Count(&audits)
	if logs != 1 || reports != 1 || audits != 1 {
		t.Fatalf("retention counts = logs:%d reports:%d audits:%d, want all 1", logs, reports, audits)
	}
}

func TestAggDailyOnlyRecentWindow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.TrafficLog{}, &models.TrafficDaily{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Date(2026, 8, 16, 5, 0, 0, 0, time.UTC)
	s := &TrafficService{DB: db, now: func() time.Time { return now }}

	old := now.AddDate(0, 0, -30)
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 1, UpBytes: 100, PeriodStart: old}).Error; err != nil {
		t.Fatalf("old log: %v", err)
	}
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 1, UpBytes: 200, PeriodStart: now.Add(-time.Hour)}).Error; err != nil {
		t.Fatalf("recent log: %v", err)
	}

	s.AggDaily()

	var dailies []models.TrafficDaily
	db.Find(&dailies)
	if len(dailies) != 1 {
		t.Fatalf("dailies = %d, want 1（窗口外旧数据不扫描）", len(dailies))
	}
	if dailies[0].UpBytes != 200 {
		t.Fatalf("recent daily up = %d, want 200", dailies[0].UpBytes)
	}
}

func TestResetInboundTrafficOncePerPeriod(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	inb := models.Inbound{ServerID: 1, Tag: "in", Protocol: "vless", Port: 443, Up: 100, Down: 200, TrafficReset: "daily"}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	s := &TrafficService{DB: db}
	s.resetInboundTraffic()
	db.First(&inb, inb.ID)
	if inb.Up != 0 || inb.Down != 0 {
		t.Fatalf("first tick should reset, got up=%d down=%d", inb.Up, inb.Down)
	}
	if inb.LastResetDate == "" {
		t.Fatal("first tick should record last_reset_date")
	}

	// 周期内新产生的流量不应被第二个 tick 清零（ISSUE-05 回归）
	db.Model(&inb).Updates(map[string]any{"up": 50, "down": 60})
	s.resetInboundTraffic()
	db.First(&inb, inb.ID)
	if inb.Up != 50 || inb.Down != 60 {
		t.Fatalf("second tick in same period should not reset, got up=%d down=%d", inb.Up, inb.Down)
	}
}

// TestSaveDuplicateDeliveryMergesAndBumpsInbound 同 (user, inbound, period) 重复投递：
// TrafficLog 合并为一行且字节累加，inbounds 每次投递都补计（P1-1）。
func TestSaveDuplicateDeliveryMergesAndBumpsInbound(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}, &models.User{}, &models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	inb := models.Inbound{ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TrafficService{DB: db}
	payload := protocol.TrafficReportPayload{
		Period: "2026-08-24T00:00:00Z",
		Entries: []protocol.TrafficEntry{
			{UserID: 42, Inbound: "vless-in", UpBytes: 100, DownBytes: 200},
		},
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.Save(payload, 1); err != nil {
			t.Fatalf("Save #%d: %v", i+1, err)
		}
	}

	var logs []models.TrafficLog
	if err := db.Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("TrafficLog 行数 = %d, want 1（重复投递应合并）", len(logs))
	}
	if logs[0].UpBytes != 200 || logs[0].DownBytes != 400 {
		t.Fatalf("合并后 up=%d down=%d, want 200/400", logs[0].UpBytes, logs[0].DownBytes)
	}
	if err := db.First(&inb, inb.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inb.Up != 200 || inb.Down != 400 {
		t.Fatalf("inbounds 补计 up=%d down=%d, want 200/400", inb.Up, inb.Down)
	}
}

// TestSaveInboundDimensionEntryOnlyBumpsInbound 入站维度条目（Email 恒空、Inbound=tag，
// agent 从 inbound>>> 计数器派生）：仅累计 inbounds.up/down，不落 traffic_logs
// （流水严格用户维度，防今日流量 KPI 双计）；未知 tag 与无从归属条目安全跳过。
func TestSaveInboundDimensionEntryOnlyBumpsInbound(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}, &models.User{}, &models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	inb := models.Inbound{ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TrafficService{DB: db}
	payload := protocol.TrafficReportPayload{
		Period: "2026-09-01T00:00:00Z",
		Entries: []protocol.TrafficEntry{
			{Inbound: "vless-in", UpBytes: 500, DownBytes: 700},  // 已知 tag：入账
			{Inbound: "ghost-tag", UpBytes: 100, DownBytes: 100}, // 未知 tag：跳过
			{UpBytes: 50, DownBytes: 50},                         // 无从归属：跳过
		},
	}
	if _, err := svc.Save(payload, 1); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var logs int64
	db.Model(&models.TrafficLog{}).Count(&logs)
	if logs != 0 {
		t.Fatalf("入站维度条目不得落 traffic_logs，行数 = %d", logs)
	}
	if err := db.First(&inb, inb.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inb.Up != 500 || inb.Down != 700 {
		t.Fatalf("inbounds 计数 up=%d down=%d, want 500/700", inb.Up, inb.Down)
	}
}

// TestSaveConcurrentDuplicateDeliveryMerges 并发双投同 (user, inbound, period)：
// upsert 合并为一行、全部字节累加、inbounds 补计齐全（P1-1 并发路径）。
func TestSaveConcurrentDuplicateDeliveryMerges(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared&_pragma=busy_timeout(5000)"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}, &models.User{}, &models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	inb := models.Inbound{ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TrafficService{DB: db}
	payload := protocol.TrafficReportPayload{
		Period: "2026-08-24T00:00:00Z",
		Entries: []protocol.TrafficEntry{
			{UserID: 42, Inbound: "vless-in", UpBytes: 100, DownBytes: 200},
		},
	}
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = svc.Save(payload, 1)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("并发 Save #%d: %v", i, err)
		}
	}

	var logs []models.TrafficLog
	if err := db.Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("TrafficLog 行数 = %d, want 1（并发双投应合并为一行）", len(logs))
	}
	if logs[0].UpBytes != 100*n || logs[0].DownBytes != 200*n {
		t.Fatalf("并发合并后 up=%d down=%d, want %d/%d", logs[0].UpBytes, logs[0].DownBytes, 100*n, 200*n)
	}
	if err := db.First(&inb, inb.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inb.Up != 100*n || inb.Down != 200*n {
		t.Fatalf("inbounds 并发补计 up=%d down=%d, want %d/%d", inb.Up, inb.Down, 100*n, 200*n)
	}
}

// TestFindViolators 事件驱动处置的判定口径（与 filterValidUsers 快照语义严格一致）：
// 跨阈值/未跨/已过期/无额度（不限）/非活跃 五态。
func TestFindViolators(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.TrafficLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	gb := int64(1024 * 1024 * 1024)
	expired := time.Now().Add(-24 * time.Hour)
	users := []models.User{
		{Username: "over", Email: "over@t.com", UUID: "11111111-1111-1111-1111-111111111111", SubscribeToken: "t1", Status: models.StatusActive, PlanTrafficBytes: 1 * gb},
		{Username: "under", Email: "under@t.com", UUID: "22222222-2222-2222-2222-222222222222", SubscribeToken: "t2", Status: models.StatusActive, PlanTrafficBytes: 10 * gb},
		{Username: "expired", Email: "expired@t.com", UUID: "33333333-3333-3333-3333-333333333333", SubscribeToken: "t3", Status: models.StatusActive, ExpireAt: &expired},
		{Username: "unlimited", Email: "unlimited@t.com", UUID: "44444444-4444-4444-4444-444444444444", SubscribeToken: "t4", Status: models.StatusActive},
		{Username: "disabled", Email: "disabled@t.com", UUID: "55555555-5555-5555-5555-555555555555", SubscribeToken: "t5", Status: models.StatusDisabled, PlanTrafficBytes: gb},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	// over 用 2GB（跨阈值）；under 用 0.5GB（未跨）；unlimited 用 100GB 但无额度快照=不限
	// （判定读计费口径 billed 两列，2026-09-06 倍率计费）
	logs := []models.TrafficLog{
		{UserID: users[0].ID, UpBytes: 2 * gb, BilledUp: 2 * gb, DownBytes: 0, PeriodStart: time.Now()},
		{UserID: users[1].ID, UpBytes: gb / 2, BilledUp: gb / 2, DownBytes: 0, PeriodStart: time.Now()},
		{UserID: users[3].ID, UpBytes: 100 * gb, BilledUp: 100 * gb, DownBytes: 0, PeriodStart: time.Now()},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TrafficService{DB: db}
	got, err := svc.FindViolators([]uint64{users[0].ID, users[1].ID, users[2].ID, users[3].ID, users[4].ID})
	if err != nil {
		t.Fatalf("FindViolators: %v", err)
	}
	want := map[uint64]bool{users[0].ID: true, users[2].ID: true}
	if len(got) != len(want) {
		t.Fatalf("violators = %v, want 仅 over+expired (%v)", got, want)
	}
	for _, id := range got {
		if !want[id] {
			t.Errorf("不应判违规: user=%d（got=%v）", id, got)
		}
	}
}

// TestSaveReturnsReportedUserIDs Save 返回本帧实际计入的用户 ID 去重集合
// （入站维度条目/零字节条目/未知用户不计入）。
func TestSaveReturnsReportedUserIDs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}, &models.User{}, &models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	inb := models.Inbound{ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TrafficService{DB: db}
	payload := protocol.TrafficReportPayload{
		Period: "2026-09-01T00:00:00Z",
		Entries: []protocol.TrafficEntry{
			{UserID: 7, Inbound: "vless-in", UpBytes: 100, DownBytes: 0},
			{UserID: 7, Inbound: "vless-in", UpBytes: 0, DownBytes: 50},
			{UserID: 8, Inbound: "vless-in", UpBytes: 10, DownBytes: 10},
			{UserID: 9, Inbound: "vless-in", UpBytes: 0, DownBytes: 0}, // 零字节：不计入
			{Inbound: "vless-in", UpBytes: 1, DownBytes: 1},            // 入站维度：不计入
		},
	}
	ids, err := svc.Save(payload, 1)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("reported ids = %v, want [7 8] 两个用户", ids)
	}
	seen := map[uint64]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	if !seen[7] || !seen[8] {
		t.Fatalf("reported ids = %v, want 含 7 与 8", ids)
	}
}

// TestSaveBillingRatioAppliesInboundRatio 倍率计费（2026-09-06）：
// 用户维度条目落库时按 (用户生效组, 服务器) 生效入站倍率折算计费口径 billed 两列，
// 原始字节恒不变；xray 用户计数器无入站维度（user>>>email 节点级汇总），
// 同服务器多入站倍率不一致取最高（对运营保守）；ratio=0 = 免费（billed=0）；
// 无命中（组未被任何入站授权）回退 1；重复投递合并时 billed 同步累加；
// 入站维度条目与 inbounds.up/down 冗余计数恒为原始字节（展示/容量口径，不乘倍率）。
func TestSaveBillingRatioAppliesInboundRatio(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}, &models.User{}, &models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	mkInbound := func(serverID uint64, tag string, ratio float64) models.Inbound {
		inb := models.Inbound{ServerID: serverID, Tag: tag, Protocol: "vless", Port: 443, Ratio: ratio, Type: models.InboundTypeUser}
		if err := db.Create(&inb).Error; err != nil {
			t.Fatalf("inbound %s: %v", tag, err)
		}
		return inb
	}
	mkAP := func(inbID uint64, groups ...uint64) {
		ap := models.UserAccessPoint{Name: "ap", TargetType: "inbound", TargetInboundID: &inbID, Enabled: true}
		if err := db.Create(&ap).Error; err != nil {
			t.Fatal(err)
		}
		for _, g := range groups {
			if err := db.Create(&models.PermissionGroupAccessPoint{PermissionGroupID: g, AccessPointID: ap.ID}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	// 服务器 1：入站 A 倍率 1.5 + 入站 B 倍率 1.0（均授权组 2）→ 混合倍率取最高 1.5
	inbA := mkInbound(1, "in-a", 1.5)
	inbB := mkInbound(1, "in-b", 1.0)
	mkAP(inbA.ID, 2)
	mkAP(inbB.ID, 2)
	// 服务器 2：入站 F 免费倍率 0（授权组 2）→ billed = 0
	// （Ratio:0 走 Create 会被 default:1 零值陷阱吞成 1——与创建端点同样的坑，测试改走显式 Update）
	inbF := mkInbound(2, "in-free", 0)
	if err := db.Model(&models.Inbound{}).Where("id = ?", inbF.ID).Update("ratio", 0).Error; err != nil {
		t.Fatal(err)
	}
	mkAP(inbF.ID, 2)

	mkUser := func(email string, group uint64) models.User {
		u := models.User{Username: email, Email: email, UUID: "uuid-" + email, SubscribeToken: email, Status: models.StatusActive, PermissionGroupID: group}
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		return u
	}
	uMixed := mkUser("mixed@t.com", 2)   // 命中 1.5/1.0 双入站 → 1.5
	uNoAuth := mkUser("noauth@t.com", 9) // 组 9 无任何入站授权 → 回退 1
	// BeforeCreate 把周期起点设为创建时刻（晚于固定上报周期），拨早使周期过滤放行
	if err := db.Model(&models.User{}).Where("1 = 1").Update("traffic_cycle_start", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TrafficService{DB: db}
	// 同帧双投（重复投递合并）：混合用户 ×1.5，无授权用户 ×1
	payload := protocol.TrafficReportPayload{
		Period: "2026-09-06T00:00:00Z",
		Entries: []protocol.TrafficEntry{
			{UserID: uMixed.ID, Inbound: "in-a", UpBytes: 100, DownBytes: 200},
			{UserID: uNoAuth.ID, Inbound: "in-a", UpBytes: 100, DownBytes: 200},
			{Inbound: "in-a", UpBytes: 999, DownBytes: 0}, // 入站维度：不落流水、不乘倍率
		},
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.Save(payload, 1); err != nil {
			t.Fatalf("Save #%d: %v", i+1, err)
		}
	}

	var logs []models.TrafficLog
	if err := db.Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("TrafficLog 行数 = %d, want 2（入站维度不落流水）", len(logs))
	}
	for _, l := range logs {
		switch l.UserID {
		case uMixed.ID:
			// 原始口径不变（2 投合并 200/400）；计费口径 ×1.5（300/600）
			if l.UpBytes != 200 || l.DownBytes != 400 {
				t.Fatalf("mixed 原始字节 up=%d down=%d, want 200/400", l.UpBytes, l.DownBytes)
			}
			if l.BilledUp != 300 || l.BilledDown != 600 {
				t.Fatalf("mixed 计费字节 billed up=%d down=%d, want 300/600（倍率 1.5）", l.BilledUp, l.BilledDown)
			}
		case uNoAuth.ID:
			if l.BilledUp != 200 || l.BilledDown != 400 {
				t.Fatalf("noauth 计费字节 billed up=%d down=%d, want 200/400（无命中回退 1）", l.BilledUp, l.BilledDown)
			}
		default:
			t.Fatalf("意外用户 %d", l.UserID)
		}
	}

	// 免费入站（ratio=0）：billed = 0，原始照记
	if _, err := svc.Save(protocol.TrafficReportPayload{
		Period:  "2026-09-06T00:00:00Z",
		Entries: []protocol.TrafficEntry{{UserID: uMixed.ID, Inbound: "in-free", UpBytes: 500, DownBytes: 0}},
	}, 2); err != nil {
		t.Fatalf("Save free: %v", err)
	}
	var freeLog models.TrafficLog
	if err := db.Where("user_id = ? AND inbound_id = ?", uMixed.ID, inbF.ID).First(&freeLog).Error; err != nil {
		t.Fatal(err)
	}
	if freeLog.UpBytes != 500 || freeLog.BilledUp != 0 {
		t.Fatalf("免费入站 raw=%d billed=%d, want raw 500 / billed 0", freeLog.UpBytes, freeLog.BilledUp)
	}

	// 入站冗余计数恒为原始字节（不乘倍率）
	var inbAAfter models.Inbound
	if err := db.First(&inbAAfter, inbA.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inbAAfter.Up != 2398 || inbAAfter.Down != 800 {
		t.Fatalf("inbA.up/down = %d/%d, want 原始 2398/800（每用户条目各补计一次×2 投 + 入站维度）", inbAAfter.Up, inbAAfter.Down)
	}

	// 原始口径恒不变（直接 SQL 聚合 up/down 两列验证）vs 计费口径 UserBilled
	var rawRow struct {
		Up, Down int64
	}
	if err := db.Model(&models.TrafficLog{}).Where("user_id = ?", uMixed.ID).
		Select("COALESCE(SUM(up_bytes),0) AS up, COALESCE(SUM(down_bytes),0) AS down").Scan(&rawRow).Error; err != nil {
		t.Fatal(err)
	}
	if rawRow.Up != 200+500 || rawRow.Down != 400 {
		t.Fatalf("原始口径 = %d/%d, want 700/400（含免费入站，不乘倍率）", rawRow.Up, rawRow.Down)
	}
	billUp, billDown, err := svc.UserBilled(uMixed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if billUp != 300 || billDown != 600 {
		t.Fatalf("UserBilled = %d/%d, want 300/600（计费口径，免费入站不贡献）", billUp, billDown)
	}
}

// period1AsTime 上报周期字符串 → time.Time（与落库存储口径一致）。
func period1AsTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// TestSaveStatsKeyInboundAttribution 统计键注入入站维度（2026-09-06）：
// 新格式 u<uid>.i<iid>@panel.local 条目反解出入站后，流水挂真实 inbound_id、
// 按该入站精确倍率计费（混合倍率服务器不再一律取 max）；iid 未命中本服务器入站
// （已删/跨服异常）回退 inbound_id=0 + 组内 max 兜底；旧格式 user-<id>@panel.local
// 同兜底；统计键条目不补计入站冗余计数（inbound>>> 计数器已记账，防双计）。
func TestSaveStatsKeyInboundAttribution(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}, &models.User{}, &models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	mkInbound := func(serverID uint64, tag string, ratio float64) models.Inbound {
		inb := models.Inbound{ServerID: serverID, Tag: tag, Protocol: "vless", Port: 443, Ratio: ratio, Type: models.InboundTypeUser}
		if err := db.Create(&inb).Error; err != nil {
			t.Fatalf("inbound %s: %v", tag, err)
		}
		return inb
	}
	mkAP := func(inbID uint64, groups ...uint64) {
		ap := models.UserAccessPoint{Name: "ap", TargetType: "inbound", TargetInboundID: &inbID, Enabled: true}
		if err := db.Create(&ap).Error; err != nil {
			t.Fatal(err)
		}
		for _, g := range groups {
			if err := db.Create(&models.PermissionGroupAccessPoint{PermissionGroupID: g, AccessPointID: ap.ID}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	// 同服务器混合倍率：in-a 1.5 + in-b 1.0（均授权组 2）
	inbA := mkInbound(1, "in-a", 1.5)
	inbB := mkInbound(1, "in-b", 1.0)
	mkAP(inbA.ID, 2)
	mkAP(inbB.ID, 2)

	user := models.User{Username: "mix@t.com", Email: "mix@t.com", UUID: "11111111-1111-1111-1111-111111111111", SubscribeToken: "t1", Status: models.StatusActive, PermissionGroupID: 2}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.User{}).Where("1 = 1").Update("traffic_cycle_start", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).Error; err != nil {
		t.Fatal(err)
	}

	svc := &TrafficService{DB: db}
	// T1：经 in-a（×1.5）与 in-b（×1.0）各 100/200，另带一条 in-a 入站维度条目
	period1 := "2026-09-06T00:00:00Z"
	payload := protocol.TrafficReportPayload{
		Period: period1,
		Entries: []protocol.TrafficEntry{
			{Email: xray.UserEmailFor(&user, inbA.ID), UpBytes: 100, DownBytes: 200},
			{Email: xray.UserEmailFor(&user, inbB.ID), UpBytes: 100, DownBytes: 200},
			{Inbound: "in-a", UpBytes: 999},
		},
	}
	if _, err := svc.Save(payload, 1); err != nil {
		t.Fatalf("Save T1: %v", err)
	}

	var logs []models.TrafficLog
	if err := db.Where("period_start = ?", period1AsTime(t, period1)).Order("inbound_id").Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("TrafficLog 行数 = %d, want 2（同用户按入站拆行）", len(logs))
	}
	for _, l := range logs {
		switch l.InboundID {
		case inbA.ID: // 精确倍率 1.5
			if l.BilledUp != 150 || l.BilledDown != 300 {
				t.Fatalf("in-a billed = %d/%d, want 150/300（精确倍率 1.5，非组内 max 混同）", l.BilledUp, l.BilledDown)
			}
		case inbB.ID: // 精确倍率 1.0
			if l.BilledUp != 100 || l.BilledDown != 200 {
				t.Fatalf("in-b billed = %d/%d, want 100/200（精确倍率 1.0）", l.BilledUp, l.BilledDown)
			}
		default:
			t.Fatalf("意外 inbound_id %d", l.InboundID)
		}
	}

	// T2：iid 未命中（入站已删/跨服异常）→ inbound_id=0、组内 max 兜底 ×1.5
	if _, err := svc.Save(protocol.TrafficReportPayload{
		Period:  "2026-09-06T01:00:00Z",
		Entries: []protocol.TrafficEntry{{Email: xray.UserEmailFor(&user, 99999), UpBytes: 10}},
	}, 1); err != nil {
		t.Fatalf("Save T2: %v", err)
	}
	// T3：旧格式（升级过渡期存量节点）→ inbound_id=0、组内 max 兜底 ×1.5
	if _, err := svc.Save(protocol.TrafficReportPayload{
		Period:  "2026-09-06T02:00:00Z",
		Entries: []protocol.TrafficEntry{{Email: "user-" + strconv.FormatUint(user.ID, 10) + "@panel.local", UpBytes: 10}},
	}, 1); err != nil {
		t.Fatalf("Save T3: %v", err)
	}
	var fb []models.TrafficLog
	if err := db.Where("user_id = ? AND inbound_id = 0", user.ID).Order("period_start").Find(&fb).Error; err != nil {
		t.Fatal(err)
	}
	if len(fb) != 2 {
		t.Fatalf("兜底行数 = %d, want 2（T2/T3 各一行）", len(fb))
	}
	for _, l := range fb {
		if l.BilledUp != 15 {
			t.Fatalf("兜底行 billed_up = %d, want 15（max 兜底 1.5）", l.BilledUp)
		}
	}

	// 入站冗余计数：仅入站维度条目补计 999；统计键条目不补计（防与 inbound>>> 双计）
	var inbAAfter models.Inbound
	if err := db.First(&inbAAfter, inbA.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inbAAfter.Up != 999 || inbAAfter.Down != 0 {
		t.Fatalf("in-a.up/down = %d/%d, want 999/0（统计键条目不得补计入站计数）", inbAAfter.Up, inbAAfter.Down)
	}
	var inbBAfter models.Inbound
	if err := db.First(&inbBAfter, inbB.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inbBAfter.Up != 0 {
		t.Fatalf("in-b.up = %d, want 0", inbBAfter.Up)
	}
}

// TestSaveBucketsTrafficByHour 写入侧按小时分桶（trafficPeriodBucket）：
// agent 的 Period 是发送时刻，同小时内多次上报必须合并为一行并求和，跨小时分属不同行；
// 计费口径（billed_*）随合并不变。这是抑制 traffic_logs 膨胀的核心行为。
func TestSaveBucketsTrafficByHour(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Inbound{}, &models.TrafficLog{}, &models.User{}, &models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&models.Inbound{ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443, Ratio: 1}).Error; err != nil {
		t.Fatal(err)
	}
	svc := &TrafficService{DB: db}
	send := func(period string, up, down int64) {
		t.Helper()
		if _, err := svc.Save(protocol.TrafficReportPayload{
			Period:  period,
			Entries: []protocol.TrafficEntry{{UserID: 42, Inbound: "vless-in", UpBytes: up, DownBytes: down}},
		}, 1); err != nil {
			t.Fatalf("Save(%s): %v", period, err)
		}
	}
	send("2026-08-24T00:05:00Z", 100, 200) // 同小时
	send("2026-08-24T00:55:00Z", 100, 200) // 同小时
	send("2026-08-24T01:10:00Z", 7, 9)     // 下一小时

	var logs []models.TrafficLog
	if err := db.Order("period_start ASC").Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("TrafficLog 行数 = %d, want 2（同小时合并、跨小时分开）", len(logs))
	}
	hour0 := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	if !logs[0].PeriodStart.UTC().Equal(hour0) {
		t.Fatalf("首行 period_start = %s, want %s（应归一到整点）", logs[0].PeriodStart, hour0)
	}
	if logs[0].UpBytes != 200 || logs[0].DownBytes != 400 {
		t.Fatalf("同小时合并 up=%d down=%d, want 200/400", logs[0].UpBytes, logs[0].DownBytes)
	}
	if logs[0].BilledUp != 200 || logs[0].BilledDown != 400 {
		t.Fatalf("计费口径 billed_up=%d billed_down=%d, want 200/400", logs[0].BilledUp, logs[0].BilledDown)
	}
	if logs[1].UpBytes != 7 || logs[1].DownBytes != 9 {
		t.Fatalf("次小时 up=%d down=%d, want 7/9", logs[1].UpBytes, logs[1].DownBytes)
	}
}
