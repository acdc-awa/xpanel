package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

func setupReservedOutboundsTest(t *testing.T) (*Deps, *gin.Engine, *models.Server) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	jwtMgr := services.NewJWTManager("test-secret-key-32-chars-long!", 0, 0)
	authSvc := &services.AuthService{DB: db, JWT: jwtMgr}
	cfg := &config.Config{}
	siteSvc := services.NewSiteService(db, cfg)

	deps := &Deps{
		DB:   db,
		Cfg:  cfg,
		JWT:  jwtMgr,
		Auth: authSvc,
		Site: siteSvc,
	}

	server := &models.Server{
		Name:   "US-Node-Test",
		Host:   "us01.example.com",
		NodeID: "node-test-01",
		Secret: "sec_test_123",
	}
	if err := db.Create(server).Error; err != nil {
		t.Fatalf("create server: %v", err)
	}

	r := gin.New()
	r.GET("/api/v1/admin/servers/:id/outbounds", deps.AdminGetServerOutbounds)
	r.POST("/api/v1/admin/servers/:id/outbounds", deps.AdminCreateServerOutbound)
	r.PUT("/api/v1/admin/servers/:id/outbounds/:outbound_id", deps.AdminUpdateServerOutbound)
	r.DELETE("/api/v1/admin/servers/:id/outbounds/:outbound_id", deps.AdminDeleteServerOutbound)

	return deps, r, server
}

func TestEnsureDefaultServerOutbounds(t *testing.T) {
	deps, _, server := setupReservedOutboundsTest(t)

	// 1. 首次触发 Ensure
	EnsureDefaultServerOutbounds(deps.DB, server.ID)

	var list []models.ServerOutbound
	deps.DB.Where("server_id = ?", server.ID).Find(&list)
	if len(list) != 2 {
		t.Fatalf("expected 2 default outbounds, got %d", len(list))
	}

	hasDirect := false
	hasBlocked := false
	for _, o := range list {
		if o.Tag == "direct" && o.Protocol == "freedom" {
			hasDirect = true
		}
		if o.Tag == "blocked" && o.Protocol == "blackhole" {
			hasBlocked = true
		}
	}
	if !hasDirect || !hasBlocked {
		t.Errorf("missing direct or blocked: direct=%v, blocked=%v", hasDirect, hasBlocked)
	}

	// 2. 插入重复的 direct 记录，验证 Ensure 自动去重
	deps.DB.Create(&models.ServerOutbound{
		ServerID: server.ID,
		Tag:      "direct",
		Protocol: "freedom",
	})
	EnsureDefaultServerOutbounds(deps.DB, server.ID)

	var listAfter []models.ServerOutbound
	deps.DB.Where("server_id = ? AND tag = ?", server.ID, "direct").Find(&listAfter)
	if len(listAfter) != 1 {
		t.Errorf("expected 1 direct after dedup, got %d", len(listAfter))
	}
}

func TestReservedOutbounds_RejectCreateAndDelete(t *testing.T) {
	deps, r, server := setupReservedOutboundsTest(t)
	EnsureDefaultServerOutbounds(deps.DB, server.ID)

	var directOb models.ServerOutbound
	deps.DB.Where("server_id = ? AND tag = ?", server.ID, "direct").First(&directOb)

	// 1. 测试禁止创建 tag=direct
	body, _ := json.Marshal(map[string]any{
		"tag":      "direct",
		"protocol": "freedom",
	})
	req := httptest.NewRequest("POST", "/api/v1/admin/servers/"+strconv.FormatUint(server.ID, 10)+"/outbounds", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("creating reserved tag should be 400, got %d: %s", w.Code, w.Body.String())
	}

	// 2. 测试禁止删除 direct 出站
	reqDel := httptest.NewRequest("DELETE", "/api/v1/admin/servers/"+strconv.FormatUint(server.ID, 10)+"/outbounds/"+strconv.FormatUint(directOb.ID, 10), nil)
	wDel := httptest.NewRecorder()
	r.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusBadRequest {
		t.Errorf("deleting reserved outbound should be 400, got %d: %s", wDel.Code, wDel.Body.String())
	}

	// 3. 测试禁止修改 direct 出站的 Tag
	bodyUpd, _ := json.Marshal(map[string]any{
		"tag": "my-direct-renamed",
	})
	reqUpd := httptest.NewRequest("PUT", "/api/v1/admin/servers/"+strconv.FormatUint(server.ID, 10)+"/outbounds/"+strconv.FormatUint(directOb.ID, 10), bytes.NewReader(bodyUpd))
	reqUpd.Header.Set("Content-Type", "application/json")
	wUpd := httptest.NewRecorder()
	r.ServeHTTP(wUpd, reqUpd)
	if wUpd.Code != http.StatusBadRequest {
		t.Errorf("renaming reserved tag should be 400, got %d: %s", wUpd.Code, wUpd.Body.String())
	}
}
