package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func newPlanArchiveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.AutoMigrate(db))
	return db
}

func testContext(method, target string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, nil)
	if params != nil {
		c.Params = params
	}
	return c, w
}

// 套餐归档（2026-10-25）：存在历史订单不再阻断删除；归档后管理端默认不可见、商城不可见，
// 订单仍能显示套餐名；归档行物理保留，可经 restore 恢复。
func TestAdminDeletePlanArchivesWithOrders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPlanArchiveTestDB(t)
	deps := &Deps{DB: db}

	plan := models.Plan{
		Name: "旧套餐", PriceCents: 1000, TrafficGB: 100, DurationDays: 30,
		Purchasable: true, Renewable: true,
	}
	require.NoError(t, db.Create(&plan).Error)
	order := models.Order{
		OrderNo: "ORD-ARCHIVE-1", UserID: 1, PlanID: plan.ID,
		AmountCents: 1000, PaymentMethod: models.PaymentMethodBalance, Status: models.OrderPaid,
	}
	require.NoError(t, db.Create(&order).Error)

	c, w := testContext(http.MethodDelete, "/api/v1/admin/plans/1", gin.Params{{Key: "id", Value: "1"}})
	deps.AdminDeletePlan(c)
	require.Equal(t, http.StatusOK, w.Code, "有订单也应允许归档: %s", w.Body.String())

	var archived models.Plan
	require.NoError(t, db.Unscoped().First(&archived, plan.ID).Error, "归档行必须保留")
	assert.True(t, archived.DeletedAt.Valid, "套餐应标记为已归档")
	assert.False(t, archived.Purchasable, "归档必须同时停售，避免旧面板重新上架")
	assert.False(t, archived.Renewable, "归档必须同时停续")

	var visible int64
	require.NoError(t, db.Model(&models.Plan{}).Count(&visible).Error)
	assert.Zero(t, visible, "常规查询不应看到归档套餐")

	// 管理端默认列表不含归档套餐
	c, w = testContext(http.MethodGet, "/api/v1/admin/plans", nil)
	deps.AdminPlans(c)
	require.Equal(t, http.StatusOK, w.Code)
	var defaultList struct {
		Data struct {
			Items []planView `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &defaultList))
	assert.Empty(t, defaultList.Data.Items)

	// include_archived=1 能看到，并带归档标记
	c, w = testContext(http.MethodGet, "/api/v1/admin/plans?include_archived=1", nil)
	deps.AdminPlans(c)
	require.Equal(t, http.StatusOK, w.Code)
	var allList struct {
		Data struct {
			Items []planView `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &allList))
	require.Len(t, allList.Data.Items, 1)
	assert.True(t, allList.Data.Items[0].Archived)
	require.NotNil(t, allList.Data.Items[0].ArchivedAt)

	// 商城（公开套餐列表）不可见
	c, w = testContext(http.MethodGet, "/api/v1/plans", nil)
	deps.PublicPlans(c)
	require.Equal(t, http.StatusOK, w.Code)
	var publicList struct {
		Data struct {
			Items []publicPlanView `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &publicList))
	assert.Empty(t, publicList.Data.Items)

	// 历史订单仍显示原套餐名：该订单没有快照，回落到 Unscoped 实时查询。
	c, w = testContext(http.MethodGet, "/api/v1/admin/orders", nil)
	deps.AdminOrders(c)
	require.Equal(t, http.StatusOK, w.Code)
	var orders struct {
		Data struct {
			Items []orderView `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &orders))
	require.Len(t, orders.Data.Items, 1)
	assert.Equal(t, "旧套餐", orders.Data.Items[0].PlanName, "归档后历史订单的套餐名不得变空白")

	// 恢复：重新进入常规查询，但保持停售/停续
	c, w = testContext(http.MethodPost, "/api/v1/admin/plans/1/restore", gin.Params{{Key: "id", Value: "1"}})
	deps.AdminRestorePlan(c)
	require.Equal(t, http.StatusOK, w.Code, "恢复失败: %s", w.Body.String())
	var restored models.Plan
	require.NoError(t, db.First(&restored, plan.ID).Error)
	assert.False(t, restored.DeletedAt.Valid)
	assert.False(t, restored.Purchasable, "恢复后应保持停售，需管理员显式开启")
}

// 有用户持有仍必须是硬阻断：用户是活着的计费关系，不能随套餐一起归档。
func TestAdminDeletePlanBlockedByUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPlanArchiveTestDB(t)
	deps := &Deps{DB: db}

	plan := models.Plan{Name: "在售套餐", PriceCents: 1000, TrafficGB: 100, DurationDays: 30, Purchasable: true, Renewable: true}
	require.NoError(t, db.Create(&plan).Error)
	user := models.User{
		Username: "holder@test.local", Email: "holder@test.local", UUID: "uuid-holder",
		PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive,
		SubscribeToken: "tok-holder", PlanID: plan.ID,
	}
	require.NoError(t, db.Create(&user).Error)

	c, w := testContext(http.MethodDelete, "/api/v1/admin/plans/1", gin.Params{{Key: "id", Value: "1"}})
	deps.AdminDeletePlan(c)
	assert.Equal(t, http.StatusBadRequest, w.Code, "有用户持有时必须拒绝归档")

	var got models.Plan
	require.NoError(t, db.First(&got, plan.ID).Error)
	assert.False(t, got.DeletedAt.Valid, "拒绝时不得归档")
	assert.True(t, got.Purchasable, "拒绝时不得改动销售属性")
}

// 归档套餐仍绑定权限组时，权限组不得被删除（恢复后引用会悬空）。
func TestArchivedPlanStillBlocksPermissionGroupDeletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPlanArchiveTestDB(t)
	deps := &Deps{DB: db}

	group := models.PermissionGroup{Name: "g-archive"}
	require.NoError(t, db.Create(&group).Error)
	plan := models.Plan{
		Name: "归档但绑组", PriceCents: 100, TrafficGB: 10, DurationDays: 30,
		PermissionGroupID: group.ID, Purchasable: true, Renewable: true,
	}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Delete(&plan).Error) // 软删除

	c, w := testContext(http.MethodDelete, "/api/v1/admin/permission-groups/1", gin.Params{{Key: "id", Value: "1"}})
	deps.AdminDeletePermissionGroup(c)
	assert.Equal(t, http.StatusBadRequest, w.Code, "归档套餐仍绑定权限组，应拒绝删除")
}
