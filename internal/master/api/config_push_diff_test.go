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

// TestXrayConfigChangedInbound 推送判定必须与「库内旧值」比较，而不是「请求里有没有这个键」。
func TestXrayConfigChangedInbound(t *testing.T) {
	old := models.Inbound{
		Tag: "in", Protocol: "vless", Port: 443, Listen: "0.0.0.0",
		SettingsJSON: `{"decryption":"none"}`, StreamSettings: `{"network":"tcp","security":"none"}`,
		Sniffing: "", Type: models.InboundTypeUser, Enabled: true, Flow: "",
	}
	cases := []struct {
		name    string
		updates map[string]any
		want    bool
	}{
		{"只改倍率", map[string]any{"ratio": 2.5}, false},
		{"只改额度与到期", map[string]any{"total": int64(100), "expiry_time": nil, "remark": "x", "traffic_reset": "daily"}, false},
		{"整表回传但值未变", map[string]any{
			"tag": "in", "protocol": "vless", "port": 443, "listen": "0.0.0.0",
			"settings_json": `{"decryption":"none"}`, "stream_settings": `{"network":"tcp","security":"none"}`,
			"sniffing": "", "type": "user", "enabled": true, "flow": "",
		}, false},
		{"JSON 仅排版不同", map[string]any{"settings_json": `{ "decryption" : "none" }`}, false},
		{"端口真的变了", map[string]any{"port": 8443}, true},
		{"协议真的变了", map[string]any{"protocol": "hysteria"}, true},
		{"启停真的变了", map[string]any{"enabled": false}, true},
		{"settings 语义变了", map[string]any{"settings_json": `{"decryption":"none","fallbacks":[]}`}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := xrayConfigChanged(old, tc.updates); got != tc.want {
				t.Errorf("xrayConfigChanged=%v 期望 %v（updates=%v）", got, tc.want, tc.updates)
			}
		})
	}
}

// TestOutboundConfigChanged 出站侧同一口径：备注不参与生成。
func TestOutboundConfigChanged(t *testing.T) {
	ref := uint64(7)
	old := models.ServerOutbound{
		Tag: "to-a", Protocol: "vless", InboundRef: &ref, InternalUUID: "uuid-a",
		SettingsJSON: "", StreamSettingsJSON: "", OutboundJSON: "",
		SendThrough: "", SendProxyProtocol: false, Enabled: true, Priority: 0,
	}
	cases := []struct {
		name    string
		updates map[string]any
		want    bool
	}{
		{"只改备注", map[string]any{"remark": "改个说明"}, false},
		{"整表回传但值未变", map[string]any{
			"tag": "to-a", "protocol": "vless", "inbound_ref": ref, "outbound_json": "",
			"settings_json": "", "stream_settings_json": "", "send_through": "",
			"enabled": true, "priority": 0, "send_proxy_protocol": false,
		}, false},
		{"优先级变了", map[string]any{"priority": 5}, true},
		{"PP 开关变了", map[string]any{"send_proxy_protocol": true}, true},
		{"解绑引用", map[string]any{"inbound_ref": nil}, true},
		{"换引用目标", map[string]any{"inbound_ref": uint64(8)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := outboundConfigChanged(old, tc.updates); got != tc.want {
				t.Errorf("outboundConfigChanged=%v 期望 %v（updates=%v）", got, tc.want, tc.updates)
			}
		})
	}
}

