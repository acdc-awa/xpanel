package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// testDB 内存 SQLite（glebarez 纯 Go 实现，无 cgo）。
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

// T1 迁移验证（05 号文档 §2/§8 步 1）：
// 新列/新表就位；旧数据默认 type=user；InboundRef 可空；Cert 存取与 NotAfter。
func TestT1MigrateInboundExtensions(t *testing.T) {
	db := testDB(t)

	// 1. 旧数据默认 type=user（不指定 Type 创建）
	inb := Inbound{ServerID: 1, Tag: "legacy-in", Protocol: "vless", Port: 443}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	var got Inbound
	if err := db.First(&got, inb.ID).Error; err != nil {
		t.Fatalf("read inbound: %v", err)
	}
	if got.Type != InboundTypeUser {
		t.Errorf("旧数据默认 type = %q, want %q", got.Type, InboundTypeUser)
	}
	if got.InternalUUID != "" {
		t.Errorf("InternalUUID 默认应为空, got %q", got.InternalUUID)
	}
	if got.CertID != nil {
		t.Errorf("CertID 默认应为 nil, got %v", *got.CertID)
	}

	// 2. 显式 relay + 覆盖字段
	cert := Cert{Domain: "example.com", CertPEM: "cert-pem", KeyPEM: "key-pem", NotAfter: time.Now().Add(24 * time.Hour)}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatalf("create cert: %v", err)
	}
	relay := Inbound{
		ServerID: 1, Tag: "relay-in", Protocol: "vless", Port: 8443,
		Type: InboundTypeRelay, InternalUUID: "11111111-2222-3333-4444-555555555555", CertID: &cert.ID,
	}
	if err := db.Create(&relay).Error; err != nil {
		t.Fatalf("create relay inbound: %v", err)
	}
	var got2 Inbound
	if err := db.First(&got2, relay.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got2.Type != InboundTypeRelay || got2.InternalUUID != relay.InternalUUID {
		t.Errorf("relay 字段读写不一致: %+v", got2)
	}
	if got2.CertID == nil || *got2.CertID != cert.ID {
		t.Errorf("CertID = %v, want %d", got2.CertID, cert.ID)
	}

	// 3. ServerOutbound.InboundRef 可空 / 可写
	ob := ServerOutbound{ServerID: 1, Tag: "landing", Protocol: "vless"}
	if err := db.Create(&ob).Error; err != nil {
		t.Fatalf("create outbound: %v", err)
	}
	ref := relay.ID
	ob2 := ServerOutbound{ServerID: 1, Tag: "landing2", Protocol: "vless", InboundRef: &ref}
	if err := db.Create(&ob2).Error; err != nil {
		t.Fatalf("create outbound with ref: %v", err)
	}
	var got3 ServerOutbound
	if err := db.First(&got3, ob2.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got3.InboundRef == nil || *got3.InboundRef != relay.ID {
		t.Errorf("InboundRef = %v, want %d", got3.InboundRef, relay.ID)
	}
	var got4 ServerOutbound
	if err := db.First(&got4, ob.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got4.InboundRef != nil {
		t.Errorf("InboundRef 默认应为 nil, got %v", *got4.InboundRef)
	}
}

func TestT1CertModel(t *testing.T) {
	db := testDB(t)

	// Domain 唯一索引
	c1 := Cert{Domain: "a.example.com", CertPEM: "c1", KeyPEM: "k1"}
	c2 := Cert{Domain: "a.example.com", CertPEM: "c2", KeyPEM: "k2"}
	if err := db.Create(&c1).Error; err != nil {
		t.Fatalf("create c1: %v", err)
	}
	if err := db.Create(&c2).Error; err == nil {
		t.Fatal("同 domain 应被唯一索引拒绝")
	}

	// 读取含 NotAfter
	var got Cert
	if err := db.First(&got, c1.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.CertPEM != "c1" || got.KeyPEM != "k1" {
		t.Errorf("PEM 读写不一致: %+v", got)
	}
	// NotAfter 零值可存（上传时解析填充）
	var got2 Cert
	if err := db.First(&got2, c1.ID).Error; err != nil {
		t.Fatal(err)
	}
	_ = got2.NotAfter
}

// 入站配额单位口径（2026-11-05）：total 列是 GB，up/down 是字节。
// 历史缺陷是四处判定直接拿字节比 GB——100GB 的配额被 100 字节的用量判超额，
// 入站收到第一批流量后 5 分钟内就被自动停用，订阅里整台入口消失。
func TestInboundQuotaBytesUnit(t *testing.T) {
	gb := int64(1024 * 1024 * 1024)
	cases := []struct {
		name string
		inb  Inbound
		want bool
	}{
		{"不限恒为未超额", Inbound{Total: 0, Up: 1 << 40, Down: 1 << 40}, false},
		{"100GB 配额用到 1KB 不算超额", Inbound{Total: 100, Up: 1024, Down: 0}, false},
		{"100GB 配额用到 99.9GB 不算超额", Inbound{Total: 100, Up: 99 * gb, Down: gb / 2}, false},
		{"恰好跑满算超额", Inbound{Total: 100, Up: 60 * gb, Down: 40 * gb}, true},
		{"超出 1 字节也算超额", Inbound{Total: 1, Up: gb, Down: 1}, true},
	}
	for _, c := range cases {
		if got := c.inb.QuotaExhausted(); got != c.want {
			t.Errorf("%s: QuotaExhausted=%v, want %v (quota=%d bytes)", c.name, got, c.want, c.inb.QuotaBytes())
		}
	}
	inb := Inbound{Total: 100}
	if got := inb.QuotaBytes(); got != 100*gb {
		t.Errorf("QuotaBytes=%d, want %d", got, 100*gb)
	}
	if got := (&Inbound{Total: 0}).QuotaBytes(); got != 0 {
		t.Errorf("不限应换算为 0，实际 %d", got)
	}
}
