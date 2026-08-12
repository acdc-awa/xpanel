package xray_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zhx/xray-panel/internal/master/xray"
	"github.com/zhx/xray-panel/internal/models"
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

func vlessTestUser() []models.User {
	return []models.User{{ID: 1, UUID: "11111111-1111-1111-1111-111111111111", Status: models.StatusActive}}
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

	users := []models.User{{ID: 1, UUID: "11111111-1111-1111-1111-111111111111", Status: models.StatusActive}}

	outbounds := []models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "warp", Protocol: "socks", SettingsJSON: `{"servers":[{"address":"127.0.0.1","port":40000}]}`, Enabled: true},
	}

	routingRules := []models.ServerRoutingRule{
		{ID: 1, ServerID: 1, OutboundTag: "warp", Domain: "geosite:netflix, geosite:google", IP: "1.1.1.1/32, 8.8.8.8/32", Enabled: true},
		{ID: 2, ServerID: 1, OutboundTag: "blocked", RuleJSON: `{"type":"field","domain":["geosite:category-ads-all"],"outboundTag":"blocked"}`, Enabled: true},
	}

	rawCfg, err := xray.Generate(inbounds, outbounds, routingRules, users, nil, nil, "", "")
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

func TestGenerateConfig_VLESS_gRPC_REALITY(t *testing.T) {

	inbounds := []models.Inbound{
		{
			ID: 101, ServerID: 1, Tag: "vless-grpc-reality-in", Protocol: "vless", Port: 443,
			StreamSettings: `{"network":"grpc","security":"reality","grpcSettings":{"serviceName":"vless-grpc-svc","authority":"grpc.example.com","multiMode":true},"realitySettings":{"serverNames":["example.com"],"publicKey":"pk123","privateKey":"sk456","shortIds":["12345678"],"dest":"1.1.1.1:443"}}`,
			Enabled:        true,
		},
	}

	rawCfg, err := xray.Generate(inbounds, nil, nil, vlessTestUser(), nil, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	inboundList := asArray(t, parsed["inbounds"], "inbounds")
	vlessIn := asObject(t, inboundList[0], "inbounds[0]")
	if vlessIn["tag"] != "vless-grpc-reality-in" {
		t.Errorf("tag mismatch: %v", vlessIn["tag"])
	}

	stream := asObject(t, vlessIn["streamSettings"], "streamSettings")
	if stream["network"] != "grpc" || stream["security"] != "reality" {
		t.Errorf("streamSettings mismatch: %v", stream)
	}
	grpcSettings := asObject(t, stream["grpcSettings"], "grpcSettings")
	if grpcSettings["serviceName"] != "vless-grpc-svc" {
		t.Errorf("grpcSettings.serviceName mismatch: %v", grpcSettings)
	}
	realitySettings := asObject(t, stream["realitySettings"], "realitySettings")
	if realitySettings["dest"] != "1.1.1.1:443" {
		t.Errorf("realitySettings mismatch: %v", realitySettings)
	}
}

func TestGenerateConfig_VLESS_gRPC_TLS(t *testing.T) {

	inbounds := []models.Inbound{
		{
			ID: 102, ServerID: 1, Tag: "vless-grpc-tls-in", Protocol: "vless", Port: 8443,
			StreamSettings: `{"network":"grpc","security":"tls","grpcSettings":{"serviceName":"grpc-tls-service"},"tlsSettings":{"serverName":"mydomain.com","certificates":[{"certificateFile":"/etc/cert.pem","keyFile":"/etc/key.pem"}]}}`,
			Enabled:        true,
		},
	}

	rawCfg, err := xray.Generate(inbounds, nil, nil, vlessTestUser(), nil, nil, "", "")
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
	if stream["network"] != "grpc" || stream["security"] != "tls" {
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
	users := vlessTestUser()

	outbounds := []models.ServerOutbound{
		{ID: 1, ServerID: 1, Tag: "direct", Protocol: "freedom", SettingsJSON: `{"domainStrategy":"UseIP"}`, StreamSettingsJSON: `{"sockopt":{"mark":255}}`, Enabled: true},
		{ID: 2, ServerID: 1, Tag: "blocked", Protocol: "blackhole", SettingsJSON: `{"response":{"type":"http"}}`, Enabled: true},
		{ID: 3, ServerID: 1, Tag: "outbound-vless-grpc", Protocol: "vless", SettingsJSON: `{"vnext":[{"address":"remote.proxy.com","port":443,"users":[{"id":"uuid","encryption":"none"}]}]}`, StreamSettingsJSON: `{"network":"grpc","security":"tls","grpcSettings":{"serviceName":"out-grpc-svc"}}`, SendThrough: "192.168.1.100", Enabled: true},
	}

	rawCfg, err := xray.Generate(inbounds, outbounds, nil, users, nil, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	obs := asArray(t, parsed["outbounds"], "outbounds")
	if len(obs) != 4 {
		t.Fatalf("expected 4 outbounds (direct+blocked+api from template + vless-grpc from DB), got %d", len(obs))
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
	users := vlessTestUser()

	outbounds := []models.ServerOutbound{
		{
			ID: 1, ServerID: 1, Tag: "proxy-out", Protocol: "vless", Enabled: true,
			SettingsJSON: `{"vnext":[{"address":"remote.proxy.com","port":443,"users":[{"id":"uuid-1"},{"id":"uuid-2","encryption":"none"}]}]}`,
		},
	}

	rawCfg, err := xray.Generate(inbounds, outbounds, nil, users, nil, nil, "", "")
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
	raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, nil, nil, nil, "", "")
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
	_, err := xray.Generate([]models.Inbound{inb}, nil, nil, nil, nil, nil, "", "")
	if err == nil {
		t.Error("relay 入站缺 InternalUUID 应报错（等 setup）")
	}
}

func TestGenerateConfig_IdleInboundSkipped(t *testing.T) {
	inbounds := []models.Inbound{
		{ID: 1, Tag: "in-idle", Protocol: "vless", Port: 8443, Type: models.InboundTypeIdle, Enabled: true},
		{ID: 2, Tag: "in-user", Protocol: "vless", Port: 443, Type: models.InboundTypeUser, Enabled: true},
	}
	raw, err := xray.Generate(inbounds, nil, nil, vlessTestUser(), nil, nil, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	json.Unmarshal(raw, &parsed)
	inboundList := asArray(t, parsed["inbounds"], "inbounds")
	for _, in := range inboundList {
		im := asObject(t, in, "inbound")
		if im["tag"] == "in-idle" {
			t.Error("idle 入站不应生成")
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
		outbounds, nil, vlessTestUser(), nil, ctx, "", "")
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
		outbounds, nil, vlessTestUser(), nil, ctx, "", "")
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
	raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessTestUser(), nil, ctx, "", "")
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
	users := vlessTestUser()

	routingRules := []models.ServerRoutingRule{
		{ID: 1, ServerID: 1, OutboundTag: "direct", Domain: "geosite:cn, geosite:apple\ndomain:internal.local", IP: "geoip:private\n10.0.0.0/8", Port: "80,443,8080-8090", Network: "tcp,udp", Enabled: true},
		{ID: 2, ServerID: 1, OutboundTag: "blocked", Domain: `["geosite:category-ads-all"]`, Enabled: true},
		{ID: 3, ServerID: 1, OutboundTag: "proxy", RuleJSON: `{"type":"field","inboundTag":["vless-in"],"protocol":["http","tls"],"outboundTag":"proxy"}`, Enabled: true},
	}

	rawCfg, err := xray.Generate(inbounds, nil, routingRules, users, nil, nil, "", "")
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

	raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessTestUser(), nil, nil, "", "")
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
		raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessTestUser(), nil, nil, "", "")
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
		raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessTestUser(), nil, nil, "", "")
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
		_, err := xray.Generate(inbounds, nil, nil, vlessTestUser(), nil, nil, "", "")
		if err == nil {
			t.Error("expected error for malformed settings_json")
		}
	})

	t.Run("no active users", func(t *testing.T) {
		inbounds := []models.Inbound{
			{ID: 3, Tag: "in3", Protocol: "vless", Port: 443, Enabled: true},
		}
		users := []models.User{{ID: 1, UUID: "", Status: models.StatusActive}, {ID: 2, UUID: "uuid2", Status: models.StatusDisabled}}
		_, err := xray.Generate(inbounds, nil, nil, users, nil, nil, "", "")
		if err == nil {
			t.Error("expected error for no active users")
		}
	})

	t.Run("invalid stream_settings", func(t *testing.T) {
		inbounds := []models.Inbound{
			{ID: 1, Tag: "in1", Protocol: "vless", Port: 443, StreamSettings: "{bad-json", Enabled: true},
		}
		_, err := xray.Generate(inbounds, nil, nil, vlessTestUser(), nil, nil, "", "")
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
			raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessTestUser(), nil, nil, "", "")
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
	users := vlessTestUser()

	routingRules := []models.ServerRoutingRule{
		{ID: 1, ServerID: 1, OutboundTag: "direct", InboundTag: "vless-in, api", Enabled: true},
		{ID: 2, ServerID: 1, OutboundTag: "blocked", InboundTag: `["vless-in"]`, Enabled: true},
	}

	rawCfg, err := xray.Generate(inbounds, nil, routingRules, users, nil, nil, "", "")
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

func TestGRPCSettingsUnmarshalJSON(t *testing.T) {
	parse := func(t *testing.T, payload string) xray.GRPCSettings {
		t.Helper()
		var g xray.GRPCSettings
		if err := json.Unmarshal([]byte(payload), &g); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		return g
	}

	t.Run("snake_case", func(t *testing.T) {
		g := parse(t, `{"service_name":"svc","multi_mode":true}`)
		if g.ServiceName != "svc" || !g.MultiMode {
			t.Errorf("parse error: %+v", g)
		}
	})
	t.Run("camelCase", func(t *testing.T) {
		g := parse(t, `{"serviceName":"svc","multiMode":true}`)
		if g.ServiceName != "svc" || !g.MultiMode {
			t.Errorf("parse error: %+v", g)
		}
	})
	t.Run("camelCase wins", func(t *testing.T) {
		g := parse(t, `{"serviceName":"camel","service_name":"snake"}`)
		if g.ServiceName != "camel" {
			t.Errorf("ServiceName = %q", g.ServiceName)
		}
	})
}

func TestGRPCSettingsMarshalJSON(t *testing.T) {
	g := xray.GRPCSettings{ServiceName: "my-svc", Authority: "grpc.example.com", MultiMode: true}
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"service_name":"my-svc"`) || !strings.Contains(s, `"multi_mode":true`) {
		t.Errorf("marshal must use snake_case: %s", s)
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
