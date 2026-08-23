package xray_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel-node/pkg/protocol"
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
		Type: models.InboundTypeRelay, // InternalUUID 空
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
	if len(rules) != 6 {
		t.Fatalf("expected 6 rules (1 API + 2 defaults + 3 DB), got %d", len(rules))
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

	r1 := asObject(t, rules[3], "rules[3]")
	inbTags1 := asArray(t, r1["inboundTag"], "rules[3].inboundTag")
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



func TestSniffingSettingsUnmarshalJSON(t *testing.T) {
	parse := func(payload string) xray.SniffingSettings {
		var s xray.SniffingSettings
		json.Unmarshal([]byte(payload), &s)
		return s
	}

	t.Run("camelCase", func(t *testing.T) {
		s := parse(`{"enabled":true,"destOverride":["http"],"metadataOnly":true,"routeOnly":true}`)
		if len(s.DestOverride) != 1 || s.DestOverride[0] != "http" || !s.MetadataOnly || !s.RouteOnly {
			t.Errorf("parse error: %+v", s)
		}
	})
	t.Run("snake_case", func(t *testing.T) {
		s := parse(`{"enabled":true,"dest_override":["http","tls"],"metadata_only":true,"route_only":true}`)
		if len(s.DestOverride) != 2 || !s.MetadataOnly || !s.RouteOnly {
			t.Errorf("parse error: %+v", s)
		}
	})
}

func TestStreamHasReality(t *testing.T) {
	if xray.StreamHasReality(``) {
		t.Error("empty should be false")
	}
	if !xray.StreamHasReality(`{"security":"reality"}`) {
		t.Error("reality should be true")
	}
	if xray.StreamHasReality(`{"security":"tls"}`) {
		t.Error("tls should be false")
	}
}

func TestStreamNetwork(t *testing.T) {
	if n := xray.StreamNetwork(`{"network":"grpc"}`); n != "grpc" {
		t.Errorf("got %q", n)
	}
	if n := xray.StreamNetwork(``); n != "" {
		t.Errorf("got %q", n)
	}
}

func TestStreamSecurity(t *testing.T) {
	if s := xray.StreamSecurity(`{"security":"reality"}`); s != "reality" {
		t.Errorf("got %q", s)
	}
}

// TestGenerate_DefaultOutboundDS：默认出口（freedom）出站解析策略注入——
// Server 配 UseIP → direct 出站 settings.domainStrategy=UseIP；AsIs → 模板默认不动；
// DB 出站显式 UseIPv4 → 不被覆盖；routing.domainStrategy 独立不受影响。
func TestGenerate_DefaultOutboundDS(t *testing.T) {
	gen := func(ds string) map[string]any {
		inbounds := []models.Inbound{{ID: 1, ServerID: 1, Tag: "in", Protocol: "vless", Port: 443, Enabled: true}}
		raw, err := xray.Generate(inbounds, nil, nil, vlessUsers("in"), nil, "direct", "IPIfNonMatch", ds)
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		return parsed
	}

	findDirectDS := func(parsed map[string]any) string {
		for _, o := range asArray(t, parsed["outbounds"], "outbounds") {
			m := asObject(t, o, "outbound")
			if m["tag"] == "direct" {
				settings := asObject(t, m["settings"], "direct settings")
				ds, _ := settings["domainStrategy"].(string)
				return ds
			}
		}
		t.Fatal("direct outbound not found")
		return ""
	}

	// UseIP 注入
	if got := findDirectDS(gen("UseIP")); got != "UseIP" {
		t.Fatalf("UseIP injection failed: %s", got)
	}
	// AsIs 不动模板默认
	if got := findDirectDS(gen("AsIs")); got != "AsIs" {
		t.Fatalf("AsIs should keep template default: %s", got)
	}
	// routing.domainStrategy 独立（IPIfNonMatch 生效）
	parsed := gen("UseIP")
	routing := asObject(t, parsed["routing"], "routing")
	if got, _ := routing["domainStrategy"].(string); got != "IPIfNonMatch" {
		t.Fatalf("routing domainStrategy should be IPIfNonMatch: %s", got)
	}

	// DB 出站显式 UseIPv4（tag=direct 被 DB 出站覆盖场景）→ 不覆盖
	outbounds := []models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "direct", Protocol: "freedom", SettingsJSON: `{"domainStrategy":"UseIPv4"}`, Enabled: true},
	}
	raw, err := xray.Generate([]models.Inbound{{ID: 1, ServerID: 1, Tag: "in", Protocol: "vless", Port: 443, Enabled: true}}, outbounds, nil, vlessUsers("in"), nil, "direct", "", "UseIP")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed2 map[string]any
	if err := json.Unmarshal(raw, &parsed2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if got := findDirectDS(parsed2); got != "UseIPv4" {
		t.Fatalf("explicit UseIPv4 should not be overwritten: %s", got)
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
			ID:           1,
			ServerID:     1,
			Tag:          "direct",
			Protocol:     "freedom",
			SettingsJSON: `{"domainStrategy":"AsIs","block_cn":true}`,
			Enabled:      true,
		},
		{
			ID:           2,
			ServerID:     1,
			Tag:          "blocked",
			Protocol:     "blackhole",
			Enabled:      true,
		},
	}

	cfgBytes, err := xray.Generate(inbounds, outbounds, nil, vlessUsers("vless-in"), nil, "direct", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var root struct {
		Routing struct {
			Rules []map[string]any `json:"rules"`
		} `json:"routing"`
	}
	if err := json.Unmarshal(cfgBytes, &root); err != nil {
		t.Fatalf("Unmarshal config failed: %v", err)
	}

	hasCNRule := false
	for _, rule := range root.Routing.Rules {
		if rule["outboundTag"] == "blocked" {
			domains, _ := rule["domain"].([]any)
			ips, _ := rule["ip"].([]any)
			if len(domains) > 0 && len(ips) > 0 && domains[0] == "geosite:cn" && ips[0] == "geoip:cn" {
				hasCNRule = true
				break
			}
		}
	}
	if !hasCNRule {
		t.Errorf("expected block CN rule (geosite:cn / geoip:cn -> blocked), but not found in rules: %+v", root.Routing.Rules)
	}
}


