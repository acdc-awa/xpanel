package api

// 出站级 PROXY Protocol 的保存语义：
//   - 开启后落地入站的 tcpSettings.acceptProxyProtocol 必须跟着打开，关闭/删除/停用要清掉；
//   - 同一个落地被多条发送侧共用时，开关必须一致（接收侧是单条监听器、REQUIRE 对每条连接生效）；
//   - pp- 是派生出站的保留前缀，用户出站不得占用。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func ppTestServer(t *testing.T, db *gorm.DB) (*Deps, *gin.Engine, models.Inbound) {
	t.Helper()
	db.Create(&models.Server{ID: 1, Name: "n1", Host: "10.0.0.1", NodeID: "n1", Secret: "x"})
	db.Create(&models.Server{ID: 2, Name: "n2", Host: "10.0.0.2", NodeID: "n2", Secret: "x"})
	landing := models.Inbound{
		ServerID: 2, Tag: "landing", Protocol: models.ProtocolVless, Port: 443,
		Type: models.InboundTypeRelay, InternalUUID: "33333333-3333-3333-3333-333333333333",
		StreamSettings: `{"network":"tcp","security":"none"}`, Enabled: true,
	}
	db.Create(&landing)
	d := &Deps{DB: db}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/servers/:id/outbounds", d.AdminCreateServerOutbound)
	r.PUT("/api/v1/admin/servers/:id/outbounds/:outbound_id", d.AdminUpdateServerOutbound)
	r.DELETE("/api/v1/admin/servers/:id/outbounds/:outbound_id", d.AdminDeleteServerOutbound)
	return d, r, landing
}

func landingAcceptsPP(t *testing.T, d *Deps, id uint64) bool {
	t.Helper()
	var inb models.Inbound
	if err := d.DB.First(&inb, id).Error; err != nil {
		t.Fatalf("读取落地入站: %v", err)
	}
	var ss map[string]any
	if err := json.Unmarshal([]byte(inb.StreamSettings), &ss); err != nil {
		t.Fatalf("落地 streamSettings 非法: %v (%s)", err, inb.StreamSettings)
	}
	tcpS, _ := ss["tcpSettings"].(map[string]any)
	v, _ := tcpS["acceptProxyProtocol"].(bool)
	return v
}

