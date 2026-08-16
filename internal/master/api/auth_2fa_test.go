package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/config"
	"github.com/zhx/xray-panel/internal/master/middleware"
	"github.com/zhx/xray-panel/internal/master/services"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// TestTwoFAVerifyRoute ISSUE-01 回归：TOTP 用户密码登录拿到 pending token 后，
// 二次验证必须能真正完成并签发 TwoFA=true 的完整 access（修复前恒 401）。
func TestTwoFAVerifyRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	uuid, _ := util.NewUUID()
	subToken, _ := util.RandomHex(32)
	user := models.User{
		Username:       "otpuser@example.com",
		UUID:           uuid,
		Email:          "otpuser@example.com",
		SubscribeToken: subToken,
		Role:           models.RoleUser,
		Status:         models.StatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	cfg := &config.Config{JWT: config.JWT{Secret: "test-secret-at-least-32-chars-long-123456", AccessTTL: 2 * time.Hour, RefreshTTL: 7 * 24 * time.Hour}}
	jwt := services.NewJWTManager(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	otpSvc := services.NewOTPService(db, cfg)

	// 通过服务启用 TOTP（Confirm 会生成真实 secret 并写入加密字段）
	secret, _, err := otpSvc.Setup(user.ID, user.Email)
	if err != nil {
		t.Fatalf("otp setup: %v", err)
	}
	confirmCode, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate confirm code: %v", err)
	}
	if _, err := otpSvc.Confirm(user.ID, secret, confirmCode); err != nil {
		t.Fatalf("otp confirm: %v", err)
	}
	db.First(&user, user.ID)

	d := &Deps{
		DB:    db,
		Cfg:   cfg,
		JWT:   jwt,
		OTP:   otpSvc,
		Audit: &services.AuditService{DB: db},
	}

	r := gin.New()
	group := r.Group("/api/v1/auth")
	group.POST("/2fa/verify", middleware.AuthPending2FA(jwt, db), d.TwoFAVerify)

	// 未携带 pending cookie：必须 401（修复前这里 handler 也会 401，但原因是 claims 缺失）
	pending, _ := jwt.GeneratePending2FA(user.ID, user.Role, user.TokenVersion)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/2fa/verify", strings.NewReader(`{"code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no pending cookie = %d, want 401", w.Code)
	}

	// 普通完整 access 也不应调用 verify
	verified, _ := jwt.GenerateVerified(user.ID, user.Role, user.TokenVersion)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/2fa/verify", strings.NewReader(`{"code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "access_token", Value: verified})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("non-pending access on verify = %d, want 401", w.Code)
	}

	// 正确流程：pending cookie + 有效 TOTP 码 → 200，并签发 TwoFA=true 的 access
	goodCode, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate good code: %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/2fa/verify", strings.NewReader(`{"code":"`+goodCode+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "access_token", Value: pending})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("2fa verify = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	var accessCookie string
	for _, c := range w.Result().Cookies() {
		if c.Name == "access_token" {
			accessCookie = c.Value
		}
	}
	if accessCookie == "" {
		t.Fatal("verify 成功后未签发 access_token cookie")
	}
	claims, err := jwt.Parse(accessCookie)
	if err != nil {
		t.Fatalf("parse issued access: %v", err)
	}
	if claims.Pending || !claims.TwoFA {
		t.Fatalf("issued access should be TwoFA=true and Pending=false, got TwoFA=%v Pending=%v", claims.TwoFA, claims.Pending)
	}

	// 错误验证码 → 400（不得换发令牌）
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/2fa/verify", strings.NewReader(`{"code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "access_token", Value: pending})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("wrong code = %d, want 400, body=%s", w.Code, w.Body.String())
	}
}
