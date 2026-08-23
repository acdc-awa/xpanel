package services

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.PendingConfig{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() }) // Windows 文件锁：必须在 TempDir 清理前关闭
	return db
}

// TestSavePendingOverwriteRace 复现 pending 覆盖竞态：
// 旧内容 cfgA 推送回执到达时，pending 行已被 cfgB 覆盖，MarkPushedIfSame(cfgA)
// 不得把 cfgB 误标为 pushed（否则节点永远收不到 cfgB）。
func TestSavePendingOverwriteRace(t *testing.T) {
	s := &ConfigService{DB: newTestDB(t)}
	const cfgA = `{"version":1}`
	const cfgB = `{"version":2}`

	if err := s.SavePending(7, cfgA); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetPending(7)
	if err != nil || p == nil {
		t.Fatal("应存在 pending 记录")
	}

	// 并发覆盖：同一行被 SavePending(cfgB) 覆盖（行 ID 不变）
	if err := s.SavePending(7, cfgB); err != nil {
		t.Fatal(err)
	}

	// 旧内容 cfgA 的推送回执到达：不得标记 pushed
	marked, err := s.MarkPushedIfSame(p.ID, cfgA)
	if err != nil {
		t.Fatal(err)
	}
	if marked {
		t.Fatal("cfgA 已被 cfgB 覆盖，MarkPushedIfSame 不应标记成功（否则 cfgB 未下发却被标为已推送）")
	}
	p2, err := s.GetPending(7)
	if err != nil || p2 == nil {
		t.Fatal("应存在 pending 记录")
	}
	if p2.Status != "pending" {
		t.Fatalf("新内容 cfgB 应保持 pending（等待下一轮推送），实际 status=%s", p2.Status)
	}
	if p2.ConfigJSON != cfgB {
		t.Fatalf("pending 内容应为 cfgB，实际 %q", p2.ConfigJSON)
	}

	// 新内容 cfgB 下发成功 → 标记成功
	marked, err = s.MarkPushedIfSame(p2.ID, cfgB)
	if err != nil {
		t.Fatal(err)
	}
	if !marked {
		t.Fatal("内容未变化的正常标记应成功")
	}
	p3, err := s.GetPending(7)
	if err != nil || p3 == nil {
		t.Fatal("应存在 pending 记录")
	}
	if p3.Status != "pushed" {
		t.Fatalf("cfgB 已下发，期望 pushed，实际 status=%s", p3.Status)
	}

	// 已 pushed 后重复标记：不应再次生效（status != pending）
	marked, err = s.MarkPushedIfSame(p3.ID, cfgB)
	if err != nil || marked {
		t.Fatalf("已 pushed 的记录不应重复标记 (marked=%v err=%v)", marked, err)
	}
}

// TestMarkPushedByServerIfSame 按 server 维度的同语义回归测试。
func TestMarkPushedByServerIfSame(t *testing.T) {
	s := &ConfigService{DB: newTestDB(t)}
	const cfgA = `{"v":1}`
	const cfgB = `{"v":2}`

	if err := s.SavePending(9, cfgA); err != nil {
		t.Fatal(err)
	}
	// 覆盖竞态：cfgB 覆盖 cfgA
	if err := s.SavePending(9, cfgB); err != nil {
		t.Fatal(err)
	}
	// 旧内容回执：不得标记
	marked, err := s.MarkPushedByServerIfSame(9, cfgA)
	if err != nil || marked {
		t.Fatalf("覆盖后旧内容不应标记成功 (marked=%v err=%v)", marked, err)
	}
	// 新内容：标记成功
	marked, err = s.MarkPushedByServerIfSame(9, cfgB)
	if err != nil || !marked {
		t.Fatalf("新内容应标记成功 (marked=%v err=%v)", marked, err)
	}
	p, _ := s.GetPending(9)
	if p.Status != "pushed" {
		t.Fatalf("期望 pushed，实际 %s", p.Status)
	}
}

// TestSavePendingOverwriteKeepsRowID SavePending 覆盖必须更新同一行（ID 不变），
// 这是 MarkPushedIfSame 竞态修复依赖的不变量。
func TestSavePendingOverwriteKeepsRowID(t *testing.T) {
	s := &ConfigService{DB: newTestDB(t)}
	if err := s.SavePending(11, `{"v":1}`); err != nil {
		t.Fatal(err)
	}
	p1, err := s.GetPending(11)
	if err != nil || p1 == nil {
		t.Fatal("应存在 pending 记录")
	}
	if err := s.SavePending(11, `{"v":2}`); err != nil {
		t.Fatal(err)
	}
	p2, _ := s.GetPending(11)
	if p1.ID != p2.ID {
		t.Fatalf("覆盖后行 ID 应不变（竞态窗口基于同一行），实际 %d != %d", p1.ID, p2.ID)
	}
	if p2.Status != "pending" {
		t.Fatalf("覆盖后应回到 pending，实际 %s", p2.Status)
	}
	if p2.PushedAt != nil {
		t.Fatal("覆盖后 pushed_at 应清空")
	}
}