func doJSON(r *gin.Engine, method, path string, body map[string]any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func obPath(id uint64) string {
	return "/api/v1/admin/servers/1/outbounds/" + strconv.FormatUint(id, 10)
}

func TestOutboundProxyProtocol_SyncsLandingOnCreateUpdateDelete(t *testing.T) {
	d, r, landing := ppTestServer(t, apiTestDB(t))

	// 1. 创建 → 落地开启接收
	w := doJSON(r, "POST", "/api/v1/admin/servers/1/outbounds", map[string]any{
		"tag": "to-landing", "protocol": "vless", "inbound_ref": landing.ID,
		"send_proxy_protocol": true, "enabled": true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("创建出站失败: %d %s", w.Code, w.Body.String())
	}
	if !landingAcceptsPP(t, d, landing.ID) {
		t.Fatal("开启 PP 后落地入站应自动打开 tcpSettings.acceptProxyProtocol")
	}

	var ob models.ServerOutbound
	if err := d.DB.Where("tag = ?", "to-landing").First(&ob).Error; err != nil {
		t.Fatalf("读取出站: %v", err)
	}
	if !ob.SendProxyProtocol {
		t.Fatal("send_proxy_protocol 未落库")
	}

	// 2. 关闭开关 → 落地接收侧清除
	w = doJSON(r, "PUT", obPath(ob.ID), map[string]any{"send_proxy_protocol": false})
	if w.Code != http.StatusOK {
		t.Fatalf("关闭开关失败: %d %s", w.Code, w.Body.String())
	}
	if landingAcceptsPP(t, d, landing.ID) {
		t.Fatal("关闭 PP 后落地入站应清除 tcpSettings.acceptProxyProtocol")
	}

	// 3. 重新开启后删除 → 落地接收侧再次清除
	if w = doJSON(r, "PUT", obPath(ob.ID), map[string]any{"send_proxy_protocol": true}); w.Code != http.StatusOK {
		t.Fatalf("重新开启失败: %d %s", w.Code, w.Body.String())
	}
	if !landingAcceptsPP(t, d, landing.ID) {
		t.Fatal("重新开启后落地应恢复接收")
	}
	if w = doJSON(r, "DELETE", obPath(ob.ID), nil); w.Code != http.StatusOK {
		t.Fatalf("删除出站失败: %d %s", w.Code, w.Body.String())
	}
	if landingAcceptsPP(t, d, landing.ID) {
		t.Fatal("删除唯一的 PP 出站后落地应清除接收开关")
	}
}

func TestOutboundProxyProtocol_DisableSenderClearsLanding(t *testing.T) {
	d, r, landing := ppTestServer(t, apiTestDB(t))

	w := doJSON(r, "POST", "/api/v1/admin/servers/1/outbounds", map[string]any{
		"tag": "to-landing", "protocol": "vless", "inbound_ref": landing.ID,
		"send_proxy_protocol": true, "enabled": true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("创建出站失败: %d %s", w.Code, w.Body.String())
	}
	var ob models.ServerOutbound
	d.DB.Where("tag = ?", "to-landing").First(&ob)

	// 停用发送侧等于它不再发 PP：落地必须回到普通 TCP，否则这条链路会按 PP 解析而失败。
	if w = doJSON(r, "PUT", obPath(ob.ID), map[string]any{"enabled": false}); w.Code != http.StatusOK {
		t.Fatalf("停用出站失败: %d %s", w.Code, w.Body.String())
	}
	if landingAcceptsPP(t, d, landing.ID) {
		t.Fatal("停用 PP 出站后落地应清除接收开关")
	}
}

func TestOutboundProxyProtocol_RejectsWhenLandingAlreadyRequiresPP(t *testing.T) {
	d, r, landing := ppTestServer(t, apiTestDB(t))

	// 直通管道（proxy_protocol 缺省=开）已经要求落地按 PP 接收
	pipe := models.Inbound{
		ServerID: 1, Tag: "pipe", Protocol: models.ProtocolTunnel, Port: 10080,
		Type: models.InboundTypeTunnel, TargetInboundID: &landing.ID,
		StreamSettings: `{"network":"tcp,udp"}`, Enabled: true,
	}
	d.DB.Create(&pipe)
	d.syncTargetProxyProtocolByID(landing.ID)
	if !landingAcceptsPP(t, d, landing.ID) {
		t.Fatal("前置条件失败：直通管道应先让落地进入 PP 接收模式")
	}

	// 同一落地的引用出站若不开 PP，落地会把它当 PP 解析 → 必须拒绝
	w := doJSON(r, "POST", "/api/v1/admin/servers/1/outbounds", map[string]any{
		"tag": "to-landing", "protocol": "vless", "inbound_ref": landing.ID,
		"send_proxy_protocol": false, "enabled": true,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("口径不一致应被拒绝，实际 %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("必须同时开启")) {
		t.Fatalf("拒绝原因应指向上游已要求 PP，实际: %s", w.Body.String())
	}
}

func TestOutboundProxyProtocol_RejectsInvalidSwitchTargets(t *testing.T) {
	d, r, landing := ppTestServer(t, apiTestDB(t))

	hys := models.Inbound{
		ServerID: 2, Tag: "hy-landing", Protocol: models.ProtocolHysteria, Port: 8443,
		Type: models.InboundTypeRelay, InternalUUID: "44444444-4444-4444-4444-444444444444",
		StreamSettings: `{"network":"hysteria"}`, Enabled: true,
	}
	d.DB.Create(&hys)

	// 无引用
	if w := doJSON(r, "POST", "/api/v1/admin/servers/1/outbounds", map[string]any{
		"tag": "freedom-pp", "protocol": "freedom",
		"settings_json":       models.DefaultFreedomDirectSettingsJSON,
		"send_proxy_protocol": true, "enabled": true,
	}); w.Code != http.StatusBadRequest {
		t.Fatalf("无引用出站开启 PP 应被拒绝，实际 %d: %s", w.Code, w.Body.String())
	}

	// Hysteria 2 落地
	if w := doJSON(r, "POST", "/api/v1/admin/servers/1/outbounds", map[string]any{
		"tag": "to-hy", "protocol": "vless", "inbound_ref": hys.ID,
		"send_proxy_protocol": true, "enabled": true,
	}); w.Code != http.StatusBadRequest {
		t.Fatalf("Hysteria 2 落地开启 PP 应被拒绝，实际 %d: %s", w.Code, w.Body.String())
	}

	// 合法的 VLESS 落地仍然可以创建（对照组）
	if w := doJSON(r, "POST", "/api/v1/admin/servers/1/outbounds", map[string]any{
		"tag": "to-landing", "protocol": "vless", "inbound_ref": landing.ID,
		"send_proxy_protocol": true, "enabled": true,
	}); w.Code != http.StatusOK {
		t.Fatalf("合法出站应创建成功，实际 %d: %s", w.Code, w.Body.String())
	}
}

func TestOutboundProxyProtocol_ReservedTagPrefix(t *testing.T) {
	d, r, landing := ppTestServer(t, apiTestDB(t))

	if w := doJSON(r, "POST", "/api/v1/admin/servers/1/outbounds", map[string]any{
		"tag": "pp-fake", "protocol": "vless", "inbound_ref": landing.ID,
		"send_proxy_protocol": false, "enabled": true,
	}); w.Code != http.StatusBadRequest {
		t.Fatalf("pp- 前缀应被拒绝，实际 %d: %s", w.Code, w.Body.String())
	}

	// 改名成保留前缀同样拒绝
	if w := doJSON(r, "POST", "/api/v1/admin/servers/1/outbounds", map[string]any{
		"tag": "normal", "protocol": "vless", "inbound_ref": landing.ID,
		"send_proxy_protocol": false, "enabled": true,
	}); w.Code != http.StatusOK {
		t.Fatalf("普通出站应创建成功，实际 %d: %s", w.Code, w.Body.String())
	}
	var ob models.ServerOutbound
	d.DB.Where("tag = ?", "normal").First(&ob)
	if w := doJSON(r, "PUT", obPath(ob.ID), map[string]any{"tag": "pp-normal"}); w.Code != http.StatusBadRequest {
		t.Fatalf("改名为 pp- 前缀应被拒绝，实际 %d: %s", w.Code, w.Body.String())
	}
}
