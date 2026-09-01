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

// stubTraffic 供 AdminUsers 使用（UserUsed 无真实流量表）。
type stubTraffic struct{}

func (stubTraffic) Save(tr protocol.TrafficReportPayload, serverID uint64) ([]uint64, error) {
	return nil, nil
}
func (stubTraffic) FindViolators(userIDs []uint64) ([]uint64, error)   { return nil, nil }
func (stubTraffic) UserUsed(userID uint64) (up, down int64, err error) { return 0, 0, nil }

func TestAdminUsersKeywordSearch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}))

	users := []models.User{
		{Username: "alice@x.com", Email: "alice@x.com", UUID: "aaaaaaaa-1111-1111-1111-111111111111", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "tok-a", TrafficCycleStart: time.Now()},
		{Username: "bob@y.com", Email: "bob@y.com", UUID: "bbbbbbbb-2222-2222-2222-222222222222", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "tok-b", TrafficCycleStart: time.Now()},
		{Username: "carol@x.com", Email: "carol@x.com", UUID: "cccccccc-3333-3333-3333-333333333333", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "tok-c", TrafficCycleStart: time.Now()},
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
					Username string `json:"username"`
				} `json:"items"`
				Total int64 `json:"total"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, 0, resp.Code, "响应: %s", w.Body.String())
		names := make([]string, 0, len(resp.Data.Items))
		for _, it := range resp.Data.Items {
			names = append(names, it.Username)
		}
		return resp.Data.Total, names
	}

	// 无 keyword：全量 + 默认分页
	total, names := fetch("")
	assert.Equal(t, int64(3), total)
	assert.Len(t, names, 3)

	// username/email 模糊匹配
	total, names = fetch("keyword=bob")
	assert.Equal(t, int64(1), total)
	assert.Equal(t, []string{"bob@y.com"}, names)

	// 域名关键词命中两个邮箱
	total, names = fetch("keyword=x.com")
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
