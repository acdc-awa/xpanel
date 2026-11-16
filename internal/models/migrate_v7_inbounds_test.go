package models

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestDropLegacyInboundColumns v7 删列迁移：模拟一个还带着四个历史列的旧库，
// 跑一次 AutoMigrate 后这四列必须消失、其余列与数据完好；二次执行幂等。
func TestDropLegacyInboundColumns(t *testing.T) {
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// 先按当前模型建表，再补上四个历史列（模拟 v6 库形态）
	if err := db.AutoMigrate(All()...); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	for _, col := range []string{"share_security", "share_host", "share_path", "previous_type"} {
		if err := db.Exec("ALTER TABLE `inbounds` ADD COLUMN `" + col + "` text").Error; err != nil {
			t.Fatalf("ADD COLUMN %s: %v", col, err)
		}
	}
	if err := db.Exec("UPDATE `inbounds` SET `share_path` = '/legacy'").Error; err != nil {
		t.Fatal(err)
	}
	inb := Inbound{ServerID: 1, Tag: "legacy-in", Protocol: "vless", Port: 443}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	for _, col := range []string{"share_security", "share_host", "share_path", "previous_type"} {
		if db.Migrator().HasColumn("inbounds", col) {
			t.Errorf("列 %s 应已删除", col)
		}
	}
	// 数据不丢：tag/port 仍在
	var got Inbound
	if err := db.First(&got, inb.ID).Error; err != nil {
		t.Fatalf("重新读取入站失败: %v", err)
	}
	if got.Tag != "legacy-in" || got.Port != 443 {
		t.Fatalf("入站数据被破坏: %+v", got)
	}

	// 幂等：再跑一次不应报错
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second AutoMigrate should be idempotent: %v", err)
	}
}
