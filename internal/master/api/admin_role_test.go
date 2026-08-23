package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/master/api"
	"github.com/acdc/xray-panel/internal/models"
)

func setupTestDBForRole(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&models.User{}, &models.Inbound{}, &models.Server{})
	require.NoError(t, err)
	return db
}

func TestAdminRoleAntiLockoutProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDBForRole(t)

	// 创建初始管理员 A
	adminA := models.User{
		Username:          "adminA@panel.local",
		Email:             "adminA@panel.local",
		UUID:              "uuid-admin-a",
		Role:              models.RoleAdmin,
		Status:            models.StatusActive,
		SubscribeToken:    "token-admin-a",
		TrafficCycleStart: time.Now(),
	}
	require.NoError(t, db.Create(&adminA).Error)

	// 创建普通用户 U
	userU := models.User{
		Username:          "userU@panel.local",
		Email:             "userU@panel.local",
		UUID:              "uuid-user-u",
		Role:              models.RoleUser,
		Status:            models.StatusActive,
		SubscribeToken:    "token-user-u",
		TrafficCycleStart: time.Now(),
	}
	require.NoError(t, db.Create(&userU).Error)

	deps := &api.Deps{DB: db}
	r := gin.New()
	r.PUT("/admin/users/:id", deps.AdminUpdateUser)
	r.POST("/admin/users/:id/toggle", deps.AdminToggleUser)
	r.DELETE("/admin/users/:id", deps.AdminDeleteUser)

	// 1. 唯一的管理员 A 尝试降级为 user -> 必须被拒绝（400）
	{
		roleUser := "user"
		body, _ := json.Marshal(map[string]any{"role": roleUser})
		req := httptest.NewRequest(http.MethodPut, "/admin/users/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "系统必须至少保留一名处于激活状态的管理员")
	}

	// 2. 唯一的管理员 A 尝试被禁用 -> 必须被拒绝（400）
	{
		req := httptest.NewRequest(http.MethodPost, "/admin/users/1/toggle", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "系统必须至少保留一名处于激活状态的管理员")
	}

	// 3. 将普通用户 U 提升为管理员 -> 成功
	{
		roleAdmin := "admin"
		body, _ := json.Marshal(map[string]any{"role": roleAdmin})
		req := httptest.NewRequest(http.MethodPut, "/admin/users/2", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var updatedU models.User
		db.First(&updatedU, 2)
		assert.Equal(t, models.RoleAdmin, updatedU.Role)
	}

	// 4. 此时已有两名激活的管理员，将 A 降级为 user -> 成功
	{
		roleUser := "user"
		body, _ := json.Marshal(map[string]any{"role": roleUser})
		req := httptest.NewRequest(http.MethodPut, "/admin/users/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		var updatedA models.User
		db.First(&updatedA, 1)
		assert.Equal(t, models.RoleUser, updatedA.Role)
	}

	// 5. 现在只剩下 U 一名管理员，U 尝试被禁用 -> 必须再次被拒绝（400）
	{
		req := httptest.NewRequest(http.MethodPost, "/admin/users/2/toggle", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "系统必须至少保留一名处于激活状态的管理员")
	}
}