// TestRoutingRuleConfigChanged 路由规则同一口径：备注不参与生成。
func TestRoutingRuleConfigChanged(t *testing.T) {
	old := models.ServerRoutingRule{
		OutboundTag: "to-a", RuleJSON: `{"domain":["a.com"]}`, Domain: "a.com", IP: "",
		Port: "", Network: "tcp", Protocol: "", InboundTag: "", Enabled: true, Priority: 0,
	}
	if routingRuleConfigChanged(old, map[string]any{"remark": "x"}) {
		t.Error("只改备注不应重推")
	}
	if routingRuleConfigChanged(old, map[string]any{"rule_json": `{ "domain" : ["a.com"] }`}) {
		t.Error("rule_json 仅排版不同不应重推")
	}
	if !routingRuleConfigChanged(old, map[string]any{"domain": "b.com"}) {
		t.Error("匹配域真的变了应重推")
	}
	if !routingRuleConfigChanged(old, map[string]any{"enabled": false}) {
		t.Error("启停真的变了应重推")
	}
}

// TestUpdateOutboundRemarkOnlyDoesNotPush 出站编辑器整表回传时只改备注：
// 本机与落地机都不该被重推。
func TestUpdateOutboundRemarkOnlyDoesNotPush(t *testing.T) {
	d, db := newFanoutDeps(t)
	s1, s2, landing, ob := seedRefPair(t, db)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/api/v1/admin/servers/:id/outbounds/:outbound_id", d.AdminUpdateServerOutbound)

	put := func(payload map[string]any) {
		t.Helper()
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("PUT",
			fmt.Sprintf("/api/v1/admin/servers/%d/outbounds/%d", s2.ID, ob.ID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("更新出站失败: code=%d body=%s", w.Code, w.Body.String())
		}
	}
	// 与编辑器一致的整表回传
	fullForm := func(overrides map[string]any) map[string]any {
		body := map[string]any{
			"tag": "to-landing", "protocol": "vless", "inbound_ref": landing.ID,
			"outbound_json": "", "settings_json": "", "stream_settings_json": "",
			"send_through": "", "enabled": true, "priority": 0,
			"send_proxy_protocol": false, "remark": "",
		}
		for k, v := range overrides {
			body[k] = v
		}
		return body
	}

	put(fullForm(map[string]any{"remark": "新的备注"}))
	for _, sid := range []uint64{s2.ID, s1.ID} {
		if p, err := d.Config.GetPending(sid); err != nil || p != nil {
			t.Fatalf("只改备注：服务器 %d 不应被重推: err=%v pending=%v", sid, err, p)
		}
	}

	// 真正的变更（优先级）仍要推本机
	put(fullForm(map[string]any{"priority": 7}))
	if p, err := d.Config.GetPending(s2.ID); err != nil || p == nil {
		t.Fatalf("改优先级后本机应有待推配置: err=%v pending=%v", err, p)
	}
}

// TestUpdateRoutingRuleRemarkOnlyDoesNotPush 路由规则只改备注不重推。
func TestUpdateRoutingRuleRemarkOnlyDoesNotPush(t *testing.T) {
	d, db := newFanoutDeps(t)
	_, s2, landing, _ := seedRefPair(t, db)

	rule := models.ServerRoutingRule{
		ServerID: s2.ID, OutboundTag: "to-landing", Domain: "example.com",
		Enabled: true, Priority: 0,
	}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatalf("创建路由规则: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/api/v1/admin/servers/:id/routing/:rule_id", d.AdminUpdateServerRoutingRule)

	put := func(payload map[string]any) {
		t.Helper()
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("PUT",
			fmt.Sprintf("/api/v1/admin/servers/%d/routing/%d", s2.ID, rule.ID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("更新路由规则失败: code=%d body=%s", w.Code, w.Body.String())
		}
	}

	put(map[string]any{"remark": "说明文字"})
	for _, sid := range []uint64{s2.ID, landing.ServerID} {
		if p, err := d.Config.GetPending(sid); err != nil || p != nil {
			t.Fatalf("只改备注：服务器 %d 不应被重推: err=%v pending=%v", sid, err, p)
		}
	}

	put(map[string]any{"domain": "other.com"})
	if p, err := d.Config.GetPending(s2.ID); err != nil || p == nil {
		t.Fatalf("改匹配域后本机应有待推配置: err=%v pending=%v", err, p)
	}
}
