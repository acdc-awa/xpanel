package api

// C2（2026-09-28）四层直通派生对象落库的行为测试：幂等、草稿/孤儿清理、改名级联、
// 不覆盖管理员编辑、managed 行的保护与删除级联、升级路径（老库只有入站行也能补齐）。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

func newTunnelDeps(t *testing.T) (*Deps, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := apiTestDB(t)
	if err := db.Create(&models.Server{ID: 1, Name: "edge", Host: "edge.example.com", NodeID: "n1", Secret: "sec"}).Error; err != nil {
		t.Fatal(err)
	}
	d := &Deps{DB: db}
	r := gin.New()
	r.GET("/api/v1/admin/servers/:id/outbounds", d.AdminGetServerOutbounds)
	r.PUT("/api/v1/admin/servers/:id/outbounds/:outbound_id", d.AdminUpdateServerOutbound)
	r.DELETE("/api/v1/admin/servers/:id/outbounds/:outbound_id", d.AdminDeleteServerOutbound)
	r.GET("/api/v1/admin/servers/:id/routing-rules", d.AdminGetServerRoutingRules)
	r.PUT("/api/v1/admin/servers/:id/routing-rules/:rule_id", d.AdminUpdateServerRoutingRule)
	r.DELETE("/api/v1/admin/servers/:id/routing-rules/:rule_id", d.AdminDeleteServerRoutingRule)
	r.PUT("/api/v1/admin/inbounds/:id", d.AdminUpdateInbound)
	r.DELETE("/api/v1/admin/inbounds/:id", d.AdminDeleteInbound)
	return d, r
}

func seedTunnelInbound(t *testing.T, db *gorm.DB, inb *models.Inbound) {
	t.Helper()
	if err := db.Create(inb).Error; err != nil {
		t.Fatal(err)
	}
}

func ownedOutbound(t *testing.T, db *gorm.DB, ownerID uint64) *models.ServerOutbound {
	t.Helper()
	var ob models.ServerOutbound
	if err := db.Where("server_id = ? AND owner_inbound_id = ?", 1, ownerID).First(&ob).Error; err != nil {
		return nil
	}
	return &ob
}

func ownedRule(t *testing.T, db *gorm.DB, ownerID uint64) *models.ServerRoutingRule {
	t.Helper()
	var rl models.ServerRoutingRule
	if err := db.Where("server_id = ? AND owner_inbound_id = ?", 1, ownerID).First(&rl).Error; err != nil {
		return nil
	}
	return &rl
}

// 升级路径 + 幂等：老库只有直通入站行（没有任何派生行），对账必须补齐，且重复跑无副作用。
func TestEnsureTunnelDerived_BackfillAndIdempotent(t *testing.T) {
	d, _ := newTunnelDeps(t)
	inb := models.Inbound{
		ServerID: 1, Tag: "tunnel-legacy", Protocol: models.ProtocolTunnel, Port: 20001,
		Type: models.InboundTypeTunnel, TargetAddress: "target.example.com", TargetPort: 3306,
		StreamSettings: `{"proxy_protocol":true}`, Enabled: true,
	}
	seedTunnelInbound(t, d.DB, &inb)

	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatalf("首次对账失败: %v", err)
	}
	ob := ownedOutbound(t, d.DB, inb.ID)
	if ob == nil {
		t.Fatal("未补齐派生出口")
	}
	if ob.Tag != "out-tunnel-legacy" || ob.Protocol != "freedom" || !ob.Enabled {
		t.Fatalf("派生出口形态不对: %+v", ob)
	}
	if ob.Priority != models.TunnelOutboundPriority {
		t.Errorf("派生出口优先级 = %d, want %d", ob.Priority, models.TunnelOutboundPriority)
	}
	var st map[string]any
	if err := json.Unmarshal([]byte(ob.SettingsJSON), &st); err != nil {
		t.Fatal(err)
	}
	if st["redirect"] != "target.example.com:3306" || st["proxyProtocol"] != float64(2) {
		t.Errorf("派生出口 settings 不对: %v", st)
	}
	rl := ownedRule(t, d.DB, inb.ID)
	if rl == nil {
		t.Fatal("未补齐派生规则")
	}
	if rl.InboundTag != "tunnel-legacy" || rl.OutboundTag != "out-tunnel-legacy" || rl.Priority != models.TunnelRulePriority {
		t.Fatalf("派生规则形态不对: %+v", rl)
	}

	// 幂等：再跑两次，行数与内容都不变（只同步必要的字段，不重复建行）
	for i := 0; i < 2; i++ {
		if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
			t.Fatalf("第 %d 次对账失败: %v", i+2, err)
		}
	}
	var obCnt, ruleCnt int64
	d.DB.Model(&models.ServerOutbound{}).Where("owner_inbound_id = ?", inb.ID).Count(&obCnt)
	d.DB.Model(&models.ServerRoutingRule{}).Where("owner_inbound_id = ?", inb.ID).Count(&ruleCnt)
	if obCnt != 1 || ruleCnt != 1 {
		t.Fatalf("幂等性破坏：出站 %d 行、规则 %d 行（应各 1）", obCnt, ruleCnt)
	}
}

