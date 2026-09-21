package nodegate

// 回归用例（2026-09-21）：热更负载的构成与推送前重算。
// 背景（docs/architecture/配置下发-热更冷更与状态机.md）：
//   - 不变量 I3：热更只按已生效结构 S_a 的 tag 下发（否则 handler not found 整批中断）；
//   - 不变量 I2：SYNCED 时热更顺带把整份配置带给节点落盘；
//   - P1b：待推内容冻结于 SavePending 那一刻，推送前必须现场重算，否则已删用户会被冷推复活。

import (
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

// newApplyHub 构造带真实 ConfigService 的 Hub（Generate 需要 servers/inbounds 等表）。
func newApplyHub(t *testing.T) (*Hub, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/apply.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []any{&models.Server{}, &models.Inbound{}, &models.User{}, &models.TrafficLog{},
		&models.UserAccessPoint{}, &models.PermissionGroupAccessPoint{}, &models.PendingConfig{},
		&models.ServerOutbound{}, &models.ServerRoutingRule{}} {
		if err := db.AutoMigrate(m); err != nil {
			t.Fatal(err)
		}
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &Hub{DB: db, Config: &services.ConfigService{DB: db},
		conns: make(map[uint64]*Conn), pending: make(map[string]*pendingReq)}, db
}

// seedServer 建服务器 + 一个启用入站（对某权限组开放），返回服务器。
func seedServer(t *testing.T, db *gorm.DB, nodeID, tag string) models.Server {
	t.Helper()
	srv := models.Server{Name: nodeID, NodeID: nodeID, Secret: "s"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}
	inb := models.Inbound{ServerID: srv.ID, Tag: tag, Protocol: "vless", Port: 1001,
		Type: models.InboundTypeUser, Enabled: true}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}
	u := models.User{Username: "u-" + nodeID, Email: nodeID + "@t.com", UUID: uuidFor(nodeID),
		SubscribeToken: "t-" + nodeID, Status: models.StatusActive, PermissionGroupID: 1}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	id := inb.ID
	ap := models.UserAccessPoint{Name: "ap-" + tag, Enabled: true, TargetType: "inbound", TargetInboundID: &id}
	if err := db.Create(&ap).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.PermissionGroupAccessPoint{PermissionGroupID: 1, AccessPointID: ap.ID}).Error; err != nil {
		t.Fatal(err)
	}
	return srv
}

