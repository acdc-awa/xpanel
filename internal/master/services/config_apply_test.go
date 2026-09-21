package services

// 回归用例（2026-09-21）：配置下发的「已生效结构」账本与推送前重算。
// 背景（docs/architecture/配置下发-热更冷更与状态机.md）：
//   - 不变量 I3：热更只按已生效结构 S_a 的 tag 下发（否则 handler not found 整批中断）；
//   - 不变量 I2：SYNCED 时热更顺带把整份配置带给节点落盘（磁盘 = 运行中配置的快照）；
//   - P1b：待推内容冻结于 SavePending 那一刻，推送前必须现场重算，否则已删用户会被冷推复活。

import (
	"strings"
	"testing"
	"time"

	"github.com/acdc-awa/xpanel/internal/models"
)

// TestGetValidUsersFiltersUnavailableInbounds 批次 G：热更与冷更的入站集合同源。
// 跑满 / 过期的入站必须同时从 GetValidUsers（热更）与 Generate（冷更）里消失，否则热更
// 会带着一个运行中 xray 里不存在的 tag 下发 → handler not found → 整批同步中断。
func TestGetValidUsersFiltersUnavailableInbounds(t *testing.T) {
	db := newTestDB(t)
	for _, m := range []any{&models.Server{}, &models.Inbound{}, &models.User{}, &models.TrafficLog{},
		&models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{},
		&models.ServerOutbound{}, &models.ServerRoutingRule{}} {
		if err := db.AutoMigrate(m); err != nil {
			t.Fatal(err)
		}
	}
	srv := models.Server{Name: "n1", NodeID: "node-1", Secret: "s"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	full := models.Inbound{ServerID: srv.ID, Tag: "in-full", Protocol: "vless", Port: 1001,
		Type: models.InboundTypeUser, Enabled: true, Total: 100, Up: 60, Down: 40}
	expired := models.Inbound{ServerID: srv.ID, Tag: "in-expired", Protocol: "vless", Port: 1002,
		Type: models.InboundTypeUser, Enabled: true, ExpiryTime: &past}
	ok := models.Inbound{ServerID: srv.ID, Tag: "in-ok", Protocol: "vless", Port: 1003,
		Type: models.InboundTypeUser, Enabled: true}
	for _, inb := range []*models.Inbound{&full, &expired, &ok} {
		if err := db.Create(inb).Error; err != nil {
			t.Fatal(err)
		}
	}
	// 三个入站都对同一权限组开放（授权不参与本用例）
	u := models.User{Username: "u1", Email: "u1@t.com", UUID: "11111111-1111-1111-1111-111111111111",
		SubscribeToken: "t1", Status: models.StatusActive, PermissionGroupID: 1}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	for _, inb := range []*models.Inbound{&full, &expired, &ok} {
		id := inb.ID
		ap := models.UserAccessPoint{Name: "ap-" + inb.Tag, Enabled: true, TargetType: "inbound", TargetInboundID: &id}
		if err := db.Create(&ap).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&models.PermissionGroupAccessPoint{PermissionGroupID: 1, AccessPointID: ap.ID}).Error; err != nil {
			t.Fatal(err)
		}
	}

	s := &ConfigService{DB: db}
	users, err := s.GetValidUsers(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, hit := users["in-full"]; hit {
		t.Fatalf("跑满总流量的入站不得出现在热更里: %v", users)
	}
	if _, hit := users["in-expired"]; hit {
		t.Fatalf("已过期的入站不得出现在热更里: %v", users)
	}
	if _, hit := users["in-ok"]; !hit {
		t.Fatalf("可用入站必须出现在热更里: %v", users)
	}

	// 冷更侧同源：Generate 的产物里也不得有这两个 tag（否则两条路的 tag 集合又会分叉）
	cfg, err := s.Generate(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"in-full", "in-expired"} {
		if strings.Contains(cfg, `"`+tag+`"`) {
			t.Fatalf("不可用入站 %s 不应出现在生成配置里: %s", tag, cfg)
		}
	}
	if !strings.Contains(cfg, `"in-ok"`) {
		t.Fatalf("可用入站应出现在生成配置里: %s", cfg)
	}
}

// TestAppliedStructureLedger S_a 账本：PENDING 期间也要能按已生效结构过滤（不变量 I3），
// 且不得把期望结构 S_p 当 S_a 用。
func TestAppliedStructureLedger(t *testing.T) {
	db := newTestDB(t)
	for _, m := range []any{&models.Server{}, &models.Inbound{}, &models.User{}, &models.TrafficLog{},
		&models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{},
		&models.ServerOutbound{}, &models.ServerRoutingRule{}} {
		if err := db.AutoMigrate(m); err != nil {
			t.Fatal(err)
		}
	}
	// SyncedConfig 会现场 Generate，需要一个真实可生成的服务器 + 一个启用入站
	srv := models.Server{Name: "n1", NodeID: "node-1", Secret: "s"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}
	inb := models.Inbound{ServerID: srv.ID, Tag: "in-a", Protocol: "vless", Port: 1001,
		Type: models.InboundTypeUser, Enabled: true}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}
	s := &ConfigService{DB: db}
	serverID := srv.ID
	const applied = `{"inbounds":[{"tag":"in-a"},{"tag":"in-b"}]}`
	const desired = `{"inbounds":[{"tag":"in-a"},{"tag":"in-b"},{"tag":"in-new"}]}`

	// 无待推送行：主控不知道 S_a
	if _, ok := s.AppliedTags(serverID); ok {
		t.Fatal("无待推送行时不应声称知道 S_a")
	}
	if _, ok := s.SyncedConfig(serverID); ok {
		t.Fatal("无待推送行时不得附带配置（磁盘不能留下未验证结构）")
	}
	if got := s.AppliedConfig(serverID); got != "" {
		t.Fatalf("无待推送行时不应有已应用内容，实际 %q", got)
	}

	// 一次成功的冷推：SavePending → 推送成功（MarkPushedIfSame）→ 记账（MarkApplied）
	if err := s.SavePending(serverID, applied); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkPushedIfSame(mustPendingID(t, s, serverID), applied); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkApplied(serverID, applied); err != nil {
		t.Fatal(err)
	}
	tags, ok := s.AppliedTags(serverID)
	if !ok || !tags["in-a"] || !tags["in-b"] || tags["in-new"] {
		t.Fatalf("S_a 的 tag 集合应为已推送内容，实际 %v ok=%v", tags, ok)
	}
	if _, ok := s.SyncedConfig(serverID); !ok {
		t.Fatal("SYNCED 时应能给出 materialize(S_a,U)")
	}

	// 结构变更 → pending：S_a 仍可查（applied_json 未被覆盖），但不得再附带配置
	if err := s.SavePending(serverID, desired); err != nil {
		t.Fatal(err)
	}
	tags, ok = s.AppliedTags(serverID)
	if !ok {
		t.Fatal("PENDING 期间仍应能按已生效结构过滤（否则新入站的 tag 会让整批同步中断）")
	}
	if tags["in-new"] {
		t.Fatalf("不得把期望结构 S_p 当 S_a（会摘掉运行中入站的用户），实际 %v", tags)
	}
	if _, ok := s.SyncedConfig(serverID); ok {
		t.Fatal("PENDING 期间不得附带配置")
	}

	// 热更落盘记账：applied_json 跟上，但不改 config_json/status（结构权威仍是待推的那份）
	hot := `{"inbounds":[{"tag":"in-a"},{"tag":"in-b"}],"note":"hot"}`
	if err := s.MarkApplied(serverID, hot); err != nil {
		t.Fatal(err)
	}
	if got := s.AppliedConfig(serverID); got != hot {
		t.Fatalf("记账后 AppliedConfig 应为热更落盘的内容，实际 %q", got)
	}
	p, err := s.GetPending(serverID)
	if err != nil {
		t.Fatal(err)
	}
	if p.ConfigJSON != desired || p.Status != "pending" {
		t.Fatalf("热更记账不得改动待推内容与状态，实际 status=%s", p.Status)
	}
	if p.AppliedHash != ContentHash(hot) {
		t.Fatalf("applied_hash 应为内容哈希，实际 %q", p.AppliedHash)
	}
}

