package xray_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
)

func asObject(t *testing.T, v any, what string) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%s: expected JSON object, got %T (%v)", what, v, v)
	}
	return m
}

func asArray(t *testing.T, v any, what string) []any {
	t.Helper()
	a, ok := v.([]any)
	if !ok {
		t.Fatalf("%s: expected JSON array, got %T (%v)", what, v, v)
	}
	return a
}

// vlessUsers 构造按入站 tag 分组的用户 fixture（批7：Generate 改为消费
// map[string][]protocol.User——与 GetValidUsers 同构；有效性/权限过滤在服务层完成）。
func vlessUsers(tags ...string) map[string][]protocol.User {
	m := make(map[string][]protocol.User, len(tags))
	u := []protocol.User{{UUID: "11111111-1111-1111-1111-111111111111", Email: "user-1@panel.local"}}
	for _, t := range tags {
		m[t] = u
	}
	return m
}

// inbStream 快捷构造一个 StreamSettings JSON。
func inbStream(network, security, extra string) string {
	s := `"network":"` + network + `","security":"` + security + `"`
	if extra != "" {
		s += "," + extra
	}
	return `{` + s + `}`
}

func TestGenerateConfigWithOutboundsAndRouting(t *testing.T) {

	inbounds := []models.Inbound{
		{
			ID: 1, ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443,
			StreamSettings: `{"network":"ws","security":"none","wsSettings":{"path":"/ws"}}`,
			Enabled:        true,
		},
	}

	users := vlessUsers("vless-in")

	outbounds := []models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "warp", Protocol: "socks", SettingsJSON: `{"servers":[{"address":"127.0.0.1","port":40000}]}`, Enabled: true},
	}

	routingRules := []models.ServerRoutingRule{
		{ID: 1, ServerID: 1, OutboundTag: "warp", Domain: "geosite:netflix, geosite:google", IP: "1.1.1.1/32, 8.8.8.8/32", Enabled: true},
		{ID: 2, ServerID: 1, OutboundTag: "blocked", RuleJSON: `{"type":"field","domain":["geosite:category-ads-all"],"outboundTag":"blocked"}`, Enabled: true},
	}

	rawCfg, err := xray.Generate(inbounds, outbounds, routingRules, users, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal config JSON failed: %v", err)
	}

	obs, _ := parsed["outbounds"].([]any)
	if len(obs) == 0 {
		t.Fatal("expected outbounds array")
	}
	hasWarp := false
	hasFreedom := false
	for _, o := range obs {
		om, _ := o.(map[string]any)
		if om["tag"] == "warp" {
			hasWarp = true
		}
		if om["tag"] == "direct" && om["protocol"] == "freedom" {
			hasFreedom = true
		}
	}
	if !hasWarp {
		t.Error("warp outbound not found")
	}
	if !hasFreedom {
		t.Error("fallback freedom outbound not found")
	}
}

