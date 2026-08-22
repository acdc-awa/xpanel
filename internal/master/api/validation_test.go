package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/models"
)

func validationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Server{}, &models.Inbound{}, &models.Plan{},
		&models.ServerOutbound{}, &models.ServerRoutingRule{},
		&models.PermissionGroup{}, &models.PermissionGroupInbound{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedServer(t *testing.T, db *gorm.DB) models.Server {
	t.Helper()
	s := models.Server{Name: "srv", Host: "127.0.0.1", NodeID: "node-1", Secret: "secret-1"}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("create server: %v", err)
	}
	return s
}

func TestAdminUpdatePlanRejectsNegativeValues(t *testing.T) {
	db := validationTestDB(t)
	plan := models.Plan{Name: "p", PriceCents: 100, TrafficGB: 10, DurationDays: 30}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	d := &Deps{DB: db}
	r.PUT("/api/v1/admin/plans/:id", d.AdminUpdatePlan)
	id := strconv.FormatUint(plan.ID, 10)

	for _, body := range []string{
		`{"price_cents":-1}`,
		`{"traffic_gb":-5}`,
		`{"duration_days":-1}`,
		`{"device_limit":-3}`,
	} {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/plans/"+id, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body=%s => HTTP %d, want 400", body, w.Code)
		}
	}
	var after models.Plan
	db.First(&after, plan.ID)
	if after.PriceCents != 100 || after.TrafficGB != 10 || after.DurationDays != 30 {
		t.Fatalf("非法值不应入库: %+v", after)
	}
}

func TestAdminCreateInboundSemanticValidation(t *testing.T) {
	db := validationTestDB(t)
	srv := seedServer(t, db)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	d := &Deps{DB: db}
	r.POST("/api/v1/admin/inbounds", d.AdminCreateInbound)
	sid := strconv.FormatUint(srv.ID, 10)

	bad := []string{
		`{"server_id":` + sid + `,"tag":"bad-port","protocol":"vless","port":70000}`,
		`{"server_id":` + sid + `,"tag":"bad-type","protocol":"vless","port":443,"type":"magic"}`,
		`{"server_id":` + sid + `,"tag":"bad-flow","protocol":"vless","port":443,"flow":"magic"}`,
		`{"server_id":` + sid + `,"tag":"bad-share-port","protocol":"vless","port":443,"share_port":70000}`,
	}
	for _, body := range bad {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/inbounds", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body=%s => HTTP %d, want 400, resp=%s", body, w.Code, w.Body.String())
		}
	}
}

func TestAdminCreateRoutingRuleValidatesOutbound(t *testing.T) {
	db := validationTestDB(t)
	srv := seedServer(t, db)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	d := &Deps{DB: db}
	r.POST("/api/v1/admin/servers/:id/routing", d.AdminCreateServerRoutingRule)

	body := `{"outbound_tag":"missing","rule_json":"{\"domain\":[\"example.com\"]}"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/servers/"+strconv.FormatUint(srv.ID, 10)+"/routing", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing outbound => HTTP %d, want 400", w.Code)
	}
}

func TestAdminCreateOutboundTagUnique(t *testing.T) {
	db := validationTestDB(t)
	srv := seedServer(t, db)
	if err := db.Create(&models.ServerOutbound{ServerID: srv.ID, Tag: "proxy", Protocol: "freedom", SettingsJSON: `{}`}).Error; err != nil {
		t.Fatalf("create outbound: %v", err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	d := &Deps{DB: db}
	r.POST("/api/v1/admin/servers/:id/outbounds", d.AdminCreateServerOutbound)

	body := `{"tag":"proxy","protocol":"freedom","settings_json":"{}"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/servers/"+strconv.FormatUint(srv.ID, 10)+"/outbounds", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("duplicate tag => HTTP %d, want 400, resp=%s", w.Code, w.Body.String())
	}
}

func TestAdminPlanDescriptionCreateAndUpdate(t *testing.T) {
	db := validationTestDB(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	d := &Deps{DB: db}
	r.POST("/api/v1/admin/plans", d.AdminCreatePlan)
	r.PUT("/api/v1/admin/plans/:id", d.AdminUpdatePlan)

	// Create with description
	createBody := `{"name":"VIP Plan","description":"Line 1\nLine 2","price_cents":5000,"traffic_gb":100,"duration_days":30,"device_limit":5}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/plans", strings.NewReader(createBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create plan => HTTP %d, resp=%s", w.Code, w.Body.String())
	}

	var plan models.Plan
	if err := db.First(&plan, "name = ?", "VIP Plan").Error; err != nil {
		t.Fatalf("find plan: %v", err)
	}
	if plan.Description != "Line 1\nLine 2" {
		t.Fatalf("plan description = %q, want %q", plan.Description, "Line 1\nLine 2")
	}

	// Update description
	updateBody := `{"description":"Updated description line"}`
	idStr := strconv.FormatUint(plan.ID, 10)
	req2 := httptest.NewRequest(http.MethodPut, "/api/v1/admin/plans/"+idStr, strings.NewReader(updateBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("update plan => HTTP %d, resp=%s", w2.Code, w2.Body.String())
	}

	db.First(&plan, plan.ID)
	if plan.Description != "Updated description line" {
		t.Fatalf("updated plan description = %q, want %q", plan.Description, "Updated description line")
	}
}