package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/master/api"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// stubTraffic 供 AdminUsers 使用（UserBilled 无真实流量表）。
type stubTraffic struct{}

func (stubTraffic) Save(tr protocol.TrafficReportPayload, serverID uint64) ([]uint64, error) {
	return nil, nil
}
func (stubTraffic) FindViolators(userIDs []uint64) ([]uint64, error) { return nil, nil }
func (stubTraffic) UserBilled(userID uint64) (up, down int64, err error) { return 0, 0, nil }
func (stubTraffic) UsersBilled(userIDs []uint64) (map[uint64][2]int64, error) {
	return map[uint64][2]int64{}, nil
}

func TestAdminUsersKeywordSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}))

	users := []models.User{
		{Email: "alice@x.com", UUID: "aaaaaaaa-1111-1111-1111-111111111111", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "tok-a", TrafficCycleStart: time.Now()},
		{Email: "bob@y.com", UUID: "bbbbbbbb-2222-2222-2222-222222222222", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "tok-b", TrafficCycleStart: time.Now()},
		{Email: "carol@x.com", UUID: "cccccccc-3333-3333-3333-333333333333", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "tok-c", TrafficCycleStart: time.Now()},
	}
	require.NoError(t, db.Create(&users).Error)

	deps := &api.Deps{DB: db, Traffic: stubTraffic{}}
	r := gin.New()
	r.GET("/api/v1/admin/users", deps.AdminUsers)

	fetch := func(qs string) (int64, []string) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?"+qs, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code int `json:"code"`
			Data struct {
				Items []struct {
					Email string `json:"email"`
				} `json:"items"`
				Total int64 `json:"total"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp.Code, "响应: %s", w.Body.String())
		names := make([]string, 0, len(resp.Data.Items))
		for _, it := range resp.Data.Items {
			names = append(names, it.Email)
		}
		return resp.Data.Total, names
	}

	// 无 keyword：全量 + 默认分页
	total, names := fetch("")
	assert.Equal(t, int64(3), total)
	assert.Len(t, names, 3)

	// email 模糊匹配
	total, names = fetch("keyword=bob")
	assert.Equal(t, int64(1), total)
	assert.Equal(t, []string{"bob@y.com"}, names)

	// 域名关键词命中两个邮箱
	total, _ = fetch("keyword=x.com")
	assert.Equal(t, int64(2), total)

	// UUID 前缀匹配
	total, names = fetch("keyword=cccccccc")
	assert.Equal(t, int64(1), total)
	assert.Equal(t, []string{"carol@x.com"}, names)

	// 无结果
	total, names = fetch("keyword=nonexistent")
	assert.Equal(t, int64(0), total)
	assert.Empty(t, names)

	// 分页 + keyword 组合：total 反映过滤后全量，items 只回当前页
	total, names = fetch("keyword=x.com&page=1&size=1")
	assert.Equal(t, int64(2), total)
	assert.Len(t, names, 1)
}

func TestAdminUsersFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}))

	now := time.Now()
	users := []models.User{
		// active admin, no plan, no group
		{Email: "admin@x.com", UUID: "11111111-1111-1111-1111-111111111111", Role: models.RoleAdmin, Status: models.StatusActive, PlanID: 0, PermissionGroupID: 0, PlanGroupID: 0, SubscribeToken: "tok-1", TrafficCycleStart: now},
		// active user, plan 10, inherits plan group 2
		{Email: "inherit@x.com", UUID: "22222222-2222-2222-2222-222222222222", Role: models.RoleUser, Status: models.StatusActive, PlanID: 10, PermissionGroupID: 0, PlanGroupID: 2, SubscribeToken: "tok-2", TrafficCycleStart: now},
		// active user, plan 10, custom group 3 overrides plan group 2
		{Email: "custom@x.com", UUID: "33333333-3333-3333-3333-333333333333", Role: models.RoleUser, Status: models.StatusActive, PlanID: 10, PermissionGroupID: 3, PlanGroupID: 2, SubscribeToken: "tok-3", TrafficCycleStart: now},
		// disabled user, plan 20, custom group 2
		{Email: "banned@y.com", UUID: "44444444-4444-4444-4444-444444444444", Role: models.RoleUser, Status: models.StatusActive, PlanID: 20, PermissionGroupID: 2, PlanGroupID: 0, SubscribeToken: "tok-4", TrafficCycleStart: now},
	}
	require.NoError(t, db.Create(&users).Error)
	// GORM default:1 会把零值 Status 覆盖成 1，这里显式置为禁用
	require.NoError(t, db.Model(&users[3]).Update("status", models.StatusDisabled).Error)

	deps := &api.Deps{DB: db, Traffic: stubTraffic{}}
	r := gin.New()
	r.GET("/api/v1/admin/users", deps.AdminUsers)

	fetch := func(qs string) (int64, []string) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?"+qs, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code int `json:"code"`
			Data struct {
				Items []struct {
					Email string `json:"email"`
				} `json:"items"`
				Total int64 `json:"total"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp.Code, "响应: %s", w.Body.String())
		names := make([]string, 0, len(resp.Data.Items))
		for _, it := range resp.Data.Items {
			names = append(names, it.Email)
		}
		return resp.Data.Total, names
	}

	// 状态筛选
	total, names := fetch("status=1")
	assert.Equal(t, int64(3), total)
	assert.ElementsMatch(t, []string{"admin@x.com", "inherit@x.com", "custom@x.com"}, names)

	total, names = fetch("status=0")
	assert.Equal(t, int64(1), total)
	assert.Equal(t, []string{"banned@y.com"}, names)

	// 套餐筛选
	total, names = fetch("plan_id=0")
	assert.Equal(t, int64(1), total)
	assert.Equal(t, []string{"admin@x.com"}, names)

	total, names = fetch("plan_id=10")
	assert.Equal(t, int64(2), total)
	assert.ElementsMatch(t, []string{"inherit@x.com", "custom@x.com"}, names)

	// 权限组筛选（严格按 EffectiveGroupID 同口径）
	total, names = fetch("group_id=0")
	assert.Equal(t, int64(1), total)
	assert.Equal(t, []string{"admin@x.com"}, names)

	// group_id=2 应命中 inherit(继承2) 与 banned(自定义2)，不得误命中 custom(自定义3覆盖套餐2)
	total, names = fetch("group_id=2")
	assert.Equal(t, int64(2), total)
	assert.ElementsMatch(t, []string{"inherit@x.com", "banned@y.com"}, names)

	total, names = fetch("group_id=3")
	assert.Equal(t, int64(1), total)
	assert.Equal(t, []string{"custom@x.com"}, names)

	// 角色筛选
	total, names = fetch("role=admin")
	assert.Equal(t, int64(1), total)
	assert.Equal(t, []string{"admin@x.com"}, names)

	total, names = fetch("role=user")
	assert.Equal(t, int64(3), total)

	// 组合筛选：status + plan_id + group_id + keyword
	total, names = fetch("status=1&plan_id=10&group_id=3&keyword=x.com")
	assert.Equal(t, int64(1), total)
	assert.Equal(t, []string{"custom@x.com"}, names)

	// 非法参数忽略，不报错
	total, _ = fetch("status=invalid&role=unknown&plan_id=abc&group_id=-1")
	assert.Equal(t, int64(4), total)
}
