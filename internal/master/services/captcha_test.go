package services

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// captchaSettingDB 建内存库（不写验证配置，供「默认关」用例使用）。
func captchaSettingDB(t *testing.T) *gorm.DB {
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

// captchaTestDB 建内存库并写入验证配置（enable + secret）。
func captchaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := captchaSettingDB(t)
	if err := db.Create(&models.Setting{Key: SettingCaptchaEnable, Value: "true"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	if err := db.Create(&models.Setting{Key: SettingTurnstileSecret, Value: "test-secret"}).Error; err != nil {
		t.Fatalf("create setting: %v", err)
	}
	return db
}

// stubSiteverify 把 siteverify 指向本地 httptest 服务，返回固定 JSON 并统计调用次数，
// 使测试脱离 Cloudflare 网络。返回的计数指针供断言「是否真的一次次问了 siteverify」。
func stubSiteverify(t *testing.T, body string) *int32 {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	old := turnstileVerifyEndpoint
	turnstileVerifyEndpoint = srv.URL
	t.Cleanup(func() {
		turnstileVerifyEndpoint = old
		srv.Close()
	})
	return &calls
}

// captchaForget 清掉消费表里的指定 token，避免全局状态在用例间串扰。
func captchaForget(tokens ...string) {
	captchaUsed.Lock()
	defer captchaUsed.Unlock()
	for _, tok := range tokens {
		delete(captchaUsed.m, tok)
	}
}

// TestVerifyCaptcha_Disabled 开关关闭时直接放行（不要求 token）。
func TestVerifyCaptcha_Disabled(t *testing.T) {
	db := captchaSettingDB(t)
	if err := VerifyCaptcha(db, "", "1.2.3.4", "panel.example.com", "login"); err != nil {
		t.Fatalf("disabled captcha should pass: %v", err)
	}
}

// TestVerifyCaptcha_EnabledRequiresToken 开关开启且无 token → 统一拒绝（不发起网络请求）。
func TestVerifyCaptcha_EnabledRequiresToken(t *testing.T) {
	db := captchaTestDB(t)
	if err := VerifyCaptcha(db, "", "1.2.3.4", "panel.example.com", "login"); err != ErrCaptchaFailed {
		t.Fatalf("enabled captcha without token should fail with ErrCaptchaFailed, got %v", err)
	}
}

// TestVerifyCaptcha_FailedVerifyKeepsTokenUsable 回归 2026-09-15 反馈：
// 一次校验失败（密码错、网络抖动、hostname 不匹配）不得把 token 登记为已消费，
// 否则用户拿同一 token 重试必然撞「已消费」，看到的是「人机验证未通过」而不是真实原因。
func TestVerifyCaptcha_FailedVerifyKeepsTokenUsable(t *testing.T) {
	db := captchaTestDB(t)
	const tok = "tok-retry-after-failure"
	captchaForget(tok)
	calls := stubSiteverify(t, `{"success":false,"error-codes":["invalid-input-response"]}`)

	if err := VerifyCaptcha(db, tok, "1.2.3.4", "panel.example.com", "login"); err != ErrCaptchaFailed {
		t.Fatalf("failed verify should return ErrCaptchaFailed, got %v", err)
	}
	if captchaAlreadyUsed(tok) {
		t.Fatal("校验失败的 token 不应被登记为已消费（否则重试必然失败）")
	}
	// 第二次提交必须真正再问一次 siteverify，而不是被本地消费表短路。
	if err := VerifyCaptcha(db, tok, "1.2.3.4", "panel.example.com", "login"); err != ErrCaptchaFailed {
		t.Fatalf("second attempt should still reach siteverify, got %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 2 {
		t.Fatalf("siteverify 应被调用 2 次（未被本地消费表短路），实际 %d 次", got)
	}
	captchaForget(tok)
}

// TestVerifyCaptcha_SuccessThenReplay 校验通过后 token 被消费；同 token 二次提交直接拒绝，
// 且不再请求 siteverify（一次性防重放）。
func TestVerifyCaptcha_SuccessThenReplay(t *testing.T) {
	db := captchaTestDB(t)
	const tok = "tok-success-once"
	captchaForget(tok)
	calls := stubSiteverify(t, `{"success":true,"hostname":"panel.example.com"}`)

	if err := VerifyCaptcha(db, tok, "1.2.3.4", "panel.example.com", "login"); err != nil {
		t.Fatalf("valid token should pass, got %v", err)
	}
	if !captchaAlreadyUsed(tok) {
		t.Fatal("校验通过的 token 应被登记为已消费")
	}
	if err := VerifyCaptcha(db, tok, "1.2.3.4", "panel.example.com", "login"); err != ErrCaptchaFailed {
		t.Fatalf("replayed token should fail with ErrCaptchaFailed, got %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("重放不应再请求 siteverify，实际调用 %d 次", got)
	}
	captchaForget(tok)
}

// TestVerifyCaptcha_HostnameMismatch 颁发站点与请求 Host 不一致 → 拒绝且不消费。
func TestVerifyCaptcha_HostnameMismatch(t *testing.T) {
	db := captchaTestDB(t)
	const tok = "tok-host-mismatch"
	captchaForget(tok)
	stubSiteverify(t, `{"success":true,"hostname":"evil.example.com"}`)

	if err := VerifyCaptcha(db, tok, "1.2.3.4", "panel.example.com", "login"); err != ErrCaptchaFailed {
		t.Fatalf("hostname mismatch should fail, got %v", err)
	}
	if captchaAlreadyUsed(tok) {
		t.Fatal("hostname 不匹配的 token 不应被登记为已消费")
	}
	captchaForget(tok)
}

// TestConsumeCaptchaToken_Atomic 消费登记原子性：并发登记同一 token 只成功一次。
func TestConsumeCaptchaToken_Atomic(t *testing.T) {
	const tok = "tok-atomic"
	captchaForget(tok)
	const n = 8
	var wg sync.WaitGroup
	var okCount int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if consumeCaptchaToken(tok, time.Now()) {
				atomic.AddInt32(&okCount, 1)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&okCount); got != 1 {
		t.Fatalf("同 token 并发登记应只成功一次，实际 %d 次", got)
	}
	captchaForget(tok)
}

// TestConsumeCaptchaToken_PrunesExpired 超出消费窗口的旧记录会被清理（表不无限增长）。
func TestConsumeCaptchaToken_PrunesExpired(t *testing.T) {
	const stale = "tok-stale"
	captchaForget(stale)
	captchaUsed.Lock()
	captchaUsed.m[stale] = time.Now().Add(-2 * turnstileTokenTTL)
	captchaUsed.Unlock()

	if !consumeCaptchaToken("tok-fresh", time.Now()) {
		t.Fatal("新 token 应登记成功")
	}
	captchaUsed.Lock()
	_, stillThere := captchaUsed.m[stale]
	captchaUsed.Unlock()
	if stillThere {
		t.Fatal("超期记录应在登记时被清理")
	}
	captchaForget(stale, "tok-fresh")
}

// TestLoadCaptchaConfig 配置读取（默认关 + 开启后）。
func TestLoadCaptchaConfig(t *testing.T) {
	db := captchaSettingDB(t)
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
