package api

import (
	"bytes"
	"encoding/json"
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
