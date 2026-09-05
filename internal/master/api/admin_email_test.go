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
	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

// setupTestDBForEmail 建内存库（含审计表）并挂 AdminUpdateUser 路由。
// 挂真实审计中间件（2026-09-04 双记清理后审计唯一入口在中间件）。
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
	r.Use(middleware.Audit(db))
	r.PUT("/api/v1/admin/users/:id", deps.AdminUpdateUser)
	return db, r
}

func putEmail(t *testing.T, r *gin.Engine, id int, email string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"email": email})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+strconv.Itoa(id), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAdminUpdateUserEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, r := setupTestDBForEmail(t)

	// 1. 实际变更：email+username 双写、token_version+1（吊销旧会话）、中间件落审计
	// （envelope body 带新邮箱；注册表 target 预读变更前邮箱；旧→新对比待二期 diff）
	w := putEmail(t, r, 1, "new@panel.local")
	require.Equal(t, http.StatusOK, w.Code)
	var u models.User
	require.NoError(t, db.First(&u, 1).Error)
	assert.Equal(t, "new@panel.local", u.Email)
	assert.Equal(t, "new@panel.local", u.Username)
	assert.EqualValues(t, 4, u.TokenVersion)
	var audit models.AuditLog
	require.NoError(t, db.Where("action = ?", "users.:id").First(&audit).Error)
	assert.Contains(t, audit.Detail, `"email":"new@panel.local"`)
	assert.Contains(t, audit.Detail, `"target":"user@panel.local"`, "注册表应预读变更前的旧邮箱")
	assert.Equal(t, "admin", audit.OperatorType)

	// 2. 大小写归一后同值：不吊销会话（token_version 维持 4）。
	// 审计语义说明：中间件记录每一次写操作（envelope 可查），手动通道
	// 「同值不刷审计」的去重语义已随双记清理退役，靠二期 diff 呈现真实变更。
	// （带空白的输入会被 binding 层的 email 校验直接 400，故此处只测大小写）
	w = putEmail(t, r, 1, "NEW@Panel.local")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, db.First(&u, 1).Error)
	assert.EqualValues(t, 4, u.TokenVersion)
	var cnt int64
	db.Model(&models.AuditLog{}).Where("action = ?", "users.:id").Count(&cnt)
	assert.EqualValues(t, 2, cnt, "每次写操作各落一条（变更+同值）")

	// 3. 再次实变：版本继续 +1（5），审计累计 3 条
	w = putEmail(t, r, 1, "again@panel.local")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, db.First(&u, 1).Error)
	assert.EqualValues(t, 5, u.TokenVersion)
	db.Model(&models.AuditLog{}).Where("action = ?", "users.:id").Count(&cnt)
	assert.EqualValues(t, 3, cnt)

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
	// 不得误吊销会话；中间件落一条常规写操作审计，手动邮箱审计通道已退役
	body, _ := json.Marshal(map[string]any{"role": "admin"})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var u models.User
	require.NoError(t, db.First(&u, 1).Error)
	assert.EqualValues(t, 3, u.TokenVersion, "未变更邮箱不应 bump token_version")
	var manualCnt, autoCnt int64
	db.Model(&models.AuditLog{}).Where("action = ?", "user.update_email").Count(&manualCnt)
	db.Model(&models.AuditLog{}).Where("action = ?", "users.:id").Count(&autoCnt)
	assert.EqualValues(t, 0, manualCnt, "手动邮箱审计通道已删除")
	assert.EqualValues(t, 1, autoCnt, "中间件统一落一条写操作审计")
}
