package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
)

func TestEnsureInternalUUID(t *testing.T) {
	db := apiTestDB(t)
	d := &Deps{DB: db}

	// 1. 无 UUID 时应生成有效 UUID
	inb := models.Inbound{ServerID: 1, Tag: "in-1", Type: models.InboundTypeUser}
	if err := d.ensureInternalUUID(&inb); err != nil {
		t.Fatalf("ensureInternalUUID: %v", err)
	}
	if inb.InternalUUID == "" {
		t.Error("应自动分配 InternalUUID")
	}

	// 2. 已有 UUID 时不应被覆盖
	existingUUID := inb.InternalUUID
	if err := d.ensureInternalUUID(&inb); err != nil {
		t.Fatalf("ensureInternalUUID idempotent: %v", err)
	}
	if inb.InternalUUID != existingUUID {
		t.Errorf("已有 UUID 不应被覆盖: old=%s, new=%s", existingUUID, inb.InternalUUID)
	}
}

func TestCreateInbound_UnifiedPhysicalInbound(t *testing.T) {
	db := apiTestDB(t)
	db.Create(&models.Server{ID: 10, Name: "node-10", Host: "1.2.3.4", NodeID: "n10", Secret: "sec"})
	d := &Deps{DB: db}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/inbounds", d.AdminCreateInbound)

	// 即使入参传入 type=relay，也会归一为通用代理入站 user
	body := map[string]any{
		"server_id":       10,
		"tag":             "relay-direct-create",
		"protocol":        "vless",
		"port":            44301,
		"listen":          "0.0.0.0",
		"type":            "relay",
		"settings_json":   `{"decryption":"none"}`,
		"stream_settings": `{"network":"tcp","security":"none"}`,
	}
	data, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/admin/inbounds", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("创建入站失败: code=%d body=%s", w.Code, w.Body.String())
	}

	var inb models.Inbound
	if err := db.Where("server_id = ? AND tag = ?", 10, "relay-direct-create").First(&inb).Error; err != nil {
		t.Fatalf("未找到新建的入站: %v", err)
	}
	if inb.Type != models.InboundTypeUser {
		t.Errorf("期望归一为 type=user, 实际 %s", inb.Type)
	}
}

func TestUpdateInbound_NormalizedType(t *testing.T) {
	db := apiTestDB(t)
	db.Create(&models.Server{ID: 11, Name: "node-11", Host: "1.2.3.5", NodeID: "n11", Secret: "sec"})
	inb := models.Inbound{
		ServerID: 11, Tag: "user-to-relay", Protocol: "vless", Port: 44302,
		Type: models.InboundTypeUser, Enabled: true,
	}
	db.Create(&inb)
	d := &Deps{DB: db}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/api/v1/admin/inbounds/:id", d.AdminUpdateInbound)

	body := map[string]any{
		"type": "relay",
	}
	data, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/v1/admin/inbounds/1", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("更新入站失败: code=%d body=%s", w.Code, w.Body.String())
	}

	var updated models.Inbound
	db.First(&updated, inb.ID)
	if updated.Type != models.InboundTypeUser {
		t.Errorf("更新后 type 应归一为 user, 实际 %s", updated.Type)
	}
}