// TestBuildSyncPayloadFiltersToAppliedStructure 不变量 I3：PENDING 期间新增的入站还不存在于
// 运行中的 xray，带着它的 tag 下发会让整批热更中断（实测 20/20 轮）。
func TestBuildSyncPayloadFiltersToAppliedStructure(t *testing.T) {
	h, db := newApplyHub(t)
	srv := seedServer(t, db, "node-1", "in-a")
	const applied = `{"inbounds":[{"tag":"in-a"}]}`
	if err := h.Config.SavePending(srv.ID, applied); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Config.MarkPushedIfSame(pendingID(t, h, srv.ID), applied); err != nil {
		t.Fatal(err)
	}
	if err := h.Config.MarkApplied(srv.ID, applied); err != nil {
		t.Fatal(err)
	}
	// 结构变更：库里多了一个入站（对同一权限组开放，所以它的 tag 会出现在用户集里）
	seedServerInbound(t, db, srv.ID, "in-new", 1002)

	payload, err := h.buildSyncPayload(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, hit := payload.Users["in-new"]; hit {
		t.Fatalf("已生效结构里没有的 tag 不得下发（会 handler not found 整批中断）: %v", payload.Users)
	}
	if _, hit := payload.Users["in-a"]; !hit {
		t.Fatalf("已生效结构里的 tag 必须下发: %v", payload.Users)
	}
}

// TestBuildSyncPayloadCarriesConfigWhenSynced 不变量 I2：SYNCED 时附带整份配置（供节点落盘），
// PENDING 时绝不附带（磁盘不能留下未经验证的结构）。
func TestBuildSyncPayloadCarriesConfigWhenSynced(t *testing.T) {
	h, db := newApplyHub(t)
	srv := seedServer(t, db, "node-1", "in-a")

	// 没有待推送行（主控不知道节点状态）→ 不附带
	payload, err := h.buildSyncPayload(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if payload.ConfigJSON != "" {
		t.Fatal("无待推送行时不得附带配置")
	}

	// 冷推成功（pushed + 记账）→ SYNCED，应附带；且内容与已应用内容不同才附带
	stale := `{"inbounds":[{"tag":"in-a"}],"note":"stale"}`
	if err := h.Config.SavePending(srv.ID, stale); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Config.MarkPushedIfSame(pendingID(t, h, srv.ID), stale); err != nil {
		t.Fatal(err)
	}
	if err := h.Config.MarkApplied(srv.ID, stale); err != nil {
		t.Fatal(err)
	}
	payload, err = h.buildSyncPayload(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if payload.ConfigJSON == "" {
		t.Fatal("SYNCED 且磁盘内容已过期时应附带现场生成的配置")
	}
	if payload.ConfigJSON == stale {
		t.Fatal("附带的不应是陈旧内容")
	}
	// 记账后内容一致 → 不再重复附带（避免让节点反复写盘 + 跑 -test）
	if err := h.Config.MarkApplied(srv.ID, payload.ConfigJSON); err != nil {
		t.Fatal(err)
	}
	payload2, err := h.buildSyncPayload(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if payload2.ConfigJSON != "" {
		t.Fatal("磁盘内容已是最新时不应重复附带配置")
	}

	// 结构变更 → PENDING → 不得附带
	if err := h.Config.SavePending(srv.ID, `{"inbounds":[{"tag":"in-a"},{"tag":"in-b"}]}`); err != nil {
		t.Fatal(err)
	}
	payload3, err := h.buildSyncPayload(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if payload3.ConfigJSON != "" {
		t.Fatal("PENDING 期间不得附带配置（磁盘不能留下未验证的结构）")
	}
}

// TestRefreshPendingRewritesStaleContent P1b：待推内容里的用户已过期 / 被删，推送前重算
// 必须改用新内容，否则冷推成功会把已删用户"复活"。
func TestRefreshPendingRewritesStaleContent(t *testing.T) {
	h, db := newApplyHub(t)
	srv := seedServer(t, db, "node-1", "in-a")
	// 待推内容生成于「用户还在」的时刻
	old, err := h.Config.Generate(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !!strings.Contains(old, "u-node-1.i") {
		t.Fatalf("待推内容应含该用户: %s", old)
	}
	if err := h.Config.SavePending(srv.ID, old); err != nil {
		t.Fatal(err)
	}
	// 用户随后被封禁（只走热更，不会更新待推内容）
	if err := db.Model(&models.User{}).Where("username = ?", "u-node-1").Update("status", "disabled").Error; err != nil {
		t.Fatal(err)
	}

	p, err := h.Config.GetPending(srv.ID)
	if err != nil || p == nil {
		t.Fatalf("应有待推记录: %v", err)
	}
	got := h.refreshPending(srv.ID, p)
	if got.ConfigJSON == old {
		t.Fatal("待推内容已陈旧（被封用户仍在），推送前必须重算")
	}
	if strings.Contains(got.ConfigJSON, "u-node-1.i") {
		t.Fatalf("重算后的内容不得再含被封用户: %s", got.ConfigJSON)
	}
	// 写回也要落库（否则 MarkPushedIfSame 会因内容不匹配而拒绝标记）
	row, err := h.Config.GetPending(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.ConfigJSON != got.ConfigJSON {
		t.Fatal("重算后的内容应写回待推送行")
	}
}

// TestRefreshPendingKeepsNewerEdit 重算期间的并发编辑不得被覆盖（内容 CAS）：
// 行内容被并发编辑换掉后，refreshPending 既不能覆盖它，也不能把旧快照当成"本次要推的内容"。
func TestRefreshPendingKeepsNewerEdit(t *testing.T) {
	h, db := newApplyHub(t)
	srv := seedServer(t, db, "node-1", "in-a")
	old, err := h.Config.Generate(srv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Config.SavePending(srv.ID, old); err != nil {
		t.Fatal(err)
	}
	p, err := h.Config.GetPending(srv.ID)
	if err != nil || p == nil {
		t.Fatal(err)
	}
	// 模拟并发编辑：重算之前 pending 行已被新内容覆盖
	const newer = `{"inbounds":[{"tag":"in-a"}],"note":"newer-edit"}`
	if err := h.Config.SavePending(srv.ID, newer); err != nil {
		t.Fatal(err)
	}

	// 调用方快照与行内容一致（fresh 也等于它）→ 早退，但绝不写库
	got := h.refreshPending(srv.ID, &models.PendingConfig{ID: p.ID, ConfigJSON: old})
	if row, _ := h.Config.GetPending(srv.ID); row.ConfigJSON != newer {
		t.Fatalf("并发编辑不得被重算覆盖，实际 %s", row.ConfigJSON)
	}
	_ = got

	// 调用方快照与行内容不一致 → CAS 以快照为期望值必然失败 → 改用行里的最新内容
	got2 := h.refreshPending(srv.ID, &models.PendingConfig{ID: p.ID, ConfigJSON: `{"stale":true}`})
	if got2.ConfigJSON != newer {
		t.Fatalf("CAS 失败后应改用库里的最新内容（否则推的是过期内容），实际 %s", got2.ConfigJSON)
	}
	if row, _ := h.Config.GetPending(srv.ID); row.ConfigJSON != newer {
		t.Fatalf("库里应保留并发编辑的内容，实际 %s", row.ConfigJSON)
	}
}

// TestEnqueueRefOutboundServers 转发内部 UUID 轮换后：落地服务器与所有引用方都要重推
// （旧实现只 Generate 丢弃返回值 + 不扇出，等于空操作，要等下一次每小时校准）。
func TestEnqueueRefOutboundServers(t *testing.T) {
	h, db := newApplyHub(t)
	landing := seedServer(t, db, "node-landing", "relay-in")
	ref := seedServer(t, db, "node-ref", "in-a")
	// 引用方：一个出站按 inbound_ref 指向落地入站
	var inb models.Inbound
	if err := db.Where("server_id = ? AND tag = ?", landing.ID, "relay-in").First(&inb).Error; err != nil {
		t.Fatal(err)
	}
	// 落地入站需已有内部账户（生成侧会预检 InternalUUID 非空），随后模拟节点轮换它
	if err := db.Model(&models.Inbound{}).Where("id = ?", inb.ID).
		Update("internal_uuid", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&inb, inb.ID).Error; err != nil {
		t.Fatal(err)
	}
	ob := models.ServerOutbound{ServerID: ref.ID, Tag: "to-relay", Protocol: "vless", Enabled: true, InboundRef: &inb.ID}
	if err := db.Create(&ob).Error; err != nil {
		t.Fatal(err)
	}
	// 轮换内部 UUID：落地服务器自己的 relay 入站 clients 与引用方的出站 client id 都要重推
	if err := db.Model(&models.Inbound{}).Where("id = ?", inb.ID).
		Update("internal_uuid", "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb").Error; err != nil {
		t.Fatal(err)
	}

	// 与 handleInternalUUIDReport 相同的两步：落地服务器自己重推 + 引用方扇出重推
	h.enqueueConfig(landing.ID)
	h.enqueueRefOutboundServers(inb.ID)

	for _, sid := range []uint64{ref.ID, landing.ID} {
		p, err := h.Config.GetPending(sid)
		if err != nil {
			t.Fatal(err)
		}
		if p == nil {
			t.Fatalf("引用方 %d 应有待推送内容（扇出缺失会让它一直跑旧 UUID）", sid)
		}
		if p.ConfigJSON == "" {
			t.Fatalf("引用方 %d 的待推内容不得为空", sid)
		}
	}
}

// uuidFor 生成合法且唯一的测试 UUID（users.uuid 有唯一约束）。
func uuidFor(seed string) string {
	h := 0
	for _, r := range seed {
		h = (h*31 + int(r)) % 0xffff
	}
	return fmt.Sprintf("11111111-1111-1111-1111-%012x", h)
}

func pendingID(t *testing.T, h *Hub, serverID uint64) uint64 {
	t.Helper()
	p, err := h.Config.GetPending(serverID)
	if err != nil || p == nil {
		t.Fatalf("应有待推送记录: %v", err)
	}
	return p.ID
}

func seedServerInbound(t *testing.T, db *gorm.DB, serverID uint64, tag string, port int) {
	t.Helper()
	inb := models.Inbound{ServerID: serverID, Tag: tag, Protocol: "vless", Port: port,
		Type: models.InboundTypeUser, Enabled: true}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}
	id := inb.ID
	ap := models.UserAccessPoint{Name: "ap-" + tag, Enabled: true, TargetType: "inbound", TargetInboundID: &id}
	if err := db.Create(&ap).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.PermissionGroupAccessPoint{PermissionGroupID: 1, AccessPointID: ap.ID}).Error; err != nil {
		t.Fatal(err)
	}
}
