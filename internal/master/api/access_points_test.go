package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

func setupAccessPointTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.AutoMigrate(db))
	return db
}

func TestUserAccessPoints_CRUD_And_Subscribe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAccessPointTestDB(t)
	deps := &Deps{DB: db}
	r := gin.New()
	r.GET("/api/v1/admin/access-points", deps.AdminGetAccessPoints)
	r.POST("/api/v1/admin/access-points", deps.AdminCreateAccessPoint)
	r.PUT("/api/v1/admin/access-points/:id", deps.AdminUpdateAccessPoint)
	r.PUT("/api/v1/admin/access-points/:id/target", deps.AdminSetAccessPointTarget)
	r.DELETE("/api/v1/admin/access-points/:id", deps.AdminDeleteAccessPoint)
	r.GET("/sub", deps.Subscribe)

	// 1. 准备权限组
	vipGroup := models.PermissionGroup{Name: "VIP 接入组"}
	require.NoError(t, deps.DB.Create(&vipGroup).Error)

	otherGroup := models.PermissionGroup{Name: "普通组"}
	require.NoError(t, deps.DB.Create(&otherGroup).Error)

	// 2. 准备落地 Xray 节点与入站
	nodeSrv := models.Server{
		ServerType: models.ServerTypeXray,
		Name:       "日本落地",
		Host:       "jp.node.com",
		NodeID:     "node-jp",
		Secret:     util.HashSecret("secret-jp"),
		Status:     1,
	}
	require.NoError(t, deps.DB.Create(&nodeSrv).Error)

	inb := models.Inbound{
		ServerID:       nodeSrv.ID,
		Tag:            "vless-in",
		Protocol:       "vless",
		Port:           443,
		Type:           models.InboundTypeUser,
		Enabled:        true,
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"serverNames":["www.apple.com"],"privateKey":"privkey","shortIds":["e69c1c"],"publicKey":"pQDGvDURYEv8nxAVW9xsbBsQjOXzX0rCh5OWDW5q8kg"}}`,
	}
	require.NoError(t, deps.DB.Create(&inb).Error)

	// 3. 准备落地 Xray 节点与入站
	apPayload := map[string]any{
		"name":                 "香港直连接入",
		"target_type":          "inbound",
		"target_inbound_id":    inb.ID,
		"permission_group_ids": []uint64{vipGroup.ID},
		"remark":               "仅 VIP 可见",
	}
	body, _ := json.Marshal(apPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/access-points", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var createResp struct {
		Code int `json:"code"`
		Data struct {
			AccessPoint AccessPointView `json:"access_point"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &createResp))
	assert.Equal(t, 0, createResp.Code)
	apID := createResp.Data.AccessPoint.ID
	assert.True(t, apID > 0)
	assert.Equal(t, "香港直连接入", createResp.Data.AccessPoint.Name)
	assert.Equal(t, "jp.node.com", createResp.Data.AccessPoint.ResolvedHost)
	assert.Equal(t, 443, createResp.Data.AccessPoint.ResolvedPort)

	// 5. 测试 POST /api/v1/admin/access-points 创建带端点覆写的接入点（L4 退役后的中转表达：
	// 直连目标入站 + CustomHost/CustomPort 覆写为转发端点，视图解析为覆写值）
	apL4Payload := map[string]any{
		"name":                 "广州 BGP 接入点",
		"target_type":          "inbound",
		"target_inbound_id":    inb.ID,
		"custom_host":          "gz.relay.com",
		"custom_port":          30001,
		"permission_group_ids": []uint64{vipGroup.ID},
		"remark":               "BGP 加速",
	}
	body2, _ := json.Marshal(apL4Payload)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/access-points", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	var createL4Resp struct {
		Code int `json:"code"`
		Data struct {
			AccessPoint AccessPointView `json:"access_point"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &createL4Resp))
	assert.Equal(t, 0, createL4Resp.Code)
	assert.Equal(t, "gz.relay.com", createL4Resp.Data.AccessPoint.ResolvedHost)
	assert.Equal(t, 30001, createL4Resp.Data.AccessPoint.ResolvedPort)

	// 6. 测试 GET /api/v1/admin/access-points
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/admin/access-points", nil)
	wGet := httptest.NewRecorder()
	r.ServeHTTP(wGet, reqGet)
	assert.Equal(t, http.StatusOK, wGet.Code)

	// 7. 测试订阅过滤：VIP 用户应订阅到两个接入点
	exp := time.Now().Add(24 * time.Hour)
	vipUser := models.User{
		Username:          "vip_user",
		Email:             "vip@test.com",
		UUID:              "11111111-1111-1111-1111-111111111111",
		Role:              models.RoleUser,
		Status:            models.StatusActive,
		PermissionGroupID: vipGroup.ID,
		SubscribeToken:    "sub-token-vip",
		ExpireAt:          &exp,
	}
	require.NoError(t, deps.DB.Create(&vipUser).Error)

	reqSubVIP := httptest.NewRequest(http.MethodGet, "/sub?token=sub-token-vip&format=base64", nil)
	wSubVIP := httptest.NewRecorder()
	r.ServeHTTP(wSubVIP, reqSubVIP)
	assert.Equal(t, http.StatusOK, wSubVIP.Code)
	bVIP, _ := base64.StdEncoding.DecodeString(wSubVIP.Body.String())
	subDecodedVIP := string(bVIP)
	assert.Contains(t, subDecodedVIP, "jp.node.com")
	assert.Contains(t, subDecodedVIP, "gz.relay.com")

	// 8. 普通用户（在 otherGroup）不应订阅到 VIP 专享的接入点
	normalUser := models.User{
		Username:          "normal_user",
		Email:             "normal@test.com",
		UUID:              "22222222-2222-2222-2222-222222222222",
		Role:              models.RoleUser,
		Status:            models.StatusActive,
		PermissionGroupID: otherGroup.ID,
		SubscribeToken:    "sub-token-normal",
		ExpireAt:          &exp,
	}
	require.NoError(t, deps.DB.Create(&normalUser).Error)

	reqSubNormal := httptest.NewRequest(http.MethodGet, "/sub?token=sub-token-normal&format=base64", nil)
	wSubNormal := httptest.NewRecorder()
	r.ServeHTTP(wSubNormal, reqSubNormal)
	assert.Equal(t, http.StatusNotFound, wSubNormal.Code)

	// 9. 测试 DELETE /api/v1/admin/access-points/:id
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/access-points/"+strconv.FormatUint(apID, 10), nil)
	wDel := httptest.NewRecorder()
	r.ServeHTTP(wDel, reqDel)
	assert.Equal(t, http.StatusOK, wDel.Code)
}

func TestUserAccessPoints_TunnelTarget_And_Subscribe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAccessPointTestDB(t)
	deps := &Deps{DB: db}
	r := gin.New()
	r.POST("/api/v1/admin/access-points", deps.AdminCreateAccessPoint)
	r.GET("/sub", deps.Subscribe)

	// 1. 准备权限组与用户
	grp := models.PermissionGroup{Name: "直通管道组"}
	require.NoError(t, deps.DB.Create(&grp).Error)

	exp := time.Now().Add(24 * time.Hour)
	user := models.User{
		Username:          "tunnel_user",
		Email:             "tunnel@test.com",
		UUID:              "33333333-3333-3333-3333-333333333333",
		Role:              models.RoleUser,
		Status:            models.StatusActive,
		PermissionGroupID: grp.ID,
		SubscribeToken:    "sub-token-tunnel",
		ExpireAt:          &exp,
	}
	require.NoError(t, deps.DB.Create(&user).Error)

	// 2. 准备入口服务器与落地服务器
	edgeSrv := models.Server{ServerType: models.ServerTypeXray, Name: "入口中转", Host: "edge.node.com", NodeID: "n-edge", Secret: util.HashSecret("sec1"), Status: 1}
	require.NoError(t, deps.DB.Create(&edgeSrv).Error)

	exitSrv := models.Server{ServerType: models.ServerTypeXray, Name: "香港落地", Host: "hk.node.com", NodeID: "n-exit", Secret: util.HashSecret("sec2"), Status: 1}
	require.NoError(t, deps.DB.Create(&exitSrv).Error)

	// 3. 落地入站 (VLESS)
	landingInb := models.Inbound{
		ServerID:       exitSrv.ID,
		Tag:            "vless-hk",
		Protocol:       "vless",
		Port:           443,
		Type:           models.InboundTypeUser,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Enabled:        true,
	}
	require.NoError(t, deps.DB.Create(&landingInb).Error)

	// 4. 草稿直通管道（未连线 target_inbound_id 为 nil）
	draftTunnel := models.Inbound{
		ServerID: edgeSrv.ID,
		Tag:      "tunnel-draft",
		Protocol: "dokodemo-door",
		Port:     10001,
		Type:     models.InboundTypeTunnel,
		Enabled:  true,
	}
	require.NoError(t, deps.DB.Create(&draftTunnel).Error)

	// 尝试将 AP 绑定到未连线的草稿管道 -> 应返回 400
	bodyDraft := map[string]any{
		"name":                 "草稿管道节点",
		"enabled":              true,
		"target_type":          "inbound",
		"target_inbound_id":    draftTunnel.ID,
		"permission_group_ids": []uint64{grp.ID},
	}
	bDraft, _ := json.Marshal(bodyDraft)
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/access-points", bytes.NewReader(bDraft))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusBadRequest, w1.Code, "未连线的草稿直通管道应拒绝绑定 AP: %s", w1.Body.String())

	// 5. 已连线的直通管道
	connectedTunnel := models.Inbound{
		ServerID:        edgeSrv.ID,
		Tag:             "tunnel-edge",
		Protocol:        "dokodemo-door",
		Port:            10002,
		Type:            models.InboundTypeTunnel,
		TargetInboundID: &landingInb.ID,
		Enabled:         true,
	}
	require.NoError(t, deps.DB.Create(&connectedTunnel).Error)

	// 将 AP 绑定到已连线的直通管道 -> 应返回 200 (P1-1 修复验证)
	bodyConn := map[string]any{
		"name":                 "香港直通节点",
		"enabled":              true,
		"target_type":          "inbound",
		"target_inbound_id":    connectedTunnel.ID,
		"permission_group_ids": []uint64{grp.ID},
	}
	bConn, _ := json.Marshal(bodyConn)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/access-points", bytes.NewReader(bConn))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code, "已连线的直通管道应允许绑定 AP: %s", w2.Body.String())

	// 6. 订阅解析直通管道节点 -> 应返回 200 且包含入口机器与落地机器信息 (P1-4 修复验证)
	reqSub := httptest.NewRequest(http.MethodGet, "/sub?token=sub-token-tunnel&format=base64", nil)
	wSub := httptest.NewRecorder()
	r.ServeHTTP(wSub, reqSub)
	assert.Equal(t, http.StatusOK, wSub.Code, "直通管道 AP 应在订阅中正常解析: %s", wSub.Body.String())

	bSub, err := base64.StdEncoding.DecodeString(wSub.Body.String())
	require.NoError(t, err)
	subStr := string(bSub)
	assert.Contains(t, subStr, "edge.node.com", "订阅应包含入口机器地址")
	assert.Contains(t, subStr, "10002", "订阅应包含入口机器端口")
	assert.Contains(t, subStr, user.UUID, "订阅应包含用户认证凭据")
}
