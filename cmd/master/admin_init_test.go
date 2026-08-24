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

// TestInvalidJWTSecretPlaceholders 模板/示例占位值必须被识别为「未配置」（触发自动生成），
// 杜绝部署者照抄 configs 模板或 .env.example 后弱密钥被采信（2026-08-24 默认值清零）。
func TestInvalidJWTSecretPlaceholders(t *testing.T) {
	placeholder := []string{
		"",
		"change-me-in-production-at-least-32-chars", // 旧 configs 模板占位值
		"change-me-in-production-must-be-32-bytes",
		"dev-secret-change-in-production",
		"replace-with-openssl-rand-hex-32", // .env.example 占位值
	}
	for _, v := range placeholder {
		if !invalidJWTSecret(v) {
			t.Errorf("占位值应视为未配置: %q", v)
		}
	}
	for _, v := range []string{"real-32-bytes-secret-0123456789abcdef", "abc123"} {
		if invalidJWTSecret(v) {
			t.Errorf("显式密钥被误判为占位值: %q", v)
		}
	}
}

// TestEnsureJWTSecretPlaceholderRegenerated config 携带旧模板占位值时，
// 必须自动生成新密钥落库，而不是把占位值当密钥用。
func TestEnsureJWTSecretPlaceholderRegenerated(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{JWT: config.JWT{Secret: "change-me-in-production-at-least-32-chars"}}

	secret := ensureJWTSecret(db, cfg)
	if secret == cfg.JWT.Secret || len(secret) < 32 {
		t.Fatalf("占位值未被替换为自动生成密钥: %q", secret)
	}
}

// TestEnsureAdminPlaceholderPasswordRandomized 照抄 .env.example 占位密码时，
// 必须自动生成强随机密码而非以占位值作密码（防默认密码攻击）。
func TestEnsureAdminPlaceholderPasswordRandomized(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.Config{
		Admin: config.Admin{
			Username: "admin@test.local",
			Password: "replace-with-strong-password",
		},
	}

	ensureAdmin(db, cfg)

	var admin models.User
	if err := db.Where("role = ?", models.RoleAdmin).First(&admin).Error; err != nil {
		t.Fatalf("admin not created: %v", err)
	}
	match, err := argon2id.ComparePasswordAndHash("replace-with-strong-password", admin.PasswordHash)
	if err != nil || match {
		t.Errorf("占位值不得作为管理员密码（match=%v err=%v）", match, err)
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