func TestAdminDeleteInbound_ProtectedByTunnelAndChannel(t *testing.T) {
	db := apiTestDB(t)
	db.Create(&models.Server{ID: 1, Name: "node-1", Host: "1.2.3.4", NodeID: "n1", Secret: "sec"})

	// 1. 落地入站与指向它的 Tunnel 入站
	landing := models.Inbound{
		ServerID: 1, Tag: "landing-vless", Protocol: "vless", Port: 443,
		Type: models.InboundTypeUser, Enabled: true,
	}
	db.Create(&landing)

	tunnel := models.Inbound{
		ServerID: 1, Tag: "tunnel-in", Protocol: "tunnel", Port: 10001,
		Type: models.InboundTypeTunnel, TargetInboundID: &landing.ID, Enabled: true,
	}
	db.Create(&tunnel)

	// 2. Channel 入站（四层直通通道）
	chanInb := models.Inbound{
		ServerID: 1, Tag: "chan-tunnel-1080", Protocol: "tunnel", Port: 1080,
		Type: models.InboundTypeChannel, Enabled: true,
	}
	db.Create(&chanInb)

	d := &Deps{DB: db}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.DELETE("/api/v1/admin/inbounds/:id", d.AdminDeleteInbound)
	r.PUT("/api/v1/admin/inbounds/:id", d.AdminUpdateInbound)

	// 尝试删除被 Tunnel 引用的 landing 入站 -> 应该被 400 拦截
	req1 := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/admin/inbounds/%d", landing.ID), nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("删除被 Tunnel 引用的入站应返回 400, 实际: %d, body: %s", w1.Code, w1.Body.String())
	}

	// 允许通过常规接口更新 Forward/Channel 入站
	upBody, _ := json.Marshal(map[string]any{"port": 1081, "remark": "更新备注"})
	req3 := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/admin/inbounds/%d", chanInb.ID), bytes.NewReader(upBody))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("更新入站应成功, 实际: %d, body: %s", w3.Code, w3.Body.String())
	}

	// 允许删除未被引用的 Forward/Channel 入站
	req2 := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/admin/inbounds/%d", chanInb.ID), nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("删除入站应成功, 实际: %d, body: %s", w2.Code, w2.Body.String())
	}
}

func TestAdminToggleInbound_ChannelGuardAndTunnelProtection(t *testing.T) {
	db := apiTestDB(t)
	db.Create(&models.Server{ID: 2, Name: "node-2", Host: "2.2.2.2", NodeID: "n2", Secret: "sec"})

	// 1. 落地入站与指向它的 Tunnel 入站
	landing := models.Inbound{
		ServerID: 2, Tag: "landing-vless", Protocol: "vless", Port: 443,
		Type: models.InboundTypeUser, Enabled: true,
	}
	db.Create(&landing)

	tunnel := models.Inbound{
		ServerID: 2, Tag: "tunnel-in", Protocol: "tunnel", Port: 10001,
		Type: models.InboundTypeTunnel, TargetInboundID: &landing.ID, Enabled: true,
	}
	db.Create(&tunnel)

	// 2. Channel 入站（四层直通通道）
	chanInb := models.Inbound{
		ServerID: 2, Tag: "chan-tunnel-1080", Protocol: "tunnel", Port: 1080,
		Type: models.InboundTypeForward, Enabled: true,
	}
	db.Create(&chanInb)

	d := &Deps{DB: db}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/inbounds/:id/toggle", d.AdminToggleInbound)
	r.PUT("/api/v1/admin/inbounds/:id", d.AdminUpdateInbound)

	// A. 切换未被引用的入站状态 -> 应该成功
	req1 := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/admin/inbounds/%d/toggle", chanInb.ID), nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("Toggle 入站应成功, 实际: %d, body: %s", w1.Code, w1.Body.String())
	}

	// B. 尝试 Toggle 停用被启用的 Tunnel 引用的落地入站 -> 应该被 400 拦截 (P2-7)
	req2 := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/admin/inbounds/%d/toggle", landing.ID), nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("Toggle 停用被 Tunnel 引用的落地入站应返回 400, 实际: %d, body: %s", w2.Code, w2.Body.String())
	}

	// C. 尝试通过 Update 接口将落地入站 enabled 置为 false -> 应该被 400 拦截 (P2-7)
	upDis, _ := json.Marshal(map[string]any{"enabled": false})
	req3 := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/admin/inbounds/%d", landing.ID), bytes.NewReader(upDis))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("Update 停用被 Tunnel 引用的落地入站应返回 400, 实际: %d, body: %s", w3.Code, w3.Body.String())
	}

	// D. 停用 Tunnel 后，落地入站应允许 Toggle 停用
	// 必须走 API 停用：C2 的派生出站随直通入站 enabled 联动，直改库不会触发对账。
	reqTgl := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/admin/inbounds/%d/toggle", tunnel.ID), nil)
	wTgl := httptest.NewRecorder()
	r.ServeHTTP(wTgl, reqTgl)
	if wTgl.Code != http.StatusOK {
		t.Fatalf("停用 Tunnel 应成功, 实际: %d, body: %s", wTgl.Code, wTgl.Body.String())
	}

	req4 := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/admin/inbounds/%d/toggle", landing.ID), nil)
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK {
		t.Fatalf("Tunnel 停用后落地入站应可停用, 实际: %d, body: %s", w4.Code, w4.Body.String())
	}
}

