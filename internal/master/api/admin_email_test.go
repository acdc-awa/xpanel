package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/api"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

// setupTestDBForEmail 建内存库（含审计表）并挂 AdminUpdateUser 路由。
func setupTestDBForEmail(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.AuditLog{}, &models.Inbound{}, &models.Server{}))

	user := models.User{
		Username:          "user@panel.local",
		Email:             "user@panel.local",
		UUID:              "uuid-email-user",
		Role:              models.RoleUser,
		Status:            models.StatusActive,
		SubscribeToken:    "tok-email-user",
		TokenVersion:      3,
		TrafficCycleStart: time.Now(),
	}
	require.NoError(t, db.Create(&user).Error)
	other := models.User{
		Username:          "other@panel.local",
		Email:             "other@panel.local",
		UUID:              "uuid-email-other",
		Role:              models.RoleUser,
		Status:            models.StatusActive,
		SubscribeToken:    "tok-email-other",
		TrafficCycleStart: time.Now(),
	}
	require.NoError(t, db.Create(&other).Error)

	deps := &api.Deps{DB: db, Audit: &services.AuditService{DB: db}}
	r := gin.New()
	r.PUT("/admin/users/:id", deps.AdminUpdateUser)
	return db, r
}

func putEmail(t *testing.T, r *gin.Engine, id int, email string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"email": email})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/"+strconv.Itoa(id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAdminUpdateUserEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, r := setupTestDBForEmail(t)

	// 1. 实际变更：email+username 双写、token_version+1（吊销旧会话）、记审计
	w := putEmail(t, r, 1, "new@panel.local")
	require.Equal(t, http.StatusOK, w.Code)
	var u models.User
	require.NoError(t, db.First(&u, 1).Error)
	assert.Equal(t, "new@panel.local", u.Email)
	assert.Equal(t, "new@panel.local", u.Username)
	assert.EqualValues(t, 4, u.TokenVersion)
	var audit models.AuditLog
	require.NoError(t, db.Where("action = ?", "user.update_email").First(&audit).Error)
	assert.Contains(t, audit.Detail, "user@panel.local → new@panel.local")
	assert.Equal(t, "admin", audit.OperatorType)

	// 2. 大小写归一后同值：不吊销、不写审计（token_version 维持 4）
	// （带空白的输入会被 binding 层的 email 校验直接 400，故此处只测大小写）
	w = putEmail(t, r, 1, "NEW@Panel.local")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, db.First(&u, 1).Error)
	assert.EqualValues(t, 4, u.TokenVersion)
	var cnt int64
	db.Model(&models.AuditLog{}).Where("action = ?", "user.update_email").Count(&cnt)
	assert.EqualValues(t, 1, cnt, "同值重复提交不应新增审计")

	// 3. 再次实变：版本继续 +1（5），审计累计 2 条
	w = putEmail(t, r, 1, "again@panel.local")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, db.First(&u, 1).Error)
	assert.EqualValues(t, 5, u.TokenVersion)
	db.Model(&models.AuditLog{}).Where("action = ?", "user.update_email").Count(&cnt)
	assert.EqualValues(t, 2, cnt)

	// 4. 冲突：改成他人邮箱 -> 400，版本不动
	w = putEmail(t, r, 1, "other@panel.local")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	require.NoError(t, db.First(&u, 1).Error)
	assert.EqualValues(t, 5, u.TokenVersion)
}

func TestAdminUpdateUserRoleOnlyKeepsTokenVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, r := setupTestDBForEmail(t)

	// 只改角色（前端保存总携带 email 字段，但本用例只发 role）：
	// 不得误吊销会话、不得产生邮箱审计
	body, _ := json.Marshal(map[string]any{"role": "admin"})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var u models.User
	require.NoError(t, db.First(&u, 1).Error)
	assert.EqualValues(t, 3, u.TokenVersion, "未变更邮箱不应 bump token_version")
	var cnt int64
	db.Model(&models.AuditLog{}).Where("action = ?", "user.update_email").Count(&cnt)
	assert.EqualValues(t, 0, cnt)
}
