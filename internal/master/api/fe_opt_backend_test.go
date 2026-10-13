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

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/nodegate"
	"github.com/acdc-awa/xpanel/internal/models"
)

func setupTestDeps(t *testing.T) (*Deps, *gorm.DB) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return &Deps{DB: db}, db
}

func TestAdminUpdateInbound_CascadeTagToRoutingRules(t *testing.T) {
	deps, db := setupTestDeps(t)
	gin.SetMode(gin.TestMode)

	srv := models.Server{Name: "S1", Host: "1.1.1.1", NodeID: "n1", Secret: "s", Status: 1}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}
	inb := models.Inbound{ServerID: srv.ID, Tag: "in-old-vless", Protocol: "vless", Port: 443, Type: "user", Enabled: true}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}
	rule := models.ServerRoutingRule{ServerID: srv.ID, InboundTag: "in-old-vless", OutboundTag: "direct", Enabled: true}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}

	newTag := "in-new-vless"
	body, _ := json.Marshal(map[string]any{"tag": newTag})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(inb.ID, 10)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/inbounds/"+strconv.FormatUint(inb.ID, 10), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	deps.AdminUpdateInbound(c)
	if w.Code != http.StatusOK {
		t.Fatalf("update inbound failed: %d, body=%s", w.Code, w.Body.String())
	}

	var updatedRule models.ServerRoutingRule
	if err := db.First(&updatedRule, rule.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedRule.InboundTag != newTag {
		t.Fatalf("expected rule inbound_tag to be %s, got %s", newTag, updatedRule.InboundTag)
	}
}

func TestAdminUpdateOutbound_CascadeTagToRoutingRulesAndDefault(t *testing.T) {
	deps, db := setupTestDeps(t)
	gin.SetMode(gin.TestMode)

	srv := models.Server{Name: "S1", Host: "1.1.1.1", NodeID: "n1", Secret: "s", Status: 1, DefaultOutboundTag: "out-custom"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}
	ob := models.ServerOutbound{ServerID: srv.ID, Tag: "out-custom", Protocol: "freedom", Enabled: true}
	if err := db.Create(&ob).Error; err != nil {
		t.Fatal(err)
	}
	rule := models.ServerRoutingRule{ServerID: srv.ID, InboundTag: "in-any", OutboundTag: "out-custom", Enabled: true}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}

	newTag := "out-renamed"
	body, _ := json.Marshal(map[string]any{"tag": newTag})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: strconv.FormatUint(srv.ID, 10)},
		{Key: "outbound_id", Value: strconv.FormatUint(ob.ID, 10)},
	}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/servers/"+strconv.FormatUint(srv.ID, 10)+"/outbounds/"+strconv.FormatUint(ob.ID, 10), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	deps.AdminUpdateServerOutbound(c)
	if w.Code != http.StatusOK {
		t.Fatalf("update outbound failed: %d, body=%s", w.Code, w.Body.String())
	}

	var updatedRule models.ServerRoutingRule
	if err := db.First(&updatedRule, rule.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedRule.OutboundTag != newTag {
		t.Fatalf("expected rule outbound_tag to be %s, got %s", newTag, updatedRule.OutboundTag)
	}

	var updatedSrv models.Server
	if err := db.First(&updatedSrv, srv.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedSrv.DefaultOutboundTag != newTag {
		t.Fatalf("expected server default_outbound_tag to be %s, got %s", newTag, updatedSrv.DefaultOutboundTag)
	}
}

type mockCfgService struct {
	contracts.ConfigService
	generateCalls int
}

func (m *mockCfgService) Generate(serverID uint64) (string, error) {
	m.generateCalls++
	return "{}", nil
}

func (m *mockCfgService) SavePending(serverID uint64, configJSON string) error {
	return nil
}

func TestAdminUpdateInbound_TieredPushSkipsNonXrayFields(t *testing.T) {
	deps, db := setupTestDeps(t)
	mockCfg := &mockCfgService{}
	deps.Config = mockCfg
	deps.Hub = nodegate.NewHub(db, nil, nil)
	gin.SetMode(gin.TestMode)

	srv := models.Server{Name: "S1", Host: "1.1.1.1", NodeID: "n1", Secret: "s", Status: 1}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}
	inb := models.Inbound{ServerID: srv.ID, Tag: "in-1", Protocol: "vless", Port: 443, Type: "user", Enabled: true, Ratio: 1.0}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatal(err)
	}

	// 1. 仅改倍率和备注（非 Xray 字段） -> 不应调用 Generate
	ratio := 1.8
	remark := "更新备注"
	body, _ := json.Marshal(map[string]any{"ratio": ratio, "remark": remark})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(inb.ID, 10)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/inbounds/"+strconv.FormatUint(inb.ID, 10), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	deps.AdminUpdateInbound(c)
	if w.Code != http.StatusOK {
		t.Fatalf("update inbound failed: %d, body=%s", w.Code, w.Body.String())
	}

	if mockCfg.generateCalls != 0 {
		t.Fatalf("expected 0 calls to Generate for non-xray field update, got %d", mockCfg.generateCalls)
	}

	var updatedInb models.Inbound
	db.First(&updatedInb, inb.ID)
	if updatedInb.Ratio != 1.8 || updatedInb.Remark != "更新备注" {
		t.Fatalf("inbound values not updated in db: ratio=%v, remark=%v", updatedInb.Ratio, updatedInb.Remark)
	}

	// 2. 改端口（核心 Xray 字段） -> 应调用 Generate
	newPort := 8443
	body2, _ := json.Marshal(map[string]any{"port": newPort})
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(inb.ID, 10)}}
	c2.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/inbounds/"+strconv.FormatUint(inb.ID, 10), bytes.NewReader(body2))
	c2.Request.Header.Set("Content-Type", "application/json")

	deps.AdminUpdateInbound(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("update inbound port failed: %d, body=%s", w2.Code, w2.Body.String())
	}

	if mockCfg.generateCalls != 1 {
		t.Fatalf("expected 1 call to Generate for core field update, got %d", mockCfg.generateCalls)
	}
}

func TestAdminServers_SortOrderBySortOrder(t *testing.T) {
	deps, db := setupTestDeps(t)
	gin.SetMode(gin.TestMode)

	s1 := models.Server{Name: "S1", Host: "1.1.1.1", NodeID: "n1", Secret: "s", Status: 1, SortOrder: 20}
	s2 := models.Server{Name: "S2", Host: "2.2.2.2", NodeID: "n2", Secret: "s", Status: 1, SortOrder: 5}
	s3 := models.Server{Name: "S3", Host: "3.3.3.3", NodeID: "n3", Secret: "s", Status: 1, SortOrder: 10}
	for _, s := range []*models.Server{&s1, &s2, &s3} {
		if err := db.Create(s).Error; err != nil {
			t.Fatal(err)
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/servers", nil)

	deps.AdminServers(c)
	if w.Code != http.StatusOK {
		t.Fatalf("AdminServers failed: %d", w.Code)
	}

	var resp struct {
		Data struct {
			Items []struct {
				ID        uint64 `json:"id"`
				Name      string `json:"name"`
				SortOrder int    `json:"sort_order"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	items := resp.Data.Items
	if len(items) != 3 {
		t.Fatalf("expected 3 servers, got %d", len(items))
	}
	if items[0].Name != "S2" || items[1].Name != "S3" || items[2].Name != "S1" {
		t.Fatalf("expected order S2(5), S3(10), S1(20), got: %+v", items)
	}
}
