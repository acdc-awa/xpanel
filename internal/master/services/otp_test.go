package services

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/config"
	"github.com/acdc/xray-panel/internal/models"
	"github.com/acdc/xray-panel/internal/pkg/util"
)

func otpTestSvc(t *testing.T) (*OTPService, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	cfg := config.Default()
	cfg.JWT.Secret = "test-secret"
	return NewOTPService(db, cfg), db
}

func mkUser(t *testing.T, db *gorm.DB, name string) models.User {
	t.Helper()
	token, _ := util.NewSubscribeToken()
	u := models.User{
		Username:       name,
		Email:          name + "@test.local",
		UUID:           "00000000-0000-0000-0000-00000000000" + name,
		SubscribeToken: token,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u
}

// TestOTP_BindVerifyDisable 绑定 → 验证码通过 → 错误码拒绝 → 锁定 → 解绑全链。
func TestOTP_BindVerifyDisable(t *testing.T) {
	svc, db := otpTestSvc(t)
	u := mkUser(t, db, "otpuser")

	secret, otpauth, err := svc.Setup(u.ID, u.Email)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if otpauth == "" {
		t.Fatal("otpauth url empty")
	}
	if u.TotpEnabled {
		t.Fatal("should not be enabled before confirm")
	}

	// 错误验证码拒绝
	if _, err := svc.Confirm(u.ID, secret, "000000"); err != ErrTOTPCodeInvalid {
		t.Fatalf("bad code should fail, got %v", err)
	}

	// 正确验证码（当前 TOTP）
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	codes, err := svc.Confirm(u.ID, secret, code)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(codes) != 8 {
		t.Fatalf("expected 8 backup codes, got %d", len(codes))
	}

	db.First(&u, u.ID)
	if !u.TotpEnabled {
		t.Fatal("totp should be enabled")
	}

	// 重复绑定拒绝
	if _, _, err := svc.Setup(u.ID, u.Email); err != ErrTOTPAlreadyOn {
		t.Fatalf("re-setup should fail, got %v", err)
	}

	// 验证码校验：正确通过 / 错误拒绝
	if err := svc.VerifyCode(&u, "000000"); err != ErrTOTPCodeInvalid {
		t.Fatalf("bad code should fail, got %v", err)
	}
	code2, _ := totp.GenerateCode(secret, time.Now())
	if err := svc.VerifyCode(&u, code2); err != nil {
		t.Fatalf("good code should pass: %v", err)
	}

	// 解绑
	if err := svc.Disable(u.ID); err != nil {
		t.Fatalf("disable: %v", err)
	}
	db.First(&u, u.ID)
	if u.TotpEnabled {
		t.Fatal("totp should be disabled")
	}
	if err := svc.VerifyCode(&u, code2); err != ErrTOTPNotEnabled {
		t.Fatalf("verify after disable should fail, got %v", err)
	}
}

// TestOTP_BackupCode 恢复码一次性消费。
func TestOTP_BackupCode(t *testing.T) {
	svc, db := otpTestSvc(t)
	u := mkUser(t, db, "backupuser")

	secret, _, err := svc.Setup(u.ID, u.Email)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	codes, err := svc.Confirm(u.ID, secret, code)
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	db.First(&u, u.ID)

	// 用恢复码
	if err := svc.VerifyBackupCode(&u, codes[0]); err != nil {
		t.Fatalf("backup code should pass: %v", err)
	}
	// 同码再次使用 → 失败（一次性）
	db.First(&u, u.ID)
	if err := svc.VerifyBackupCode(&u, codes[0]); err != ErrBackupInvalid {
		t.Fatalf("reused backup code should fail, got %v", err)
	}
	// 剩余 7 个
	if err := svc.VerifyBackupCode(&u, codes[1]); err != nil {
		t.Fatalf("second backup code should pass: %v", err)
	}
}

// TestOTP_Lock 连续错误 5 次锁定 30 分钟。
func TestOTP_Lock(t *testing.T) {
	svc, db := otpTestSvc(t)
	u := mkUser(t, db, "lockuser")

	secret, _, err := svc.Setup(u.ID, u.Email)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	if _, err := svc.Confirm(u.ID, secret, code); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	db.First(&u, u.ID)

	for i := 0; i < 4; i++ {
		if err := svc.VerifyCode(&u, "000000"); err != ErrTOTPCodeInvalid {
			t.Fatalf("iteration %d: want invalid code, got %v", i, err)
		}
	}
	if err := svc.VerifyCode(&u, "000000"); err != ErrTOTPLocked {
		t.Fatalf("5th fail should lock, got %v", err)
	}
	db.First(&u, u.ID)
	if u.TotpLockedUntil == nil || !u.TotpLockedUntil.After(time.Now()) {
		t.Fatal("locked_until should be set in future")
	}
	// 锁定期间正确码也拒绝
	good, _ := totp.GenerateCode(secret, time.Now())
	if err := svc.VerifyCode(&u, good); err != ErrTOTPLocked {
		t.Fatalf("locked state should reject even good code, got %v", err)
	}
}

// TestOTP_ResetPassword 忘记密码重置（TOTP 通过 + token_version bump）。
func TestOTP_ResetPassword(t *testing.T) {
	svc, db := otpTestSvc(t)
	u := mkUser(t, db, "resetuser")

	secret, _, err := svc.Setup(u.ID, u.Email)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	if _, err := svc.Confirm(u.ID, secret, code); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	// Confirm 启用 TOTP 现在会 bump token_version（吊销旧会话），因此以 Confirm 后的值为基准。
	db.First(&u, u.ID)
	before := u.TokenVersion

	good, _ := totp.GenerateCode(secret, time.Now())
	if err := svc.ResetPassword(t.Context(), u.Email, good, "newpass123"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	db.First(&u, u.ID)
	if u.TokenVersion != before+1 {
		t.Fatalf("token_version should bump, got %d (before %d)", u.TokenVersion, before)
	}
	// 错误码重置失败
	if err := svc.ResetPassword(t.Context(), u.Email, "000000", "x"); err == nil {
		t.Fatal("bad code reset should fail")
	}
	// 未绑定 TOTP 用户重置 → ErrTOTPNotEnabled
	u2 := mkUser(t, db, "no2fauser")
	if err := svc.ResetPassword(t.Context(), u2.Email, "000000", "x"); err != ErrTOTPNotEnabled {
		t.Fatalf("unbound user reset should fail with ErrTOTPNotEnabled, got %v", err)
	}
}
