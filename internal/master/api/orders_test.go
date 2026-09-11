package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func setupOrdersTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}, &models.Plan{}, &models.Order{}))

	users := []models.User{
		{ID: 1, Username: "alice", Email: "alice@example.com", UUID: "11111111-1111-1111-1111-111111111111", SubscribeToken: "tok-1"},
		{ID: 2, Username: "bob", Email: "bob@domain.org", UUID: "22222222-2222-2222-2222-222222222222", SubscribeToken: "tok-2"},
	}
	require.NoError(t, db.Create(&users).Error)

	plan := models.Plan{ID: 1, Name: "Standard Plan"}
	require.NoError(t, db.Create(&plan).Error)

	now := time.Now()
	orders := []models.Order{
		{ID: 1, OrderNo: "ORD-2026-0001", UserID: 1, PlanID: 1, AmountCents: 1000, Status: "paid", CreatedAt: now.Add(-3 * time.Hour), PaidAt: &now},
		{ID: 2, OrderNo: "ORD-2026-0002", UserID: 1, PlanID: 1, AmountCents: 2000, Status: "pending", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: 3, OrderNo: "ORD-2026-0003", UserID: 2, PlanID: 1, AmountCents: 3000, Status: "cancelled", CreatedAt: now.Add(-1 * time.Hour)},
	}
	require.NoError(t, db.Create(&orders).Error)
	return db
}

func TestAdminOrdersFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupOrdersTestDB(t)
	deps := &Deps{DB: db}

	r := gin.New()
	r.GET("/api/v1/admin/orders", deps.AdminOrders)

	type response struct {
		Code int `json:"code"`
		Data struct {
			Total int         `json:"total"`
			Items []orderView `json:"items"`
		} `json:"data"`
	}

	fetch := func(query string) response {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/orders"+query, nil)
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp response
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp
	}

	// 1. All orders
	{
		res := fetch("")
		assert.Equal(t, 3, res.Data.Total)
		assert.Len(t, res.Data.Items, 3)
	}

	// 2. Filter by status=paid
	{
		res := fetch("?status=paid")
		assert.Equal(t, 1, res.Data.Total)
		require.Len(t, res.Data.Items, 1)
		assert.Equal(t, "ORD-2026-0001", res.Data.Items[0].OrderNo)
		assert.Equal(t, "paid", res.Data.Items[0].Status)
	}

	// 3. Filter by keyword on order_no
	{
		res := fetch("?keyword=0002")
		assert.Equal(t, 1, res.Data.Total)
		require.Len(t, res.Data.Items, 1)
		assert.Equal(t, "ORD-2026-0002", res.Data.Items[0].OrderNo)
	}

	// 4. Filter by keyword on username
	{
		res := fetch("?keyword=alice")
		assert.Equal(t, 2, res.Data.Total)
		require.Len(t, res.Data.Items, 2)
	}

	// 5. Filter by keyword on user email
	{
		res := fetch("?keyword=domain.org")
		assert.Equal(t, 1, res.Data.Total)
		require.Len(t, res.Data.Items, 1)
		assert.Equal(t, "ORD-2026-0003", res.Data.Items[0].OrderNo)
	}

	// 6. Combined filter: keyword=alice & status=pending
	{
		res := fetch("?keyword=alice&status=pending")
		assert.Equal(t, 1, res.Data.Total)
		require.Len(t, res.Data.Items, 1)
		assert.Equal(t, "ORD-2026-0002", res.Data.Items[0].OrderNo)
	}

	// 7. Non-matching filter
	{
		res := fetch("?keyword=notfound")
		assert.Equal(t, 0, res.Data.Total)
		assert.Empty(t, res.Data.Items)
	}
}