// TestTemplateCacheNotPolluted 回归：自定义 DB 模板 + 默认出口 domainStrategy 注入
// 不得污染共享模板缓存。修复前 cloneMap 仅浅拷贝顶层，Generate 原地改写嵌套
// settings（config.go 默认出口 DS 注入段），会导致：①服务器 A（DS=UseIPv4）生成后，
// 服务器 B（DS 空）继承 A 的值——配置跨服务器串扰；②并发 Generate 对共享 map
// 读写构成数据竞争（race 必报）。
func TestTemplateCacheNotPolluted(t *testing.T) {
	t.Cleanup(func() { _ = xray.SetTemplate(nil) }) // 模板缓存为包级全局，用后还原

	custom := `{
		"log": {"loglevel": "warning"},
		"stats": {},
		"policy": {"levels": {"0": {"statsUserUplink": true, "statsUserDownlink": true}}},
		"inbounds": [],
		"outbounds": [
			{"protocol": "freedom", "tag": "direct", "settings": {}},
			{"protocol": "blackhole", "tag": "blocked"}
		],
		"routing": {"rules": []}
	}`
	if err := xray.SetTemplate([]byte(custom)); err != nil {
		t.Fatalf("SetTemplate failed: %v", err)
	}

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

	// 服务器 A：DS=UseIPv4 → 注入生效
	rawA, err := xray.Generate(inbounds, nil, nil, users, nil, "direct", "", "UseIPv4")
	if err != nil {
		t.Fatalf("Generate A failed: %v", err)
	}
	if ds := directDS(rawA); ds != "UseIPv4" {
		t.Fatalf("服务器 A 的 direct.domainStrategy = %q, 期望 UseIPv4", ds)
	}

	// 缓存本体不得被 A 的注入污染
	cached := xray.LoadTemplate()
	for _, item := range asArray(t, cached["outbounds"], "cached.outbounds") {
		m := asObject(t, item, "cached.outbound")
		if m["tag"] == "direct" {
			settings := asObject(t, m["settings"], "cached.direct.settings")
			if ds, _ := settings["domainStrategy"].(string); ds != "" {
				t.Fatalf("模板缓存被污染：cached direct.domainStrategy = %q", ds)
			}
		}
	}

	// 服务器 B：DS 为空 → 不得继承 A 注入的 UseIPv4
	rawB, err := xray.Generate(inbounds, nil, nil, users, nil, "direct", "")
	if err != nil {
		t.Fatalf("Generate B failed: %v", err)
	}
	if ds := directDS(rawB); ds != "" {
		t.Fatalf("服务器 B 继承了服务器 A 的配置：direct.domainStrategy = %q", ds)
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