func TestGenerateConfig_VLESS_XHTTP_REALITY(t *testing.T) {
	inbounds := []models.Inbound{
		{
			ID: 101, ServerID: 1, Tag: "vless-xhttp-reality-in", Protocol: "vless", Port: 443,
			StreamSettings: `{"network":"xhttp","security":"reality","xhttpSettings":{"mode":"auto","path":"/xhttp-stream","host":"xhttp.example.com"},"realitySettings":{"serverNames":["example.com"],"publicKey":"pk123","privateKey":"sk456","shortIds":["12345678"],"dest":"1.1.1.1:443"}}`,
			Enabled:        true,
		},
	}

	rawCfg, err := xray.Generate(inbounds, nil, nil, vlessUsers("vless-xhttp-reality-in"), nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	inboundList := asArray(t, parsed["inbounds"], "inbounds")
	vlessIn := asObject(t, inboundList[0], "inbounds[0]")
	if vlessIn["tag"] != "vless-xhttp-reality-in" {
		t.Errorf("tag mismatch: %v", vlessIn["tag"])
	}

	stream := asObject(t, vlessIn["streamSettings"], "streamSettings")
	if stream["network"] != "xhttp" || stream["security"] != "reality" {
		t.Errorf("streamSettings mismatch: %v", stream)
	}
	xhttpSettings := asObject(t, stream["xhttpSettings"], "xhttpSettings")
	if xhttpSettings["path"] != "/xhttp-stream" {
		t.Errorf("xhttpSettings.path mismatch: %v", xhttpSettings)
	}
	realitySettings := asObject(t, stream["realitySettings"], "realitySettings")
	if realitySettings["dest"] != "1.1.1.1:443" {
		t.Errorf("realitySettings mismatch: %v", realitySettings)
	}
}

func TestGenerateConfig_VLESS_XHTTP_TLS(t *testing.T) {
	inbounds := []models.Inbound{
		{
			ID: 102, ServerID: 1, Tag: "vless-xhttp-tls-in", Protocol: "vless", Port: 8443,
			StreamSettings: `{"network":"xhttp","security":"tls","xhttpSettings":{"mode":"stream-up","path":"/xp"},"tlsSettings":{"serverName":"mydomain.com","certificates":[{"certificateFile":"/etc/cert.pem","keyFile":"/etc/key.pem"}]}}`,
			Enabled:        true,
		},
	}

	rawCfg, err := xray.Generate(inbounds, nil, nil, vlessUsers("vless-xhttp-tls-in"), nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	inboundList := asArray(t, parsed["inbounds"], "inbounds")
	vlessIn := asObject(t, inboundList[0], "inbounds[0]")
	stream := asObject(t, vlessIn["streamSettings"], "streamSettings")
	if stream["network"] != "xhttp" || stream["security"] != "tls" {
		t.Errorf("streamSettings mismatch: %v", stream)
	}
	tlsSettings := asObject(t, stream["tlsSettings"], "tlsSettings")
	if tlsSettings["serverName"] != "mydomain.com" {
		t.Errorf("tlsSettings mismatch: %v", tlsSettings)
	}
}

func TestGenerateConfig_ComplexOutbounds(t *testing.T) {

	inbounds := []models.Inbound{
		{ID: 1, Tag: "vless-in", Protocol: "vless", Port: 443, Enabled: true},
	}
	users := vlessUsers("vless-in")

	outbounds := []models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "direct", Protocol: "freedom", SettingsJSON: `{"domainStrategy":"UseIP"}`, StreamSettingsJSON: `{"sockopt":{"mark":255}}`, Enabled: true},
		{ID: 2, ServerID: 1, Tag: "blocked", Protocol: "blackhole", SettingsJSON: `{"response":{"type":"http"}}`, Enabled: true},
		{ID: 3, ServerID: 1, Tag: "outbound-vless-xhttp", Protocol: "vless", SettingsJSON: `{"vnext":[{"address":"remote.proxy.com","port":443,"users":[{"id":"uuid","encryption":"none"}]}]}`, StreamSettingsJSON: `{"network":"xhttp","security":"tls","xhttpSettings":{"mode":"auto","path":"/out-xhttp"}}`, SendThrough: "192.168.1.100", Enabled: true},
	}

	rawCfg, err := xray.Generate(inbounds, outbounds, nil, users, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	obs := asArray(t, parsed["outbounds"], "outbounds")
	if len(obs) != 4 {
		t.Fatalf("expected 4 outbounds (direct+blocked+api from template + vless-xhttp from DB), got %d", len(obs))
	}
	obDirect := asObject(t, obs[0], "obs[0]")
	if obDirect["tag"] != "direct" {
		t.Errorf("outbound 0 mismatch: %v", obDirect)
	}
	obProxy := asObject(t, obs[3], "obs[3]")
	if obProxy["sendThrough"] != "192.168.1.100" {
		t.Errorf("sendThrough mismatch: %v", obProxy)
	}
}

func TestGenerateConfig_VLESSOutbound_Normalize(t *testing.T) {
	// 01 号文档 §4 第 4 项 + 附注：
	// - 裸 {vnext:[...]} settings 只进 settings，不得顶层 + settings 双写
	// - vless 出站 users 缺 encryption 时兜底注入 "none"；已有值不覆盖
	inbounds := []models.Inbound{
		{ID: 1, Tag: "vless-in", Protocol: "vless", Port: 443, Enabled: true},
	}
	users := vlessUsers("vless-in")

	outbounds := []models.ServerOutbound{
		{
			ID: 1, ServerID: 1, Tag: "proxy-out", Protocol: "vless", Enabled: true,
			SettingsJSON: `{"vnext":[{"address":"remote.proxy.com","port":443,"users":[{"id":"uuid-1"},{"id":"uuid-2","encryption":"none"}]}]}`,
		},
	}

	rawCfg, err := xray.Generate(inbounds, outbounds, nil, users, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	obs := asArray(t, parsed["outbounds"], "outbounds")
	var ob map[string]any
	for _, o := range obs {
		if om, _ := o.(map[string]any); om["tag"] == "proxy-out" {
			ob = om
		}
	}
	if ob == nil {
		t.Fatal("proxy-out outbound not found")
	}
	if _, has := ob["vnext"]; has {
		t.Error("bare vnext leaked to outbound top level (double-write, 附注)")
	}
	settings := asObject(t, ob["settings"], "settings")
	vnext := asArray(t, settings["vnext"], "settings.vnext")
	users0 := asObject(t, asArray(t, asObject(t, vnext[0], "vnext[0]")["users"], "vnext[0].users")[0], "users[0]")
	if users0["encryption"] != "none" {
		t.Errorf("users[0].encryption = %v, want none (兜底注入)", users0["encryption"])
	}
	users1 := asObject(t, asArray(t, asObject(t, vnext[0], "vnext[0]")["users"], "vnext[0].users")[1], "users[1]")
	if users1["encryption"] != "none" {
		t.Errorf("users[1].encryption = %v, want none (原值保留)", users1["encryption"])
	}
}

func TestValidateRealityStream(t *testing.T) {
	// 01 号文档 §2.2/§4 第 6 项：x25519 密钥须 base64 RawURL 解码 32 字节，非法直接报错
	valid := "uOF-_qWw55cTMdM8CbaDieJg6HiKUV2g7BOj1GIvf04" // xray x25519 输出格式
	cases := []struct {
		name      string
		stream    string
		expectErr bool
	}{
		{name: "empty", stream: "", expectErr: false},
		{name: "no reality", stream: `{"network":"tcp","security":"tls"}`, expectErr: false},
		{name: "valid inbound privateKey", stream: `{"network":"tcp","security":"reality","realitySettings":{"serverNames":["e.com"],"privateKey":"` + valid + `","shortIds":["abcd"]}}`, expectErr: false},
		{name: "valid outbound password", stream: `{"network":"tcp","security":"reality","realitySettings":{"serverName":"e.com","password":"` + valid + `","shortId":"abcd"}}`, expectErr: false},
		{name: "valid legacy publicKey", stream: `{"network":"tcp","security":"reality","realitySettings":{"serverName":"e.com","publicKey":"` + valid + `"}}`, expectErr: false},
		{name: "short privateKey", stream: `{"security":"reality","realitySettings":{"privateKey":"pk123"}}`, expectErr: true},
		{name: "bad base64 publicKey", stream: `{"security":"reality","realitySettings":{"publicKey":"not-a-base64!!!"}}`, expectErr: true},
		{name: "bad json", stream: `{bad`, expectErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := xray.ValidateRealityStream(tc.stream)
			if tc.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateOutbound(t *testing.T) {
	if err := xray.ValidateOutbound(`{"vnext":[]}`, `{"network":"tcp"}`); err != nil {
		t.Errorf("valid outbound rejected: %v", err)
	}
	if err := xray.ValidateOutbound(`{bad`, ``); err == nil {
		t.Error("expected error for bad settings json")
	}
	if err := xray.ValidateOutbound(`{}`, `{"security":"reality","realitySettings":{"password":"short"}}`); err == nil {
		t.Error("expected error for invalid reality password")
	}
}

func TestGenerateConfig_RelayInbound(t *testing.T) {
	// Phase T：relay 入站 clients 固定为 InternalUUID，tcp+reality 自动 vision
	inb := models.Inbound{
		ID: 1, ServerID: 1, Tag: "in-relay", Protocol: "vless", Port: 8443,
		Type: models.InboundTypeRelay, InternalUUID: "22222222-2222-2222-2222-222222222222",
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"dest":"1.2.3.4:443","serverNames":["r.example.com"],"privateKey":"sk","shortIds":["abcd"],"publicKey":"pk"}}`,
		Enabled:        true,
	}
	raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, nil, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	json.Unmarshal(raw, &parsed)
	inbounds := asArray(t, parsed["inbounds"], "inbounds")
	relayIn := asObject(t, inbounds[0], "inbounds[0]")
	settings := asObject(t, relayIn["settings"], "settings")
	clients := asArray(t, settings["clients"], "clients")
	if len(clients) != 1 {
		t.Fatalf("relay clients = %d, want 1", len(clients))
	}
	c0 := asObject(t, clients[0], "clients[0]")
	if c0["id"] != inb.InternalUUID {
		t.Errorf("client id = %v", c0["id"])
	}
	if c0["flow"] != "xtls-rprx-vision" {
		t.Errorf("tcp+reality relay 应自动 vision: %v", c0["flow"])
	}
	if c0["email"] != "relay-in-relay@panel.local" {
		t.Errorf("relay email = %v", c0["email"])
	}
}

func TestGenerateConfig_RelayMissingUUID(t *testing.T) {
	inb := models.Inbound{
		ID: 1, ServerID: 1, Tag: "in-relay", Protocol: "vless", Port: 8443,
		Type:    models.InboundTypeRelay, // InternalUUID 空
		Enabled: true,
	}
	_, err := xray.Generate([]models.Inbound{inb}, nil, nil, nil, nil, "", "")
	if err == nil {
		t.Error("relay 入站缺 InternalUUID 应报错（等 setup）")
	}
}

func TestGenerateConfig_DisabledInboundSkipped(t *testing.T) {
	inbounds := []models.Inbound{
		{ID: 1, Tag: "in-disabled", Protocol: "vless", Port: 8443, Type: models.InboundTypeUser, Enabled: false},
		{ID: 2, Tag: "in-user", Protocol: "vless", Port: 443, Type: models.InboundTypeUser, Enabled: true},
	}
	raw, err := xray.Generate(inbounds, nil, nil, vlessUsers("in"), nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	json.Unmarshal(raw, &parsed)
	inboundList := asArray(t, parsed["inbounds"], "inbounds")
	for _, in := range inboundList {
		im := asObject(t, in, "inbound")
		if im["tag"] == "in-disabled" {
			t.Error("禁用入站不应生成")
		}
	}
	if len(inboundList) != 2 { // in-user + api
		t.Errorf("inbounds = %d, want 2 (user + api)", len(inboundList))
	}
}

func TestGenerateConfig_InboundRefOutbound(t *testing.T) {
	// 中转出站：InboundRef 引用落地入站 → vnext/realitySettings 自动构造
	target := models.Inbound{
		ID: 99, ServerID: 2, Tag: "landing", Protocol: "vless", Port: 443,
		Type: models.InboundTypeRelay, InternalUUID: "33333333-3333-3333-3333-333333333333",
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"dest":"1.2.3.4:443","serverNames":["land.example.com"],"privateKey":"sk","shortIds":["abcd"],"publicKey":"pk-123"}}`,
		Enabled:        true,
	}
	ref := target.ID
	ctx := &xray.GenerateContext{
		RefTargets: map[uint64]xray.RefTarget{
			target.ID: {Inbound: target, ServerHost: "10.0.0.5"},
		},
	}
	outbounds := []models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "to-landing", Protocol: "vless", InboundRef: &ref, Enabled: true,
			SettingsJSON: `{"vnext":[{"address":"手填应被忽略","port":1,"users":[{"id":"x"}]}]}`},
	}
	raw, err := xray.Generate([]models.Inbound{{ID: 1, Tag: "in", Protocol: "vless", Port: 443, Type: models.InboundTypeUser, Enabled: true}},
		outbounds, nil, vlessUsers("in"), ctx, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	json.Unmarshal(raw, &parsed)
	obs := asArray(t, parsed["outbounds"], "outbounds")
	var ob map[string]any
	for _, o := range obs {
		if om, _ := o.(map[string]any); om["tag"] == "to-landing" {
			ob = om
		}
	}
	if ob == nil {
		t.Fatal("to-landing outbound not found")
	}
	if ob["protocol"] != "vless" {
		t.Errorf("protocol = %v", ob["protocol"])
	}
	settings := asObject(t, ob["settings"], "settings")
	vnext := asArray(t, settings["vnext"], "vnext")
	vn0 := asObject(t, vnext[0], "vnext[0]")
	if vn0["address"] != "10.0.0.5" || vn0["port"] != float64(443) {
		t.Errorf("vnext address/port 自动构造失败: %v", vn0)
	}
	users := asArray(t, vn0["users"], "users")
	u0 := asObject(t, users[0], "users[0]")
	if u0["id"] != target.InternalUUID || u0["encryption"] != "none" || u0["flow"] != "xtls-rprx-vision" {
		t.Errorf("vnext users 自动构造失败: %v", u0)
	}
	stream := asObject(t, ob["streamSettings"], "streamSettings")
	reality := asObject(t, stream["realitySettings"], "realitySettings")
	if reality["serverName"] != "land.example.com" || reality["password"] != "pk-123" || reality["shortId"] != "abcd" {
		t.Errorf("realitySettings 自动派生失败: %v", reality)
	}
}

func TestGenerateConfig_InboundRefUnsetupFails(t *testing.T) {
	// 引用落地入站未 setup（InternalUUID 空）→ 预检报错
	target := models.Inbound{
		ID: 99, ServerID: 2, Tag: "landing", Protocol: "vless", Port: 443,
		Type: models.InboundTypeRelay, // InternalUUID 空
	}
	ref := target.ID
	ctx := &xray.GenerateContext{
		RefTargets: map[uint64]xray.RefTarget{target.ID: {Inbound: target, ServerHost: "10.0.0.5"}},
	}
	outbounds := []models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "to-landing", Protocol: "vless", InboundRef: &ref, Enabled: true},
	}
	_, err := xray.Generate([]models.Inbound{{ID: 1, Tag: "in", Protocol: "vless", Port: 443, Type: models.InboundTypeUser, Enabled: true}},
		outbounds, nil, vlessUsers("in-tls"), ctx, "", "")
	if err == nil {
		t.Error("引用未 setup 的落地入站应报错")
	}
}

