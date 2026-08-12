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

func TestGenerateConfig_VLESS_gRPC_REALITY(t *testing.T) {

	inbounds := []models.Inbound{
		{
			ID: 101, ServerID: 1, Tag: "vless-grpc-reality-in", Protocol: "vless", Port: 443,
			StreamSettings: `{"network":"grpc","security":"reality","grpcSettings":{"serviceName":"vless-grpc-svc","authority":"grpc.example.com","multiMode":true},"realitySettings":{"serverNames":["example.com"],"publicKey":"pk123","privateKey":"sk456","shortIds":["12345678"],"dest":"1.1.1.1:443"}}`,
			Enabled:        true,
		},
	}

	rawCfg, err := xray.Generate(inbounds, nil, nil, vlessTestUser(), nil, "", "")
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

	rawCfg, err := xray.Generate(inbounds, nil, nil, vlessTestUser(), nil, "", "")
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

	raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessTestUser(), nil, "", "")
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
		raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessTestUser(), nil, "", "")
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
		raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessTestUser(), nil, "", "")
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
		_, err := xray.Generate(inbounds, nil, nil, vlessTestUser(), nil, "", "")
		if err == nil {
			t.Error("expected error for malformed settings_json")
		}
	})

	t.Run("no active users", func(t *testing.T) {
		inbounds := []models.Inbound{
			{ID: 3, Tag: "in3", Protocol: "vless", Port: 443, Enabled: true},
		}
		users := []models.User{{ID: 1, UUID: "", Status: models.StatusActive}, {ID: 2, UUID: "uuid2", Status: models.StatusDisabled}}
		_, err := xray.Generate(inbounds, nil, nil, users, nil, "", "")
		if err == nil {
			t.Error("expected error for no active users")
		}
	})

	t.Run("invalid stream_settings", func(t *testing.T) {
		inbounds := []models.Inbound{
			{ID: 1, Tag: "in1", Protocol: "vless", Port: 443, StreamSettings: "{bad-json", Enabled: true},
		}
		_, err := xray.Generate(inbounds, nil, nil, vlessTestUser(), nil, "", "")
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
			raw, err := xray.Generate([]models.Inbound{inb}, nil, nil, vlessTestUser(), nil, "", "")
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
