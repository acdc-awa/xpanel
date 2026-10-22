package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
)

// 删除当前默认出口后必须回退到 direct，否则 servers.default_outbound_tag 会悬空，
// 配置生成找不到该 tag，xray 退回 outbounds[0]（顺序不确定）。
func TestAdminDeleteOutbound_ResetsDanglingDefault(t *testing.T) {
	deps, db := setupTestDeps(t)
	gin.SetMode(gin.TestMode)

	srv := models.Server{
		Name: "S1", Host: "1.1.1.1", NodeID: "n1", Secret: "s", Status: 1,
		DefaultOutboundTag: "out-custom",
	}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}
	ob := models.ServerOutbound{ServerID: srv.ID, Tag: "out-custom", Protocol: "freedom", Enabled: true}
	if err := db.Create(&ob).Error; err != nil {
		t.Fatal(err)
	}
	rule := models.ServerRoutingRule{
		ServerID: srv.ID, OutboundTag: "out-custom", Domain: "example.com", Enabled: true,
	}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: strconv.FormatUint(srv.ID, 10)},
		{Key: "outbound_id", Value: strconv.FormatUint(ob.ID, 10)},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/servers/1/outbounds/1", nil)

	deps.AdminDeleteServerOutbound(c)

	if w.Code != http.StatusOK {
		t.Fatalf("delete outbound failed: %d, body=%s", w.Code, w.Body.String())
	}

	var got models.Server
	if err := db.First(&got, srv.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.DefaultOutboundTag != "direct" {
		t.Fatalf("expected default_outbound_tag to fall back to direct, got %q", got.DefaultOutboundTag)
	}

	var outboundCount, ruleCount int64
	db.Model(&models.ServerOutbound{}).Where("id = ?", ob.ID).Count(&outboundCount)
	db.Model(&models.ServerRoutingRule{}).Where("id = ?", rule.ID).Count(&ruleCount)
	if outboundCount != 0 {
		t.Fatalf("expected outbound to be deleted, still has %d row(s)", outboundCount)
	}
	if ruleCount != 0 {
		t.Fatalf("expected referencing rule to be cascade-deleted, still has %d row(s)", ruleCount)
	}
}

// 删除非默认出口不应改动默认出口。
func TestAdminDeleteOutbound_KeepsOtherDefault(t *testing.T) {
	deps, db := setupTestDeps(t)
	gin.SetMode(gin.TestMode)

	srv := models.Server{
		Name: "S1", Host: "1.1.1.1", NodeID: "n1", Secret: "s", Status: 1,
		DefaultOutboundTag: "out-keep",
	}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}
	ob := models.ServerOutbound{ServerID: srv.ID, Tag: "out-other", Protocol: "freedom", Enabled: true}
	if err := db.Create(&ob).Error; err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{
		{Key: "id", Value: strconv.FormatUint(srv.ID, 10)},
		{Key: "outbound_id", Value: strconv.FormatUint(ob.ID, 10)},
	}
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/servers/1/outbounds/1", nil)

	deps.AdminDeleteServerOutbound(c)

	if w.Code != http.StatusOK {
		t.Fatalf("delete outbound failed: %d, body=%s", w.Code, w.Body.String())
	}

	var got models.Server
	if err := db.First(&got, srv.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.DefaultOutboundTag != "out-keep" {
		t.Fatalf("expected default_outbound_tag to stay %q, got %q", "out-keep", got.DefaultOutboundTag)
	}
}
