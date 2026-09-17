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
	siteSvc := services.NewSiteService(db)

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
			// 种子 direct 必须带模板同款 finalRules：同 tag 出站在生成时整体覆盖模板，
			// 只写 domainStrategy 会让模板的内网段出站级兜底规则消失，
			// 且出站编辑器从 finalRules 反推「屏蔽内网私有 IP」开关，会误显示为关闭。
			var settings map[string]any
			if err := json.Unmarshal([]byte(o.SettingsJSON), &settings); err != nil {
				t.Fatalf("解析种子 direct settings 失败: %v", err)
			}
			if ds, _ := settings["domainStrategy"].(string); ds != "AsIs" {
				t.Errorf("种子 direct domainStrategy = %q, 期望 AsIs", ds)
			}
			rules, _ := settings["finalRules"].([]any)
			if len(rules) == 0 {
				t.Error("种子 direct 缺少 finalRules：模板的私网拦截规则会丢失")
			}
			// 私网 block 必须显式 blockDelay=0：路由层已不再注入私网规则，
			// 缺省会退化为官方默认的 30-90s 黑洞挂起（原先由路由层 blocked 出站立即断开）
			first, _ := rules[0].(map[string]any)
			if action, _ := first["action"].(string); action != "block" {
				t.Errorf("种子 direct finalRules[0] 应为 block: %+v", first)
			}
			if delay, _ := first["blockDelay"].(string); delay != "0" {
				t.Errorf("种子 direct 私网 block 应显式 blockDelay=0, got %+v", first)
			}
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
