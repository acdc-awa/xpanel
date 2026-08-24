package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/subscribe"
	"github.com/acdc-awa/xpanel/internal/models"
)

func setupTestDBForLayer(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.AutoMigrate(db))
	return db
}

// 对外接入层：CRUD / 入站挂层校验 / 删除层回退原生 / 订阅消费层端点（含 L4 不沿链）。
func TestAccessLayers_CRUD_Bind_And_Subscribe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDBForLayer(t)

	deps := &Deps{DB: db}
	r := gin.New()
	r.POST("/api/v1/admin/servers/:id/layers", deps.AdminCreateLayer)
	r.PUT("/api/v1/admin/servers/:id/layers/:layer_id", deps.AdminUpdateLayer)
	r.DELETE("/api/v1/admin/servers/:id/layers/:layer_id", deps.AdminDeleteLayer)
	r.GET("/api/v1/admin/servers/:id/layers", deps.AdminGetLayers)
	r.POST("/api/v1/admin/inbounds", deps.AdminCreateInbound)
	r.PUT("/api/v1/admin/inbounds/:id", deps.AdminUpdateInbound)
	r.GET("/api/v1/admin/topology", deps.AdminTopology)

	// 1. 服务器（挂层服务器 + 另一台服务器用于越权校验）
	hkSrv := models.Server{ServerType: models.ServerTypeXray, Name: "香港01", Host: "hk.node.com", NodeID: "node-hk", Secret: "s", Status: 1}
	gzSrv := models.Server{ServerType: models.ServerTypeXray, Name: "广州02", Host: "gz.node.com", NodeID: "node-gz", Secret: "s", Status: 1}
	require.NoError(t, db.Create(&hkSrv).Error)
	require.NoError(t, db.Create(&gzSrv).Error)

	// 2. 创建对外层
	layerBody := map[string]any{
		"name": "HK 443 反代层", "host": "hk.edge.example.com", "port": 443, "security": "tls", "remark": "Caddy 前置",
	}
	bb, _ := json.Marshal(layerBody)
	req := httptest.NewRequest("POST", "/api/v1/admin/servers/1/layers", bytes.NewReader(bb))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())
	var createResp struct {
		Code int `json:"code"`
		Data struct {
			Layer accessLayerView `json:"layer"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	require.Equal(t, "tls", createResp.Data.Layer.Security)
	layerID := createResp.Data.Layer.ID
	require.NotZero(t, layerID)

	// 3. 创建挂层入站（XHTTP 明文内部监听）
	inbBody := map[string]any{
		"server_id": hkSrv.ID, "tag": "xhttp-web", "protocol": "vless", "port": 10086,
		"listen":              "127.0.0.1",
		"stream_settings":     `{"network":"xhttp","security":"none","xhttpSettings":{"mode":"auto","path":"/xhttp"}}`,
		"share_path":          "/web",
		"layer_id":            layerID,
		"share_addr_strategy": "node",
	}
	bb, _ = json.Marshal(inbBody)
	req = httptest.NewRequest("POST", "/api/v1/admin/inbounds", bytes.NewReader(bb))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())
	var inbResp struct {
		Data struct {
			Inbound inboundView `json:"inbound"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &inbResp))
	require.NotNil(t, inbResp.Data.Inbound.LayerID)
	require.Equal(t, layerID, *inbResp.Data.Inbound.LayerID)

	// 4. 跨服务器挂层拒绝
	badBody := map[string]any{
		"server_id": gzSrv.ID, "tag": "xhttp-web2", "protocol": "vless", "port": 20086,
		"layer_id": layerID,
	}
	bb, _ = json.Marshal(badBody)
	req = httptest.NewRequest("POST", "/api/v1/admin/inbounds", bytes.NewReader(bb))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 400, w.Code, w.Body.String())

	// 5. 订阅消费：AP 直连挂层入站 → host/port/security 由层决议
	layerMap := map[uint64]models.AccessLayer{layerID: {ID: layerID, ServerID: hkSrv.ID, Name: "HK 443 反代层", Host: "hk.edge.example.com", Port: 443, Security: "tls"}}
	inbMap := map[uint64]models.Inbound{inbResp.Data.Inbound.ID: {
		ID: inbResp.Data.Inbound.ID, ServerID: hkSrv.ID, Tag: "xhttp-web", Protocol: "vless",
		Port: 10086, StreamSettings: `{"network":"xhttp","security":"none","xhttpSettings":{"mode":"auto","path":"/xhttp"}}`,
		SharePath: "/web", LayerID: &[]uint64{layerID}[0],
	}}
	srvMap := map[uint64]models.Server{hkSrv.ID: hkSrv}
	apInbID := inbResp.Data.Inbound.ID
	ap := models.UserAccessPoint{Name: "海外直连", TargetType: "inbound", TargetInboundID: &apInbID, Enabled: true}
	dto := subscribe.ResolveAPSubscription(&ap, srvMap, inbMap, map[uint64]models.L4PortRule{}, layerMap, "11111111-2222-3333-4444-555555555555")
	require.NotNil(t, dto)
	require.Equal(t, "hk.edge.example.com", dto.ServerHost)
	require.Equal(t, 443, dto.ServerPort)
	require.NotNil(t, dto.Security)
	require.Equal(t, "tls", dto.Security.Type)
	require.Equal(t, "hk.edge.example.com", dto.Security.SNI)

	// 6. L4 链不沿层：AP → L4 规则 → 挂层入站，订阅 host/port = 转发机，security 跟随入站（none）
	l4Srv := models.Server{ServerType: models.ServerTypeL4Relay, Name: "广州中转", Host: "gz.relay.com", NodeID: "node-l4", Secret: "s", Status: 1}
	require.NoError(t, db.Create(&l4Srv).Error)
	srvMap[l4Srv.ID] = l4Srv
	l4Rule := models.L4PortRule{ServerID: l4Srv.ID, ListenPort: 30001, TargetServerID: hkSrv.ID, TargetInboundID: apInbID, Enabled: true}
	require.NoError(t, db.Create(&l4Rule).Error)
	apL4ID := l4Rule.ID
	ap2 := models.UserAccessPoint{Name: "中转入口", TargetType: "l4_rule", TargetL4RuleID: &apL4ID, Enabled: true}
	dto2 := subscribe.ResolveAPSubscription(&ap2, srvMap, inbMap, map[uint64]models.L4PortRule{l4Rule.ID: l4Rule}, layerMap, "11111111-2222-3333-4444-555555555555")
	require.NotNil(t, dto2)
	require.Equal(t, "gz.relay.com", dto2.ServerHost)
	require.Equal(t, 30001, dto2.ServerPort)
	require.NotNil(t, dto2.Security)
	require.Equal(t, "none", dto2.Security.Type, "L4 链不应消费层对外 TLS")

	// 7. 拓扑聚合返回 layers 且入站带 layer_id
	req = httptest.NewRequest("GET", "/api/v1/admin/topology", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	var topo struct {
		Data struct {
			Layers []accessLayerView `json:"layers"`
			Inbs   []inboundView     `json:"inbounds"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &topo))
	require.Len(t, topo.Data.Layers, 1)
	require.Equal(t, 1, topo.Data.Layers[0].InboundCount)

	// 8. 删除层 → 挂层入站回退原生（layer_id 置空）
	req = httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/admin/servers/%d/layers/%d", hkSrv.ID, layerID), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())
	var inbAfter models.Inbound
	require.NoError(t, db.First(&inbAfter, apInbID).Error)
	require.Nil(t, inbAfter.LayerID)

	// 9. 删除后订阅走直连端点（node → 服务器 Host + 入站端口）
	dto3 := subscribe.ResolveAPSubscription(&ap, srvMap, inbMap, map[uint64]models.L4PortRule{}, map[uint64]models.AccessLayer{}, "11111111-2222-3333-4444-555555555555")
	require.NotNil(t, dto3)
	require.Equal(t, "hk.node.com", dto3.ServerHost)
	require.Equal(t, 10086, dto3.ServerPort)
}