// 草稿（无目标）不产生派生行；从有目标改成草稿时，原有派生行要被清掉。
func TestEnsureTunnelDerived_DraftCleansUp(t *testing.T) {
	d, _ := newTunnelDeps(t)
	inb := models.Inbound{
		ServerID: 1, Tag: "tunnel-draft", Protocol: models.ProtocolTunnel, Port: 20002,
		Type: models.InboundTypeTunnel, TargetAddress: "1.2.3.4", TargetPort: 443, Enabled: true,
	}
	seedTunnelInbound(t, d.DB, &inb)
	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatal(err)
	}
	if ownedOutbound(t, d.DB, inb.ID) == nil {
		t.Fatal("有目标时应产生派生行")
	}

	// 清掉目标（草稿态）
	if err := d.DB.Model(&models.Inbound{}).Where("id = ?", inb.ID).
		Updates(map[string]any{"target_address": "", "target_port": 0}).Error; err != nil {
		t.Fatal(err)
	}
	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatal(err)
	}
	if ob := ownedOutbound(t, d.DB, inb.ID); ob != nil {
		t.Errorf("草稿态不该留下派生出口: %+v", ob)
	}
	if rl := ownedRule(t, d.DB, inb.ID); rl != nil {
		t.Errorf("草稿态不该留下派生规则: %+v", rl)
	}
}

// 改名级联：直通 tag 改了，派生出口 tag 与规则的 inbound/outbound tag 都要跟上（不能悬空）。
func TestEnsureTunnelDerived_TagRenameCascade(t *testing.T) {
	d, _ := newTunnelDeps(t)
	inb := models.Inbound{
		ServerID: 1, Tag: "tunnel-old", Protocol: models.ProtocolTunnel, Port: 20003,
		Type: models.InboundTypeTunnel, TargetAddress: "1.2.3.4", TargetPort: 443, Enabled: true,
	}
	seedTunnelInbound(t, d.DB, &inb)
	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatal(err)
	}

	if err := d.DB.Model(&models.Inbound{}).Where("id = ?", inb.ID).Update("tag", "tunnel-new").Error; err != nil {
		t.Fatal(err)
	}
	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatal(err)
	}

	ob := ownedOutbound(t, d.DB, inb.ID)
	if ob == nil || ob.Tag != "out-tunnel-new" {
		t.Fatalf("派生出口 tag 未跟随改名: %+v", ob)
	}
	rl := ownedRule(t, d.DB, inb.ID)
	if rl == nil || rl.InboundTag != "tunnel-new" || rl.OutboundTag != "out-tunnel-new" {
		t.Fatalf("派生规则未跟随改名: %+v", rl)
	}
}