// TestGetValidUsers_GroupFilterAndFlow 批7 核心修复验证：GetValidUsers（热更新/全量生成共用）
// 必须按接入点白名单派生过滤用户、按入站流控三态计算 flow、按三级继承计算设备限制。
func TestGetValidUsers_GroupFilterAndFlow(t *testing.T) {
	db := newTestDB(t)
	migrateModels := []any{
		&models.User{}, &models.Plan{}, &models.Inbound{},
		&models.PermissionGroup{}, &models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{},
		&models.L4PortRule{},
	}
	for _, m := range migrateModels {
		if err := db.AutoMigrate(m); err != nil {
			t.Fatal(err)
		}
	}

	// 权限组与套餐
	g1 := models.PermissionGroup{Name: "g1"}
	g2 := models.PermissionGroup{Name: "g2"}
	if err := db.Create(&g1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&g2).Error; err != nil {
		t.Fatal(err)
	}
	planA := models.Plan{Name: "planA", PriceCents: 1000, TrafficGB: 100, DurationDays: 30, DeviceLimit: 3, PermissionGroupID: g2.ID, Enabled: true}
	if err := db.Create(&planA).Error; err != nil {
		t.Fatal(err)
	}

	// 三个入站：inb1→组1（tcp+reality，Flow 空=自动）；inb2→组2（tcp+tls，Flow=none）；
	// inb3→无开放组（Flow=xtls-rprx-vision 显式）
	reality := `{"network":"tcp","security":"reality","realitySettings":{"dest":"1.2.3.4:443","serverNames":["r.example.com"],"privateKey":"sk","shortIds":["abcd"]}}`
	tls := `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"t.example.com"}}`
	inb1 := models.Inbound{ServerID: 1, Tag: "in1", Protocol: "vless", Port: 443, StreamSettings: reality, Flow: "", Type: models.InboundTypeUser, Enabled: true}
	inb2 := models.Inbound{ServerID: 1, Tag: "in2", Protocol: "vless", Port: 8443, StreamSettings: tls, Flow: "none", Type: models.InboundTypeUser, Enabled: true}
	inb3 := models.Inbound{ServerID: 1, Tag: "in3", Protocol: "vless", Port: 9443, Flow: "xtls-rprx-vision", Type: models.InboundTypeUser, Enabled: true}
	if err := db.Create(&inb1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&inb2).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&inb3).Error; err != nil {
		t.Fatal(err)
	}
	// 入站授权由「用户接入点白名单」单点派生：inb1←ap1(组1)；inb2←ap2(组2)；inb3 无接入点指向
	u64 := func(v uint64) *uint64 { return &v }
	ap1 := models.UserAccessPoint{Name: "ap1", Enabled: true, TargetType: "inbound", TargetInboundID: u64(inb1.ID)}
	ap2 := models.UserAccessPoint{Name: "ap2", Enabled: true, TargetType: "inbound", TargetInboundID: u64(inb2.ID)}
	if err := db.Create(&ap1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ap2).Error; err != nil {
		t.Fatal(err)
	}
	_ = db.Create(&models.PermissionGroupAccessPoint{PermissionGroupID: g1.ID, AccessPointID: ap1.ID})
	_ = db.Create(&models.PermissionGroupAccessPoint{PermissionGroupID: g2.ID, AccessPointID: ap2.ID})

	// 用户：u1 显式组1（设备限制 2）；u2 套餐→组2（继承限制 3）；u3 无组；u4 已过期
	expired := time.Now().Add(-24 * time.Hour)
	users := []models.User{
		{Username: "u1", Email: "u1@t.com", UUID: "11111111-1111-1111-1111-111111111111", SubscribeToken: "t1", Status: models.StatusActive, PermissionGroupID: g1.ID, DeviceLimit: 2},
		{Username: "u2", Email: "u2@t.com", UUID: "22222222-2222-2222-2222-222222222222", SubscribeToken: "t2", Status: models.StatusActive, PlanID: planA.ID},
		{Username: "u3", Email: "u3@t.com", UUID: "33333333-3333-3333-3333-333333333333", SubscribeToken: "t3", Status: models.StatusActive},
		{Username: "u4", Email: "u4@t.com", UUID: "44444444-4444-4444-4444-444444444444", SubscribeToken: "t4", Status: models.StatusActive, PermissionGroupID: g1.ID, ExpireAt: &expired},
	}
	for i := range users {
		if err := db.Create(&users[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	s := &ConfigService{DB: db}
	m, err := s.GetValidUsers(1)
	if err != nil {
		t.Fatal(err)
	}

	// in1：仅 u1，自动 vision，限制 2
	got := m["in1"]
	if len(got) != 1 || got[0].UUID != users[0].UUID {
		t.Fatalf("in1 users = %+v, want only u1", got)
	}
	if got[0].Flow != "xtls-rprx-vision" {
		t.Errorf("in1 flow = %q, want 自动 vision", got[0].Flow)
	}
	if got[0].Limit != 2 {
		t.Errorf("in1 limit = %d, want 2（用户级）", got[0].Limit)
	}

	// in2：仅 u2，none 禁自动 → flow 空，限制继承套餐 3
	got = m["in2"]
	if len(got) != 1 || got[0].UUID != users[1].UUID {
		t.Fatalf("in2 users = %+v, want only u2", got)
	}
	if got[0].Flow != "" {
		t.Errorf("in2 flow = %q, want 空（none 禁自动）", got[0].Flow)
	}
	if got[0].Limit != 3 {
		t.Errorf("in2 limit = %d, want 3（套餐继承）", got[0].Limit)
	}

	// in3：无接入点指向 → 默认不对任何人开放（0 个用户）
	got = m["in3"]
	if len(got) != 0 {
		t.Fatalf("in3 users = %d, want 0（无接入点指向不对任何人开放）", len(got))
	}

	// u3（无组）与 u4（过期）不得出现在任何入站
	total := len(m["in1"]) + len(m["in2"]) + len(m["in3"])
	if total != 2 {
		t.Errorf("用户总数 = %d, want 2（u1×1 + u2×1）", total)
	}
	for _, u := range append(append(m["in1"], m["in2"]...), m["in3"]...) {
		if u.UUID == users[2].UUID || u.UUID == users[3].UUID {
			t.Errorf("无组/过期用户不应注入: %s", u.UUID)
		}
	}
}