// TestSavePendingIfSameCAS 内容 CAS：并发编辑不被重算覆盖（E2 的写回保护）。
func TestSavePendingIfSameCAS(t *testing.T) {
	s := &ConfigService{DB: newTestDB(t)}
	const serverID = 11
	if err := s.SavePending(serverID, `{"v":1}`); err != nil {
		t.Fatal(err)
	}
	// 期望内容不匹配（已被并发编辑覆盖）→ 不替换
	ok, err := s.SavePendingIfSame(serverID, `{"v":0}`, `{"v":9}`)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("期望内容不匹配时不得替换（否则会覆盖并发编辑）")
	}
	// 匹配 → 替换并回到 pending
	ok, err = s.SavePendingIfSame(serverID, `{"v":1}`, `{"v":2}`)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("期望内容匹配时应替换成功")
	}
	p, err := s.GetPending(serverID)
	if err != nil {
		t.Fatal(err)
	}
	if p.ConfigJSON != `{"v":2}` || p.Status != "pending" {
		t.Fatalf("CAS 后内容应为新内容且状态 pending，实际 %q/%s", p.ConfigJSON, p.Status)
	}
}

// TestParseInboundTagsToleratesBadJSON 解析失败不得 panic（对账路径永远不能把主控带崩）。
func TestParseInboundTagsToleratesBadJSON(t *testing.T) {
	if got := ParseInboundTags("{ 不是 JSON"); len(got) != 0 {
		t.Fatalf("坏 JSON 应返回空集合，实际 %v", got)
	}
	if got := ParseInboundTags(""); len(got) != 0 {
		t.Fatalf("空内容应返回空集合，实际 %v", got)
	}
	tags := ParseInboundTags(`{"inbounds":[{"tag":"a"},{"tag":""},{"no_tag":1}]}`)
	if len(tags) != 1 || !tags["a"] {
		t.Fatalf("应只收非空 tag，实际 %v", tags)
	}
}

func mustPendingID(t *testing.T, s *ConfigService, serverID uint64) uint64 {
	t.Helper()
	p, err := s.GetPending(serverID)
	if err != nil || p == nil {
		t.Fatalf("应有待推送记录: %v", err)
	}
	return p.ID
}
