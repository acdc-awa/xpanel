package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func forwardUsersTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Server{}, &models.Inbound{}, &models.PermissionGroup{}, &models.ForwardUser{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func setupForwardUserRouter(d *Deps) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/v1/admin/forward-users")
	{
		g.GET("", d.AdminListForwardUsers)
		g.GET("/summary", d.AdminForwardUsersSummary)
		g.POST("", d.AdminCreateForwardUser)
		g.PUT("/:id", d.AdminUpdateForwardUser)
		g.POST("/:id/toggle", d.AdminToggleForwardUser)
		g.POST("/:id/reset-traffic", d.AdminResetForwardUserTraffic)
		g.DELETE("/:id", d.AdminDeleteForwardUser)
	}
	return r
}

func TestForwardUsersCRUD(t *testing.T) {
	db := forwardUsersTestDB(t)
	d := &Deps{DB: db}
	r := setupForwardUserRouter(d)

	// Seed permission group
	pg := models.PermissionGroup{Name: "转发测试组"}
	if err := db.Create(&pg).Error; err != nil {
		t.Fatalf("create pg: %v", err)
	}

	// 1. Create ForwardUser
	createBody := map[string]any{
		"username":            "user_socks1",
		"password":            "pass123456",
		"permission_group_id": pg.ID,
		"traffic_limit_gb":    50,
		"traffic_reset":       "monthly",
		"remark":              "测试账号",
	}
	bodyBytes, _ := json.Marshal(createBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/forward-users", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create forward user failed: code=%d, body=%s", w.Code, w.Body.String())
	}

	// Duplicate create should fail
	reqDup := httptest.NewRequest(http.MethodPost, "/api/v1/admin/forward-users", bytes.NewReader(bodyBytes))
	reqDup.Header.Set("Content-Type", "application/json")
	wDup := httptest.NewRecorder()
	r.ServeHTTP(wDup, reqDup)
	if wDup.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for duplicate username, got %d", wDup.Code)
	}

	// 2. List ForwardUsers
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/admin/forward-users", nil)
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("list forward users failed: %d", wList.Code)
	}
	var listResp struct {
		Code int `json:"code"`
		Data struct {
			Items []forwardUserView `json:"items"`
			Total int               `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(wList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if listResp.Data.Total != 1 || len(listResp.Data.Items) != 1 {
		t.Fatalf("expected 1 user, got total=%d items=%d", listResp.Data.Total, len(listResp.Data.Items))
	}
	u := listResp.Data.Items[0]
	if u.Username != "user_socks1" || u.GroupName != "转发测试组" {
		t.Fatalf("unexpected user view: %+v", u)
	}

	// 3. Summary
	reqSum := httptest.NewRequest(http.MethodGet, "/api/v1/admin/forward-users/summary", nil)
	wSum := httptest.NewRecorder()
	r.ServeHTTP(wSum, reqSum)
	if wSum.Code != http.StatusOK {
		t.Fatalf("summary failed: %d", wSum.Code)
	}

	// 4. Update
	newLimit := int64(100)
	updateBody := map[string]any{
		"traffic_limit_gb": newLimit,
		"remark":           "已更新备注",
	}
	upBytes, _ := json.Marshal(updateBody)
	reqUp := httptest.NewRequest(http.MethodPut, "/api/v1/admin/forward-users/1", bytes.NewReader(upBytes))
	reqUp.Header.Set("Content-Type", "application/json")
	wUp := httptest.NewRecorder()
	r.ServeHTTP(wUp, reqUp)
	if wUp.Code != http.StatusOK {
		t.Fatalf("update failed: code=%d body=%s", wUp.Code, wUp.Body.String())
	}

	var dbUser models.ForwardUser
	db.First(&dbUser, 1)
	if dbUser.TrafficLimitGB != 100 || dbUser.Remark != "已更新备注" {
		t.Fatalf("update not persisted in DB: %+v", dbUser)
	}

	// 5. Toggle
	reqToggle := httptest.NewRequest(http.MethodPost, "/api/v1/admin/forward-users/1/toggle", nil)
	wToggle := httptest.NewRecorder()
	r.ServeHTTP(wToggle, reqToggle)
	if wToggle.Code != http.StatusOK {
		t.Fatalf("toggle failed: %d", wToggle.Code)
	}
	db.First(&dbUser, 1)
	if dbUser.Enabled {
		t.Fatalf("expected enabled=false after toggle, got true")
	}

	// 6. Reset Traffic
	db.Model(&dbUser).Update("traffic_used_bytes", 12345678)
	reqReset := httptest.NewRequest(http.MethodPost, "/api/v1/admin/forward-users/1/reset-traffic", nil)
	wReset := httptest.NewRecorder()
	r.ServeHTTP(wReset, reqReset)
	if wReset.Code != http.StatusOK {
		t.Fatalf("reset traffic failed: %d", wReset.Code)
	}
	db.First(&dbUser, 1)
	if dbUser.TrafficUsedBytes != 0 || !dbUser.Enabled {
		t.Fatalf("expected traffic=0 and enabled=true after reset, got %+v", dbUser)
	}

	// 7. Delete
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/forward-users/1", nil)
	wDel := httptest.NewRecorder()
	r.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("delete failed: %d", wDel.Code)
	}
	var cnt int64
	db.Model(&models.ForwardUser{}).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("expected 0 forward users after delete, got %d", cnt)
	}
}

func TestAdminSyncForwardUsersConfigs(t *testing.T) {
	db := apiTestDB(t)
	d := &Deps{DB: db}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/forward-users/sync", d.AdminSyncForwardUsersConfigs)

	// 创建带 SOCKS5 入站的服务器
	db.Create(&models.Server{ID: 10, Name: "node-10", Host: "1.2.3.4", NodeID: "n10", Secret: "sec"})
	db.Create(&models.Inbound{
		ServerID: 10,
		Tag:      "socks-10",
		Protocol: "socks",
		Port:     1080,
		Enabled:  true,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/forward-users/sync", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("sync failed: %d, body: %s", w.Code, w.Body.String())
	}

	var res struct {
		Code int `json:"code"`
		Data struct {
			SyncedServers int `json:"synced_servers"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if res.Data.SyncedServers != 1 {
		t.Fatalf("expected 1 synced server, got %d", res.Data.SyncedServers)
	}
}
