package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/api"
	"github.com/acdc-awa/xpanel/internal/master/middleware"
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

	plan := models.Plan{Name: "p1", PriceCents: 1000, TrafficGB: 100, DurationDays: 30, DeviceLimit: 3, PermissionGroupID: 5, Purchasable: true, Renewable: true}
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

// TestPublicPlansIdentityFilter 商店身份感知过滤（2026-09-03 销售两属性）：
// 匿名/非持有者只见可新购套餐；持有者额外见自己可续费的当前套餐（续费入口）；
// 自己当前套餐不可续费 → 隐藏；双关全闭 → 全员隐藏。
func TestPublicPlansIdentityFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.AutoMigrate(db))
	deps := &api.Deps{DB: db}

	mk := func(name string, purchasable, renewable bool) models.Plan {
		pl := models.Plan{Name: name, PriceCents: 100, TrafficGB: 10, DurationDays: 30, Purchasable: purchasable, Renewable: renewable}
		require.NoError(t, db.Create(&pl).Error)
		return pl
	}
	a := mk("A", true, true)  // 可新购+可续费
	b := mk("B", false, true) // 停售但存量可续费
	c := mk("C", true, false) // 可新购但停续
	mk("D", false, false)     // 双关全闭：全员不可见

	mkUser := func(name string, planID uint64) uint64 {
		u := models.User{Username: name, Email: name + "@x.com", UUID: "uuid-" + name, PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "tok-" + name, PlanID: planID}
		require.NoError(t, db.Create(&u).Error)
		return u.ID
	}
	holderB := mkUser("uB", b.ID) // 持有停售可续费套餐
	holderC := mkUser("uC", c.ID) // 持有可新购停续套餐

	doList := func(uid uint64) []uint64 {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/plans", nil)
		if uid > 0 {
			c.Set(middleware.CtxClaimsKey, &contracts.JWTClaims{UserID: uid})
		}
		deps.PublicPlans(c)
		require.Equal(t, http.StatusOK, w.Code, "PublicPlans 应 200: %s", w.Body.String())
		var resp struct {
			Data struct {
				Items []struct {
					ID uint64 `json:"id"`
				} `json:"items"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		ids := make([]uint64, 0, len(resp.Data.Items))
		for _, it := range resp.Data.Items {
			ids = append(ids, it.ID)
		}
		return ids
	}

	assert.Equal(t, []uint64{a.ID, c.ID}, doList(0), "匿名应只见可新购套餐")
	assert.Equal(t, []uint64{a.ID, b.ID, c.ID}, doList(holderB), "持有可续费套餐应保留续费入口")
	assert.Equal(t, []uint64{a.ID}, doList(holderC), "自己当前套餐停续应隐藏")
}

// TestAdminUpdateUserSnapshotOnlyOnPlanChange 3.2 快照守卫（2026-09-03）：
// 同 plan_id 重复提交（编辑别的字段）不重写快照——「改套餐权限组未勾同步」后的
// 任何一次用户编辑都不再把套餐新值带进快照；只有真的换绑套餐才重新快照。
func TestAdminUpdateUserSnapshotOnlyOnPlanChange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.AutoMigrate(db))
	require.NoError(t, db.Create(&models.PermissionGroup{ID: 5, Name: "g5"}).Error)
	require.NoError(t, db.Create(&models.PermissionGroup{ID: 9, Name: "g9"}).Error)
	require.NoError(t, db.Create(&models.Plan{Name: "p1", PriceCents: 1000, TrafficGB: 100, DurationDays: 30, PermissionGroupID: 5, Purchasable: true}).Error)
	require.NoError(t, db.Create(&models.Plan{Name: "p2", PriceCents: 1000, TrafficGB: 200, DurationDays: 60, PermissionGroupID: 9, Purchasable: true}).Error)
	var p1, p2 models.Plan
	require.NoError(t, db.Where("name = ?", "p1").First(&p1).Error)
	require.NoError(t, db.Where("name = ?", "p2").First(&p2).Error)

	user := models.User{Username: "a@x.com", Email: "a@x.com", UUID: "uuid-a", PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "tok-a", PlanID: p1.ID}
	user.ApplyPlanSnapshot(&p1)
	require.NoError(t, db.Create(&user).Error)

	deps := &api.Deps{DB: db}
	doUpdateUser := func(body map[string]any) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/1", bytes.NewReader(b))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		deps.AdminUpdateUser(c)
		return w
	}

	// 1) 同 plan_id 重复提交（仅改设备数）→ 快照不得被重写
	w := doUpdateUser(map[string]any{"plan_id": p1.ID, "device_limit": 1})
	require.Equal(t, http.StatusOK, w.Code, "同套餐编辑应成功: %s", w.Body.String())
	var after models.User
	require.NoError(t, db.First(&after, user.ID).Error)
	assert.Equal(t, uint64(5), after.PlanGroupID, "同套餐编辑不得重写权限组快照")
	assert.Equal(t, int64(100)*1024*1024*1024, after.PlanTrafficBytes, "同套餐编辑不得重写流量快照")
	assert.Equal(t, 1, after.DeviceLimit, "同套餐编辑其他字段应正常生效")

	// 2) 换绑套餐 → 按新套餐值重新快照
	w = doUpdateUser(map[string]any{"plan_id": p2.ID})
	require.Equal(t, http.StatusOK, w.Code, "换套餐应成功: %s", w.Body.String())
	require.NoError(t, db.First(&after, user.ID).Error)
	assert.Equal(t, uint64(9), after.PlanGroupID, "换套餐应重写权限组快照")
	assert.Equal(t, int64(200)*1024*1024*1024, after.PlanTrafficBytes, "换套餐应重写流量快照")
}
