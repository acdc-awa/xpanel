package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/acdc-awa/xpanel/internal/models"
)

// TestAdminReorderPlans 覆盖套餐拖拽重排：只收未归档套餐，数组下标即新 sort_order。
func TestAdminReorderPlans(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := orderTestDB(t, "file:mem_reorder_plans?mode=memory&cache=shared")
	deps := &Deps{DB: db}
	r := gin.New()
	r.PUT("/plans/order", deps.AdminReorderPlans)

	mk := func(name string, sort int) models.Plan {
		p := models.Plan{
			Name: name, PriceCents: 100, TrafficGB: 10, DurationDays: 30,
			SortOrder: sort,
		}
		require.NoError(t, db.Create(&p).Error)
		return p
	}
	a, b, c := mk("a", 0), mk("b", 1), mk("c", 2)

	// 归档套餐不参与：它在商城与管理端默认列表都不可见。
	archived := mk("old", 0)
	require.NoError(t, db.Delete(&models.Plan{}, archived.ID).Error)

	body := `{"ids":[` + strconv.FormatUint(c.ID, 10) + `,` + strconv.FormatUint(a.ID, 10) + `,` + strconv.FormatUint(b.ID, 10) + `]}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/plans/order", strings.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var rows []models.Plan
	require.NoError(t, db.Order("sort_order ASC, id ASC").Find(&rows).Error)
	require.Len(t, rows, 3)
	assert.Equal(t, []uint64{c.ID, a.ID, b.ID}, []uint64{rows[0].ID, rows[1].ID, rows[2].ID})
	for i, row := range rows {
		assert.Equal(t, i, row.SortOrder)
	}

	// 已归档套餐的排序值不被重排触碰。
	var archivedAfter models.Plan
	require.NoError(t, db.Unscoped().First(&archivedAfter, archived.ID).Error)
	assert.Equal(t, 0, archivedAfter.SortOrder)

	// 带上已归档 ID = 页面数据过期，必须拒绝且不改动现有顺序。
	stale := `{"ids":[` + strconv.FormatUint(archived.ID, 10) + `,` + strconv.FormatUint(a.ID, 10) + `,` + strconv.FormatUint(b.ID, 10) + `,` + strconv.FormatUint(c.ID, 10) + `]}`
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/plans/order", strings.NewReader(stale)))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var unchanged []models.Plan
	require.NoError(t, db.Order("sort_order ASC, id ASC").Find(&unchanged).Error)
	assert.Equal(t, []uint64{c.ID, a.ID, b.ID}, []uint64{unchanged[0].ID, unchanged[1].ID, unchanged[2].ID})
}

// TestReorderRoutesCoexist 静态段 /order 与 :id 通配段同级共存：Gin 的路由树在
// 新增 order 路由时必须不 panic（与 servers 的 /order 同形）。
func TestReorderRoutesCoexist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	assert.NotPanics(t, func() {
		r.PUT("/plans/order", func(*gin.Context) {})
		r.PUT("/plans/:id", func(*gin.Context) {})
		r.POST("/plans/:id/restore", func(*gin.Context) {})
		r.PUT("/permission-groups/order", func(*gin.Context) {})
		r.PUT("/permission-groups/:id", func(*gin.Context) {})
		r.PUT("/permission-groups/:id/access-points", func(*gin.Context) {})
	})
}

// TestAdminCreatePlanAppendsSortOrder 新建套餐未显式指定排序时追加到列表末尾。
func TestAdminCreatePlanAppendsSortOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := orderTestDB(t, "file:mem_create_plan_sort?mode=memory&cache=shared")
	deps := &Deps{DB: db}
	r := gin.New()
	r.POST("/plans", deps.AdminCreatePlan)

	for i := 0; i < 3; i++ {
		p := models.Plan{
			Name: "p" + strconv.Itoa(i), PriceCents: 100, TrafficGB: 10, DurationDays: 30,
			SortOrder: i,
		}
		require.NoError(t, db.Create(&p).Error)
	}

	body := `{"name":"new","price_cents":100,"traffic_gb":10,"duration_days":30}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var created models.Plan
	require.NoError(t, db.Where("name = ?", "new").First(&created).Error)
	assert.Equal(t, 3, created.SortOrder)
}