func TestEnsureTargetAcceptProxyProtocol(t *testing.T) {
	db := apiTestDB(t)
	db.Create(&models.Server{ID: 3, Name: "node-3", Host: "3.3.3.3", NodeID: "n3", Secret: "sec"})

	// 1. 落地入站初始未开启 acceptProxyProtocol
	landing := models.Inbound{
		ServerID:       3,
		Tag:            "landing-target",
		Protocol:       "vless",
		Port:           8443,
		Type:           models.InboundTypeUser,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Enabled:        true,
	}
	db.Create(&landing)

	d := &Deps{DB: db}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/inbounds", d.AdminCreateInbound)
	r.PUT("/api/v1/admin/inbounds/:id", d.AdminUpdateInbound)

	// 2. 创建管道指定 TargetInboundID -> 应自动将落地入站 acceptProxyProtocol 置为 true (P2-8)
	bodyCreate := map[string]any{
		"server_id":         3,
		"tag":               "tunnel-auto-pp",
		"protocol":          "tunnel",
		"port":              18443,
		"type":              "tunnel",
		"target_inbound_id": landing.ID,
	}
	rawCreate, _ := json.Marshal(bodyCreate)
	req1 := httptest.NewRequest("POST", "/api/v1/admin/inbounds", bytes.NewReader(rawCreate))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("Create tunnel failed: %d, body: %s", w1.Code, w1.Body.String())
	}

	var updatedLanding models.Inbound
	db.First(&updatedLanding, landing.ID)
	var ss map[string]any
	json.Unmarshal([]byte(updatedLanding.StreamSettings), &ss)
	tcpS, _ := ss["tcpSettings"].(map[string]any)
	if app, _ := tcpS["acceptProxyProtocol"].(bool); !app {
		t.Fatalf("Landing inbound tcpSettings.acceptProxyProtocol should be set to true automatically, got: %s", updatedLanding.StreamSettings)
	}
}

// 协议退役：Mixed / SOCKS5 / HTTP 入站不再允许创建（Fail-closed，避免任何匿名代理形态复活）。
func TestAdminInbounds_RetiredProtocolsRejected(t *testing.T) {
	db := apiTestDB(t)
	db.Create(&models.Server{ID: 5, Name: "node-5", Host: "1.2.3.4", NodeID: "n5", Secret: "sec"})

	d := &Deps{DB: db}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/inbounds", d.AdminCreateInbound)

	for _, proto := range []string{"mixed", "socks", "http"} {
		body := map[string]any{
			"server_id":     5,
			"tag":           proto + "-test",
			"protocol":      proto,
			"port":          10801,
			"settings_json": `{"auth":"password","accounts":[{"user":"admin","pass":"secret"}],"udp":true}`,
		}
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/api/v1/admin/inbounds", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("创建 %s 入站应被拒绝，实际: %d, body: %s", proto, w.Code, w.Body.String())
		}
	}

	var cnt int64
	db.Model(&models.Inbound{}).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("退役协议不应落库，实际 %d 条", cnt)
	}
}

