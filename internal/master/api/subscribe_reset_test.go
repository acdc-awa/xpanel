package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/api"
	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// TestUserAndAdminResetSubscribe 测试用户端与管理端重置订阅时同时轮换 token 与 UUID。
func TestUserAndAdminResetSubscribe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AuditLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	origUUID, _ := util.NewUUID()
	origToken, _ := util.RandomHex(32)
	user := models.User{
		Username:       "user_sub@example.com",
		UUID:           origUUID,
		Email:          "user_sub@example.com",
		SubscribeToken: origToken,
		Role:           models.RoleUser,
		Status:         models.StatusActive,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	cfg := &config.Config{JWT: config.JWT{Secret: "test-secret-at-least-32-chars-long-123456", AccessTTL: 2 * time.Hour, RefreshTTL: 7 * 24 * time.Hour}}
	jwt := services.NewJWTManager(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	authSvc := &services.AuthService{DB: db, JWT: jwt}
	auditSvc := &services.AuditService{DB: db}

	deps := &api.Deps{
		DB:    db,
		Cfg:   cfg,
		JWT:   jwt,
		Auth:  authSvc,
		Audit: auditSvc,
	}

	// 1. 用户端重置测试
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/user/subscribe/reset", nil)
	c.Set(middleware.CtxClaimsKey, &contracts.JWTClaims{UserID: user.ID, Role: models.RoleUser})
	deps.UserResetSubscribe(c)

	if w.Code != http.StatusOK {
		t.Fatalf("UserResetSubscribe status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			SubscribeToken string `json:"subscribe_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal resp: %v", err)
	}
	if resp.Data.SubscribeToken == "" || resp.Data.SubscribeToken == origToken {
		t.Fatalf("新 token 不应为空且必须与旧 token 不同: got %q", resp.Data.SubscribeToken)
	}

	var userAfterUserReset models.User
	if err := db.First(&userAfterUserReset, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if userAfterUserReset.UUID == "" || userAfterUserReset.UUID == origUUID {
		t.Fatalf("用户端重置后 UUID 未改变: got %q", userAfterUserReset.UUID)
	}
	if userAfterUserReset.SubscribeToken != resp.Data.SubscribeToken {
		t.Fatalf("数据库 token 与响应不符: got %q, want %q", userAfterUserReset.SubscribeToken, resp.Data.SubscribeToken)
	}

	var audit models.AuditLog
	if err := db.Where("operator_type = ? AND operator_id = ? AND action = ?", "user", user.ID, "subscribe.reset_token").First(&audit).Error; err != nil {
		t.Fatalf("应记录用户重置订阅审计日志: %v", err)
	}
	if audit.Detail != "重置订阅地址与连接凭据" {
		t.Fatalf("审计日志详情不符: got %q, want %q", audit.Detail, "重置订阅地址与连接凭据")
	}

	midUUID := userAfterUserReset.UUID
	midToken := userAfterUserReset.SubscribeToken

	// 2. 管理端重置测试
	wAdmin := httptest.NewRecorder()
	cAdmin, _ := gin.CreateTestContext(wAdmin)
	cAdmin.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/users/%d/subscribe-token/reset", user.ID), nil)
	cAdmin.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", user.ID)}}
	cAdmin.Set(middleware.CtxClaimsKey, &contracts.JWTClaims{UserID: 999, Role: models.RoleAdmin})
	deps.AdminResetUserSubscribeToken(cAdmin)

	if wAdmin.Code != http.StatusOK {
		t.Fatalf("AdminResetUserSubscribeToken status = %d, want 200: %s", wAdmin.Code, wAdmin.Body.String())
	}
	var respAdmin struct {
		Code int `json:"code"`
		Data struct {
			ID             uint64 `json:"id"`
			SubscribeToken string `json:"subscribe_token"`
			UUID           string `json:"uuid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(wAdmin.Body.Bytes(), &respAdmin); err != nil {
		t.Fatalf("unmarshal respAdmin: %v", err)
	}
	if respAdmin.Data.SubscribeToken == "" || respAdmin.Data.SubscribeToken == midToken {
		t.Fatalf("管理端重置后 token 必须与上次不同: got %q", respAdmin.Data.SubscribeToken)
	}
	if respAdmin.Data.UUID == "" || respAdmin.Data.UUID == midUUID {
		t.Fatalf("管理端重置后 UUID 必须与上次不同: got %q", respAdmin.Data.UUID)
	}

	var userAfterAdminReset models.User
	if err := db.First(&userAfterAdminReset, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if userAfterAdminReset.UUID != respAdmin.Data.UUID {
		t.Fatalf("数据库 UUID 与管理端响应不符: got %q, want %q", userAfterAdminReset.UUID, respAdmin.Data.UUID)
	}
	if userAfterAdminReset.SubscribeToken != respAdmin.Data.SubscribeToken {
		t.Fatalf("数据库 token 与管理端响应不符: got %q, want %q", userAfterAdminReset.SubscribeToken, respAdmin.Data.SubscribeToken)
	}
}
