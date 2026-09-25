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
	require.NoError(t, db.AutoMigrate(&models.Server{}, &models.Inbound{}, &models.AccessLayer{}, &models.ServerOutbound{}, &models.ServerRoutingRule{}, &models.PendingConfig{}, &models.PendingCert{}, &models.NodeReport{}, &models.UserAccessPoint{}, &models.ProxyChannel{}))

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

// 删除完整性回归：删服务器受其他服务器直通管道（tunnel）引用保护，并在删除成功时级联清理 ProxyChannel。
func TestAdminDeleteServer_ProtectedByTunnelAndCascadesChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.Server{}, &models.Inbound{}, &models.AccessLayer{}, &models.ServerOutbound{}, &models.ServerRoutingRule{}, &models.PendingConfig{}, &models.PendingCert{}, &models.NodeReport{}, &models.UserAccessPoint{}, &models.ProxyChannel{}))

	srv1 := models.Server{Name: "landing-srv", Host: "1.2.3.4", NodeID: "n1", Secret: "s1"}
	require.NoError(t, db.Create(&srv1).Error)
	landingInb := models.Inbound{ServerID: srv1.ID, Tag: "landing-vless", Protocol: "vless", Port: 443, Type: models.InboundTypeUser, Enabled: true}
	require.NoError(t, db.Create(&landingInb).Error)

	// srv1 上有一条 ProxyChannel
	chanInb := models.Inbound{ServerID: srv1.ID, Tag: "chan-1", Protocol: "socks", Port: 1080, Type: models.InboundTypeChannel, Enabled: true}
	require.NoError(t, db.Create(&chanInb).Error)
	ch := models.ProxyChannel{ServerID: srv1.ID, InboundID: chanInb.ID, Name: "通道1", Port: 1080, Protocol: "socks5", Enabled: true}
	require.NoError(t, db.Create(&ch).Error)

	// srv2 上的 tunnel 引用 srv1 的 landingInb
	srv2 := models.Server{Name: "edge-srv", Host: "5.6.7.8", NodeID: "n2", Secret: "s2"}
	require.NoError(t, db.Create(&srv2).Error)
	tunnelInb := models.Inbound{ServerID: srv2.ID, Tag: "tunnel-edge", Protocol: "dokodemo-door", Port: 10001, Type: models.InboundTypeTunnel, TargetInboundID: &landingInb.ID, Enabled: true}
	require.NoError(t, db.Create(&tunnelInb).Error)

	deps := &api.Deps{DB: db}

	// 1. 尝试删除 srv1 -> 应该被拦截（被 srv2 的直通管道引用）
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/servers/%d", srv1.ID), nil)
	c1.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", srv1.ID)}}
	deps.AdminDeleteServer(c1)
	assert.Equal(t, http.StatusBadRequest, w1.Code, "被其他机器直通管道引用的服务器不得删除: %s", w1.Body.String())

	// 2. 解除 tunnel 引用
	tunnelInb.TargetInboundID = nil
	require.NoError(t, db.Save(&tunnelInb).Error)

	// 3. 再次删除 srv1 -> 应该成功
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/admin/servers/%d", srv1.ID), nil)
	c2.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", srv1.ID)}}
	deps.AdminDeleteServer(c2)
	assert.Equal(t, http.StatusOK, w2.Code, "解除引用后应可删除服务器: %s", w2.Body.String())

	// 4. 验证 ProxyChannel 已随服务器级联删除
	var chCnt int64
	require.NoError(t, db.Model(&models.ProxyChannel{}).Where("server_id = ?", srv1.ID).Count(&chCnt).Error)
	assert.Zero(t, chCnt, "服务器上的 ProxyChannel 应随服务器级联删除")
}
