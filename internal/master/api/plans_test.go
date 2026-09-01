package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/acdc-awa/xpanel/internal/master/api"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestAdminUpdatePlanSnapshotIsolation 套餐快照隔离（2026-09-01 Xboard 式）：
// 默认编辑套餐零影响存量用户（快照不变）；显式 sync_users=true 才批量重快照。
func TestAdminUpdatePlanSnapshotIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Plan{}, &models.Inbound{}, &models.PermissionGroup{}))
	require.NoError(t, db.Create(&models.PermissionGroup{ID: 9, Name: "g9"}).Error)
	deps := &api.Deps{DB: db} // Hub/Config 为 nil → TriggerUserChange 为安全空操作

	plan := models.Plan{Name: "p1", PriceCents: 1000, TrafficGB: 100, DurationDays: 30, DeviceLimit: 3, PermissionGroupID: 5, Enabled: true}
	require.NoError(t, db.Create(&plan).Error)
	user := models.User{Username: "a@x.com", Email: "a@x.com", UUID: "uuid-a", PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "tok-a", PlanID: plan.ID}
	user.ApplyPlanSnapshot(&plan)
	require.NoError(t, db.Create(&user).Error)

	doUpdate := func(body map[string]any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/plans/1", bytes.NewReader(b))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		deps.AdminUpdatePlan(c)
		return w
	}

	// 1) 不勾 sync_users：改小额度 → 存量用户快照不变（隔离）
	w := doUpdate(map[string]any{"traffic_gb": 10, "device_limit": 1, "permission_group_id": 9})
	require.Equal(t, http.StatusOK, w.Code, "更新应成功: %s", w.Body.String())
	var after models.User
	require.NoError(t, db.First(&after, user.ID).Error)
	assert.Equal(t, int64(100)*1024*1024*1024, after.PlanTrafficBytes, "不勾同步时快照额度不得变")
	assert.Equal(t, 3, after.PlanDeviceLimit, "不勾同步时设备限制快照不得变")
	assert.Equal(t, uint64(5), after.PlanGroupID, "不勾同步时权限组快照不得变")

	// 2) 勾 sync_users=true：批量重快照 → 存量用户按新套餐值生效
	w = doUpdate(map[string]any{"traffic_gb": 10, "device_limit": 1, "permission_group_id": 9, "sync_users": true})
	require.Equal(t, http.StatusOK, w.Code, "同步更新应成功: %s", w.Body.String())
	require.NoError(t, db.First(&after, user.ID).Error)
	assert.Equal(t, int64(10)*1024*1024*1024, after.PlanTrafficBytes, "勾选同步后快照额度应为新值")
	assert.Equal(t, 1, after.PlanDeviceLimit, "勾选同步后设备限制快照应为新值")
	assert.Equal(t, uint64(9), after.PlanGroupID, "勾选同步后权限组快照应为新值")
}
