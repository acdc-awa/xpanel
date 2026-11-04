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

// TestAdminReorderPermissionGroups 覆盖权限组卡片拖拽重排（全量 ID、下标即 sort_order）。
func TestAdminReorderPermissionGroups(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := orderTestDB(t, "file:mem_reorder_pgs?mode=memory&cache=shared")
	deps := &Deps{DB: db}
	r := gin.New()
	r.PUT("/permission-groups/order", deps.AdminReorderPermissionGroups)

	ids := make([]uint64, 0, 3)
	for i, name := range []string{"a", "b", "c"} {
		g := models.PermissionGroup{Name: name, SortOrder: i}
		require.NoError(t, db.Create(&g).Error)
		ids = append(ids, g.ID)
	}

	body := `{"ids":[` + strconv.FormatUint(ids[2], 10) + `,` + strconv.FormatUint(ids[0], 10) + `,` + strconv.FormatUint(ids[1], 10) + `]}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/permission-groups/order", strings.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var rows []models.PermissionGroup
	require.NoError(t, db.Order("sort_order ASC, id ASC").Find(&rows).Error)
	require.Len(t, rows, 3)
	assert.Equal(t, []uint64{ids[2], ids[0], ids[1]}, []uint64{rows[0].ID, rows[1].ID, rows[2].ID})
	for i, row := range rows {
		assert.Equal(t, i, row.SortOrder)
	}

	// 缺一个 = 页面数据过期，必须拒绝且不改动现有顺序。
	stale := `{"ids":[` + strconv.FormatUint(ids[0], 10) + `,` + strconv.FormatUint(ids[1], 10) + `]}`
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/permission-groups/order", strings.NewReader(stale)))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// TestAdminCreatePermissionGroupAppendsSortOrder 新建权限组追加到列表末尾。
func TestAdminCreatePermissionGroupAppendsSortOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := orderTestDB(t, "file:mem_create_pg_sort?mode=memory&cache=shared")
	deps := &Deps{DB: db}
	r := gin.New()
	r.POST("/permission-groups", deps.AdminCreatePermissionGroup)

	for i := 0; i < 2; i++ {
		g := models.PermissionGroup{Name: "g" + strconv.Itoa(i), SortOrder: i}
		require.NoError(t, db.Create(&g).Error)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/permission-groups", strings.NewReader(`{"name":"new"}`)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var created models.PermissionGroup
	require.NoError(t, db.Where("name = ?", "new").First(&created).Error)
	assert.Equal(t, 2, created.SortOrder)
}
