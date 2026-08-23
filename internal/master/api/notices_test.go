package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

func setupNoticesTest(t *testing.T) (*gin.Engine, *gorm.DB, *Deps, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite memory db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	cfg := &config.Config{
		JWT: config.JWT{
			Secret:     "notices-test-secret-key-32bytes-123456",
			AccessTTL:  2 * time.Hour,
			RefreshTTL: 7 * 24 * time.Hour,
		},
	}
	jwt := services.NewJWTManager(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	auditSvc := &services.AuditService{DB: db}
	deps := &Deps{
		DB:    db,
		Cfg:   cfg,
		JWT:   jwt,
		Audit: auditSvc,
	}

	u1, _ := util.NewUUID()
	u2, _ := util.NewUUID()
	sub1, _ := util.RandomHex(32)
	sub2, _ := util.RandomHex(32)
	adminUser := models.User{Username: "admin@test.com", UUID: u1, Email: "admin@test.com", SubscribeToken: sub1, Role: models.RoleAdmin, Status: models.StatusActive}
	normalUser := models.User{Username: "user@test.com", UUID: u2, Email: "user@test.com", SubscribeToken: sub2, Role: models.RoleUser, Status: models.StatusActive}
	db.Create(&adminUser)
	db.Create(&normalUser)

	adminToken, _ := jwt.GenerateVerified(adminUser.ID, adminUser.Role, adminUser.TokenVersion)
	userToken, _ := jwt.GenerateVerified(normalUser.ID, normalUser.Role, normalUser.TokenVersion)

	r := gin.New()
	adminGroup := r.Group("/api/v1/admin", middleware.AuthRequired(jwt, db), middleware.RequireRole("admin"))
	{
		adminGroup.GET("/notices", deps.AdminListNotices)
		adminGroup.POST("/notices", deps.AdminCreateNotice)
		adminGroup.PUT("/notices/:id", deps.AdminUpdateNotice)
		adminGroup.DELETE("/notices/:id", deps.AdminDeleteNotice)
		adminGroup.POST("/notices/:id/toggle", deps.AdminToggleNotice)
	}
	userGroup := r.Group("/api/v1/user", middleware.AuthRequired(jwt, db))
	{
		userGroup.GET("/notices", deps.UserListNotices)
		userGroup.GET("/notices/:id", deps.UserGetNotice)
	}

	return r, db, deps, adminToken, userToken
}

func TestNotice_AdminCRUD_And_UserQuery(t *testing.T) {
	r, db, _, adminToken, userToken := setupNoticesTest(t)

	// 1. 管理端创建两条公告
	body1, _ := json.Marshal(map[string]any{
		"title":     "欢迎使用 Xray 面板",
		"content":   "这是普通公告内容",
		"is_pinned": false,
		"is_popup":  false,
		"status":    1,
	})
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/notices", bytes.NewReader(body1))
	req1.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("create notice 1 failed: code=%d body=%s", w1.Code, w1.Body.String())
	}

	var resp1 struct {
		Code int           `json:"code"`
		Data models.Notice `json:"data"`
	}
	json.Unmarshal(w1.Body.Bytes(), &resp1)
	n1ID := resp1.Data.ID

	// 创建第 2 条：置顶 + 弹窗
	body2, _ := json.Marshal(map[string]any{
		"title":     "重要维护通知",
		"content":   "今晚 12 点进行节点升级维护",
		"is_pinned": true,
		"is_popup":  true,
		"status":    1,
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/notices", bytes.NewReader(body2))
	req2.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("create notice 2 failed: code=%d body=%s", w2.Code, w2.Body.String())
	}

	// 2. 用户端查询公告列表（置顶排在最前）
	reqU := httptest.NewRequest(http.MethodGet, "/api/v1/user/notices", nil)
	reqU.AddCookie(&http.Cookie{Name: "access_token", Value: userToken})
	wU := httptest.NewRecorder()
	r.ServeHTTP(wU, reqU)
	if wU.Code != http.StatusOK {
		t.Fatalf("user list notices failed: code=%d body=%s", wU.Code, wU.Body.String())
	}
	var uListResp struct {
		Code int             `json:"code"`
		Data []models.Notice `json:"data"`
	}
	json.Unmarshal(wU.Body.Bytes(), &uListResp)
	if len(uListResp.Data) != 2 {
		t.Fatalf("want 2 notices, got %d", len(uListResp.Data))
	}
	// 置顶必须排在第一位
	if uListResp.Data[0].Title != "重要维护通知" || !uListResp.Data[0].IsPinned || !uListResp.Data[0].IsPopup {
		t.Errorf("first notice should be pinned: %+v", uListResp.Data[0])
	}

	// 3. 管理端切换公告 1 为隐藏 (toggle status)
	toggleBody, _ := json.Marshal(map[string]any{
		"field": "status",
	})
	reqT := httptest.NewRequest(http.MethodPost, "/api/v1/admin/notices/"+strconv.FormatUint(n1ID, 10)+"/toggle", bytes.NewReader(toggleBody))
	reqT.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	reqT.Header.Set("Content-Type", "application/json")
	wT := httptest.NewRecorder()
	r.ServeHTTP(wT, reqT)
	if wT.Code != http.StatusOK {
		t.Fatalf("toggle status failed: code=%d body=%s", wT.Code, wT.Body.String())
	}

	// 用户端再次查询，只剩一条启用的公告
	wU2 := httptest.NewRecorder()
	reqU2 := httptest.NewRequest(http.MethodGet, "/api/v1/user/notices", nil)
	reqU2.AddCookie(&http.Cookie{Name: "access_token", Value: userToken})
	r.ServeHTTP(wU2, reqU2)
	var uListResp2 struct {
		Data []models.Notice `json:"data"`
	}
	json.Unmarshal(wU2.Body.Bytes(), &uListResp2)
	if len(uListResp2.Data) != 1 {
		t.Fatalf("want 1 active notice, got %d", len(uListResp2.Data))
	}

	// 4. 管理端删除公告 1
	reqD := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/notices/"+strconv.FormatUint(n1ID, 10), nil)
	reqD.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	wD := httptest.NewRecorder()
	r.ServeHTTP(wD, reqD)
	if wD.Code != http.StatusOK {
		t.Fatalf("delete notice failed: code=%d", wD.Code)
	}

	var count int64
	db.Model(&models.Notice{}).Count(&count)
	if count != 1 {
		t.Fatalf("want 1 notice remaining in DB, got %d", count)
	}
}
