package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/nodegate"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

// newFanoutDeps 构造带真实 ConfigService 与 Hub 的 Deps：
// 只验证「生成 + 落待推送」，Hub 无连接，PushPending 会走离线分支，不会尝试下发。
func newFanoutDeps(t *testing.T) (*Deps, *gorm.DB) {
	t.Helper()
	db := apiTestDB(t)
	cfg := &services.ConfigService{DB: db}
	return &Deps{DB: db, Config: cfg, Hub: nodegate.NewHub(db, nil, cfg)}, db
}

// seedRefPair 造一对机间链式引用：s1 上是落地入站，s2 上有指向它的出站。
func seedRefPair(t *testing.T, db *gorm.DB) (s1, s2 models.Server, landing models.Inbound, ref models.ServerOutbound) {
	t.Helper()
	s1 = models.Server{Name: "target", Host: "10.0.0.1", NodeID: "n1", Secret: "sec-1"}
	s2 = models.Server{Name: "relay", Host: "10.0.0.2", NodeID: "n2", Secret: "sec-2"}
	if err := db.Create(&s1).Error; err != nil {
		t.Fatalf("创建服务器 1: %v", err)
	}
	if err := db.Create(&s2).Error; err != nil {
		t.Fatalf("创建服务器 2: %v", err)
	}
	landing = models.Inbound{
		ServerID: s1.ID, Tag: "landing", Protocol: "vless", Port: 44300,
		SettingsJSON:   `{"decryption":"none"}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Type:           models.InboundTypeUser, Enabled: true,
	}
	if err := db.Create(&landing).Error; err != nil {
		t.Fatalf("创建落地入站: %v", err)
	}
	refID := landing.ID
	ref = models.ServerOutbound{
		ServerID: s2.ID, Tag: "to-landing", Protocol: "vless",
		InboundRef: &refID, InternalUUID: "11111111-2222-3333-4444-555555555555", Enabled: true,
	}
	if err := db.Create(&ref).Error; err != nil {
		t.Fatalf("创建引用出站: %v", err)
	}
	return s1, s2, landing, ref
}

// pendingVnext 从待推送配置里取指定出站 tag 的 vnext[0]（不存在返回 nil）。
func pendingVnext(t *testing.T, cfgJSON, tag string) map[string]any {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal([]byte(cfgJSON), &root); err != nil {
		t.Fatalf("待推送配置不是合法 JSON: %v", err)
	}
	outbounds, _ := root["outbounds"].([]any)
	for _, item := range outbounds {
		ob, _ := item.(map[string]any)
		if ob == nil || ob["tag"] != tag {
			continue
		}
		settings, _ := ob["settings"].(map[string]any)
		vnext, _ := settings["vnext"].([]any)
		if len(vnext) == 0 {
			return nil
		}
		vn, _ := vnext[0].(map[string]any)
		return vn
	}
	return nil
}

// TestUpdateInboundRepushesReferencingOutboundServers：改落地入站的端口后，
// 引用方服务器的待推送配置必须跟着变（而不是等 1 小时校准）。
func TestUpdateInboundRepushesReferencingOutboundServers(t *testing.T) {
	d, db := newFanoutDeps(t)
	s1, s2, landing, _ := seedRefPair(t, db)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/api/v1/admin/inbounds/:id", d.AdminUpdateInbound)

	body, _ := json.Marshal(map[string]any{"port": 44301})
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/admin/inbounds/%d", landing.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("更新入站失败: code=%d body=%s", w.Code, w.Body.String())
	}

	pending, err := d.Config.GetPending(s2.ID)
	if err != nil || pending == nil {
		t.Fatalf("引用方服务器应有待推送配置: err=%v pending=%v", err, pending)
	}
	vn := pendingVnext(t, pending.ConfigJSON, "to-landing")
	if vn == nil {
		t.Fatalf("引用方配置里应出现引用出站 to-landing")
	}
	if vn["port"] != float64(44301) {
		t.Errorf("引用出站 port 应跟随落地入站更新，实际 %v", vn["port"])
	}
	if vn["address"] != s1.Host {
		t.Errorf("引用出站 address 应取落地服务器 host，实际 %v", vn["address"])
	}

	// 落地服务器自身仍要重推（原有行为不能回退）
	if self, err := d.Config.GetPending(s1.ID); err != nil || self == nil {
		t.Fatalf("落地服务器自身应有待推送配置: err=%v self=%v", err, self)
	}
}

// TestServerHostChangeRepushesReferencingOutboundServers：改服务器地址后，
// 所有引用该机入站的出站都必须重生成（vnext address 是生成期现读的）。
func TestServerHostChangeRepushesReferencingOutboundServers(t *testing.T) {
	d, db := newFanoutDeps(t)
	s1, s2, _, _ := seedRefPair(t, db)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/api/v1/admin/servers/:id", d.AdminUpdateServer)

	host := "10.9.9.9"
	body, _ := json.Marshal(map[string]any{"host": host})
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/admin/servers/%d", s1.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("更新服务器失败: code=%d body=%s", w.Code, w.Body.String())
	}

	pending, err := d.Config.GetPending(s2.ID)
	if err != nil || pending == nil {
		t.Fatalf("引用方服务器应有待推送配置: err=%v pending=%v", err, pending)
	}
	vn := pendingVnext(t, pending.ConfigJSON, "to-landing")
	if vn == nil {
		t.Fatalf("引用方配置里应出现引用出站 to-landing")
	}
	if vn["address"] != host {
		t.Errorf("引用出站 address 应跟随服务器新地址，实际 %v", vn["address"])
	}
}

// TestInboundUpdateDoesNotRepushUnrelatedServers：没有消费关系的服务器不该被顺带重推
// （每次多推一次就是一次多余的 xray 重启）。
func TestInboundUpdateDoesNotRepushUnrelatedServers(t *testing.T) {
	d, db := newFanoutDeps(t)
	_, _, landing, _ := seedRefPair(t, db)

	unrelated := models.Server{Name: "unrelated", Host: "10.0.0.3", NodeID: "n3", Secret: "sec-3"}
	if err := db.Create(&unrelated).Error; err != nil {
		t.Fatalf("创建无关服务器: %v", err)
	}
	if err := db.Create(&models.ServerOutbound{
		ServerID: unrelated.ID, Tag: "direct", Protocol: "freedom", Enabled: true,
	}).Error; err != nil {
		t.Fatalf("创建无关出站: %v", err)
	}
	// 停用的引用出站同样不算消费方（生成侧会跳过它）
	disabledRefID := landing.ID
	disabledServer := models.Server{Name: "disabled-ref", Host: "10.0.0.4", NodeID: "n4", Secret: "sec-4"}
	if err := db.Create(&disabledServer).Error; err != nil {
		t.Fatalf("创建停用引用服务器: %v", err)
	}
	if err := db.Create(&models.ServerOutbound{
		ServerID: disabledServer.ID, Tag: "off", Protocol: "vless",
		InboundRef: &disabledRefID, InternalUUID: "22222222-3333-4444-5555-666666666666", Enabled: true,
	}).Error; err != nil {
		t.Fatalf("创建停用引用出站: %v", err)
	}
	// GORM 的 default:true 会把零值 false 当成"未设置"重新填成 true，必须显式改停用。
	if err := db.Model(&models.ServerOutbound{}).Where("server_id = ?", disabledServer.ID).
		Update("enabled", false).Error; err != nil {
		t.Fatalf("停用引用出站: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/api/v1/admin/inbounds/:id", d.AdminUpdateInbound)
	body, _ := json.Marshal(map[string]any{"port": 44302})
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/admin/inbounds/%d", landing.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("更新入站失败: code=%d body=%s", w.Code, w.Body.String())
	}

	for _, sid := range []uint64{unrelated.ID, disabledServer.ID} {
		if p, err := d.Config.GetPending(sid); err != nil || p != nil {
			t.Errorf("服务器 %d 与该入站无消费关系，不应生成待推送配置: err=%v pending=%v", sid, err, p)
		}
	}
}

// TestServerUpdateRepushesOwnConfigForRoutingDefaults：默认出口 / 解析策略只在生成期读取，
// 改完必须重推本机（此前只写库，节点一直跑旧值）；与配置无关的字段不得触发重推。
func TestServerUpdateRepushesOwnConfigForRoutingDefaults(t *testing.T) {
	d, db := newFanoutDeps(t)
	s1, _, _, _ := seedRefPair(t, db)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/api/v1/admin/servers/:id", d.AdminUpdateServer)

	put := func(payload map[string]any) {
		t.Helper()
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/admin/servers/%d", s1.ID), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("更新服务器失败: code=%d body=%s", w.Code, w.Body.String())
		}
	}

	// 纯展示字段不触发重推
	put(map[string]any{"location": "Tokyo"})
	if p, err := d.Config.GetPending(s1.ID); err != nil || p != nil {
		t.Fatalf("与生成无关的字段不应产生待推配置: err=%v pending=%v", err, p)
	}

	// 解析策略真正变化 → 本机重推，且生成内容反映新值
	put(map[string]any{"routing_domain_strategy": "IPIfNonMatch"})
	p, err := d.Config.GetPending(s1.ID)
	if err != nil || p == nil {
		t.Fatalf("改解析策略后本机应有待推配置: err=%v pending=%v", err, p)
	}
	if !strings.Contains(p.ConfigJSON, `"domainStrategy": "IPIfNonMatch"`) {
		t.Errorf("待推配置应反映新的 routing domainStrategy")
	}
}
