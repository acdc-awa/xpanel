package api

import (
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/tlscert"
)

func apiTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// 每个测试独立内存库（cache=shared + 唯一名，避免跨测试串库）
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Server{}, &models.Inbound{}, &models.ServerOutbound{},
		&models.Cert{}, &models.Plan{}, &models.PermissionGroup{}, &models.PermissionGroupInbound{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func seedRefGraph(t *testing.T, db *gorm.DB) (s1, s2, aIn, bIn uint64) {
	t.Helper()
	s1 = uint64(1)
	s2 = uint64(2)
	db.Create(&models.Server{ID: s1, Name: "s1", Host: "10.0.0.1", NodeID: "n1", Secret: "x"})
	db.Create(&models.Server{ID: s2, Name: "s2", Host: "10.0.0.2", NodeID: "n2", Secret: "x"})
	a := models.Inbound{ServerID: s1, Tag: "in-a", Protocol: "vless", Port: 44301, Type: models.InboundTypeUser, Enabled: true}
	b := models.Inbound{ServerID: s2, Tag: "in-b", Protocol: "vless", Port: 44302, Type: models.InboundTypeRelay, InternalUUID: "uuid-b", Enabled: true}
	db.Create(&a)
	db.Create(&b)
	return s1, s2, a.ID, b.ID
}

func TestWouldCreateRefCycle(t *testing.T) {
	db := apiTestDB(t)
	s1, s2, aIn, bIn := seedRefGraph(t, db)
	d := &Deps{DB: db}

	// X (S1) → B：合法（无环）
	if msg := d.checkInboundRef(s1, bIn, 0); msg != "" {
		t.Fatalf("X(S1)→B 应通过: %s", msg)
	}
	refB := bIn
	db.Create(&models.ServerOutbound{ServerID: s1, Tag: "x-to-b", Protocol: "vless", InboundRef: &refB, Enabled: true})

	// 再建 Y (S2) → A：从 A 出发可达 B（在 S2 上）→ 环
	if msg := d.checkInboundRef(s2, aIn, 0); msg == "" {
		t.Fatal("Y(S2)→A 应判为环（A→B→A）")
	}

	// 同服务器引用：Z (S1) → A（A 在 S1 上）→ 环
	if msg := d.checkInboundRef(s1, aIn, 0); msg == "" {
		t.Fatal("同服务器引用应判为环")
	}

	// 更新 X 自身（exclude X）：X→B 维持，不因自身产生新环
	if msg := d.checkInboundRef(s1, bIn, 1); msg != "" {
		t.Fatalf("更新 X 自身应通过: %s", msg)
	}
}

func TestRelayMarkLifecycle(t *testing.T) {
	db := apiTestDB(t)
	_, _, aIn, bIn := seedRefGraph(t, db)
	d := &Deps{DB: db}

	// 设置引用 → 目标自动标 relay
	d.ensureRelayMark(bIn)
	var b models.Inbound
	db.First(&b, bIn)
	if b.Type != models.InboundTypeRelay {
		t.Errorf("目标应为 relay, got %s", b.Type)
	}

	// 仍被引用 → demote 不生效
	refB := bIn
	db.Create(&models.ServerOutbound{ServerID: 1, Tag: "x", Protocol: "vless", InboundRef: &refB, Enabled: true})
	d.demoteIfUnreferenced(bIn)
	db.First(&b, bIn)
	if b.Type != models.InboundTypeRelay {
		t.Error("仍被引用时不应降级")
	}

	// 删除引用后 → 回 idle
	db.Where("tag = ?", "x").Delete(&models.ServerOutbound{})
	d.demoteIfUnreferenced(bIn)
	db.First(&b, bIn)
	if b.Type != models.InboundTypeIdle {
		t.Errorf("无引用应回 idle, got %s", b.Type)
	}
	_ = aIn
}

func TestCertNotAfter(t *testing.T) {
	// 用 agent certs 测试同款自签证书逻辑不便引入；这里校验 tlscert.NotAfter 对坏输入报错
	if _, err := tlscert.NotAfter("not-a-pem"); err == nil {
		t.Error("非法 PEM 应报错")
	}
}

func TestCheckInboundRefTargets(t *testing.T) {
	db := apiTestDB(t)
	s1, _, aIn, _ := seedRefGraph(t, db)
	d := &Deps{DB: db}

	// 目标不存在
	if msg := d.checkInboundRef(s1, 9999, 0); msg == "" {
		t.Error("引用不存在的入站应报错")
	}
	// 目标为 idle
	var a models.Inbound
	db.First(&a, aIn)
	db.Model(&a).Update("type", models.InboundTypeIdle)
	if msg := d.checkInboundRef(s1, aIn, 0); msg == "" {
		t.Error("引用 idle 入站应报错")
	}
	// 目标停用
	db.Model(&a).Updates(map[string]any{"type": models.InboundTypeRelay, "enabled": false})
	if msg := d.checkInboundRef(s1, aIn, 0); msg == "" {
		t.Error("引用停用入站应报错")
	}
	_ = time.Now
}
