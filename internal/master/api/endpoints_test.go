package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/models"
	"github.com/acdc/xray-panel/internal/master/services"
)

func setupEndpointTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Open sqlite: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func TestAdminInboundEndpoints_CRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupEndpointTestDB(t)

	srv := models.Server{Name: "香港01", Host: "hk.node.com", NodeID: "node-1", Secret: "sec-1", Status: 1}
	db.Create(&srv)
	inb := models.Inbound{ServerID: srv.ID, Tag: "hk-vless", Protocol: "vless", Port: 443, Enabled: true, Type: "user"}
	db.Create(&inb)
	group1 := models.PermissionGroup{Name: "VIP组"}
	group2 := models.PermissionGroup{Name: "企业组"}
	db.Create(&group1)
	db.Create(&group2)

	deps := &Deps{DB: db}
	r := gin.New()
	r.GET("/api/v1/admin/inbounds/:id/endpoints", deps.AdminGetInboundEndpoints)
	r.POST("/api/v1/admin/inbounds/:id/endpoints", deps.AdminCreateInboundEndpoint)
	r.PUT("/api/v1/admin/inbounds/:id/endpoints/:ep_id", deps.AdminUpdateInboundEndpoint)
	r.DELETE("/api/v1/admin/inbounds/:id/endpoints/:ep_id", deps.AdminDeleteInboundEndpoint)

	// 1. Create Endpoint
	body := map[string]any{
		"name":                 "广州 BGP 中转",
		"host":                 "gz.bgp.com",
		"port":                 30001,
		"permission_group_ids": []uint64{group1.ID},
		"enabled":              true,
		"priority":             1,
		"remark":               "移动优化线路",
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/admin/inbounds/1/endpoints", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("POST endpoint status: %d, body: %s", w.Code, w.Body.String())
	}
	var createdRes struct {
		Code int `json:"code"`
		Data struct {
			Endpoint InboundEndpointView `json:"endpoint"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &createdRes)
	epID := createdRes.Data.Endpoint.ID
	if epID == 0 || createdRes.Data.Endpoint.Host != "gz.bgp.com" {
		t.Fatalf("unexpected created endpoint: %+v", createdRes.Data.Endpoint)
	}
	if len(createdRes.Data.Endpoint.PermissionGroupIDs) != 1 || createdRes.Data.Endpoint.PermissionGroupIDs[0] != group1.ID {
		t.Fatalf("unexpected permission groups: %+v", createdRes.Data.Endpoint.PermissionGroupIDs)
	}

	// 2. Get Endpoints
	req = httptest.NewRequest("GET", "/api/v1/admin/inbounds/1/endpoints", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("GET endpoints status: %d", w.Code)
	}
	var listRes struct {
		Data struct {
			Items []InboundEndpointView `json:"items"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listRes)
	if len(listRes.Data.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(listRes.Data.Items))
	}

	// 3. Update Endpoint
	newPort := 30002
	upBody := map[string]any{
		"port":                 newPort,
		"permission_group_ids": []uint64{group1.ID, group2.ID},
	}
	upRaw, _ := json.Marshal(upBody)
	req = httptest.NewRequest("PUT", "/api/v1/admin/inbounds/1/endpoints/1", bytes.NewReader(upRaw))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("PUT endpoint status: %d", w.Code)
	}
	gids := services.EndpointPermissionGroupIDs(db, epID)
	if len(gids) != 2 {
		t.Fatalf("expected 2 group ids, got %v", gids)
	}

	// 4. Delete Endpoint
	req = httptest.NewRequest("DELETE", "/api/v1/admin/inbounds/1/endpoints/1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("DELETE endpoint status: %d", w.Code)
	}
	var cnt int64
	db.Model(&models.InboundEndpoint{}).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("expected 0 endpoints, got %d", cnt)
	}
	db.Model(&models.PermissionGroupEndpoint{}).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("expected 0 link rows, got %d", cnt)
	}
}