// 不覆盖管理员编辑：管理员给派生规则加了来源限制、改了优先级、把指向改到 blocked，
// 对账只允许改 inbound_tag/enabled/（悬空时的）outbound_tag，其余一律不动。
func TestEnsureTunnelDerived_KeepsAdminEdits(t *testing.T) {
	d, _ := newTunnelDeps(t)
	inb := models.Inbound{
		ServerID: 1, Tag: "tunnel-edit", Protocol: models.ProtocolTunnel, Port: 20004,
		Type: models.InboundTypeTunnel, TargetAddress: "1.2.3.4", TargetPort: 443, Enabled: true,
	}
	seedTunnelInbound(t, d.DB, &inb)
	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatal(err)
	}
	rl := ownedRule(t, d.DB, inb.ID)
	if err := d.DB.Model(&models.ServerRoutingRule{}).Where("id = ?", rl.ID).Updates(map[string]any{
		"ip":           "203.0.113.0/24",
		"priority":     -200,
		"outbound_tag": "blocked",
		"remark":       "管理员备注",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatal(err)
	}
	got := ownedRule(t, d.DB, inb.ID)
	if got.IP != "203.0.113.0/24" {
		t.Errorf("匹配条件被覆盖: ip = %q", got.IP)
	}
	if got.Priority != -200 {
		t.Errorf("优先级被覆盖: %d（管理员把规则排到直通之前的能力必须保留）", got.Priority)
	}
	if got.OutboundTag != "blocked" {
		t.Errorf("管理员改的指向被覆盖: %q（指向存活出站时应尊重）", got.OutboundTag)
	}
	if got.Remark != "管理员备注" {
		t.Errorf("备注被覆盖: %q", got.Remark)
	}
}

// managed 保护：派生出口不可改不可删；派生规则可编辑但不可删；规则改指向/加来源后仍可保存。
func TestManagedTunnelObjects_Guards(t *testing.T) {
	d, r := newTunnelDeps(t)
	inb := models.Inbound{
		ServerID: 1, Tag: "tunnel-guard", Protocol: models.ProtocolTunnel, Port: 20005,
		Type: models.InboundTypeTunnel, TargetAddress: "1.2.3.4", TargetPort: 443, Enabled: true,
	}
	seedTunnelInbound(t, d.DB, &inb)
	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatal(err)
	}
	ob := ownedOutbound(t, d.DB, inb.ID)
	rl := ownedRule(t, d.DB, inb.ID)

	// 1. 派生出口：改 → 400；删 → 400
	body, _ := json.Marshal(map[string]any{"settings_json": `{"redirect":"6.6.6.6:80"}`})
	req := httptest.NewRequest("PUT", "/api/v1/admin/servers/1/outbounds/"+u64s(ob.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("派生出口可被手改: %d %s", w.Code, w.Body.String())
	}
	reqDel := httptest.NewRequest("DELETE", "/api/v1/admin/servers/1/outbounds/"+u64s(ob.ID), nil)
	wDel := httptest.NewRecorder()
	r.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusBadRequest {
		t.Errorf("派生出口可被删除: %d %s", wDel.Code, wDel.Body.String())
	}

	// 2. 派生规则：删 → 400
	reqRuleDel := httptest.NewRequest("DELETE", "/api/v1/admin/servers/1/routing-rules/"+u64s(rl.ID), nil)
	wRuleDel := httptest.NewRecorder()
	r.ServeHTTP(wRuleDel, reqRuleDel)
	if wRuleDel.Code != http.StatusBadRequest {
		t.Errorf("派生规则可被删除: %d %s", wRuleDel.Code, wRuleDel.Body.String())
	}

	// 3. 派生规则：编辑（加来源限制 + 调优先级）→ 200，且落库生效
	editBody, _ := json.Marshal(map[string]any{
		"outbound_tag": "out-tunnel-guard",
		"inbound_tag":  "tunnel-guard",
		"ip":           "198.51.100.0/24",
		"priority":     -300,
		"enabled":      true,
	})
	reqEdit := httptest.NewRequest("PUT", "/api/v1/admin/servers/1/routing-rules/"+u64s(rl.ID), bytes.NewReader(editBody))
	reqEdit.Header.Set("Content-Type", "application/json")
	wEdit := httptest.NewRecorder()
	r.ServeHTTP(wEdit, reqEdit)
	if wEdit.Code != http.StatusOK {
		t.Fatalf("派生规则应可编辑（这是给直通做来源限制的唯一途径）: %d %s", wEdit.Code, wEdit.Body.String())
	}
	after := ownedRule(t, d.DB, inb.ID)
	if after.IP != "198.51.100.0/24" || after.Priority != -300 {
		t.Errorf("编辑未落库: %+v", after)
	}
	// 编辑后再对账：管理员的编辑仍然保留
	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatal(err)
	}
	after2 := ownedRule(t, d.DB, inb.ID)
	if after2.IP != "198.51.100.0/24" || after2.Priority != -300 {
		t.Errorf("对账覆盖了管理员的编辑: %+v", after2)
	}
}

