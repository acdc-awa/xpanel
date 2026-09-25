package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
)

func TestEnsureInternalUUID(t *testing.T) {
	db := apiTestDB(t)
	d := &Deps{DB: db}

	// 1. user 入站不应分配 internal_uuid
	inbUser := models.Inbound{ServerID: 1, Tag: "in-user", Type: models.InboundTypeUser}
	if err := d.ensureInternalUUID(&inbUser); err != nil {
		t.Fatalf("ensureInternalUUID for user: %v", err)
	}
	if inbUser.InternalUUID != "" {
		t.Errorf("user 入站 InternalUUID 应为空，实际为 %s", inbUser.InternalUUID)
	}

	// 2. relay 入站无 UUID 时应兜底生成有效 UUID
	inbRelay := models.Inbound{ServerID: 1, Tag: "in-relay", Type: models.InboundTypeRelay}
	if err := d.ensureInternalUUID(&inbRelay); err != nil {
		t.Fatalf("ensureInternalUUID for relay: %v", err)
	}
	if inbRelay.InternalUUID == "" {
		t.Error("relay 入站应自动分配 InternalUUID")
	}

	// 3. relay 入站已有 UUID 时不应被覆盖
	existingUUID := inbRelay.InternalUUID
	if err := d.ensureInternalUUID(&inbRelay); err != nil {
		t.Fatalf("ensureInternalUUID idempotent: %v", err)
	}
	if inbRelay.InternalUUID != existingUUID {
		t.Errorf("已有 UUID 不应被覆盖: old=%s, new=%s", existingUUID, inbRelay.InternalUUID)
	}
}

func TestCreateRelayInbound_AutoUUID(t *testing.T) {
	db := apiTestDB(t)
	db.Create(&models.Server{ID: 10, Name: "node-10", Host: "1.2.3.4", NodeID: "n10", Secret: "sec"})
	d := &Deps{DB: db}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/inbounds", d.AdminCreateInbound)

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
		t.Fatalf("创建 relay 入站失败: code=%d body=%s", w.Code, w.Body.String())
	}

	var inb models.Inbound
	if err := db.Where("server_id = ? AND tag = ?", 10, "relay-direct-create").First(&inb).Error; err != nil {
		t.Fatalf("未找到新建的 relay 入站: %v", err)
	}
	if inb.Type != models.InboundTypeRelay {
		t.Errorf("期望 type=relay, 实际 %s", inb.Type)
	}
	if inb.InternalUUID == "" {
		t.Error("新建的 relay 入站应自动分配 InternalUUID")
	}
}

func TestUpdateInboundToRelay_AutoUUID(t *testing.T) {
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
		t.Fatalf("更新入站为 relay 失败: code=%d body=%s", w.Code, w.Body.String())
	}

	var updated models.Inbound
	db.First(&updated, inb.ID)
	if updated.Type != models.InboundTypeRelay {
		t.Errorf("更新后 type 应为 relay, 实际 %s", updated.Type)
	}
	if updated.InternalUUID == "" {
		t.Error("更新为 relay 后应自动分配 InternalUUID")
	}
}

func TestUpdateInboundToRelay_ExplicitEmptyUUIDStillEnsured(t *testing.T) {
	db := apiTestDB(t)
	db.Create(&models.Server{ID: 13, Name: "node-13", Host: "1.2.3.7", NodeID: "n13", Secret: "sec"})
	inb := models.Inbound{
		ServerID: 13, Tag: "user-to-relay-empty-uuid", Protocol: "vless", Port: 44304,
		Type: models.InboundTypeUser, Enabled: true,
	}
	db.Create(&inb)
	d := &Deps{DB: db}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/api/v1/admin/inbounds/:id", d.AdminUpdateInbound)

	// type 切 relay 与显式空 internal_uuid 同请求：空串不应覆盖自动补齐的 UUID
	body := map[string]any{"type": "relay", "internal_uuid": ""}
	data, _ := json.Marshal(body)
	req := httptest.NewRequest("PUT", "/api/v1/admin/inbounds/1", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("更新入站为 relay 失败: code=%d body=%s", w.Code, w.Body.String())
	}

	var updated models.Inbound
	db.First(&updated, inb.ID)
	if updated.Type != models.InboundTypeRelay {
		t.Errorf("更新后 type 应为 relay, 实际 %s", updated.Type)
	}
	if updated.InternalUUID == "" {
		t.Error("显式传空 internal_uuid 不应覆盖自动补齐结果")
	}
}

