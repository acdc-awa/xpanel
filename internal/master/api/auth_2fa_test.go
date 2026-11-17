package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
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
	otpSvc := services.NewOTPService(db, cfg, cfg.JWT.Secret)

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

// TestTwoFAVerifyFifthAttemptWithBackupCode 回归：连续 4 次输错 TOTP 后第 5 次提交恢复码必须成功登录，
// 且 Login 签发 2FA pending cookie 时同时清除遗留的 refresh_token。
func TestTwoFAVerifyFifthAttemptWithBackupCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AuditLog{}, &models.Setting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pwdHash, err := argon2id.CreateHash("Passw0rd!123", argon2id.DefaultParams)
	if err != nil {
		t.Fatal(err)
	}
	uuid, _ := util.NewUUID()
	subToken, _ := util.RandomHex(32)
	user := models.User{
		UUID:           uuid,
		Email:          "backup-http@example.com",
		PasswordHash:   pwdHash,
		SubscribeToken: subToken,
		Role:           models.RoleUser,
		Status:         models.StatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{JWT: config.JWT{Secret: "test-secret-at-least-32-chars-long-123456", AccessTTL: 2 * time.Hour, RefreshTTL: 7 * 24 * time.Hour}}
	jwt := services.NewJWTManager(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	otpSvc := services.NewOTPService(db, cfg, cfg.JWT.Secret)
	authSvc := &services.AuthService{DB: db, JWT: jwt}

	secret, _, err := otpSvc.Setup(user.ID, user.Email)
	if err != nil {
		t.Fatal(err)
	}
	confirmCode, _ := totp.GenerateCode(secret, time.Now())
	backupCodes, err := otpSvc.Confirm(user.ID, secret, confirmCode)
	if err != nil {
		t.Fatal(err)
	}
	db.First(&user, user.ID)

	d := &Deps{
		DB:    db,
		Cfg:   cfg,
		JWT:   jwt,
		Auth:  authSvc,
		OTP:   otpSvc,
		Audit: &services.AuditService{DB: db},
	}
	r := gin.New()
	r.POST("/api/v1/auth/login", d.Login)
	r.POST("/api/v1/auth/2fa/verify", middleware.AuthPending2FA(jwt, db), d.TwoFAVerify)

	// 1. Login 返回 twofa_required，并主动失效旧 refresh_token Cookie
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"backup-http@example.com","password":"Passw0rd!123"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var pending string
	var clearedRefresh bool
	for _, ck := range w.Result().Cookies() {
		if ck.Name == "access_token" {
			pending = ck.Value
		}
		if ck.Name == "refresh_token" && ck.MaxAge < 0 {
			clearedRefresh = true
		}
	}
	if pending == "" || !clearedRefresh {
		t.Fatalf("2FA pending step 应下发 pending access 且清除旧 refresh_token，pending=%v clearedRefresh=%v", pending != "", clearedRefresh)
	}

	// 2. 连错 4 次
	for i := 0; i < 4; i++ {
		vr := httptest.NewRequest(http.MethodPost, "/api/v1/auth/2fa/verify", strings.NewReader(`{"code":"000000"}`))
		vr.Header.Set("Content-Type", "application/json")
		vr.AddCookie(&http.Cookie{Name: "access_token", Value: pending})
		vw := httptest.NewRecorder()
		r.ServeHTTP(vw, vr)
		if vw.Code != http.StatusBadRequest {
			t.Fatalf("错码第 %d 次 = %d, want 400", i+1, vw.Code)
		}
	}

	// 3. 第 5 次传有效恢复码 → 必须 200 完成登录
	vr := httptest.NewRequest(http.MethodPost, "/api/v1/auth/2fa/verify", strings.NewReader(`{"code":"`+backupCodes[0]+`"}`))
	vr.Header.Set("Content-Type", "application/json")
	vr.AddCookie(&http.Cookie{Name: "access_token", Value: pending})
	vw := httptest.NewRecorder()
	r.ServeHTTP(vw, vr)
	if vw.Code != http.StatusOK {
		t.Fatalf("第 5 次提交有效恢复码 = %d, want 200, body=%s", vw.Code, vw.Body.String())
	}
}