// 删除级联：删直通入站不再被自己的派生规则挡住，且派生行随之消失。
func TestDeleteTunnelInbound_CascadesDerived(t *testing.T) {
	d, r := newTunnelDeps(t)
	inb := models.Inbound{
		ServerID: 1, Tag: "tunnel-del", Protocol: models.ProtocolTunnel, Port: 20006,
		Type: models.InboundTypeTunnel, TargetAddress: "1.2.3.4", TargetPort: 443, Enabled: true,
	}
	seedTunnelInbound(t, d.DB, &inb)
	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatal(err)
	}
	if ownedRule(t, d.DB, inb.ID) == nil {
		t.Fatal("前置条件：应有派生规则")
	}

	req := httptest.NewRequest("DELETE", "/api/v1/admin/inbounds/"+u64s(inb.ID), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("直通入站删除被挡住（派生规则不该算管理员引用）: %d %s", w.Code, w.Body.String())
	}
	var obCnt, ruleCnt int64
	d.DB.Model(&models.ServerOutbound{}).Where("owner_inbound_id = ?", inb.ID).Count(&obCnt)
	d.DB.Model(&models.ServerRoutingRule{}).Where("owner_inbound_id = ?", inb.ID).Count(&ruleCnt)
	if obCnt != 0 || ruleCnt != 0 {
		t.Errorf("删除直通后留下孤儿派生行: 出站 %d、规则 %d", obCnt, ruleCnt)
	}
}

// 管理员自己建的规则引用了直通 tag 时，删除直通仍应被拦住（保持既有语义）。
func TestDeleteTunnelInbound_AdminRuleStillBlocks(t *testing.T) {
	d, r := newTunnelDeps(t)
	inb := models.Inbound{
		ServerID: 1, Tag: "tunnel-keep", Protocol: models.ProtocolTunnel, Port: 20007,
		Type: models.InboundTypeTunnel, TargetAddress: "1.2.3.4", TargetPort: 443, Enabled: true,
	}
	seedTunnelInbound(t, d.DB, &inb)
	if err := d.DB.Create(&models.ServerRoutingRule{
		ServerID: 1, OutboundTag: "blocked", InboundTag: "tunnel-keep", Enabled: true, Priority: 5,
	}).Error; err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/api/v1/admin/inbounds/"+u64s(inb.ID), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("管理员规则引用该 tag 时必须拦删除: %d %s", w.Code, w.Body.String())
	}
}

