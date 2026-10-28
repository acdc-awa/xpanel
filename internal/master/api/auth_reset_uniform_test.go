package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// 2026-10-28 审查 P2-1：/auth/reset 对「邮箱不存在 / 未开启 TOTP / 验证码错误」
// 必须返回同码同文案，否则可批量判定邮箱是否注册、是否开启 2FA。
func TestResetPasswordUniformMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Setting{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := &config.Config{JWT: config.JWT{Secret: "test-secret-at-least-32-chars-long-123456", AccessTTL: 2 * time.Hour, RefreshTTL: 7 * 24 * time.Hour}}
	otpSvc := services.NewOTPService(db, cfg, cfg.JWT.Secret)
	d := &Deps{DB: db, Cfg: cfg, OTP: otpSvc}

	newUser := func(email string) models.User {
		uuid, _ := util.NewUUID()
		token, _ := util.RandomHex(32)
		u := models.User{
			Username: email, Email: email, UUID: uuid, SubscribeToken: token,
			Role: models.RoleUser, Status: models.StatusActive,
		}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("create user %s: %v", email, err)
		}
		return u
	}

	// 用户 A：存在但未开启 TOTP（走管理员重置）。
	newUser("no-totp@example.com")
	// 用户 B：存在且开启 TOTP。
	otpUser := newUser("with-totp@example.com")
	secret, _, err := otpSvc.Setup(otpUser.ID, otpUser.Email)
	if err != nil {
		t.Fatalf("otp setup: %v", err)
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("totp code: %v", err)
	}
	if _, err := otpSvc.Confirm(otpUser.ID, secret, code); err != nil {
		t.Fatalf("otp confirm: %v", err)
	}

	r := gin.New()
	r.POST("/api/v1/auth/reset", d.ResetPassword)

	post := func(email, code string) (int, string) {
		body := `{"email":"` + email + `","code":"` + code + `","password":"newpassword123"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var resp struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp.Message
	}

	codeMissing, msgMissing := post("nobody@example.com", "000000")
	codeNoTOTP, msgNoTOTP := post("no-totp@example.com", "000000")
	codeBad, msgBad := post("with-totp@example.com", "000000")

	if codeMissing != http.StatusBadRequest || codeNoTOTP != http.StatusBadRequest || codeBad != http.StatusBadRequest {
		t.Fatalf("三类失败都应为 400，实际: missing=%d no_totp=%d bad_code=%d", codeMissing, codeNoTOTP, codeBad)
	}
	if msgMissing != msgNoTOTP || msgMissing != msgBad {
		t.Fatalf("三类失败文案必须一致（防枚举），实际: missing=%q no_totp=%q bad_code=%q", msgMissing, msgNoTOTP, msgBad)
	}
	if msgMissing != "邮箱或验证码错误" {
		t.Fatalf("统一文案 = %q, want 邮箱或验证码错误", msgMissing)
	}
	if strings.Contains(msgMissing, "未开启两步验证") {
		t.Fatal("不得泄露账号是否开启 2FA")
	}

	// 正常路径未被统一文案误伤：有效 TOTP 码仍可重置（200）。
	validCode, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("valid totp: %v", err)
	}
	if status, msg := post("with-totp@example.com", validCode); status != http.StatusOK {
		t.Fatalf("有效验证码重置应 200，实际 %d / %q", status, msg)
	}
}
