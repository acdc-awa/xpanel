package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func setupAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.AuditLog{}))

	now := time.Now()
	logs := []models.AuditLog{
		{ID: 1, OperatorType: "admin", OperatorID: 1, Action: "servers.1.command", Detail: `{"type":"restart_xray"}`, IP: "1.1.1.1", CreatedAt: now.Add(-10 * time.Minute)},
		{ID: 2, OperatorType: "admin", OperatorID: 1, Action: "users.2", Detail: `{"email":"test@example.com"}`, IP: "1.1.1.2", CreatedAt: now.Add(-5 * time.Minute)},
		{ID: 3, OperatorType: "admin", OperatorID: 2, Action: "plans.1", Detail: `{"name":"VIP Plan"}`, IP: "2.2.2.2", CreatedAt: now.Add(-2 * time.Minute)},
		{ID: 4, OperatorType: "system", OperatorID: 0, Action: "auth.login", Detail: `login success`, IP: "3.3.3.3", CreatedAt: now.Add(-1 * time.Minute)},
	}
	for _, l := range logs {
		assert.NoError(t, db.Create(&l).Error)
	}
	return db
}

func TestAdminAuditLogsFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAuditTestDB(t)
	deps := &Deps{DB: db}

	r := gin.New()
	r.GET("/audit-logs", deps.AdminAuditLogs)

	// 1. All logs
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/audit-logs", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Code int `json:"code"`
			Data struct {
				Total int `json:"total"`
				Items []struct {
					ID     uint64 `json:"id"`
					Action string `json:"action"`
				} `json:"items"`
			} `json:"data"`
		}
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 4, resp.Data.Total)
	}

	// 2. Filter by category=servers
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/audit-logs?category=servers", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Data struct {
				Total int `json:"total"`
				Items []struct {
					Action string `json:"action"`
				} `json:"items"`
			} `json:"data"`
		}
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.Data.Total)
		assert.Equal(t, "servers.1.command", resp.Data.Items[0].Action)
	}

	// 3. Filter by keyword=VIP
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/audit-logs?keyword=VIP", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Data struct {
				Total int `json:"total"`
				Items []struct {
					Detail string `json:"detail"`
				} `json:"items"`
			} `json:"data"`
		}
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.Data.Total)
		assert.Contains(t, resp.Data.Items[0].Detail, "VIP")
	}

	// 4. Filter by operator_id=2
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/audit-logs?operator_id=2", nil)
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp struct {
			Data struct {
				Total int `json:"total"`
			} `json:"data"`
		}
		assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.Data.Total)
	}
}
