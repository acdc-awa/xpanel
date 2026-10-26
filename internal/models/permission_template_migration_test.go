package models

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestPermissionGroupSubTemplateMigration 老库升级：permission_groups 没有 sub_template_id 列时，
// AutoMigrate 只补一个可空列，存量行必须保持「未绑定」（NULL = 自定义），clash_template 原样不动。
// 这是「老库全部保持自定义（空键）」的回归护栏。
func TestPermissionGroupSubTemplateMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	legacyDDL := `
CREATE TABLE permission_groups (
	id integer PRIMARY KEY AUTOINCREMENT,
	name text NOT NULL,
	remark text,
	clash_template text,
	created_at datetime,
	updated_at datetime
);
CREATE UNIQUE INDEX idx_permission_groups_name ON permission_groups(name);
`
	for _, stmt := range strings.Split(legacyDDL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create legacy schema: %v", err)
		}
	}
	if err := db.Exec(`INSERT INTO permission_groups (name, remark, clash_template) VALUES (?, ?, ?)`,
		"旧权限组", "升级前已保存的副本", "proxies:\n$PROXIES$\n").Error; err != nil {
		t.Fatalf("insert legacy group: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate from legacy schema: %v", err)
	}

	if !db.Migrator().HasColumn(&PermissionGroup{}, "sub_template_id") {
		t.Fatal("AutoMigrate 应补出 permission_groups.sub_template_id 列")
	}
	if !db.Migrator().HasIndex(&PermissionGroup{}, "idx_permission_groups_sub_template_id") {
		t.Fatal("AutoMigrate 应为 sub_template_id 建索引")
	}

	var got PermissionGroup
	if err := db.Where("name = ?", "旧权限组").First(&got).Error; err != nil {
		t.Fatalf("load migrated group: %v", err)
	}
	if got.SubTemplateID != nil {
		t.Fatalf("存量权限组应保持未绑定（NULL），实际 %v", *got.SubTemplateID)
	}
	if got.ClashTemplate != "proxies:\n$PROXIES$\n" {
		t.Fatalf("存量 clash_template 不应被改写，实际 %q", got.ClashTemplate)
	}

	// 再次执行也必须幂等（启动可能重跑）。
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate should be idempotent: %v", err)
	}
}
