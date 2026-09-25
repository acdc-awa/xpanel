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

func TestAdminChannels_CRUD(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := apiTestDB(t)
	srv := models.Server{ID: 1, Name: "Test Server", Host: "1.1.1.1", NodeID: "n1", Secret: "sec"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatalf("Create server failed: %v", err)
	}

	d := &Deps{DB: db}
	r := gin.New()
	r.GET("/api/v1/admin/channels", d.AdminChannels)
	r.POST("/api/v1/admin/channels", d.AdminCreateChannel)
	r.PUT("/api/v1/admin/channels/:id", d.AdminUpdateChannel)
	r.DELETE("/api/v1/admin/channels/:id", d.AdminDeleteChannel)
	r.POST("/api/v1/admin/channels/:id/toggle", d.AdminToggleChannel)
	r.POST("/api/v1/admin/channels/:id/reset-traffic", d.AdminResetChannelTraffic)

	// 1. 创建 SOCKS5 通道
	bodySocks := map[string]any{
		"name":             "我的爬虫代理",
		"server_id":        1,
		"port":             1080,
		"protocol":         "socks5",
		"username":         "spider",
		"password":         "secret123",
		"allow_udp":        true,
		"traffic_limit_gb": 50,
		"traffic_reset":    "monthly",
	}
	rawSocks, _ := json.Marshal(bodySocks)
	req1 := httptest.NewRequest("POST", "/api/v1/admin/channels", bytes.NewReader(rawSocks))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("Create socks5 channel failed, code: %d, body: %s", w1.Code, w1.Body.String())
	}

	// 验证数据库中 ProxyChannel 与底层 Inbound
	var ch models.ProxyChannel
	if err := db.First(&ch, "port = ?", 1080).Error; err != nil {
		t.Fatalf("ProxyChannel not found in DB: %v", err)
	}
	if ch.Name != "我的爬虫代理" || ch.Protocol != "socks5" || ch.TrafficLimitGB != 50 {
		t.Errorf("ProxyChannel fields mismatch: %+v", ch)
	}

	var inb models.Inbound
	if err := db.First(&inb, ch.InboundID).Error; err != nil {
		t.Fatalf("Underlying Inbound not found: %v", err)
	}
	if inb.Protocol != "socks" || inb.Type != models.InboundTypeChannel || !inb.Enabled {
		t.Errorf("Underlying Inbound mismatch: %+v", inb)
	}
	if inb.Total != 0 {
		t.Errorf("Underlying Inbound Total must be 0 for channels to prevent unit mismatch, got: %d", inb.Total)
	}

	// 2. 测试端口冲突防重
	reqDup := httptest.NewRequest("POST", "/api/v1/admin/channels", bytes.NewReader(rawSocks))
	reqDup.Header.Set("Content-Type", "application/json")
	wDup := httptest.NewRecorder()
	r.ServeHTTP(wDup, reqDup)
	if wDup.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for duplicate port, got: %d", wDup.Code)
	}

	// 3. 创建四层端口转发 (Tunnel)
	bodyTunnel := map[string]any{
		"name":             "自建海外落地",
		"server_id":        1,
		"port":             10001,
		"protocol":         "tunnel",
		"target_address":   "exit.target.com",
		"target_port":      443,
		"proxy_protocol":   true,
		"traffic_limit_gb": 100,
	}
	rawTunnel, _ := json.Marshal(bodyTunnel)
	req2 := httptest.NewRequest("POST", "/api/v1/admin/channels", bytes.NewReader(rawTunnel))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("Create tunnel channel failed: %d, body: %s", w2.Code, w2.Body.String())
	}

	// 4. 查询列表
	reqList := httptest.NewRequest("GET", "/api/v1/admin/channels", nil)
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("List channels failed: %d", wList.Code)
	}
	var listResp struct {
		Code int `json:"code"`
		Data struct {
			Channels []channelView `json:"channels"`
		} `json:"data"`
	}
	if err := json.Unmarshal(wList.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("Unmarshal list response failed: %v", err)
	}
	if len(listResp.Data.Channels) != 2 {
		t.Fatalf("Expected 2 channels, got %d", len(listResp.Data.Channels))
	}

	// 4.1 更新通道 (Update)
	bodyUp := map[string]any{
		"name":             "更新后的爬虫代理",
		"server_id":        1,
		"protocol":         "socks5",
		"port":             1080,
		"traffic_limit_gb": 80,
	}
	rawUp, _ := json.Marshal(bodyUp)
	reqUp := httptest.NewRequest("PUT", "/api/v1/admin/channels/1", bytes.NewReader(rawUp))
	reqUp.Header.Set("Content-Type", "application/json")
	wUp := httptest.NewRecorder()
	r.ServeHTTP(wUp, reqUp)
	if wUp.Code != http.StatusOK {
		t.Fatalf("Update channel failed: %d, body: %s", wUp.Code, wUp.Body.String())
	}
	db.First(&inb, ch.InboundID)
	if inb.Total != 0 {
		t.Errorf("Underlying Inbound Total must remain 0 after update, got: %d", inb.Total)
	}

	// 5. 切换状态 (Toggle)
	reqToggle := httptest.NewRequest("POST", "/api/v1/admin/channels/1/toggle", nil)
	wToggle := httptest.NewRecorder()
	r.ServeHTTP(wToggle, reqToggle)
	if wToggle.Code != http.StatusOK {
		t.Fatalf("Toggle channel failed: %d", wToggle.Code)
	}
	db.First(&ch, 1)
	if ch.Enabled || ch.Status != models.ChannelStatusDisabled {
		t.Errorf("Expected channel disabled, got enabled=%v status=%s", ch.Enabled, ch.Status)
	}
	db.First(&inb, ch.InboundID)
	if inb.Enabled {
		t.Errorf("Expected underlying inbound disabled")
	}

	// 6. 重置流量 (Reset Traffic)
	db.Model(&ch).Updates(map[string]any{"traffic_used_bytes": 1024000, "up_bytes": 512000, "down_bytes": 512000})
	reqReset := httptest.NewRequest("POST", "/api/v1/admin/channels/1/reset-traffic", nil)
	wReset := httptest.NewRecorder()
	r.ServeHTTP(wReset, reqReset)
	if wReset.Code != http.StatusOK {
		t.Fatalf("Reset traffic failed: %d", wReset.Code)
	}
	db.First(&ch, 1)
	if ch.TrafficUsedBytes != 0 || ch.UpBytes != 0 || ch.DownBytes != 0 {
		t.Errorf("Expected traffic reset to 0, got used=%d", ch.TrafficUsedBytes)
	}

	// 7. 删除通道
	reqDel := httptest.NewRequest("DELETE", "/api/v1/admin/channels/1", nil)
	wDel := httptest.NewRecorder()
	r.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("Delete channel failed: %d", wDel.Code)
	}
	if err := db.First(&models.ProxyChannel{}, 1).Error; err == nil {
		t.Errorf("Channel 1 should be deleted from DB")
	}
	if err := db.First(&models.Inbound{}, inb.ID).Error; err == nil {
		t.Errorf("Underlying inbound should be deleted from DB")
	}
}