func TestGenerateConfig_CertPathInjection(t *testing.T) {
	certID := uint64(7)
	inb := models.Inbound{
		ID: 1, ServerID: 1, Tag: "in-tls", Protocol: "vless", Port: 443,
		Type: models.InboundTypeUser, CertID: &certID,
		StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"t.example.com","certificates":[{"certificateFile":"/旧路径.pem","keyFile":"/旧key.pem"}]}}`,
		Enabled:        true,
	}
	ctx := &xray.GenerateContext{CertDomains: map[uint64]string{certID: "t.example.com"}}
	raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessUsers("in-tls"), ctx, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	json.Unmarshal(raw, &parsed)
	inboundList := asArray(t, parsed["inbounds"], "inbounds")
	tlsIn := asObject(t, inboundList[0], "inbounds[0]")
	stream := asObject(t, tlsIn["streamSettings"], "streamSettings")
	tls := asObject(t, stream["tlsSettings"], "tlsSettings")
	certs := asArray(t, tls["certificates"], "certificates")
	c0 := asObject(t, certs[0], "certificates[0]")
	if c0["certificateFile"] != "/etc/xray/certs/t.example.com/fullchain.pem" ||
		c0["keyFile"] != "/etc/xray/certs/t.example.com/key.pem" {
		t.Errorf("证书路径注入失败: %v", c0)
	}
}

func TestGenerateConfig_RichRoutingRules(t *testing.T) {

	inbounds := []models.Inbound{
		{ID: 1, Tag: "vless-in", Protocol: "vless", Port: 443, Enabled: true},
	}
	users := vlessUsers("vless-fallback-tcp")

	routingRules := []models.ServerRoutingRule{
		{ID: 1, ServerID: 1, OutboundTag: "direct", Domain: "geosite:cn, geosite:apple\ndomain:internal.local", IP: "geoip:private\n10.0.0.0/8", Port: "80,443,8080-8090", Network: "tcp,udp", Enabled: true},
		{ID: 2, ServerID: 1, OutboundTag: "blocked", Domain: `["geosite:category-ads-all"]`, Enabled: true},
		{ID: 3, ServerID: 1, OutboundTag: "proxy", RuleJSON: `{"type":"field","inboundTag":["vless-in"],"protocol":["http","tls"],"outboundTag":"proxy"}`, Enabled: true},
	}

	rawCfg, err := xray.Generate(inbounds, nil, routingRules, users, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	rt := asObject(t, parsed["routing"], "routing")
	rules := asArray(t, rt["rules"], "routing.rules")
	if len(rules) != 5 {
		t.Fatalf("expected 5 rules (1 API + 1 default BT + 3 DB), got %d", len(rules))
	}
}

func TestGenerateConfig_TCPWithFallbacks(t *testing.T) {

	inb := models.Inbound{
		ID: 1, ServerID: 1, Tag: "vless-fallback-tcp", Protocol: "vless", Port: 50443,
		SettingsJSON:   `{"fallbacks":[{"dest":"8080","xver":1},{"path":"/web","dest":"8081","xver":1}]}`,
		StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"main.example.com","certificates":[{"certificateFile":"/etc/cert.pem","keyFile":"/etc/key.pem"}]}}`,
		Enabled:        true,
	}

	raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessUsers("vless-fallback-tcp"), nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	inbounds := asArray(t, parsed["inbounds"], "inbounds")
	vlessIn := asObject(t, inbounds[0], "inbounds[0]")
	inSettings := asObject(t, vlessIn["settings"], "settings")
	fbs := asArray(t, inSettings["fallbacks"], "fallbacks")
	if len(fbs) != 2 {
		t.Fatalf("expected 2 fallbacks, got %d", len(fbs))
	}
	fb0 := asObject(t, fbs[0], "fb0")
	if fb0["xver"] != float64(1) {
		t.Errorf("xver = %v", fb0["xver"])
	}
}

func TestGenerateConfig_Sniffing(t *testing.T) {

	t.Run("sniffing emitted", func(t *testing.T) {
		inb := models.Inbound{
			ID: 1, ServerID: 1, Tag: "in-sniff", Protocol: "vless", Port: 443,
			StreamSettings: `{"network":"ws","security":"none","wsSettings":{"path":"/ws"}}`,
			Sniffing:       `{"enabled":true,"destOverride":["http","tls"],"routeOnly":true}`,
			Enabled:        true,
		}
		raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessUsers("in1"), nil, "", "")
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		inbounds := asArray(t, parsed["inbounds"], "inbounds")
		vlessIn := asObject(t, inbounds[0], "inbounds[0]")
		sniff := asObject(t, vlessIn["sniffing"], "sniffing")
		if sniff["enabled"] != true {
			t.Errorf("enabled = %v", sniff["enabled"])
		}
		do := asArray(t, sniff["destOverride"], "destOverride")
		if len(do) != 2 || do[0] != "http" {
			t.Errorf("destOverride = %v", do)
		}
	})

	t.Run("sniffing omitted when empty", func(t *testing.T) {
		inb := models.Inbound{
			ID: 1, ServerID: 1, Tag: "in-no-sniff", Protocol: "vless", Port: 443,
			StreamSettings: `{"network":"ws","security":"none","wsSettings":{"path":"/ws"}}`,
			Enabled:        true,
		}
		raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessUsers("in1"), nil, "", "")
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		var parsed map[string]any
		json.Unmarshal(raw, &parsed)
		inbounds := asArray(t, parsed["inbounds"], "inbounds")
		vlessIn := asObject(t, inbounds[0], "inbounds[0]")
		if _, has := vlessIn["sniffing"]; has {
			t.Error("expected no sniffing key")
		}
	})
}

func TestGenerateConfig_ErrorHandling(t *testing.T) {

	t.Run("invalid settings_json", func(t *testing.T) {
		inbounds := []models.Inbound{
			{ID: 1, Tag: "in1", Protocol: "vless", Port: 443, SettingsJSON: "{bad-json", Enabled: true},
		}
		_, err := xray.Generate(inbounds, nil, nil, vlessUsers("in-reality"), nil, "", "")
		if err == nil {
			t.Error("expected error for malformed settings_json")
		}
	})

	t.Run("no active users", func(t *testing.T) {
		// U25：无可用用户时输出空 clients（清空配置推得动，节点立即移除失效用户），不再报错
		inbounds := []models.Inbound{
			{ID: 3, Tag: "in3", Protocol: "vless", Port: 443, Enabled: true},
		}
		usersByTag := map[string][]protocol.User{"in3": {{UUID: ""}}}
		cfg, err := xray.Generate(inbounds, nil, nil, usersByTag, nil, "", "")
		if err != nil {
			t.Fatalf("expected success with empty clients, got: %v", err)
		}
		if !strings.Contains(string(cfg), `"clients": []`) {
			t.Errorf("expected empty clients array in config, got: %s", cfg)
		}
	})

	t.Run("invalid stream_settings", func(t *testing.T) {
		inbounds := []models.Inbound{
			{ID: 1, Tag: "in1", Protocol: "vless", Port: 443, StreamSettings: "{bad-json", Enabled: true},
		}
		_, err := xray.Generate(inbounds, nil, nil, vlessUsers("in-reality"), nil, "", "")
		if err == nil {
			t.Error("expected error for malformed streamSettings")
		}
	})
}

