package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/api"
	"github.com/acdc-awa/xpanel/internal/master/billing"
	"github.com/acdc-awa/xpanel/internal/master/store/gormstore"
	"github.com/acdc-awa/xpanel/internal/models"
)

// TestAdminVoidGiftCardKeepsRecord 回归：作废必须是软作废（status=disabled、行保留），
// 只有 /purge 才物理删除；否则「已作废」筛选永远为空、卡密历史不可追溯。
func TestAdminVoidGiftCardKeepsRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.GiftCard{}))
	deps := &api.Deps{DB: db, GiftCard: billing.NewGiftCardService(gormstore.NewBillingStore(db))}

	unused := models.GiftCard{Code: "VOID-1", Name: "作废测试", FaceValueCents: 1000, Status: models.GiftCardUnused, CreatedAt: time.Now()}
	require.NoError(t, db.Create(&unused).Error)
	used := models.GiftCard{Code: "VOID-2", Name: "已兑换", FaceValueCents: 1000, Status: models.GiftCardUsed, UsedBy: 7, CreatedAt: time.Now()}
	require.NoError(t, db.Create(&used).Error)

	call := func(handler gin.HandlerFunc, id uint64) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/gift-cards/%d", id), nil)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", id)}}
		handler(c)
		return w
	}

	// 1. 作废未使用的卡 → 200，行仍在且状态为 disabled
	w := call(deps.AdminDeleteGiftCard, unused.ID)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got models.GiftCard
	require.NoError(t, db.First(&got, unused.ID).Error, "作废后行必须保留")
	assert.Equal(t, models.GiftCardDisabled, got.Status)

	// 2. 已兑换的卡作废被拒（余额已发出，作废会造成账实不符）
	w = call(deps.AdminDeleteGiftCard, used.ID)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 3. 彻底删除：已作废的卡可以删，已兑换的不行
	w = call(deps.AdminPurgeGiftCard, unused.ID)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var n int64
	require.NoError(t, db.Model(&models.GiftCard{}).Where("id = ?", unused.ID).Count(&n).Error)
	assert.Zero(t, n, "彻底删除后行应消失")

	w = call(deps.AdminPurgeGiftCard, used.ID)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
