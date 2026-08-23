package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/master/services"
	"github.com/acdc/xray-panel/internal/models"
	"github.com/acdc/xray-panel/internal/pkg/util"
)

// newAuthTestEnv 构造带一个真实用户的测试环境与 /ok（AuthRequired）和 /verify（AuthPending2FA）路由。
func newAuthTestEnv(t *testing.T) (*gin.Engine, *services.JWTManager, *gorm.DB, models.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("migrate user: %v", err)
	}
	uuid, _ := util.NewUUID()
	token, _ := util.RandomHex(32)
	user := models.User{
		Username:       "alice@example.com",
		UUID:           uuid,
		Email:          "alice@example.com",
		SubscribeToken: token,
		Role:           models.RoleUser,
		Status:         models.StatusActive,
		TokenVersion:   1,
		TotpEnabled:    false,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	jwt := services.NewJWTManager("test-secret-at-least-32-chars-long-123456", 2*time.Hour, 7*24*time.Hour)
	r := gin.New()
	r.GET("/ok", AuthRequired(jwt, db), func(c *gin.Context) { c.Status(http.StatusOK) })
	r.POST("/verify", AuthPending2FA(jwt, db), func(c *gin.Context) { c.Status(http.StatusOK) })
	return r, jwt, db, user
}

func authRequest(r *gin.Engine, method, path, cookie string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: cookie})
	}
	r.ServeHTTP(w, req)
	return w
}

func TestAuthRequiredRejectsPendingToken(t *testing.T) {
	r, jwt, _, user := newAuthTestEnv(t)
	pending, _ := jwt.GeneratePending2FA(user.ID, user.Role, user.TokenVersion)
	if w := authRequest(r, http.MethodGet, "/ok", pending); w.Code != http.StatusUnauthorized {
		t.Fatalf("pending token on protected route = %d, want 401", w.Code)
	}
}

func TestAuthRequiredAcceptsActiveUser(t *testing.T) {
	r, jwt, _, user := newAuthTestEnv(t)
	access, _ := jwt.Generate(user.ID, user.Role, services.TokenAccess, user.TokenVersion)
	if w := authRequest(r, http.MethodGet, "/ok", access); w.Code != http.StatusOK {
		t.Fatalf("valid access = %d, want 200", w.Code)
	}
}

func TestAuthRequiredRejectsDisabledUser(t *testing.T) {
	r, jwt, db, user := newAuthTestEnv(t)
	access, _ := jwt.Generate(user.ID, user.Role, services.TokenAccess, user.TokenVersion)
	db.Model(&user).Update("status", models.StatusDisabled)
	if w := authRequest(r, http.MethodGet, "/ok", access); w.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user access = %d, want 401", w.Code)
	}
}

func TestAuthRequiredRejectsStaleTokenVersion(t *testing.T) {
	r, jwt, db, user := newAuthTestEnv(t)
	access, _ := jwt.Generate(user.ID, user.Role, services.TokenAccess, user.TokenVersion)
	db.Model(&user).Update("token_version", gorm.Expr("token_version + 1"))
	if w := authRequest(r, http.MethodGet, "/ok", access); w.Code != http.StatusUnauthorized {
		t.Fatalf("stale version access = %d, want 401", w.Code)
	}
}

func TestAuthRequiredRejectsChangedRole(t *testing.T) {
	r, jwt, db, user := newAuthTestEnv(t)
	access, _ := jwt.Generate(user.ID, models.RoleAdmin, services.TokenAccess, user.TokenVersion)
	db.Model(&user).Update("role", models.RoleUser)
	if w := authRequest(r, http.MethodGet, "/ok", access); w.Code != http.StatusUnauthorized {
		t.Fatalf("role changed access = %d, want 401", w.Code)
	}
}

func TestAuthRequiredRejectsNonVerifiedTokenForTOTPUser(t *testing.T) {
	r, jwt, db, user := newAuthTestEnv(t)
	db.Model(&user).Update("totp_enabled", true)
	access, _ := jwt.Generate(user.ID, user.Role, services.TokenAccess, user.TokenVersion)
	if w := authRequest(r, http.MethodGet, "/ok", access); w.Code != http.StatusUnauthorized {
		t.Fatalf("non-verified access for TOTP user = %d, want 401", w.Code)
	}
}

func TestAuthRequiredAcceptsVerifiedTokenForTOTPUser(t *testing.T) {
	r, jwt, db, user := newAuthTestEnv(t)
	db.Model(&user).Update("totp_enabled", true)
	access, _ := jwt.GenerateVerified(user.ID, user.Role, user.TokenVersion)
	if w := authRequest(r, http.MethodGet, "/ok", access); w.Code != http.StatusOK {
		t.Fatalf("verified access for TOTP user = %d, want 200", w.Code)
	}
}

func TestAuthPending2FAPolicy(t *testing.T) {
	r, jwt, db, user := newAuthTestEnv(t)
	db.Model(&user).Update("totp_enabled", true)
	pending, _ := jwt.GeneratePending2FA(user.ID, user.Role, user.TokenVersion)

	// pending token 只应被 verify 路由放行
	if w := authRequest(r, http.MethodPost, "/verify", pending); w.Code != http.StatusOK {
		t.Fatalf("pending on verify = %d, want 200", w.Code)
	}
	// 普通 access 不应调用 verify（必须重新走密码登录）
	access, _ := jwt.GenerateVerified(user.ID, user.Role, user.TokenVersion)
	if w := authRequest(r, http.MethodPost, "/verify", access); w.Code != http.StatusUnauthorized {
		t.Fatalf("verified access on verify = %d, want 401", w.Code)
	}
	// 未开 TOTP 的 pending token 不放行
	uuid2, _ := util.NewUUID()
	token2, _ := util.RandomHex(32)
	user2 := models.User{
		Username:       "bob@example.com",
		UUID:           uuid2,
		Email:          "bob@example.com",
		SubscribeToken: token2,
		Role:           models.RoleUser,
		Status:         models.StatusActive,
	}
	if err := db.Create(&user2).Error; err != nil {
		t.Fatalf("create user2: %v", err)
	}
	pending2, _ := jwt.GeneratePending2FA(user2.ID, user2.Role, user2.TokenVersion)
	if w := authRequest(r, http.MethodPost, "/verify", pending2); w.Code != http.StatusBadRequest {
		t.Fatalf("pending for non-TOTP user on verify = %d, want 400", w.Code)
	}
}
