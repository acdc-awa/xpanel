package services

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/models"
)

func captchaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

// TestVerifyCaptcha_Disabled 开关关闭时直接放行（不要求 token）。
func TestVerifyCaptcha_Disabled(t *testing.T) {
	db := captchaTestDB(t)
	if err := VerifyCaptcha(db, "", "1.2.3.4", "panel.example.com", "login"); err != nil {
		t.Fatalf("disabled captcha should pass: %v", err)
	}
}

// TestVerifyCaptcha_EnabledRequiresToken 开关开启且无 token → 统一拒绝。
func TestVerifyCaptcha_EnabledRequiresToken(t *testing.T) {
	db := captchaTestDB(t)
	if err := db.Create(&models.Setting{Key: SettingCaptchaEnable, Value: "true"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	if err := db.Create(&models.Setting{Key: SettingTurnstileSecret, Value: "test-secret"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	if err := VerifyCaptcha(db, "", "1.2.3.4", "panel.example.com", "login"); err == nil {
		t.Fatal("enabled captcha without token should fail")
	}
}

// TestVerifyCaptcha_TokenReplay 同一 token 二次消费被拒（一次性防重放，不实际请求 CF——
// 首次调用因 secret 无效也返回失败，但二次调用必须与首次同样失败，且不区分原因）。
func TestVerifyCaptcha_TokenReplay(t *testing.T) {
	db := captchaTestDB(t)
	if err := db.Create(&models.Setting{Key: SettingCaptchaEnable, Value: "true"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	if err := db.Create(&models.Setting{Key: SettingTurnstileSecret, Value: "s"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	if err := VerifyCaptcha(db, "tok-abc", "1.2.3.4", "h", "login"); err == nil {
		t.Fatal("first call should fail (bad secret)")
	}
	// 第二次：token 已消费 → 失败（与第一次同为 ErrCaptchaFailed，不泄漏原因）
	if err := VerifyCaptcha(db, "tok-abc", "1.2.3.4", "h", "login"); err != ErrCaptchaFailed {
		t.Fatalf("replayed token should fail with ErrCaptchaFailed, got %v", err)
	}
}

// TestLoadCaptchaConfig 配置读取（默认关 + 开启后）。
func TestLoadCaptchaConfig(t *testing.T) {
	db := captchaTestDB(t)
	cfg := LoadCaptchaConfig(db)
	if cfg.Enabled {
		t.Fatal("default should be disabled")
	}
	if cfg.Type != DefaultCaptchaType {
		t.Fatalf("default type should be turnstile, got %s", cfg.Type)
	}
	if err := db.Create(&models.Setting{Key: SettingCaptchaEnable, Value: "1"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	if err := db.Create(&models.Setting{Key: SettingTurnstileSiteKey, Value: "1x00000000000000000000AA"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	cfg = LoadCaptchaConfig(db)
	if !cfg.Enabled || cfg.SiteKey == "" {
		t.Fatalf("enabled config not loaded: %+v", cfg)
	}
}
