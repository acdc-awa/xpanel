package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func orderTestDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, models.AutoMigrate(db))
	return db
}

// TestServerRoutesCoexist 静态段与 :id 通配段同级共存（Gin 的树对静态/通配兄弟是允许的，
// 但不同命名的通配符会 panic）。这里把本次新增/相邻的服务器路由按真实形状注册一遍，
// 防止未来升级 Gin 或调整路由顺序时静默炸在启动阶段。
func TestServerRoutesCoexist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	assert.NotPanics(t, func() {
		r.POST("/servers/command", func(*gin.Context) {})
		r.PUT("/servers/order", func(*gin.Context) {})
		r.POST("/servers/batch-upgrade", func(*gin.Context) {})
		r.GET("/servers/upgrade-status", func(*gin.Context) {})
		r.GET("/servers/agent-version", func(*gin.Context) {})
		r.PUT("/servers/:id", func(*gin.Context) {})
		r.POST("/servers/:id/reset-secret", func(*gin.Context) {})
		r.GET("/servers/:id/upgrade-status", func(*gin.Context) {})
	})
}

func TestValidOrder(t *testing.T) {	cases := []struct {
		name string
		ids  []uint64
		all  []uint64
		want bool
	}{
		{"empty", nil, nil, false},
		{"same", []uint64{1, 2, 3}, []uint64{1, 2, 3}, true},
		{"reordered", []uint64{3, 1, 2}, []uint64{1, 2, 3}, true},
		{"missing", []uint64{1, 2}, []uint64{1, 2, 3}, false},
		{"extra", []uint64{1, 2, 3, 4}, []uint64{1, 2, 3}, false},
		{"duplicate", []uint64{1, 1, 2}, []uint64{1, 2, 3}, false},
		{"different set", []uint64{1, 2, 4}, []uint64{1, 2, 3}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, validOrder(tc.ids, tc.all))
		})
	}
}

func TestDedupeIDs(t *testing.T) {
	assert.Nil(t, dedupeIDs(nil))
	assert.Equal(t, []uint64{3, 1, 2}, dedupeIDs([]uint64{3, 1, 3, 2, 1}))
}

func TestNextServerSortOrder(t *testing.T) {
	db := orderTestDB(t, "file:mem_next_sort?mode=memory&cache=shared")
	assert.Equal(t, 0, nextServerSortOrder(db), "空表应返回 0")

	mk := func(name string, sort int) models.Server {
		s := models.Server{Name: name, Host: "h", NodeID: "node-" + name, Secret: "sec", SortOrder: sort}
		require.NoError(t, db.Create(&s).Error)
		return s
	}
	mk("a", 0)
	mk("b", 3)
	last := mk("c", 7)
	assert.Equal(t, 8, nextServerSortOrder(db))

	// 归档（软删除）的服务器不参与：否则新服务器会被已删行顶到中间。
	require.NoError(t, db.Delete(&models.Server{}, last.ID).Error)
	assert.Equal(t, 4, nextServerSortOrder(db))
}

func TestAdminReorderServers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := orderTestDB(t, "file:mem_reorder?mode=memory&cache=shared")
	deps := &Deps{DB: db}
	r := gin.New()
	r.PUT("/servers/order", deps.AdminReorderServers)

	ids := make([]uint64, 0, 3)
	for i, name := range []string{"a", "b", "c"} {
		s := models.Server{Name: name, Host: "h", NodeID: "node-" + name, Secret: "sec", SortOrder: i}
		require.NoError(t, db.Create(&s).Error)
		ids = append(ids, s.ID)
	}

	body := `{"ids":[` + strconv.FormatUint(ids[2], 10) + `,` + strconv.FormatUint(ids[0], 10) + `,` + strconv.FormatUint(ids[1], 10) + `]}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/servers/order", strings.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var rows []models.Server
	require.NoError(t, db.Order("sort_order ASC, id ASC").Find(&rows).Error)
	require.Len(t, rows, 3)
	assert.Equal(t, []uint64{ids[2], ids[0], ids[1]}, []uint64{rows[0].ID, rows[1].ID, rows[2].ID})
	for i, row := range rows {
		assert.Equal(t, i, row.SortOrder)
	}

	// 缺一台 = 页面数据过期，必须拒绝且不改动现有顺序。
	stale := `{"ids":[` + strconv.FormatUint(ids[0], 10) + `,` + strconv.FormatUint(ids[1], 10) + `]}`
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/servers/order", strings.NewReader(stale)))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}
