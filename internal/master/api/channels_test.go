package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

	// 验证数据库中入站（收口至 Inbound 模型）
	var inb models.Inbound
	if err := db.First(&inb, "port = ?", 1080).Error; err != nil {
		t.Fatalf("Inbound not found in DB: %v", err)
	}
	if inb.Remark != "我的爬虫代理" || inb.Protocol != "socks" || inb.Type != models.InboundTypeForward || !inb.Enabled {
		t.Errorf("Inbound fields mismatch: %+v", inb)
	}
	if inb.Total != 50*1024*1024*1024 {
		t.Errorf("Inbound Total mismatch, expected 50GB in bytes, got: %d", inb.Total)
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
	// 注意：PUT 是整表单提交，必须带上认证账号/密码——漏传会被拒（禁止免密开放）。
	// 改动前漏传不报错，而是静默把 ch.Username 置空 → 通道变成匿名开放代理。
	bodyUp := map[string]any{
		"name":             "更新后的爬虫代理",
		"server_id":        1,
		"protocol":         "socks5",
		"port":             1080,
		"username":         "spider",
		"password":         "secret123",
		"traffic_limit_gb": 80,
	}
	rawUp, _ := json.Marshal(bodyUp)
	reqUp := httptest.NewRequest("PUT", "/api/v1/admin/channels/"+strconv.FormatUint(inb.ID, 10), bytes.NewReader(rawUp))
	reqUp.Header.Set("Content-Type", "application/json")
	wUp := httptest.NewRecorder()
	r.ServeHTTP(wUp, reqUp)
	if wUp.Code != http.StatusOK {
		t.Fatalf("Update channel failed: %d, body: %s", wUp.Code, wUp.Body.String())
	}
	db.First(&inb, inb.ID)
	if inb.Total != 80*1024*1024*1024 {
		t.Errorf("Inbound Total mismatch, expected 80GB in bytes, got: %d", inb.Total)
	}

	// 5. 切换状态 (Toggle)
	reqToggle := httptest.NewRequest("POST", "/api/v1/admin/channels/"+strconv.FormatUint(inb.ID, 10)+"/toggle", nil)
	wToggle := httptest.NewRecorder()
	r.ServeHTTP(wToggle, reqToggle)
	if wToggle.Code != http.StatusOK {
		t.Fatalf("Toggle channel failed: %d", wToggle.Code)
	}
	db.First(&inb, inb.ID)
	if inb.Enabled {
		t.Errorf("Expected inbound disabled, got enabled=true")
	}

	// 6. 重置流量 (Reset Traffic)
	db.Model(&inb).Updates(map[string]any{"up": 512000, "down": 512000})
	reqReset := httptest.NewRequest("POST", "/api/v1/admin/channels/"+strconv.FormatUint(inb.ID, 10)+"/reset-traffic", nil)
	wReset := httptest.NewRecorder()
	r.ServeHTTP(wReset, reqReset)
	if wReset.Code != http.StatusOK {
		t.Fatalf("Reset traffic failed: %d", wReset.Code)
	}
	db.First(&inb, inb.ID)
	if inb.Up != 0 || inb.Down != 0 {
		t.Errorf("Expected traffic reset to 0, got up=%d down=%d", inb.Up, inb.Down)
	}

	// 7. 删除通道
	reqDel := httptest.NewRequest("DELETE", "/api/v1/admin/channels/"+strconv.FormatUint(inb.ID, 10), nil)
	wDel := httptest.NewRecorder()
	r.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("Delete channel failed: %d", wDel.Code)
	}
	if err := db.First(&models.Inbound{}, inb.ID).Error; err == nil {
		t.Errorf("Inbound should be deleted from DB")
	}
}

// TestAdminChannels_RejectsAnonymousCredentials 禁止免密开放（2026-09-28 拍板）：
// socks5/http 通道在创建与更新两条路径上都必须带认证账号与密码；tunnel 无认证概念，不受约束。
func TestAdminChannels_RejectsAnonymousCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := apiTestDB(t)
	srv := models.Server{ID: 1, Name: "Test Server", Host: "1.1.1.1", NodeID: "n1", Secret: "sec"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatalf("Create server failed: %v", err)
	}
	d := &Deps{DB: db}
	r := gin.New()
	r.POST("/api/v1/admin/channels", d.AdminCreateChannel)
	r.PUT("/api/v1/admin/channels/:id", d.AdminUpdateChannel)

	post := func(body map[string]any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/api/v1/admin/channels", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	put := func(id uint64, body map[string]any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest("PUT", "/api/v1/admin/channels/"+strconv.FormatUint(id, 10), bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"socks5 空账号被拒", map[string]any{"name": "a", "server_id": 1, "port": 1081, "protocol": "socks5"}, http.StatusBadRequest},
		{"socks5 有账号无密码被拒", map[string]any{"name": "b", "server_id": 1, "port": 1082, "protocol": "socks5", "username": "u"}, http.StatusBadRequest},
		{"http 空账号被拒", map[string]any{"name": "c", "server_id": 1, "port": 1083, "protocol": "http"}, http.StatusBadRequest},
		{"http 纯空白账号被拒", map[string]any{"name": "d", "server_id": 1, "port": 1084, "protocol": "http", "username": "   ", "password": "x"}, http.StatusBadRequest},
		{"tunnel 无认证概念不受约束", map[string]any{"name": "e", "server_id": 1, "port": 1085, "protocol": "tunnel", "target_address": "1.2.3.4", "target_port": 443}, http.StatusOK},
		{"socks5 带账号密码通过", map[string]any{"name": "f", "server_id": 1, "port": 1086, "protocol": "socks5", "username": "u", "password": "p"}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if w := post(tc.body); w.Code != tc.want {
				t.Fatalf("create code = %d, want %d, body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}

	// 更新路径：清空认证必须被拒（补齐账号是合法修复动作，必须放行）
	var chInb models.Inbound
	if err := db.First(&chInb, "port = ?", 1086).Error; err != nil {
		t.Fatalf("channel inbound not found: %v", err)
	}
	if w := put(chInb.ID, map[string]any{"name": "f", "server_id": 1, "protocol": "socks5", "port": 1086, "username": "", "password": ""}); w.Code != http.StatusBadRequest {
		t.Errorf("清空认证应被拒，code = %d, body = %s", w.Code, w.Body.String())
	}
	if w := put(chInb.ID, map[string]any{"name": "f", "server_id": 1, "protocol": "socks5", "port": 1086, "username": "u2", "password": "p2"}); w.Code != http.StatusOK {
		t.Errorf("补齐认证应放行，code = %d, body = %s", w.Code, w.Body.String())
	}
	db.First(&chInb, chInb.ID)
	if strings.Contains(chInb.SettingsJSON, `"noauth"`) {
		t.Errorf("更新后底层入站仍是免密形态: %s", chInb.SettingsJSON)
	}
}
