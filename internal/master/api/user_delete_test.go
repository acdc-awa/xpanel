package api_test

import (
	"bytes"
	"encoding/json"
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

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/api"
	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/models"
)

// TestAdminDeleteUserHardDeleteRecreate 回归：删除用户必须物理删除（释放 username/email
// 等唯一索引槽位）。曾因软删残留（注释写"硬删除"，实现却走 gorm 软删）导致同邮箱重建
// 撞 "该邮箱已用作用户名"。
func TestAdminDeleteUserHardDeleteRecreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.User{}))

	admin := models.User{Username: "root@x.com", Email: "root@x.com", UUID: "dddddddd-4444-4444-4444-444444444444", PasswordHash: "h", Role: models.RoleAdmin, Status: models.StatusActive, SubscribeToken: "tok-root", TrafficCycleStart: time.Now()}
	require.NoError(t, db.Create(&admin).Error)
	target := models.User{Username: "del@x.com", Email: "del@x.com", UUID: "eeeeeeee-5555-5555-5555-555555555555", PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "tok-del", TrafficCycleStart: time.Now()}
	require.NoError(t, db.Create(&target).Error)

	deps := &api.Deps{DB: db, Traffic: stubTraffic{}}

	// 以管理员身份删除目标用户（注入 claims，走 AdminDeleteUser 的 CurrentUser 路径）
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/users/%d", target.ID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", target.ID)}}
	c.Set(middleware.CtxClaimsKey, &contracts.JWTClaims{UserID: admin.ID, Role: models.RoleAdmin})
	deps.AdminDeleteUser(c)
	assert.Equal(t, http.StatusOK, w.Code, "删除应成功: %s", w.Body.String())

	// 必须物理删除：连 Unscoped 都查不到行
	var n int64
	require.NoError(t, db.Unscoped().Model(&models.User{}).Where("id = ?", target.ID).Count(&n).Error)
	assert.Zero(t, n, "用户行应被物理删除，而非软删残留")

	// 同邮箱重建必须成功
	body, _ := json.Marshal(map[string]any{"email": "del@x.com", "password": "password123"})
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/admin/users", bytes.NewReader(body))
	c2.Request.Header.Set("Content-Type", "application/json")
	c2.Set(middleware.CtxClaimsKey, &contracts.JWTClaims{UserID: admin.ID, Role: models.RoleAdmin})
	deps.AdminCreateUser(c2)
	assert.Equal(t, http.StatusOK, w2.Code, "同邮箱重建应成功: %s", w2.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			ID       uint64 `json:"id"`
			Username string `json:"username"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Code)
	assert.NotEqual(t, target.ID, resp.Data.ID)
	assert.Equal(t, "del@x.com", resp.Data.Username)
}
