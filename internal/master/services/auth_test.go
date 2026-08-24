package services

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func newAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() }) // Windows 下不关句柄会导致 TempDir 清理失败
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// resetLoginFailures 清空全局失败计数，避免用例间互相污染。
func resetLoginFailures() {
	loginFailures.Lock()
	loginFailures.m = map[string]loginFail{}
	loginFailures.Unlock()
}

func loginTestUser(t *testing.T, db *gorm.DB) {
	t.Helper()
	hash, err := argon2id.CreateHash("correct-password", argon2id.DefaultParams)
	if err != nil {
		t.Fatal(err)
	}
	u := models.User{
		Username:     "user@example.com",
		Email:        "user@example.com",
		UUID:         "11111111-1111-1111-1111-111111111111",
		PasswordHash: hash,
		Status:       models.StatusActive,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
}

// TestLoginLockByUsernameAndIP 锁定按 邮箱+IP 组合：同 IP 5 次失败锁住该组合，
// 受害者从其他 IP 登录不受影响（P1-4 锁号 DoS 修复）。
func TestLoginLockByUsernameAndIP(t *testing.T) {
	resetLoginFailures()
	db := newAuthTestDB(t)
	loginTestUser(t, db)
	svc := &AuthService{DB: db}

	const (
		attackerIP = "203.0.113.9"
		victimIP   = "198.51.100.7"
	)
	for i := 0; i < loginFailLimit; i++ {
		if _, err := svc.Login(t.Context(), "user@example.com", "wrong-password", attackerIP); err == nil {
			t.Fatalf("第 %d 次错误密码未返回错误", i+1)
		}
	}
	// 第 6 次：组合已锁定
	if _, err := svc.Login(t.Context(), "user@example.com", "wrong-password", attackerIP); err == nil || !strings.Contains(err.Error(), "次数过多") {
		t.Fatalf("同 IP 第 6 次应报锁定，got %v", err)
	}
	// 正确密码也打不开被锁的组合
	if _, err := svc.Login(t.Context(), "user@example.com", "correct-password", attackerIP); err == nil {
		t.Fatal("被锁组合的正确密码不应通过")
	}
	// 受害者从其他 IP 登录正常（DoS 消除）
	u, err := svc.Login(t.Context(), "user@example.com", "correct-password", victimIP)
	if err != nil {
		t.Fatalf("其他 IP 登录应成功: %v", err)
	}
	if u.Username != "user@example.com" {
		t.Fatalf("返回用户不符: %s", u.Username)
	}
}

// TestLoginLockExpiry 锁定期过期后自动解除。
func TestLoginLockExpiry(t *testing.T) {
	resetLoginFailures()
	db := newAuthTestDB(t)
	loginTestUser(t, db)
	svc := &AuthService{DB: db}

	loginFailures.Lock()
	loginFailures.m["user@example.com|203.0.113.9"] = loginFail{until: time.Now().Add(-time.Minute)}
	loginFailures.Unlock()

	if _, err := svc.Login(t.Context(), "user@example.com", "correct-password", "203.0.113.9"); err != nil {
		t.Fatalf("过期锁定应自动清除并放行: %v", err)
	}
}
