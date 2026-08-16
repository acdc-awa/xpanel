package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/config"
	"github.com/zhx/xray-panel/internal/models"
)

func TestAdminSystemStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&models.User{Username: "u1", Email: "u1@t.com", Role: models.RoleUser, Status: models.StatusActive}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/status", nil)
	d := &Deps{DB: db, Cfg: &config.Config{App: config.App{Name: "xray-panel", Env: "test"}}}
	d.AdminSystemStatus(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			AppName string `json:"app_name"`
			DBOK    bool   `json:"db_ok"`
			Counts  struct {
				Users int64 `json:"users"`
			} `json:"counts"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.AppName != "xray-panel" || !resp.Data.DBOK {
		t.Fatalf("unexpected status: %+v", resp.Data)
	}
	if resp.Data.Counts.Users != 1 {
		t.Fatalf("users count = %d, want 1", resp.Data.Counts.Users)
	}
}