func TestAdminUpdateInbound_ClearExpiryTime(t *testing.T) {
	db := apiTestDB(t)
	db.Create(&models.Server{ID: 30, Name: "node-30", Host: "1.2.3.4", NodeID: "n30", Secret: "sec"})
	future := time.Now().Add(24 * time.Hour).UTC()
	inb := models.Inbound{
		ServerID:   30,
		Tag:        "expiry-inbound",
		Protocol:   "vless",
		Port:       44310,
		Type:       models.InboundTypeUser,
		ExpiryTime: &future,
		Enabled:    true,
	}
	db.Create(&inb)
	d := &Deps{DB: db}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/api/v1/admin/inbounds/:id", d.AdminUpdateInbound)

	// 传入空字符串 "" 作为 expiry_time（前端重置时传的空字串）
	body := map[string]any{
		"expiry_time": "",
	}
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/admin/inbounds/%d", inb.ID), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("清空 expiry_time 失败: code=%d body=%s", w.Code, w.Body.String())
	}

	var updated models.Inbound
	if err := db.First(&updated, inb.ID).Error; err != nil {
		t.Fatalf("查询更新后入站失败: %v", err)
	}
	if updated.ExpiryTime != nil {
		t.Fatalf("期望 ExpiryTime 为 nil，实际为: %v", updated.ExpiryTime)
	}
}

func TestAdminUpdateInbound_TunnelProxyProtocolOption(t *testing.T) {
	db := apiTestDB(t)
	db.Create(&models.Server{ID: 41, Name: "node-landing", Host: "1.2.3.4", NodeID: "n41", Secret: "sec"})
	db.Create(&models.Server{ID: 42, Name: "node-tunnel", Host: "1.2.3.5", NodeID: "n42", Secret: "sec"})

	// 落地入站，初始无 acceptProxyProtocol
	landing := models.Inbound{
		ServerID:       41,
		Tag:            "landing-vless",
		Protocol:       "vless",
		Port:           44320,
		Type:           models.InboundTypeUser,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Enabled:        true,
	}
	db.Create(&landing)

	// 直通管道入站
	tunnel := models.Inbound{
		ServerID: 42,
		Tag:      "tunnel-doko",
		Protocol: "tunnel",
		Port:     10080,
		Type:     models.InboundTypeTunnel,
		Enabled:  true,
	}
	db.Create(&tunnel)

	d := &Deps{DB: db}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/api/v1/admin/inbounds/:id", d.AdminUpdateInbound)

	// 1. 连线直通管道，但显式指定 proxy_protocol: false
	bodyNoPP := map[string]any{
		"target_inbound_id": landing.ID,
		"stream_settings":   `{"network":"tcp,udp","proxy_protocol":false}`,
	}
	raw, _ := json.Marshal(bodyNoPP)
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/admin/inbounds/%d", tunnel.ID), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("连接直通管道 (no PP) 失败: code=%d body=%s", w.Code, w.Body.String())
	}

	var landingAfterNoPP models.Inbound
	db.First(&landingAfterNoPP, landing.ID)
	var ssNoPP map[string]any
	_ = json.Unmarshal([]byte(landingAfterNoPP.StreamSettings), &ssNoPP)
	tcpNoPP, _ := ssNoPP["tcpSettings"].(map[string]any)
	if tcpNoPP["acceptProxyProtocol"] == true {
		t.Fatalf("当直通管道 proxy_protocol: false 时，落地入站不应被强制开启 acceptProxyProtocol")
	}

	// 2. 更新直通管道为 proxy_protocol: true
	bodyWithPP := map[string]any{
		"target_inbound_id": landing.ID,
		"stream_settings":   `{"network":"tcp,udp","proxy_protocol":true}`,
	}
	raw, _ = json.Marshal(bodyWithPP)
	req = httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/admin/inbounds/%d", tunnel.ID), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("连接直通管道 (with PP) 失败: code=%d body=%s", w.Code, w.Body.String())
	}

	var landingAfterPP models.Inbound
	db.First(&landingAfterPP, landing.ID)
	var ssPP map[string]any
	_ = json.Unmarshal([]byte(landingAfterPP.StreamSettings), &ssPP)
	if ssPP["acceptProxyProtocol"] == true {
		t.Fatalf("顶层 acceptProxyProtocol 是旧版私有键，不应再由 ensure 写入")
	}
	tcpSettings, _ := ssPP["tcpSettings"].(map[string]any)
	if tcpSettings == nil || tcpSettings["acceptProxyProtocol"] != true {
		t.Fatalf("当直通管道 proxy_protocol: true 时，落地入站应开启 tcpSettings.acceptProxyProtocol")
	}
}