func TestGenerateConfig_RealityShortIdsAlwaysEmitted(t *testing.T) {

	cases := []struct {
		name     string
		shortIDs string
		wantList []any
	}{
		{name: "empty", shortIDs: `""`, wantList: []any{""}},
		{name: "with value", shortIDs: `"6ba7b810"`, wantList: []any{"6ba7b810"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inb := models.Inbound{
				ID: 1, ServerID: 1, Tag: "in-reality", Protocol: "vless", Port: 443,
				StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"serverNames":["example.com"],"publicKey":"pk","privateKey":"sk","shortIds":[` + tc.shortIDs + `],"dest":"1.1.1.1:443"}}`,
				Enabled:        true,
			}
			raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessUsers("in-reality"), nil, "", "")
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			var parsed map[string]any
			json.Unmarshal(raw, &parsed)
			inbounds := asArray(t, parsed["inbounds"], "inbounds")
			vlessIn := asObject(t, inbounds[0], "inbounds[0]")
			stream := asObject(t, vlessIn["streamSettings"], "streamSettings")
			reality := asObject(t, stream["realitySettings"], "realitySettings")
			shortIds := asArray(t, reality["shortIds"], "shortIds")
			if len(shortIds) != 1 || shortIds[0] != tc.wantList[0] {
				t.Errorf("shortIds = %v, want %v", shortIds, tc.wantList)
			}
		})
	}
}

func TestGenerateConfig_RoutingInboundTag(t *testing.T) {
	inbounds := []models.Inbound{{ID: 1, Tag: "vless-in", Protocol: "vless", Port: 443, Enabled: true}}
	users := vlessUsers("vless-in")

	routingRules := []models.ServerRoutingRule{
		{ID: 1, ServerID: 1, OutboundTag: "direct", InboundTag: "vless-in, api", Enabled: true},
		{ID: 2, ServerID: 1, OutboundTag: "blocked", InboundTag: `["vless-in"]`, Enabled: true},
	}

	rawCfg, err := xray.Generate(inbounds, nil, routingRules, users, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	json.Unmarshal(rawCfg, &parsed)
	rt := asObject(t, parsed["routing"], "routing")
	rules := asArray(t, rt["rules"], "routing.rules")

	// 顺序：0=api 保护，1=内置 BT 屏蔽，2 起为节点规则
	r1 := asObject(t, rules[2], "rules[2]")
	inbTags1 := asArray(t, r1["inboundTag"], "rules[2].inboundTag")
	if len(inbTags1) != 2 || inbTags1[0] != "vless-in" || inbTags1[1] != "api" {
		t.Errorf("inboundTag = %v", inbTags1)
	}
}

func TestValidateInbound(t *testing.T) {
	cases := []struct {
		name      string
		settings  string
		stream    string
		sniffing  string
		expectErr bool
	}{
		{name: "all valid", settings: `{"decryption":"none"}`, stream: `{"network":"tcp"}`, sniffing: `{"enabled":true}`, expectErr: false},
		{name: "all empty", expectErr: false},
		{name: "bad settings", settings: `{bad`, expectErr: true},
		{name: "bad stream", stream: `{bad`, expectErr: true},
		{name: "bad sniffing", sniffing: `{bad`, expectErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := xray.ValidateInbound(tc.settings, tc.stream, tc.sniffing)
			if tc.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGenerateShortID(t *testing.T) {
	id1 := xray.GenerateShortID()
	id2 := xray.GenerateShortID()
	if len(id1) != 16 {
		t.Errorf("shortID length = %d, want 16", len(id1))
	}
	if id1 == id2 {
		t.Error("consecutive shortIDs must differ")
	}
	// must be valid hex
	for _, r := range id1 {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			t.Errorf("shortID contains non-hex character: %c", r)
		}
	}
}

// TestGenerate_OutboundDomainStrategySingleSource：出站域名解析策略唯一入口——
// freedom 出站的 settings.domainStrategy 一律取自该出站在库内的自有值（原样落到生成配置），
// 不存在任何服务器级默认值/兜底注入可覆盖它；defaultOutboundTag 只决定 outbounds[0]；
// routing.domainStrategy 独立不受影响。
func TestGenerate_OutboundDomainStrategySingleSource(t *testing.T) {
	inbounds := []models.Inbound{{ID: 1, ServerID: 1, Tag: "in", Protocol: "vless", Port: 443, Enabled: true}}

	gen := func(outs []models.ServerOutbound, defaultTag string) map[string]any {
		t.Helper()
		raw, err := xray.Generate(inbounds, outs, nil, vlessUsers("in"), nil, defaultTag, "IPIfNonMatch")
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		return parsed
	}

	findDS := func(parsed map[string]any, tag string) string {
		t.Helper()
		for _, o := range asArray(t, parsed["outbounds"], "outbounds") {
			m := asObject(t, o, "outbound")
			if m["tag"] == tag {
				settings := asObject(t, m["settings"], tag+" settings")
				ds, _ := settings["domainStrategy"].(string)
				return ds
			}
		}
		t.Fatalf("outbound %s not found", tag)
		return ""
	}

	// DB 出站自有 UseIPv4 → 原样落到生成配置
	if got := findDS(gen([]models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "direct", Protocol: "freedom", SettingsJSON: `{"domainStrategy":"UseIPv4"}`, Enabled: true},
	}, "direct"), "direct"); got != "UseIPv4" {
		t.Fatalf("出站自有 domainStrategy 应原样生成，got %q", got)
	}

	// 出站显式 AsIs 同样原样生效（无服务器级兜底可改写它）
	if got := findDS(gen([]models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "direct", Protocol: "freedom", SettingsJSON: `{"domainStrategy":"AsIs"}`, Enabled: true},
	}, "direct"), "direct"); got != "AsIs" {
		t.Fatalf("显式 AsIs 应原样生效，got %q", got)
	}

	// 无 DB 出站 → 回落模板默认 AsIs
	if got := findDS(gen(nil, "direct"), "direct"); got != "AsIs" {
		t.Fatalf("模板默认应为 AsIs，got %q", got)
	}

	// defaultOutboundTag 只决定 outbounds[0]（Xray 未命中规则的兜底出口）
	proxyOut := []models.ServerOutbound{
		{ID: 2, ServerID: 1, Tag: "proxy-out", Protocol: "freedom", SettingsJSON: `{"domainStrategy":"UseIP"}`, Enabled: true, Priority: 5},
	}
	parsedDef := gen(proxyOut, "proxy-out")
	first := asObject(t, asArray(t, parsedDef["outbounds"], "outbounds")[0], "outbounds[0]")
	if first["tag"] != "proxy-out" {
		t.Fatalf("默认出口应置于 outbounds[0]，got %v", first["tag"])
	}
	// 默认出口的解析策略仍取自出站自有值
	if got := findDS(parsedDef, "proxy-out"); got != "UseIP" {
		t.Fatalf("默认出口出站自有 domainStrategy 应原样生成，got %q", got)
	}

	// routing.domainStrategy 独立（IPIfNonMatch 生效，与出站解析策略互不影响）
	routing := asObject(t, gen(nil, "direct")["routing"], "routing")
	if got, _ := routing["domainStrategy"].(string); got != "IPIfNonMatch" {
		t.Fatalf("routing domainStrategy should be IPIfNonMatch: %s", got)
	}
}

// clientFlowOf 生成配置并提取 inbounds[0].settings.clients[0].flow（线格式透传测试）。
// 流控三态计算在服务层（GetValidUsers/protoUsersFor，services/config.go）；
// xray 层只负责把协议层的 Flow 字段写入 clients JSON。
func clientFlowOf(t *testing.T, streamJSON, userFlow string) (string, bool) {
	t.Helper()
	users := map[string][]protocol.User{"in": {{
		UUID: "11111111-1111-1111-1111-111111111111", Email: "user-1@panel.local", Flow: userFlow,
	}}}
	inb := models.Inbound{ID: 1, ServerID: 1, Tag: "in", Protocol: "vless", Port: 443,
		StreamSettings: streamJSON, Enabled: true}
	raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, users, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	inList := asArray(t, parsed["inbounds"], "inbounds")
	im := asObject(t, inList[0], "inbounds[0]")
	settings := asObject(t, im["settings"], "settings")
	clients := asArray(t, settings["clients"], "clients")
	c0 := asObject(t, clients[0], "clients[0]")
	flow, has := c0["flow"].(string)
	return flow, has
}

// TestGenerate_ClientFlowPassthrough 用户 Flow 线格式透传：
// 有值 → clients[0].flow 输出；空 → 不输出 flow 字段。
// （入站级流控三态：空=自动 / xtls-rprx-vision=全开 / none=禁自动——的计算与测试在服务层）
// TestGenerateConfig_VLESSInboundVlessEnc vlessenc 安全层：relay 入站 settings_json.decryption
// 保留（不再强制 none），stream 层 security none + tcp → 客户端自动 vision。
func TestGenerateConfig_VLESSInboundVlessEnc(t *testing.T) {
	dec := "mlkem768x25519plus.native.600s." + goldenMlkemSeed
	inb := models.Inbound{
		ID: 1, ServerID: 1, Tag: "in-relay-ve", Protocol: "vless", Port: 8443,
		Type: models.InboundTypeRelay, InternalUUID: "22222222-2222-2222-2222-222222222222",
		SettingsJSON:   `{"decryption":"` + dec + `"}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Enabled:        true,
	}
	raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, nil, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	json.Unmarshal(raw, &parsed)
	inbounds := asArray(t, parsed["inbounds"], "inbounds")
	relayIn := asObject(t, inbounds[0], "inbounds[0]")
	settings := asObject(t, relayIn["settings"], "settings")
	if settings["decryption"] != dec {
		t.Errorf("decryption 应原样保留: %v", settings["decryption"])
	}
	clients := asArray(t, settings["clients"], "clients")
	c0 := asObject(t, clients[0], "clients[0]")
	if c0["flow"] != "xtls-rprx-vision" {
		t.Errorf("vlessenc + tcp + security none 应自动 vision: %v", c0["flow"])
	}
}

func TestGenerateConfig_VLESSInboundVlessEncInvalid(t *testing.T) {
	// 裸密钥写法（v26.6.27 infra/conf 拒绝）→ 生成期报错，不让坏配置推送后 agent 端才暴露
	inb := models.Inbound{
		ID: 1, ServerID: 1, Tag: "in-relay-bad", Protocol: "vless", Port: 8443,
		Type: models.InboundTypeRelay, InternalUUID: "22222222-2222-2222-2222-222222222222",
		SettingsJSON:   `{"decryption":"cNUvTuN33QybBBHanFzy8LFC0wyvAsa__HSc0-KRRWo"}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Enabled:        true,
	}
	_, err := xray.Generate([]models.Inbound{inb}, nil, nil, nil, nil, "", "")
	if err == nil {
		t.Error("非法 decryption 应在生成期报错")
	}
}

func TestGenerateConfig_InboundRefVlessEnc(t *testing.T) {
	// 中转出站按目标入站 decryption 自动派生 encryption（0rtt + 公钥），
	// stream 层保持目标 security（none），flow 自动 vision。
	dec := "mlkem768x25519plus.native.600s." + goldenMlkemSeed
	target := models.Inbound{
		ID: 99, ServerID: 2, Tag: "landing-ve", Protocol: "vless", Port: 443,
		Type: models.InboundTypeRelay, InternalUUID: "33333333-3333-3333-3333-333333333333",
		SettingsJSON:   `{"decryption":"` + dec + `"}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Enabled:        true,
	}
	ref := target.ID
	ctx := &xray.GenerateContext{
		RefTargets: map[uint64]xray.RefTarget{
			target.ID: {Inbound: target, ServerHost: "10.0.0.5"},
		},
	}
	outbounds := []models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "to-landing-ve", Protocol: "vless", InboundRef: &ref, Enabled: true},
	}
	raw, err := xray.Generate([]models.Inbound{{ID: 1, Tag: "in", Protocol: "vless", Port: 443, Type: models.InboundTypeUser, Enabled: true}},
		outbounds, nil, vlessUsers("in"), ctx, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	json.Unmarshal(raw, &parsed)
	obs := asArray(t, parsed["outbounds"], "outbounds")
	var ob map[string]any
	for _, o := range obs {
		if om, _ := o.(map[string]any); om["tag"] == "to-landing-ve" {
			ob = om
		}
	}
	if ob == nil {
		t.Fatal("to-landing-ve outbound not found")
	}
	settings := asObject(t, ob["settings"], "settings")
	vnext := asArray(t, settings["vnext"], "vnext")
	users := asArray(t, asObject(t, vnext[0], "vnext[0]")["users"], "users")
	u0 := asObject(t, users[0], "users[0]")
	wantEnc := "mlkem768x25519plus.native.0rtt." + goldenMlkemEK
	if u0["encryption"] != wantEnc {
		t.Errorf("encryption 派生失败:\n got %v\nwant %s", u0["encryption"], wantEnc)
	}
	if u0["flow"] != "xtls-rprx-vision" {
		t.Errorf("vlessenc 目标 + tcp 应自动 vision: %v", u0["flow"])
	}
	stream := asObject(t, ob["streamSettings"], "streamSettings")
	if stream["security"] != "none" {
		t.Errorf("vlessenc 目标 stream security 应为 none: %v", stream["security"])
	}
}

// TestGenerateConfig_VLESSInboundDecryptionForced ISSUE-14 回归：
// API 允许空 settings_json 创建 VLESS 入站，生成器必须强制注入 decryption:"none" 并移除 encryption。
func TestGenerateConfig_VLESSInboundDecryptionForced(t *testing.T) {
	inbounds := []models.Inbound{{
		ID: 1, ServerID: 1, Tag: "vless-empty", Protocol: "vless", Port: 443,
		Type:           models.InboundTypeUser,
		StreamSettings: inbStream("tcp", "none", ""),
		Enabled:        true,
	}}
	cfg, err := xray.Generate(inbounds, nil, nil, vlessUsers("vless-empty"), &xray.GenerateContext{}, "direct", "AsIs")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var root struct {
		Inbounds []struct {
			Tag      string         `json:"tag"`
			Settings map[string]any `json:"settings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(cfg, &root); err != nil {
		t.Fatalf("unmarshal generated config: %v", err)
	}
	var target map[string]any
	for _, inb := range root.Inbounds {
		if inb.Tag == "vless-empty" {
			target = inb.Settings
			break
		}
	}
	if target == nil {
		t.Fatalf("generated config 中未找到入站 vless-empty: %s", cfg)
	}
	if target["decryption"] != "none" {
		t.Fatalf("settings.decryption = %v, want none", target["decryption"])
	}
	if _, ok := target["encryption"]; ok {
		t.Fatalf("VLESS 入站不应包含 encryption 字段: %v", target)
	}
}

func TestGenerate_ClientFlowPassthrough(t *testing.T) {
	realityTCP := inbStream("tcp", "reality", `"realitySettings":{"dest":"1.2.3.4:443","serverNames":["r.example.com"],"privateKey":"sk","shortIds":["abcd"]}`)

	// 1. 有值 → 透传
	flow, has := clientFlowOf(t, realityTCP, "xtls-rprx-vision")
	if !has || flow != "xtls-rprx-vision" {
		t.Errorf("Flow 有值应透传: flow=%q has=%v", flow, has)
	}
	// 2. 空 → 不输出
	flow, has = clientFlowOf(t, realityTCP, "")
	if has {
		t.Errorf("Flow 为空不应输出 flow 字段: flow=%q", flow)
	}
}

func TestGenerate_BlockCN(t *testing.T) {
	inbounds := []models.Inbound{
		{ID: 1, ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443, Enabled: true},
	}
	outbounds := []models.ServerOutbound{
		{
			ID:       1,
			ServerID: 1,
			Tag:      "direct",
			Protocol: "freedom",
			// block_cn 为面板专用开关：DB 存原文，生成时拆分为 finalRules，面板键不透传
			SettingsJSON: `{"domainStrategy":"AsIs","block_cn":true,"finalRules":[{"action":"block","ip":["geoip:private"],"blockDelay":"0"},{"action":"allow"}]}`,
			Enabled:      true,
		},
		{
			ID:       2,
			ServerID: 1,
			Tag:      "blocked",
			Protocol: "blackhole",
			Enabled:  true,
		},
	}

	cfgBytes, err := xray.Generate(inbounds, outbounds, nil, vlessUsers("vless-in"), nil, "direct", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var root struct {
		Outbounds []struct {
			Tag      string         `json:"tag"`
			Settings map[string]any `json:"settings"`
		} `json:"outbounds"`
		Routing struct {
			Rules []map[string]any `json:"rules"`
		} `json:"routing"`
	}
	if err := json.Unmarshal(cfgBytes, &root); err != nil {
		t.Fatalf("Unmarshal config failed: %v", err)
	}

	// 1. 面板键不透传：settings 中不应有 block_cn
	var direct *struct {
		Tag      string
		Settings map[string]any
	}
	for i := range root.Outbounds {
		if root.Outbounds[i].Tag == "direct" {
			o := root.Outbounds[i]
			direct = (*struct {
				Tag      string
				Settings map[string]any
			})(&o)
		}
	}
	if direct == nil {
		t.Fatalf("direct outbound not found: %+v", root.Outbounds)
	}
	if _, ok := direct.Settings["block_cn"]; ok {
		t.Errorf("block_cn 面板键不应透传给 xray: %+v", direct.Settings)
	}

	// 2. block_cn 拆分为 finalRules：{"block","geoip:cn"} 注入且位于 allow 兜底之前
	rules, _ := direct.Settings["finalRules"].([]any)
	blockCNIdx, allowIdx := -1, -1
	for i, r := range rules {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		action, _ := m["action"].(string)
		ips, _ := m["ip"].([]any)
		if action == "block" && len(ips) > 0 && ips[0] == "geoip:cn" {
			blockCNIdx = i
		}
		if action == "allow" && allowIdx == -1 {
			allowIdx = i
		}
	}
	if blockCNIdx == -1 {
		t.Errorf("finalRules 中缺少 {block, geoip:cn}: %+v", rules)
	}
	if allowIdx != -1 && blockCNIdx > allowIdx {
		t.Errorf("block geoip:cn 必须在 allow 兜底之前: %+v", rules)
	}
	if allowIdx != -1 && blockCNIdx != -1 {
		// 已有的 block private 应保留在 block cn 之前
		first := rules[0].(map[string]any)
		if ips, _ := first["ip"].([]any); len(ips) == 0 || ips[0] != "geoip:private" {
			t.Errorf("既有 block private 规则应保留: %+v", rules)
		}
	}

	// 3. 路由层不再注入 AND 短路的 cn 规则（domain+ip 同规则需同时满足，基本永不命中）
	for _, rule := range root.Routing.Rules {
		if rule["outboundTag"] != "blocked" {
			continue
		}
		domains, _ := rule["domain"].([]any)
		ips, _ := rule["ip"].([]any)
		if len(domains) > 0 && len(ips) > 0 {
			t.Errorf("路由规则不应同时含 domain 与 ip（AND 语义短路）: %+v", rule)
		}
	}
}

// TestGenerate_PrivateBlockScopedToOutbound：私网阻断不在路由层注入（2026-09-17 收口）——
// 路由规则自上而下首个命中生效且不带 inbound/outbound 维度限定，注入的全局私网规则会抢在管理员
// 「入站 → 其他出站」规则之前把私网目标丢给 blocked（注入规则在 UI 中不可见，无从排查），
// 使需要访问私网的链路（如中转落地）无法工作。私网拦截改由 freedom 出站 finalRules 承担。
// BT 屏蔽仍为服务器级（滥用防护与出站无关，freedom finalRules 亦无对应能力）。
func TestGenerate_PrivateBlockScopedToOutbound(t *testing.T) {
	inbounds := []models.Inbound{
		{ID: 1, ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443, Enabled: true},
	}
	// 管理员规则：该入站全部流量走 relay（catch-all），其中的私网目标必须能到达 relay
	rules := []models.ServerRoutingRule{
		{ID: 1, ServerID: 1, OutboundTag: "relay", InboundTag: "vless-in", Enabled: true},
	}
	outbounds := []models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "direct", Protocol: "freedom",
			SettingsJSON: models.DefaultFreedomDirectSettingsJSON, Enabled: true},
	}
	cfgBytes, err := xray.Generate(inbounds, outbounds, rules, vlessUsers("vless-in"), nil, "direct", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var root struct {
		Routing struct {
			Rules []map[string]any `json:"rules"`
		} `json:"routing"`
		Outbounds []map[string]any `json:"outbounds"`
	}
	if err := json.Unmarshal(cfgBytes, &root); err != nil {
		t.Fatalf("Unmarshal config failed: %v", err)
	}

	// 1. 路由层不得再出现私网规则（否则会抢在管理员规则之前命中）
	hasBT := false
	for _, rule := range root.Routing.Rules {
		ips, _ := rule["ip"].([]any)
		if len(ips) > 0 && ips[0] == "geoip:private" {
			t.Errorf("路由层不应注入私网规则（会抢在管理员「入站→出站」规则之前，且 UI 不可见）: %+v", rule)
		}
		if protos, _ := rule["protocol"].([]any); len(protos) > 0 && protos[0] == "bittorrent" {
			hasBT = true
			if rule["outboundTag"] != "blocked" {
				t.Errorf("BT 屏蔽应路由至 blocked: %+v", rule)
			}
		}
	}
	if !hasBT {
		t.Errorf("BT 屏蔽仍应在路由层注入（服务器级滥用防护）: %+v", root.Routing.Rules)
	}

	// 2. 管理员 catch-all 规则应存在且未被私网规则抢占（顺序上私网规则已不存在）
	foundRelayRule := false
	for _, rule := range root.Routing.Rules {
		if rule["outboundTag"] == "relay" {
			foundRelayRule = true
		}
	}
	if !foundRelayRule {
		t.Errorf("管理员 relay 规则丢失: %+v", root.Routing.Rules)
	}

	// 3. 私网拦截落在 direct 出站的 finalRules 上，且 blockDelay=0（立即断开，非官方默认 30-90s 黑洞）
	var directSettings map[string]any
	for _, o := range root.Outbounds {
		if o["tag"] == "direct" {
			directSettings, _ = o["settings"].(map[string]any)
		}
	}
	if directSettings == nil {
		t.Fatal("direct 出站缺失")
	}
	finalRules, _ := directSettings["finalRules"].([]any)
	if len(finalRules) == 0 {
		t.Fatalf("direct 缺 finalRules：私网拦截会整体失效: %+v", directSettings)
	}
	firstRule, _ := finalRules[0].(map[string]any)
	if action, _ := firstRule["action"].(string); action != "block" {
		t.Errorf("finalRules[0] 应为 block: %+v", firstRule)
	}
	if ips, _ := firstRule["ip"].([]any); len(ips) == 0 || ips[0] != "geoip:private" {
		t.Errorf("finalRules[0] 应拦 geoip:private: %+v", firstRule)
	}
	if delay, _ := firstRule["blockDelay"].(string); delay != "0" {
		t.Errorf("私网 block 应显式 blockDelay=0（否则退化为 30-90s 黑洞挂起）: %+v", firstRule)
	}
	lastRule, _ := finalRules[len(finalRules)-1].(map[string]any)
	if action, _ := lastRule["action"].(string); action != "allow" {
		t.Errorf("finalRules 末位应为 allow 兜底（freedom 内建安全策略只在无显式规则命中时生效）: %+v", finalRules)
	}
}

// TestTemplateDirectMatchesCanonicalDefault 漂移守卫：嵌入式模板的 direct 段必须与
// models.DefaultFreedomDirectSettingsJSON 语义一致。模板的 direct 会被 DB 同 tag 出站整体覆盖，
// 两处一旦不一致，新装（读模板）与存量/种子行（读 DB）的行为就会分叉，而分叉点只在生成结果里可见。
func TestTemplateDirectMatchesCanonicalDefault(t *testing.T) {
	var canon map[string]any
	if err := json.Unmarshal([]byte(models.DefaultFreedomDirectSettingsJSON), &canon); err != nil {
		t.Fatalf("解析 canonical 默认值失败: %v", err)
	}
	tmpl := xray.LoadTemplate()
	var tmplSettings map[string]any
	for _, item := range asArray(t, tmpl["outbounds"], "outbounds") {
		m := asObject(t, item, "outbound")
		if m["tag"] == "direct" {
			tmplSettings = asObject(t, m["settings"], "template direct settings")
		}
	}
	if tmplSettings == nil {
		t.Fatal("模板缺少 direct 出站")
	}
	canonJSON, _ := json.Marshal(canon)
	tmplJSON, _ := json.Marshal(tmplSettings)
	if string(canonJSON) != string(tmplJSON) {
		t.Fatalf("模板 direct settings 与 canonical 默认值不一致：\n  模板:      %s\n  canonical: %s", tmplJSON, canonJSON)
	}
}

// TestTemplateCacheNotPolluted 回归：节点出站/策略不得污染共享模板。
// 修复前 cloneMap 仅浅拷贝顶层，Generate 原地改写嵌套 settings（config.go 默认出口
// DS 注入段，已于 2026-09-17 随该特性移除），会导致配置跨服务器串扰，
// 且并发 Generate 对共享 map 读写构成数据竞争（race 必报）。
// 注入段移除后隔离要求不变：模板现为只读内嵌默认，每次 Generate 领独立深拷贝，
// 节点 DB 出站只在本次生成的副本上覆盖同 tag 出站（mergeOutbounds）。
func TestTemplateCacheNotPolluted(t *testing.T) {
	inbounds := []models.Inbound{{
		ID: 1, ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443,
		StreamSettings: inbStream("tcp", "none", ""), Enabled: true,
	}}
	users := vlessUsers("vless-in")

	directDS := func(rawCfg []byte) string {
		var parsed map[string]any
		if err := json.Unmarshal(rawCfg, &parsed); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		for _, item := range asArray(t, parsed["outbounds"], "outbounds") {
			m := asObject(t, item, "outbound")
			if m["tag"] == "direct" {
				settings := asObject(t, m["settings"], "direct.settings")
				ds, _ := settings["domainStrategy"].(string)
				return ds
			}
		}
		t.Fatal("direct outbound not found")
		return ""
	}

	// 服务器 A：DB 出站自有 UseIPv4 → 本次生成生效
	rawA, err := xray.Generate(inbounds, []models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "direct", Protocol: "freedom", SettingsJSON: `{"domainStrategy":"UseIPv4"}`, Enabled: true},
	}, nil, users, nil, "direct", "")
	if err != nil {
		t.Fatalf("Generate A failed: %v", err)
	}
	if ds := directDS(rawA); ds != "UseIPv4" {
		t.Fatalf("服务器 A 的 direct.domainStrategy = %q, 期望 UseIPv4", ds)
	}

	// 模板本体不得被 A 的节点配置污染（默认值仍为 AsIs）
	cached := xray.LoadTemplate()
	for _, item := range asArray(t, cached["outbounds"], "cached.outbounds") {
		m := asObject(t, item, "cached.outbound")
		if m["tag"] == "direct" {
			settings := asObject(t, m["settings"], "cached.direct.settings")
			if ds, _ := settings["domainStrategy"].(string); ds != "AsIs" {
				t.Fatalf("模板被污染：cached direct.domainStrategy = %q, 期望 AsIs", ds)
			}
		}
	}

	// 服务器 B：无 DB 出站 → 不得继承 A 的 UseIPv4（回落模板默认 AsIs）
	rawB, err := xray.Generate(inbounds, nil, nil, users, nil, "direct", "")
	if err != nil {
		t.Fatalf("Generate B failed: %v", err)
	}
	if ds := directDS(rawB); ds != "AsIs" {
		t.Fatalf("服务器 B 继承了服务器 A 的配置：direct.domainStrategy = %q, 期望模板默认 AsIs", ds)
	}
}

// TestGenerateConfig_InboundRefPinnedCert 链式代理 TLS 证书固定：
// 目标入站绑定证书带 pin 时，中转出站 tlsSettings 注入 pinnedPeerCertSha256；
// 无 pin（未绑证书）则不注入（走系统 CA 验证）。
func TestGenerateConfig_InboundRefPinnedCert(t *testing.T) {
	const pin = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	target := models.Inbound{
		ID: 99, ServerID: 2, Tag: "landing-tls", Protocol: "vless", Port: 8443,
		Type: models.InboundTypeRelay, InternalUUID: "44444444-4444-4444-4444-444444444444",
		StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"relay.example.com"}}`,
		Enabled:        true,
	}
	ref := target.ID
	ctx := &xray.GenerateContext{
		RefTargets: map[uint64]xray.RefTarget{
			target.ID: {Inbound: target, ServerHost: "10.0.0.5", CertPin: pin},
		},
	}
	outbounds := []models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "to-landing", Protocol: "vless", InboundRef: &ref, Enabled: true},
	}
	localIn := []models.Inbound{{ID: 1, Tag: "in", Protocol: "vless", Port: 443, Type: models.InboundTypeUser, Enabled: true}}

	tlsSettingsOf := func(t *testing.T, raw []byte) map[string]any {
		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		for _, o := range asArray(t, parsed["outbounds"], "outbounds") {
			if om, _ := o.(map[string]any); om["tag"] == "to-landing" {
				stream := asObject(t, om["streamSettings"], "streamSettings")
				return asObject(t, stream["tlsSettings"], "tlsSettings")
			}
		}
		t.Fatal("to-landing outbound not found")
		return nil
	}

	// 有 pin → 注入
	raw, err := xray.Generate(localIn, outbounds, nil, vlessUsers("in"), ctx, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	tlsS := tlsSettingsOf(t, raw)
	if tlsS["pinnedPeerCertSha256"] != pin {
		t.Errorf("pinnedPeerCertSha256 = %v, 期望注入 %s", tlsS["pinnedPeerCertSha256"], pin)
	}
	if tlsS["serverName"] != "relay.example.com" {
		t.Errorf("serverName = %v", tlsS["serverName"])
	}

	// 无 pin → 不注入
	ctxNoPin := &xray.GenerateContext{
		RefTargets: map[uint64]xray.RefTarget{target.ID: {Inbound: target, ServerHost: "10.0.0.5"}},
	}
	raw2, err := xray.Generate(localIn, outbounds, nil, vlessUsers("in"), ctxNoPin, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if _, exists := tlsSettingsOf(t, raw2)["pinnedPeerCertSha256"]; exists {
		t.Error("无 pin 证书不应注入 pinnedPeerCertSha256")
	}
}

// TestUserEmailForRoundtrip 统计键按 (用户, 入站) 注入与反解：xray 仅要求 email 入站内
// 唯一（validator 按 ToLower 建键），格式须全小写、不含 ">"；旧格式仅保留解析兼容
// （升级过渡期存量节点仍按旧键上报，回收端必须认得）。
func TestUserEmailForRoundtrip(t *testing.T) {
	u := &models.User{ID: 42}
	if got := xray.UserEmailFor(u, 7); got != "u42.i7@panel.local" {
		t.Fatalf("UserEmailFor = %q, want u42.i7@panel.local", got)
	}
	if uid, iid, ok := xray.ParseUserEmailFor("u42.i7@panel.local"); !ok || uid != 42 || iid != 7 {
		t.Fatalf("ParseUserEmailFor = %d/%d/%v, want 42/7/true", uid, iid, ok)
	}
	// xray validator 会折叠大小写，解析端同样先小写化再匹配
	if uid, iid, ok := xray.ParseUserEmailFor("U42.I7@PANEL.LOCAL"); !ok || uid != 42 || iid != 7 {
		t.Fatalf("ParseUserEmailFor(大写) = %d/%d/%v, want 42/7/true", uid, iid, ok)
	}
	// 旧格式 → 仅回用户 ID（入站维度 0）
	if uid, iid, ok := xray.ParseUserEmailAny("user-5@panel.local"); !ok || uid != 5 || iid != 0 {
		t.Fatalf("ParseUserEmailAny(旧格式) = %d/%d/%v, want 5/0/true", uid, iid, ok)
	}
	if uid, iid, ok := xray.ParseUserEmailAny("USER-5@PANEL.LOCAL"); !ok || uid != 5 || iid != 0 {
		t.Fatalf("ParseUserEmailAny(旧格式大写) = %d/%d/%v, want 5/0/true", uid, iid, ok)
	}
	// 非面板键（真实邮箱/relay/其他域名/杂项）不误判
	for _, em := range []string{"", "a@b.com", "relay-in-relay@panel.local", "xu5.i7@panel.local", "u42.i7@example.com", "u42@panel.local", "u.i7@panel.local"} {
		if _, _, ok := xray.ParseUserEmailAny(em); ok {
			t.Errorf("ParseUserEmailAny(%q) 不应解析成功", em)
		}
	}
}

// TestCountDistinctOnlineUsers 在线快照去重计数：同一用户跨入站多键只计一人，
// relay 内部账户与自定义 email 各计一。
func TestCountDistinctOnlineUsers(t *testing.T) {
	users := []protocol.OnlineUserIPs{
		{Email: "u1.i7@panel.local", IPs: []string{"1.1.1.1"}},
		{Email: "u1.i9@panel.local", IPs: []string{"1.1.1.1", "2.2.2.2"}},
		{Email: "user-2@panel.local", IPs: []string{"3.3.3.3"}},
		{Email: "relay-in-relay@panel.local", IPs: []string{"4.4.4.4"}},
		{Email: "custom@x.com", IPs: []string{"5.5.5.5"}},
	}
	if got := xray.CountDistinctOnlineUsers(users); got != 4 {
		t.Fatalf("CountDistinctOnlineUsers = %d, want 4（用户1 两键去重 + 用户2 + relay + 自定义）", got)
	}
	if got := xray.CountDistinctOnlineUsers(nil); got != 0 {
		t.Fatalf("empty = %d, want 0", got)
	}
}

// 溢出数字不得被误解析为 uid=0 的合法键
func TestParseUserEmailAnyOverflow(t *testing.T) {
}

// TestGenerateConfig_TunnelInbound_Linked 测试四层直通管道关联落地入站时的生成结果
func TestGenerateConfig_TunnelInbound_Linked(t *testing.T) {
	targetID := uint64(201)
	tunnelInb := models.Inbound{
		ID:              101,
		ServerID:        1,
		Tag:             "tunnel-10001",
		Protocol:        models.ProtocolDokodemo,
		Port:            10001,
		Type:            models.InboundTypeTunnel,
		TargetInboundID: &targetID,
		Enabled:         true,
	}
	ctx := &xray.GenerateContext{
		RefTargets: map[uint64]xray.RefTarget{
			targetID: {
				Inbound: models.Inbound{
					ID:       targetID,
					ServerID: 2,
					Tag:      "vless-in-443",
					Port:     443,
					Protocol: "vless",
				},
				ServerHost: "exit-node.example.com",
			},
		},
	}

	raw, err := xray.Generate([]models.Inbound{tunnelInb}, nil, nil, nil, ctx, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// 1. 验证入站生成了 dokodemo-door，address 和 port 为落地端
	inbounds, ok := root["inbounds"].([]any)
	if !ok || len(inbounds) < 2 { // tunnel + api
		t.Fatalf("inbounds count = %d, want >= 2", len(inbounds))
	}
	tInb := asObject(t, inbounds[0], "inbounds[0]")
	if tInb["protocol"] != "dokodemo-door" {
		t.Errorf("protocol = %v, want dokodemo-door", tInb["protocol"])
	}
	settings := asObject(t, tInb["settings"], "settings")
	if settings["address"] != "exit-node.example.com" {
		t.Errorf("settings.address = %v, want exit-node.example.com", settings["address"])
	}
	if fmt.Sprintf("%v", settings["port"]) != "443" {
		t.Errorf("settings.port = %v, want 443", settings["port"])
	}

	// 2. 验证自动派生了 out-tunnel-10001 的 freedom 出站并包含 proxyProtocol: 2
	outbounds, ok := root["outbounds"].([]any)
	if !ok {
		t.Fatalf("missing outbounds")
	}
	var freedomOut map[string]any
	for _, ob := range outbounds {
		m := asObject(t, ob, "outbound")
		if m["tag"] == "out-tunnel-10001" {
			freedomOut = m
			break
		}
	}
	if freedomOut == nil {
		t.Fatalf("out-tunnel-10001 freedom outbound not found in outbounds")
	}
	if freedomOut["protocol"] != "freedom" {
		t.Errorf("outbound protocol = %v, want freedom", freedomOut["protocol"])
	}
	obSettings := asObject(t, freedomOut["settings"], "outbound settings")
	if fmt.Sprintf("%v", obSettings["proxyProtocol"]) != "2" {
		t.Errorf("proxyProtocol = %v, want 2", obSettings["proxyProtocol"])
	}

	// 3. 验证直通路由规则已注入
	routing := asObject(t, root["routing"], "routing")
	rules, ok := routing["rules"].([]any)
	if !ok {
		t.Fatalf("missing routing.rules")
	}
	var tunnelRule map[string]any
	for _, r := range rules {
		rm := asObject(t, r, "rule")
		if rm["outboundTag"] == "out-tunnel-10001" {
			tunnelRule = rm
			break
		}
	}
	if tunnelRule == nil {
		t.Fatalf("tunnel routing rule not found")
	}
	inTags, _ := tunnelRule["inboundTag"].([]any)
	if len(inTags) == 0 || inTags[0] != "tunnel-10001" {
		t.Errorf("rule.inboundTag = %v, want [tunnel-10001]", inTags)
	}
}

// TestGenerateConfig_TunnelInbound_ManualExternal 测试手动外部 IP:Port 的直通管道
func TestGenerateConfig_TunnelInbound_ManualExternal(t *testing.T) {
	tunnelInb := models.Inbound{
		ID:            102,
		ServerID:      1,
		Tag:           "tunnel-manual",
		Protocol:      models.ProtocolDokodemo,
		Port:          10002,
		Type:          models.InboundTypeTunnel,
		TargetAddress: "198.51.100.2",
		TargetPort:    8443,
		Enabled:       true,
	}

	raw, err := xray.Generate([]models.Inbound{tunnelInb}, nil, nil, nil, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	inbounds := root["inbounds"].([]any)
	tInb := asObject(t, inbounds[0], "inbounds[0]")
	settings := asObject(t, tInb["settings"], "settings")
	if settings["address"] != "198.51.100.2" || fmt.Sprintf("%v", settings["port"]) != "8443" {
		t.Errorf("manual target = %v:%v, want 198.51.100.2:8443", settings["address"], settings["port"])
	}
}

// TestGenerateConfig_TunnelInbound_DraftSkipped 测试未配置目标的草稿状态下安全跳过
func TestGenerateConfig_TunnelInbound_DraftSkipped(t *testing.T) {
	tunnelInb := models.Inbound{
		ID:       103,
		ServerID: 1,
		Tag:      "tunnel-draft",
		Protocol: models.ProtocolDokodemo,
		Port:     10003,
		Type:     models.InboundTypeTunnel,
		// TargetInboundID 和 TargetAddress 均为空
		Enabled: true,
	}

	raw, err := xray.Generate([]models.Inbound{tunnelInb}, nil, nil, nil, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	inbounds := root["inbounds"].([]any)
	// 仅包含 api 内建入站，草稿 tunnel 被安全跳过
	if len(inbounds) != 1 {
		t.Fatalf("inbounds count = %d, want 1 (only api)", len(inbounds))
	}
}

// TestGenerateConfig_Tunnel_XrayTestValidation 生成并调用真实的 xray.exe 校验语法语义
func TestGenerateConfig_Tunnel_XrayTestValidation(t *testing.T) {
	xrayBin := `..\..\..\tools\xray-windows-64\xray.exe`
	if _, err := os.Stat(xrayBin); err != nil {
		t.Skip("跳过 xray 二进制实测（文件不存在）")
	}

	targetID := uint64(301)
	// 前置机配置
	tunnelInb := models.Inbound{
		ID:              104,
		ServerID:        1,
		Tag:             "tunnel-edge",
		Protocol:        models.ProtocolDokodemo,
		Port:            10004,
		Type:            models.InboundTypeTunnel,
		TargetInboundID: &targetID,
		Enabled:         true,
	}
	ctx := &xray.GenerateContext{
		RefTargets: map[uint64]xray.RefTarget{
			targetID: {
				Inbound: models.Inbound{
					ID:       targetID,
					ServerID: 2,
					Tag:      "vless-exit",
					Port:     443,
					Protocol: "vless",
				},
				ServerHost: "192.0.2.1",
			},
		},
	}
	edgeConfig, err := xray.Generate([]models.Inbound{tunnelInb}, nil, nil, nil, ctx, "", "")
	if err != nil {
		t.Fatalf("Generate edgeConfig failed: %v", err)
	}

	tmpEdge := filepath.Join(t.TempDir(), "edge.json")
	if err := os.WriteFile(tmpEdge, edgeConfig, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	cmd := exec.Command(xrayBin, "-test", "-config", tmpEdge)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("前置机配置 xray -test 校验失败: %v, out: %s", err, string(out))
	}

	// 落地机配置（开启 acceptProxyProtocol）
	exitInb := models.Inbound{
		ID:             targetID,
		ServerID:       2,
		Tag:            "vless-exit",
		Protocol:       "vless",
		Port:           443,
		Type:           models.InboundTypeUser,
		StreamSettings: `{"network":"tcp","security":"none","acceptProxyProtocol":true}`,
		Enabled:        true,
	}
	usersByTag := map[string][]protocol.User{
		"vless-exit": {
			{UUID: "a6a0e69e-5c62-4b2a-89a7-8f5b82143719", Email: "u1.i301@panel.local"},
		},
	}
	exitConfig, err := xray.Generate([]models.Inbound{exitInb}, nil, nil, usersByTag, nil, "", "")
	if err != nil {
		t.Fatalf("Generate exitConfig failed: %v", err)
	}

	var exitParsed struct {
		Inbounds []struct {
			Tag            string         `json:"tag"`
			StreamSettings map[string]any `json:"streamSettings"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(exitConfig, &exitParsed); err != nil {
		t.Fatalf("Unmarshal exitConfig failed: %v", err)
	}
	if len(exitParsed.Inbounds) == 0 {
		t.Fatalf("Expected at least 1 inbound in exitConfig")
	}
	ss := exitParsed.Inbounds[0].StreamSettings
	if ss["acceptProxyProtocol"] != nil {
		t.Errorf("acceptProxyProtocol 顶层私有键未在输出前清理: %v", ss)
	}
	tcpS, ok := ss["tcpSettings"].(map[string]any)
	if !ok || tcpS["acceptProxyProtocol"] != true {
		t.Errorf("acceptProxyProtocol 未正确注入 tcpSettings: %v", ss)
	}
	tmpExit := filepath.Join(t.TempDir(), "exit.json")
	if err := os.WriteFile(tmpExit, exitConfig, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	cmdExit := exec.Command(xrayBin, "-test", "-config", tmpExit)
	if out, err := cmdExit.CombinedOutput(); err != nil {
		t.Fatalf("落地机配置 xray -test 校验失败: %v, out: %s", err, string(out))
	}
}

func TestGenerateConfig_ChannelSocks5AndHTTPAndTunnel(t *testing.T) {
	inbounds := []models.Inbound{
		{
			ID:           101,
			ServerID:     1,
			Tag:          "chan-101-1080",
			Protocol:     "socks",
			Port:         1080,
			Type:         models.InboundTypeChannel,
			SettingsJSON: `{"auth":"password","accounts":[{"user":"testuser","pass":"testpass"}],"udp":true}`,
			Enabled:      true,
		},
		{
			ID:           102,
			ServerID:     1,
			Tag:          "chan-102-8080",
			Protocol:     "http",
			Port:         8080,
			Type:         models.InboundTypeChannel,
			SettingsJSON: `{"accounts":[{"user":"testuser","pass":"testpass"}]}`,
			Enabled:      true,
		},
		{
			ID:            103,
			ServerID:      1,
			Tag:           "chan-103-3306",
			Protocol:      "dokodemo-door",
			Port:          3306,
			Type:          models.InboundTypeChannel,
			TargetAddress: "db.remote.com",
			TargetPort:    3306,
			Enabled:       true,
		},
	}

	raw, err := xray.Generate(inbounds, nil, nil, nil, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	inbList := parsed["inbounds"].([]any)
	// 期望有 4 个入站（socks + http + tunnel + api）
	if len(inbList) != 4 {
		t.Fatalf("期望 4 个入站，实际 %d", len(inbList))
	}

	// 验证 socks 入站没有 clients 字段
	socksInb := inbList[0].(map[string]any)
	socksSettings := socksInb["settings"].(map[string]any)
	if _, hasClients := socksSettings["clients"]; hasClients {
		t.Errorf("socks 入站不应存在 clients: %v", socksSettings)
	}
	if socksSettings["auth"] != "password" {
		t.Errorf("socks auth 不正确: %v", socksSettings["auth"])
	}

	// 验证 http 入站没有 clients 字段
	httpInb := inbList[1].(map[string]any)
	httpSettings := httpInb["settings"].(map[string]any)
	if _, hasClients := httpSettings["clients"]; hasClients {
		t.Errorf("http 入站不应存在 clients: %v", httpSettings)
	}

	// 真实 xray 二进制语义校验
	xrayBin := filepath.Join("..", "..", "..", "tools", "xray-windows-64", "xray.exe")
	if _, err := os.Stat(xrayBin); err == nil {
		tmp := filepath.Join(t.TempDir(), "channel_config.json")
		if err := os.WriteFile(tmp, raw, 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(xrayBin, "-test", "-config", tmp)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xray -test 校验通道配置失败: %v, out: %s", err, string(out))
		}
	}
}
