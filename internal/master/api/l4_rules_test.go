package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/master/services"
	"github.com/acdc/xray-panel/internal/master/xray"
	"github.com/acdc/xray-panel/internal/models"
)

func setupTestDBForL4(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.AutoMigrate(db))
	return db
}

func TestL4Rules_CRUD_And_Subscribe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDBForL4(t)

	deps := &Deps{DB: db}
	r := gin.New()
	r.GET("/api/v1/admin/servers/:id/l4-rules", deps.AdminGetL4Rules)
	r.POST("/api/v1/admin/servers/:id/l4-rules", deps.AdminCreateL4Rule)
	r.PUT("/api/v1/admin/servers/:id/l4-rules/:rule_id", deps.AdminUpdateL4Rule)
	r.DELETE("/api/v1/admin/servers/:id/l4-rules/:rule_id", deps.AdminDeleteL4Rule)
	r.GET("/api/v1/admin/topology", deps.AdminTopology)
	r.GET("/api/v1/client/subscribe", deps.Subscribe)

	// 1. 创建权限组
	vipGroup := models.PermissionGroup{Name: "VIP组"}
	stdGroup := models.PermissionGroup{Name: "标准组"}
	require.NoError(t, db.Create(&vipGroup).Error)
	require.NoError(t, db.Create(&stdGroup).Error)

	// 2. 创建落地服务器 (Xray) 与 L4 转发服务器 (L4Relay)
	hkSrv := models.Server{
		ServerType: models.ServerTypeXray, Name: "香港01", Host: "hk.node.com",
		NodeID: "node-hk", Secret: "sec-hk", Status: 1, DefaultOutboundTag: "direct",
	}
	gzSrv := models.Server{
		ServerType: models.ServerTypeL4Relay, Name: "广州中转", Host: "gz.relay.com",
		NodeID: "node-gz", Secret: "sec-gz", Status: 1, DefaultOutboundTag: "direct",
	}
	require.NoError(t, db.Create(&hkSrv).Error)
	require.NoError(t, db.Create(&gzSrv).Error)

	// 3. 创建入站 (香港 Reality 入站)
	inb := models.Inbound{
		ServerID: hkSrv.ID, Tag: "hk-reality", Protocol: "vless", Port: 443,
		Enabled: true, Type: models.InboundTypeUser,
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"serverName":"www.apple.com","publicKey":"pQDGvDURYEv8nxAVW9xsbBsQjOXzX0rCh5OWDW5q8kg","shortId":"e69c1c"}}`,
	}
	require.NoError(t, db.Create(&inb).Error)
	require.NoError(t, services.SyncInboundPermissionGroups(db, inb.ID, []uint64{stdGroup.ID, vipGroup.ID}))

	// 4. POST 创建 L4 规则 (仅 VIP 可见)
	createBody := map[string]any{
		"listen_port":          30001,
		"target_server_id":     hkSrv.ID,
		"target_inbound_id":    inb.ID,
		"remark":               "广州移动 10G",
		"enabled":              true,
		"permission_group_ids": []uint64{vipGroup.ID},
	}
	bodyBytes, _ := json.Marshal(createBody)
	req := httptest.NewRequest("POST", "/api/v1/admin/servers/2/l4-rules", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	var createResp struct {
		Code int `json:"code"`
		Data struct {
			ID         uint64 `json:"id"`
			ListenPort int    `json:"listen_port"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	assert.Equal(t, 0, createResp.Code)
	ruleID := createResp.Data.ID
	assert.Equal(t, 30001, createResp.Data.ListenPort)

	// 5. GET L4 规则列表
	req = httptest.NewRequest("GET", "/api/v1/admin/servers/2/l4-rules", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	// 6. GET /admin/topology
	req = httptest.NewRequest("GET", "/api/v1/admin/topology", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	var topoResp struct {
		Code int `json:"code"`
		Data struct {
			L4Rules []struct {
				ID         uint64   `json:"id"`
				ListenPort int      `json:"listen_port"`
				PermGroups []uint64 `json:"permission_group_ids"`
			} `json:"l4_rules"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &topoResp))
	assert.Equal(t, 0, topoResp.Code)
	require.Len(t, topoResp.Data.L4Rules, 1)
	assert.Equal(t, ruleID, topoResp.Data.L4Rules[0].ID)
	assert.Equal(t, []uint64{vipGroup.ID}, topoResp.Data.L4Rules[0].PermGroups)

	// 7. 验证普通用户 (标准组) 订阅 -> 只有直连节点，不派生 VIP L4 中转节点
	stdUser := models.User{
		Username:          "stduser",
		Email:             "stduser@test.com",
		Status:            models.StatusActive,
		UUID:              "11111111-1111-1111-1111-111111111111",
		SubscribeToken:    "sub-std",
		PermissionGroupID: stdGroup.ID,
	}
	require.NoError(t, db.Create(&stdUser).Error)

	req = httptest.NewRequest("GET", "/api/v1/client/subscribe?token=sub-std", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	decStd, _ := base64.StdEncoding.DecodeString(w.Body.String())
	stdSubBody := string(decStd)
	assert.Contains(t, stdSubBody, "hk.node.com:443")
	assert.NotContains(t, stdSubBody, "gz.relay.com:30001") // 严格白名单过滤

	// 8. 验证 VIP 用户订阅 -> 同时拥有直连节点和 L4 中转派生节点
	vipUser := models.User{
		Username:          "vipuser",
		Email:             "vipuser@test.com",
		Status:            models.StatusActive,
		UUID:              "22222222-2222-2222-2222-222222222222",
		SubscribeToken:    "sub-vip",
		PermissionGroupID: vipGroup.ID,
	}
	require.NoError(t, db.Create(&vipUser).Error)

	req = httptest.NewRequest("GET", "/api/v1/client/subscribe?token=sub-vip", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	decVIP, _ := base64.StdEncoding.DecodeString(w.Body.String())
	vipSubBody := string(decVIP)
	assert.Contains(t, vipSubBody, "hk.node.com:443")
	assert.Contains(t, vipSubBody, "gz.relay.com:30001") // VIP 成功获取 L4 派生节点
	unescaped, _ := url.QueryUnescape(vipSubBody)
	assert.True(t, strings.Contains(unescaped, "广州中转") && strings.Contains(unescaped, "广州移动 10G"))
}

func TestXrayGenerate_SafeSkip_UnlinkedOutbound(t *testing.T) {
	// 测试未连线的草稿 VLESS 出站能够被生成器安全跳过，不报语法错误
	inbounds := []models.Inbound{
		{
			Tag: "vless-in", Protocol: "vless", Port: 443, Type: "user", Enabled: true,
			StreamSettings: `{"network":"tcp","security":"none"}`,
		},
	}
	outbounds := []models.ServerOutbound{
		{
			Tag: "direct", Protocol: "freedom", Enabled: true, Priority: 0,
		},
		{
			// 草稿未连线出站（InboundRef == nil 且 SettingsJSON 为空）
			Tag: "via-draft", Protocol: "vless", InboundRef: nil, SettingsJSON: "", Enabled: true, Priority: 1,
		},
	}

	cfgBytes, err := xray.Generate(inbounds, outbounds, nil, nil, nil, "direct", "AsIs")
	require.NoError(t, err)

	var cfgMap map[string]any
	require.NoError(t, json.Unmarshal(cfgBytes, &cfgMap))

	outs, ok := cfgMap["outbounds"].([]any)
	require.True(t, ok)

	// 确认 via-draft 没有被注入（安全跳过）
	hasDraft := false
	for _, o := range outs {
		om, _ := o.(map[string]any)
		if om["tag"] == "via-draft" {
			hasDraft = true
		}
	}
	assert.False(t, hasDraft, "草稿未连线出站应被安全跳过，不注入 Xray 配置")
}
