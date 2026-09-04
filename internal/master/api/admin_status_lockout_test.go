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

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/api"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 补充 anti-lockout 覆盖：AdminUpdateUser 的 status 通道（原测试只覆盖 role 降级与 toggle）。
func TestAdminUpdateUserStatusAntiLockout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Inbound{}, &models.Server{}))

	adminA := models.User{
		Username: "a@panel.local", Email: "a@panel.local", UUID: "uuid-a",
		Role: models.RoleAdmin, Status: models.StatusActive,
		SubscribeToken: "tok-a", TrafficCycleStart: time.Now(),
	}
	require.NoError(t, db.Create(&adminA).Error)

	deps := &api.Deps{DB: db}
	r := gin.New()
	r.PUT("/admin/users/:id", deps.AdminUpdateUser)
	r.POST("/admin/users/:id/toggle", deps.AdminToggleUser)

	// 1. 唯一管理员经 PUT status=0 禁用自己 -> 必须被拒绝
	{
		body, _ := json.Marshal(map[string]any{"status": 0})
		req := httptest.NewRequest(http.MethodPut, "/admin/users/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "系统必须至少保留一名处于激活状态的管理员")
	}

	// 2. role+status 同请求双改 -> 同样必须被拒绝
	{
		body, _ := json.Marshal(map[string]any{"role": "user", "status": 0})
		req := httptest.NewRequest(http.MethodPut, "/admin/users/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "系统必须至少保留一名处于激活状态的管理员")
	}

	// 3. 双管理员场景：A 先禁用 B（放行），再禁用自己（必须拒绝）
	adminB := models.User{
		Username: "b@panel.local", Email: "b@panel.local", UUID: "uuid-b",
		Role: models.RoleAdmin, Status: models.StatusActive,
		SubscribeToken: "tok-b", TrafficCycleStart: time.Now(),
	}
	require.NoError(t, db.Create(&adminB).Error)
	{
		req := httptest.NewRequest(http.MethodPost, "/admin/users/2/toggle", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
	{
		req := httptest.NewRequest(http.MethodPost, "/admin/users/1/toggle", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		var a models.User
		db.First(&a, 1)
		assert.Equal(t, models.StatusActive, a.Status, "最后一名活跃管理员必须保持激活")
	}
}

// 自我守卫：管理员不能封禁自己，不因存在其他活跃管理员而放行（2026-09-04 实机反馈）。
// 登录态经 gin context "claims" 注入（键名须与 middleware.CtxClaimsKey 一致，该常量未导出）。
func TestAdminSelfBanForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Inbound{}, &models.Server{}))

	adminA := models.User{
		Username: "a@panel.local", Email: "a@panel.local", UUID: "uuid-a",
		Role: models.RoleAdmin, Status: models.StatusActive,
		SubscribeToken: "tok-a", TrafficCycleStart: time.Now(),
	}
	require.NoError(t, db.Create(&adminA).Error)
	adminB := models.User{
		Username: "b@panel.local", Email: "b@panel.local", UUID: "uuid-b",
		Role: models.RoleAdmin, Status: models.StatusActive,
		SubscribeToken: "tok-b", TrafficCycleStart: time.Now(),
	}
	require.NoError(t, db.Create(&adminB).Error)

	claims := &contracts.JWTClaims{UserID: adminA.ID, Role: models.RoleAdmin, Type: "access"}
	identity := func(c *gin.Context) {
		c.Set("claims", claims)
		c.Next()
	}
	deps := &api.Deps{DB: db}
	r := gin.New()
	r.Use(identity)
	r.PUT("/admin/users/:id", deps.AdminUpdateUser)
	r.POST("/admin/users/:id/toggle", deps.AdminToggleUser)

	// 1. A（登录态）封禁自己 -> 拒绝，状态不变
	{
		req := httptest.NewRequest(http.MethodPost, "/admin/users/1/toggle", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "不能封禁自己的账号")
		var a models.User
		db.First(&a, 1)
		assert.Equal(t, models.StatusActive, a.Status)
	}

	// 2. A 经 PUT status=0 封禁自己 -> 同样拒绝
	{
		body, _ := json.Marshal(map[string]any{"status": 0})
		req := httptest.NewRequest(http.MethodPut, "/admin/users/1", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "不能封禁自己的账号")
	}

	// 3. 存在另一名管理员时封禁对方 -> 放行（自我守卫不约束他人）
	{
		req := httptest.NewRequest(http.MethodPost, "/admin/users/2/toggle", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		var b models.User
		db.First(&b, 2)
		assert.Equal(t, models.StatusDisabled, b.Status)
	}
}
