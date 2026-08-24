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