func TestEnsureRelayMark_AutoUUID(t *testing.T) {
	db := apiTestDB(t)
	_, _, _, bIn := seedRefGraph(t, db)
	d := &Deps{DB: db}

	// 设为 user 且清空 UUID
	db.Model(&models.Inbound{}).Where("id = ?", bIn).Updates(map[string]any{
		"type":          models.InboundTypeUser,
		"internal_uuid": "",
	})

	d.ensureRelayMark(bIn)

	var b models.Inbound
	db.First(&b, bIn)
	if b.Type != models.InboundTypeRelay {
		t.Errorf("ensureRelayMark 后 type 应为 relay, 实际 %s", b.Type)
	}
	if b.InternalUUID == "" {
		t.Error("ensureRelayMark 后应自动补齐 InternalUUID")
	}
}

func TestEnqueueConfig_AutoHealsBrokenRelayInbound(t *testing.T) {
	db := apiTestDB(t)
	db.Create(&models.Server{ID: 12, Name: "node-12", Host: "1.2.3.6", NodeID: "n12", Secret: "sec"})
	// 模拟历史残留的没有 UUID 的损坏 relay 入站
	broken := models.Inbound{
		ServerID: 12, Tag: "broken-relay", Protocol: "vless", Port: 44303,
		Type: models.InboundTypeRelay, InternalUUID: "", Enabled: true,
	}
	db.Create(&broken)
	d := &Deps{DB: db}

	// 此时调用 enqueueConfig（即使 Config 为 nil），也会先执行自愈修复
	_ = d.enqueueConfig(12)

	var healed models.Inbound
	db.First(&healed, broken.ID)
	if healed.InternalUUID == "" {
		t.Error("enqueueConfig 应自动修复缺少 InternalUUID 的 relay 入站")
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
		ServerID: 1, Tag: "tunnel-in", Protocol: "dokodemo-door", Port: 10001,
		Type: models.InboundTypeTunnel, TargetInboundID: &landing.ID, Enabled: true,
	}
	db.Create(&tunnel)

	// 2. Channel 入站
	chanInb := models.Inbound{
		ServerID: 1, Tag: "chan-socks-1080", Protocol: "socks", Port: 1080,
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

	// 尝试删除 Channel 入站 -> 应该被 400 拦截
	req2 := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/admin/inbounds/%d", chanInb.ID), nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("删除 Channel 入站应返回 400, 实际: %d, body: %s", w2.Code, w2.Body.String())
	}

	// 尝试通过常规接口更新 Channel 入站 -> 应该被 400 拦截
	upBody, _ := json.Marshal(map[string]any{"port": 1081})
	req3 := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/admin/inbounds/%d", chanInb.ID), bytes.NewReader(upBody))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("更新 Channel 入站应返回 400, 实际: %d, body: %s", w3.Code, w3.Body.String())
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
		ServerID: 2, Tag: "tunnel-in", Protocol: "dokodemo-door", Port: 10001,
		Type: models.InboundTypeTunnel, TargetInboundID: &landing.ID, Enabled: true,
	}
	db.Create(&tunnel)

	// 2. Channel 入站
	chanInb := models.Inbound{
		ServerID: 2, Tag: "chan-socks-1080", Protocol: "socks", Port: 1080,
		Type: models.InboundTypeChannel, Enabled: true,
	}
	db.Create(&chanInb)

	d := &Deps{DB: db}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/inbounds/:id/toggle", d.AdminToggleInbound)
	r.PUT("/api/v1/admin/inbounds/:id", d.AdminUpdateInbound)

	// A. 尝试通过 Toggle 切换 Channel 入站 -> 应该被 400 拦截 (P2-6)
	req1 := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/admin/inbounds/%d/toggle", chanInb.ID), nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("Toggle Channel 入站应返回 400, 实际: %d, body: %s", w1.Code, w1.Body.String())
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
	tunnel.Enabled = false
	db.Save(&tunnel)

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
		"protocol":          "dokodemo-door",
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
	if app, _ := ss["acceptProxyProtocol"].(bool); !app {
		t.Fatalf("Landing inbound acceptProxyProtocol should be set to true automatically, got: %s", updatedLanding.StreamSettings)
	}
}

