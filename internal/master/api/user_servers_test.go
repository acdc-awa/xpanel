package api_test

import (
	"encoding/json"
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
	"github.com/acdc-awa/xpanel/internal/models"
)

// TestUserServersOmitsHost 验证用户端节点列表不返回源站服务器的真实 Host/IP，避免暴露落地源站。
func TestUserServersOmitsHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&models.User{}, &models.PermissionGroup{}, &models.UserAccessPoint{},
		&models.PermissionGroupAccessPoint{}, &models.Server{}, &models.Inbound{},
	))

	grp := models.PermissionGroup{Name: "VIP"}
	require.NoError(t, db.Create(&grp).Error)
	user := models.User{
		Email: "user@example.com", UUID: "uuid-u1", SubscribeToken: "sub-u1",
		Role: models.RoleUser, Status: models.StatusActive, PermissionGroupID: grp.ID,
		TrafficCycleStart: time.Now(),
	}
	require.NoError(t, db.Create(&user).Error)

	now := time.Now()
	srv := models.Server{Name: "Tokyo-01", Host: "203.0.113.88", Location: "JP", NodeID: "n-jp", Secret: "s", LastSeenAt: &now}
	require.NoError(t, db.Create(&srv).Error)
	inb := models.Inbound{ServerID: srv.ID, Tag: "vless-1", Protocol: models.ProtocolVless, Port: 443, Enabled: true}
	require.NoError(t, db.Create(&inb).Error)
	ap := models.UserAccessPoint{Name: "JP Node", TargetType: "inbound", TargetInboundID: &inb.ID, Enabled: true}
	require.NoError(t, db.Create(&ap).Error)
	require.NoError(t, db.Create(&models.PermissionGroupAccessPoint{PermissionGroupID: grp.ID, AccessPointID: ap.ID}).Error)

	deps := &api.Deps{DB: db}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("claims", &contracts.JWTClaims{UserID: user.ID, Role: models.RoleUser, Type: "access"})
		c.Next()
	})
	r.GET("/api/v1/user/servers", deps.UserServers)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/servers", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data.Items, 1)
	_, hasHost := resp.Data.Items[0]["host"]
	assert.False(t, hasHost, "用户端响应不得暴露服务器 host 字段")
	assert.NotContains(t, w.Body.String(), "203.0.113.88")
	assert.Equal(t, "Tokyo-01", resp.Data.Items[0]["name"])
}
