package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/api"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 删除完整性回归：删入站被盒内路由规则引用必须拒绝（InboundTag 支持单 tag 与 JSON 数组复合格式）。
func TestAdminDeleteInboundBlockedByRoutingRule(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Server{}, &models.Inbound{}, &models.ServerRoutingRule{}, &models.ServerOutbound{}, &models.UserAccessPoint{}))

	srv := models.Server{Name: "s1", Host: "1.2.3.4", NodeID: "n1", Secret: "s1"}
	require.NoError(t, db.Create(&srv).Error)
	inb := models.Inbound{ServerID: srv.ID, Tag: "in-1", Protocol: "vless", Type: "user"}
	require.NoError(t, db.Create(&inb).Error)
	// 单 tag 规则 + JSON 数组复合格式规则（同服务器，另一入站 in-2 不存在也要拦截）
	rule1 := models.ServerRoutingRule{ServerID: srv.ID, OutboundTag: "direct", InboundTag: "in-1"}
	rule2 := models.ServerRoutingRule{ServerID: srv.ID, OutboundTag: "direct", InboundTag: `["in-1","in-2"]`}
	require.NoError(t, db.Create(&rule1).Error)
	require.NoError(t, db.Create(&rule2).Error)

	deps := &api.Deps{DB: db}
	del := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/inbounds/%d", inb.ID), nil)
		c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", inb.ID)}}
		deps.AdminDeleteInbound(c)
		return w
	}

	w := del()
	assert.Equal(t, http.StatusBadRequest, w.Code, "被路由规则引用应拒绝删除: %s", w.Body.String())

	var ruleCnt int64
	require.NoError(t, db.Model(&models.ServerRoutingRule{}).Count(&ruleCnt).Error)
	assert.Equal(t, int64(2), ruleCnt, "拒绝删除时规则不得被误删")

	// 清掉规则后允许删除
	require.NoError(t, db.Delete(&rule1).Error)
	require.NoError(t, db.Delete(&rule2).Error)
	w = del()
	assert.Equal(t, http.StatusOK, w.Code, "解除引用后应可删除: %s", w.Body.String())
	var n int64
	require.NoError(t, db.Model(&models.Inbound{}).Where("id = ?", inb.ID).Count(&n).Error)
	assert.Zero(t, n)
}

// 删除完整性回归：删服务器必须级联删对外接入层（层无宿主后成孤儿记录）。
func TestAdminDeleteServerCascadesAccessLayer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Server{}, &models.Inbound{}, &models.AccessLayer{}, &models.ServerOutbound{}, &models.ServerRoutingRule{}, &models.PendingConfig{}, &models.PendingCert{}, &models.NodeReport{}, &models.UserAccessPoint{}))

	srv := models.Server{Name: "s1", Host: "1.2.3.4", NodeID: "n1", Secret: "s1"}
	require.NoError(t, db.Create(&srv).Error)
	layer := models.AccessLayer{ServerID: srv.ID, Name: "反代层", Host: "cdn.example.com", Port: 443, Security: "tls"}
	require.NoError(t, db.Create(&layer).Error)

	deps := &api.Deps{DB: db}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/servers/%d", srv.ID), nil)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", srv.ID)}}
	deps.AdminDeleteServer(c)
	assert.Equal(t, http.StatusOK, w.Code, "删除服务器失败: %s", w.Body.String())

	var n int64
	require.NoError(t, db.Model(&models.AccessLayer{}).Where("server_id = ?", srv.ID).Count(&n).Error)
	assert.Zero(t, n, "对外接入层应随服务器级联删除")
	var srvN int64
	require.NoError(t, db.Model(&models.Server{}).Where("id = ?", srv.ID).Count(&srvN).Error)
	assert.Zero(t, srvN)
}
