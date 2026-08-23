package main

import (
	"testing"

	"github.com/alexedwards/argon2id"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/models"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func TestEnsureJWTSecret(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{}

	// 1. 首次调用：自动生成并存入 DB
	secret1 := ensureJWTSecret(db, cfg)
	if len(secret1) < 32 {
		t.Fatalf("generated jwt secret too short: %q", secret1)
	}

	// 2. 再次调用：必须复用 DB 中的已有 secret
	secret2 := ensureJWTSecret(db, cfg)
	if secret1 != secret2 {
		t.Fatalf("jwt secret not idempotent: %q vs %q", secret1, secret2)
	}

	// 3. 校验 DB 表真实存在
	var s models.Setting
	if err := db.Where("`key` = ?", "jwt_secret").First(&s).Error; err != nil || s.Value != secret1 {
		t.Fatalf("setting not saved in DB: %v", err)
	}
}

func TestEnsureAdmin_RandomPassword(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{
		Admin: config.Admin{
			Username: "admin@test.local",
			Password: "", // 留空触发自动强随机生成
		},
	}

	ensureAdmin(db, cfg)

	var admin models.User
	if err := db.Where("role = ?", models.RoleAdmin).First(&admin).Error; err != nil {
		t.Fatalf("admin not created: %v", err)
	}

	if admin.Username != "admin@test.local" {
		t.Errorf("got username %q", admin.Username)
	}
	if !admin.MustChangePwd {
		t.Errorf("must_change_pwd should be true on first auto init")
	}
	if admin.PasswordHash == "" {
		t.Errorf("password hash is empty")
	}
}

func TestHandleResetAdmin(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{
		Admin: config.Admin{
			Username: "superadmin@panel.com",
			Password: "OldPassword123!",
		},
	}
	ensureAdmin(db, cfg)

	var admin models.User
	db.Where("role = ?", models.RoleAdmin).First(&admin)
	oldVersion := admin.TokenVersion

	// 执行重置
	newPass := "NewSuperPass2026#!"
	handleResetAdmin(db, []string{"-password", newPass})

	// 验证新密码与 token_version
	var updated models.User
	db.Where("id = ?", admin.ID).First(&updated)

	if updated.TokenVersion != oldVersion+1 {
		t.Errorf("token_version want %d, got %d", oldVersion+1, updated.TokenVersion)
	}

	match, err := argon2id.ComparePasswordAndHash(newPass, updated.PasswordHash)
	if err != nil || !match {
		t.Errorf("new password hash does not match: %v", err)
	}
}