// 派生出口 tag 撞名（管理员先手建了 out-<tag>）时报错，不静默抢占管理员的出站。
func TestEnsureTunnelDerived_TagConflict(t *testing.T) {
	d, _ := newTunnelDeps(t)
	if err := d.DB.Create(&models.ServerOutbound{
		ServerID: 1, Tag: "out-tunnel-clash", Protocol: "freedom", SettingsJSON: `{}`, Enabled: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	inb := models.Inbound{
		ServerID: 1, Tag: "tunnel-clash", Protocol: models.ProtocolTunnel, Port: 20008,
		Type: models.InboundTypeTunnel, TargetAddress: "1.2.3.4", TargetPort: 443, Enabled: true,
	}
	seedTunnelInbound(t, d.DB, &inb)

	err := services.EnsureTunnelDerivedObjects(d.DB, 1)
	if err == nil {
		t.Fatal("撞名时必须报错，而不是静默占用管理员的出站")
	}
	var ob models.ServerOutbound
	d.DB.Where("tag = ?", "out-tunnel-clash").First(&ob)
	if ob.OwnerInboundID != 0 || ob.SettingsJSON != `{}` {
		t.Errorf("管理员的出站被改写了: %+v", ob)
	}
}

// 通道直通（type=channel + dokodemo-door）同样纳入维护：宿主入站改名（改端口）后派生行跟随。
func TestEnsureTunnelDerived_ChannelHosted(t *testing.T) {
	d, _ := newTunnelDeps(t)
	inb := models.Inbound{
		ServerID: 1, Tag: "chan-tunnel-3306", Protocol: models.ProtocolTunnel, Port: 3306,
		Type: models.InboundTypeChannel, TargetAddress: "db.remote.com", TargetPort: 3306, Enabled: true,
	}
	seedTunnelInbound(t, d.DB, &inb)
	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatal(err)
	}
	if ob := ownedOutbound(t, d.DB, inb.ID); ob == nil || ob.Tag != "out-chan-tunnel-3306" {
		t.Fatalf("通道直通未纳入维护: %+v", ob)
	}
}

// 非直通入站不受影响（普通 user 入站不该产生任何派生行）。
func TestEnsureTunnelDerived_IgnoresUserInbound(t *testing.T) {
	d, _ := newTunnelDeps(t)
	if err := d.DB.Create(&models.Inbound{
		ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443, Type: models.InboundTypeUser, Enabled: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatal(err)
	}
	var cnt int64
	d.DB.Model(&models.ServerOutbound{}).Where("owner_inbound_id > 0").Count(&cnt)
	if cnt != 0 {
		t.Errorf("普通入站产生了 %d 行派生对象", cnt)
	}
}

func u64s(v uint64) string {
	return json.Number(jsonInt(v)).String()
}

func jsonInt(v uint64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// 阶段一防御：四层直通派生路由规则锁定保护（禁止修改 InboundTag 与 Enabled）。
func TestAdminUpdateServerRoutingRule_DerivedRuleProtection(t *testing.T) {
	d, r := newTunnelDeps(t)
	inb := models.Inbound{
		ServerID: 1, Tag: "tun-protect", Protocol: models.ProtocolTunnel, Port: 20000,
		Type: models.InboundTypeTunnel, TargetAddress: "remote.host", TargetPort: 443, Enabled: true,
	}
	seedTunnelInbound(t, d.DB, &inb)
	if err := services.EnsureTunnelDerivedObjects(d.DB, 1); err != nil {
		t.Fatal(err)
	}

	rl := ownedRule(t, d.DB, inb.ID)
	if rl == nil {
		t.Fatal("未找到直通派生规则")
	}

	// 1. 尝试修改 InboundTag -> 必须拒绝 (400)
	bodyChangeTag, _ := json.Marshal(map[string]any{"inbound_tag": "other-inbound"})
	req1 := httptest.NewRequest("PUT", "/api/v1/admin/servers/1/routing-rules/"+u64s(rl.ID), bytes.NewReader(bodyChangeTag))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusBadRequest {
		t.Fatalf("修改直通派生规则 InboundTag 应返回 400，实际: %d, body: %s", w1.Code, w1.Body.String())
	}

	// 2. 尝试修改 Enabled -> 必须拒绝 (400)
	bodyChangeEnabled, _ := json.Marshal(map[string]any{"enabled": false})
	req2 := httptest.NewRequest("PUT", "/api/v1/admin/servers/1/routing-rules/"+u64s(rl.ID), bytes.NewReader(bodyChangeEnabled))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("修改直通派生规则 Enabled 应返回 400，实际: %d, body: %s", w2.Code, w2.Body.String())
	}

	// 3. 允许修改优先级与备注
	bodyChangePriority, _ := json.Marshal(map[string]any{"priority": -150, "remark": "调整优先级"})
	req3 := httptest.NewRequest("PUT", "/api/v1/admin/servers/1/routing-rules/"+u64s(rl.ID), bytes.NewReader(bodyChangePriority))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("修改直通派生规则 Priority 应成功 (200)，实际: %d, body: %s", w3.Code, w3.Body.String())
	}
}