func TestSubscribe_InboundEndpoints_PermissionWhitelist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupEndpointTestDB(t)

	srv := models.Server{Name: "香港01", Host: "hk.node.com", NodeID: "node-1", Secret: "sec-1", Status: 1}
	db.Create(&srv)
	inb := models.Inbound{
		ServerID:       srv.ID,
		Tag:            "hk-vless",
		Protocol:       "vless",
		Port:           443,
		Enabled:        true,
		Type:           "user",
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"serverName":"www.apple.com","publicKey":"pQDGvDURYEv8nxAVW9xsbBsQjOXzX0rCh5OWDW5q8kg","shortId":"e69c1c"}}`,
	}
	db.Create(&inb)

	groupBasic := models.PermissionGroup{Name: "基础组"}
	groupVIP := models.PermissionGroup{Name: "VIP组"}
	db.Create(&groupBasic)
	db.Create(&groupVIP)

	// 入站开放给基础组和 VIP 组
	_ = services.SyncInboundPermissionGroups(db, inb.ID, []uint64{groupBasic.ID, groupVIP.ID})

	// 附加接入点 1: 广州 BGP（仅 VIP 组）
	epVIP := models.InboundEndpoint{
		InboundID: inb.ID,
		Name:      "广州BGP",
		Host:      "gz.bgp.com",
		Port:      30001,
		Enabled:   true,
	}
	db.Create(&epVIP)
	_ = services.SyncEndpointPermissionGroups(db, epVIP.ID, []uint64{groupVIP.ID})

	// 附加接入点 2: 未配置权限组（空 -> 全部不可见）
	epUnassigned := models.InboundEndpoint{
		InboundID: inb.ID,
		Name:      "未开放专线",
		Host:      "secret.iplc.com",
		Port:      40001,
		Enabled:   true,
	}
	db.Create(&epUnassigned)

	userBasic := models.User{
		Username:          "basic@test.com",
		Email:             "basic@test.com",
		UUID:              "11111111-1111-1111-1111-111111111111",
		Status:            models.StatusActive,
		SubscribeToken:    "token-basic",
		PermissionGroupID: groupBasic.ID,
	}
	userVIP := models.User{
		Username:          "vip@test.com",
		Email:             "vip@test.com",
		UUID:              "22222222-2222-2222-2222-222222222222",
		Status:            models.StatusActive,
		SubscribeToken:    "token-vip",
		PermissionGroupID: groupVIP.ID,
	}
	db.Create(&userBasic)
	db.Create(&userVIP)

	trafficSvc := &services.TrafficService{DB: db}
	deps := &Deps{DB: db, Traffic: trafficSvc}
	r := gin.New()
	r.GET("/sub", deps.Subscribe)

	// 1. 普通基础组用户订阅：只应拉到主节点，不应有广州BGP，也不应有未开放专线
	req := httptest.NewRequest("GET", "/sub?token=token-basic&format=base64", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("basic user sub status: %d", w.Code)
	}
	decBasicBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(w.Body.String()))
	if err != nil {
		t.Fatalf("Base64 decode error: %v", err)
	}
	contentBasic := string(decBasicBytes)
	if !strings.Contains(contentBasic, "hk.node.com:443") {
		t.Errorf("basic user should see main endpoint: %s", contentBasic)
	}
	if strings.Contains(contentBasic, "gz.bgp.com") {
		t.Errorf("basic user should not see BGP endpoint: %s", contentBasic)
	}
	if strings.Contains(contentBasic, "secret.iplc.com") {
		t.Errorf("unassigned endpoint should be invisible to all: %s", contentBasic)
	}

	// 2. VIP 组用户订阅：拉到主节点 + 广州BGP（两个节点），不包含未开放专线
	req = httptest.NewRequest("GET", "/sub?token=token-vip&format=base64", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("vip user sub status: %d", w.Code)
	}
	decVIPBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(w.Body.String()))
	if err != nil {
		t.Fatalf("Base64 decode error: %v", err)
	}
	contentVIP := string(decVIPBytes)
	if !strings.Contains(contentVIP, "hk.node.com:443") {
		t.Errorf("vip user should see main endpoint: %s", contentVIP)
	}
	if !strings.Contains(contentVIP, "gz.bgp.com:30001") {
		t.Errorf("vip user should see BGP endpoint: %s", contentVIP)
	}
	// # 后的 fragment 是 QueryEscape 编码的 (香港01 x1 | hk-vless | 广州BGP)
	if !strings.Contains(contentVIP, "%E5%B9%BF%E5%B7%9EBGP") && !strings.Contains(contentVIP, "广州BGP") {
		t.Errorf("vip user should see BGP endpoint name: %s", contentVIP)
	}
	if strings.Contains(contentVIP, "secret.iplc.com") {
		t.Errorf("unassigned endpoint should be invisible to all: %s", contentVIP)
	}
}