func TestFormToInbound_PreservesTargetFields(t *testing.T) {
	targetID := uint64(12)
	f := &inboundForm{
		ID:              99,
		ServerID:        1,
		Tag:             "test-http",
		Protocol:        "http",
		Port:            8080,
		TargetInboundID: &targetID,
		Remark:          "my remark",
		TrafficReset:    "daily",
	}
	inb := formToInbound(f)
	if inb.TargetInboundID == nil || *inb.TargetInboundID != 12 {
		t.Errorf("formToInbound 未保留 TargetInboundID: want 12, got %v", inb.TargetInboundID)
	}
	if inb.Remark != "my remark" {
		t.Errorf("formToInbound 未保留 Remark: want 'my remark', got %s", inb.Remark)
	}
	if inb.TrafficReset != "daily" {
		t.Errorf("formToInbound 未保留 TrafficReset: want 'daily', got %s", inb.TrafficReset)
	}
}

// S4：字段交叉规则失败时返回 400 + 结构化 issues（data.issues），
// 前端可按 path 定位控件；message 保持可读。
func TestCreateInbound_StructuredFieldIssues(t *testing.T) {
	db := apiTestDB(t)
	db.Create(&models.Server{ID: 11, Name: "node-11", Host: "1.2.3.4", NodeID: "n11", Secret: "sec"})
	d := &Deps{DB: db}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/inbounds", d.AdminCreateInbound)

	body := map[string]any{
		"server_id":       11,
		"tag":             "bad-reality",
		"protocol":        "vless",
		"port":            44302,
		"type":            "user",
		"settings_json":   `{}`,
		"stream_settings": `{"network":"tcp","security":"reality","realitySettings":{}}`,
	}
	data, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/admin/inbounds", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("缺少 REALITY 必填项应 400，got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Issues []struct {
				Path string `json:"path"`
				Code string `json:"code"`
			} `json:"issues"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应解析失败: %v", err)
	}
	got := map[string]string{}
	for _, is := range resp.Data.Issues {
		got[is.Path] = is.Code
	}
	for _, path := range []string{
		"stream.realitySettings.privateKey",
		"stream.realitySettings.dest",
		"stream.realitySettings.serverNames",
		"stream.realitySettings.shortIds",
	} {
		if got[path] != "required" {
			t.Errorf("缺少结构化问题 %s=required，got %+v", path, got)
		}
	}
}

// S4：协议字段声明接口（GET /admin/protocols）返回三套声明与协议描述符的并集。
func TestAdminProtocols(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/admin/protocols", (&Deps{}).AdminProtocols)

	req := httptest.NewRequest("GET", "/api/v1/admin/protocols", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Protocols []struct {
				FieldSet struct {
					Protocol string `json:"protocol"`
					Fields   []any  `json:"fields"`
					Rules    []any  `json:"rules"`
				} `json:"field_set"`
				Descriptor *struct {
					Name string `json:"Name"`
				} `json:"descriptor"`
			} `json:"protocols"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应解析失败: %v", err)
	}
	found := map[string]bool{}
	for _, p := range resp.Data.Protocols {
		found[p.FieldSet.Protocol] = true
		if len(p.FieldSet.Fields) == 0 {
			t.Errorf("%s 字段声明为空", p.FieldSet.Protocol)
		}
	}
	for _, proto := range []string{"vless", "hysteria", "tunnel"} {
		if !found[proto] {
			t.Errorf("缺少协议声明: %s", proto)
		}
	}
}
