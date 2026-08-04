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

func TestGenerateConfigWithOutboundsAndRouting(t *testing.T) {
	srv := &models.Server{
		ID:     1,
		Name:   "Test Server",
		NodeID: "node-test",
	}

	inbounds := []models.Inbound{
		{
			ID:           1,
			ServerID:     1,
			Tag:          "vless-in",
			Protocol:     "vless",
			Port:         443,
			Network:      "ws",
			TLSType:      "none",
			SettingsJSON: `{"ws":{"path":"/ws"}}`,
			Enabled:      true,
		},
	}

	users := []models.User{
		{
			ID:     1,
			UUID:   "11111111-1111-1111-1111-111111111111",
			Status: models.StatusActive,
		},
	}

	outbounds := []models.ServerOutbound{
		{
			ID:           1,
			ServerID:     1,
			Tag:          "warp",
			Protocol:     "socks",
			SettingsJSON: `{"servers":[{"address":"127.0.0.1","port":40000}]}`,
			Enabled:      true,
		},
	}

	routingRules := []models.ServerRoutingRule{
		{
			ID:          1,
			ServerID:    1,
			OutboundTag: "warp",
			Domain:      "geosite:netflix, geosite:google",
			IP:          "1.1.1.1/32, 8.8.8.8/32",
			Enabled:     true,
		},
		{
			ID:          2,
			ServerID:    1,
			OutboundTag: "blocked",
			RuleJSON:    `{"type":"field","domain":["geosite:category-ads-all"],"outboundTag":"blocked"}`,
			Enabled:     true,
		},
	}

	rawCfg, err := xray.Generate(srv, inbounds, outbounds, routingRules, users)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal config JSON failed: %v", err)
	}

	// Verify outbounds
	obs, ok := parsed["outbounds"].([]any)
	if !ok || len(obs) == 0 {
		t.Fatalf("expected outbounds array in config")
	}

	hasWarp := false
	hasFreedomFallback := false
	for _, o := range obs {
		om, _ := o.(map[string]any)
		if om["tag"] == "warp" && om["protocol"] == "socks" {
			hasWarp = true
		}
		if om["tag"] == "direct" && om["protocol"] == "freedom" {
			hasFreedomFallback = true
		}
	}

	if !hasWarp {
		t.Errorf("warp outbound not found in config outbounds")
	}
	if !hasFreedomFallback {
		t.Errorf("fallback freedom outbound not found in config outbounds")
	}

	// Verify routing rules
	rt, ok := parsed["routing"].(map[string]any)
	if !ok {
		t.Fatalf("expected routing object in config")
	}
	rules, ok := rt["rules"].([]any)
	if !ok || len(rules) < 3 {
		t.Fatalf("expected at least 3 routing rules (1 api + 2 custom), got %d", len(rules))
	}

	rule1, ok := rules[1].(map[string]any)
	if !ok || rule1["outboundTag"] != "warp" {
		t.Errorf("expected rule 1 outboundTag to be warp, got %v", rule1)
	}
}

func TestGenerateConfig_VLESS_gRPC_REALITY(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-gRPC-REALITY", NodeID: "node-grpc-reality"}

	inbounds := []models.Inbound{
		{
			ID:           101,
			ServerID:     1,
			Tag:          "vless-grpc-reality-in",
			Protocol:     "vless",
			Port:         443,
			Network:      "grpc",
			TLSType:      "reality",
			SettingsJSON: `{"grpc":{"service_name":"vless-grpc-svc","authority":"grpc.example.com","multiMode":true},"reality":{"server_name":"example.com","public_key":"pubkey123","private_key":"privkey456","short_id":"12345678","dest":"1.1.1.1:443"}}`,
			Enabled:      true,
		},
	}

	users := []models.User{
		{
			ID:     10,
			UUID:   "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			Status: models.StatusActive,
		},
	}

	rawCfg, err := xray.Generate(srv, inbounds, nil, nil, users)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	inboundList, ok := parsed["inbounds"].([]any)
	if !ok || len(inboundList) < 2 {
		t.Fatalf("expected at least 2 inbounds (vless + api dokodemo-door), got %d", len(inboundList))
	}

	vlessIn := asObject(t, inboundList[0], "inbounds[0]")
	if vlessIn["tag"] != "vless-grpc-reality-in" {
		t.Errorf("expected tag vless-grpc-reality-in, got %v", vlessIn["tag"])
	}

	stream, ok := vlessIn["streamSettings"].(map[string]any)
	if !ok {
		t.Fatalf("expected streamSettings object")
	}

	if stream["network"] != "grpc" {
		t.Errorf("expected network grpc, got %v", stream["network"])
	}
	if stream["security"] != "reality" {
		t.Errorf("expected security reality, got %v", stream["security"])
	}

	grpcSettings, ok := stream["grpcSettings"].(map[string]any)
	if !ok || grpcSettings["serviceName"] != "vless-grpc-svc" {
		t.Errorf("expected grpcSettings.serviceName to be vless-grpc-svc, got %v", stream["grpcSettings"])
	}
	if grpcSettings["authority"] != "grpc.example.com" {
		t.Errorf("expected grpcSettings.authority to be grpc.example.com, got %v", grpcSettings["authority"])
	}
	if grpcSettings["multiMode"] != true {
		t.Errorf("expected grpcSettings.multiMode to be true, got %v", grpcSettings["multiMode"])
	}

	realitySettings, ok := stream["realitySettings"].(map[string]any)
	if !ok {
		t.Fatalf("expected realitySettings object")
	}
	if realitySettings["dest"] != "1.1.1.1:443" || realitySettings["privateKey"] != "privkey456" {
		t.Errorf("realitySettings content mismatch: %v", realitySettings)
	}
}

func TestGenerateConfig_VLESS_gRPC_TLS(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-gRPC-TLS", NodeID: "node-grpc-tls"}

	inbounds := []models.Inbound{
		{
			ID:           102,
			ServerID:     1,
			Tag:          "vless-grpc-tls-in",
			Protocol:     "vless",
			Port:         8443,
			Network:      "grpc",
			TLSType:      "tls",
			SettingsJSON: `{"grpc":{"serviceName":"grpc-tls-service"},"tls":{"server_name":"mydomain.com","cert_file":"/etc/ssl/cert.pem","key_file":"/etc/ssl/key.pem"}}`,
			Enabled:      true,
		},
	}

	users := []models.User{
		{
			ID:     11,
			UUID:   "b2c3d4e5-f6a7-8901-bcde-f23456789012",
			Status: models.StatusActive,
		},
	}

	rawCfg, err := xray.Generate(srv, inbounds, nil, nil, users)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	inboundList := asArray(t, parsed["inbounds"], "inbounds")
	vlessIn := asObject(t, inboundList[0], "inbounds[0]")
	stream := asObject(t, vlessIn["streamSettings"], "inbounds[0].streamSettings")

	if stream["network"] != "grpc" || stream["security"] != "tls" {
		t.Errorf("streamSettings mismatch: %v", stream)
	}

	tlsSettings, ok := stream["tlsSettings"].(map[string]any)
	if !ok || tlsSettings["serverName"] != "mydomain.com" {
		t.Errorf("tlsSettings mismatch: %v", stream["tlsSettings"])
	}
}

func TestGenerateConfig_ComplexOutbounds(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-Outbounds", NodeID: "node-outbounds"}

	inbounds := []models.Inbound{
		{
			ID:       1,
			Tag:      "vless-in",
			Protocol: "vless",
			Port:     443,
			Network:  "tcp",
			TLSType:  "none",
			Enabled:  true,
		},
	}
	users := []models.User{{ID: 1, UUID: "11111111-1111-1111-1111-111111111111", Status: models.StatusActive}}

	outbounds := []models.ServerOutbound{
		{
			ID:                 1,
			ServerID:           1,
			Tag:                "direct",
			Protocol:           "freedom",
			SettingsJSON:       `{"domainStrategy":"UseIP","userLevel":0}`,
			StreamSettingsJSON: `{"sockopt":{"mark":255,"tcpFastOpen":true}}`,
			Enabled:            true,
		},
		{
			ID:           2,
			ServerID:     1,
			Tag:          "blocked",
			Protocol:     "blackhole",
			SettingsJSON: `{"response":{"type":"http"}}`,
			Enabled:      true,
		},
		{
			ID:                 3,
			ServerID:           1,
			Tag:                "outbound-vless-grpc",
			Protocol:           "vless",
			SettingsJSON:       `{"vnext":[{"address":"remote.proxy.com","port":443,"users":[{"id":"33333333-3333-3333-3333-333333333333","encryption":"none"}]}]}`,
			StreamSettingsJSON: `{"network":"grpc","security":"tls","grpcSettings":{"serviceName":"out-grpc-svc"},"tlsSettings":{"serverName":"remote.proxy.com"},"sockopt":{"mark":100}}`,
			SendThrough:        "192.168.1.100",
			Enabled:            true,
		},
	}

	rawCfg, err := xray.Generate(srv, inbounds, outbounds, nil, users)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	obs, ok := parsed["outbounds"].([]any)
	if !ok || len(obs) != 3 {
		t.Fatalf("expected exactly 3 outbounds (user freedom, blackhole, proxy; no fallback duplicates), got %d", len(obs))
	}

	// Verify freedom
	obDirect := obs[0].(map[string]any)
	if obDirect["tag"] != "direct" || obDirect["protocol"] != "freedom" {
		t.Errorf("outbound 0 mismatch: %v", obDirect)
	}
	settingsDirect := obDirect["settings"].(map[string]any)
	if settingsDirect["domainStrategy"] != "UseIP" {
		t.Errorf("freedom domainStrategy mismatch: %v", settingsDirect)
	}

	// Verify blackhole
	obBlocked := obs[1].(map[string]any)
	if obBlocked["tag"] != "blocked" || obBlocked["protocol"] != "blackhole" {
		t.Errorf("outbound 1 mismatch: %v", obBlocked)
	}

	// Verify proxy outbound
	obProxy := obs[2].(map[string]any)
	if obProxy["tag"] != "outbound-vless-grpc" || obProxy["sendThrough"] != "192.168.1.100" {
		t.Errorf("outbound proxy mismatch: %v", obProxy)
	}
	streamProxy := obProxy["streamSettings"].(map[string]any)
	if streamProxy["network"] != "grpc" || streamProxy["security"] != "tls" {
		t.Errorf("outbound proxy streamSettings mismatch: %v", streamProxy)
	}
}

func TestGenerateConfig_RichRoutingRules(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-Routing", NodeID: "node-routing"}

	inbounds := []models.Inbound{
		{ID: 1, Tag: "vless-in", Protocol: "vless", Port: 443, Network: "tcp", TLSType: "none", Enabled: true},
	}
	users := []models.User{{ID: 1, UUID: "11111111-1111-1111-1111-111111111111", Status: models.StatusActive}}

	routingRules := []models.ServerRoutingRule{
		{
			ID:          1,
			ServerID:    1,
			OutboundTag: "direct",
			Domain:      "geosite:cn, geosite:apple\ndomain:internal.local",
			IP:          "geoip:private\n10.0.0.0/8, 172.16.0.0/12",
			Port:        "80,443,8080-8090",
			Network:     "tcp,udp",
			Enabled:     true,
		},
		{
			ID:          2,
			ServerID:    1,
			OutboundTag: "blocked",
			Domain:      `["geosite:category-ads-all","geosite:malware"]`,
			IP:          `["1.1.1.1/32","8.8.8.8/32"]`,
			Enabled:     true,
		},
		{
			ID:          3,
			ServerID:    1,
			OutboundTag: "proxy",
			RuleJSON:    `{"type":"field","inboundTag":["vless-in"],"domain":["geosite:google"],"protocol":["http","tls"],"outboundTag":"proxy"}`,
			Enabled:     true,
		},
	}

	rawCfg, err := xray.Generate(srv, inbounds, nil, routingRules, users)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	rt := asObject(t, parsed["routing"], "routing")
	rules := asArray(t, rt["rules"], "routing.rules")

	// Total rules = 1 (system API) + 3 (custom) = 4
	if len(rules) != 4 {
		t.Fatalf("expected 4 routing rules, got %d", len(rules))
	}

	// System API rule check
	ruleAPI := asObject(t, rules[0], "rules[0]")
	if ruleAPI["outboundTag"] != "api" {
		t.Errorf("rule 0 expected outboundTag api, got %v", ruleAPI)
	}

	// Custom rule 1 check (comma/newline string list)
	r1 := asObject(t, rules[1], "rules[1]")
	domains1 := asArray(t, r1["domain"], "rules[1].domain")
	if len(domains1) != 3 || r1["port"] != "80,443,8080-8090" || r1["network"] != "tcp,udp" {
		t.Errorf("rule 1 fields mismatch: %v", r1)
	}

	// Custom rule 2 check (JSON array string list)
	r2 := asObject(t, rules[2], "rules[2]")
	domains2 := asArray(t, r2["domain"], "rules[2].domain")
	if len(domains2) != 2 || r2["outboundTag"] != "blocked" {
		t.Errorf("rule 2 fields mismatch: %v", r2)
	}

	// Custom rule 3 check (RuleJSON)
	r3 := asObject(t, rules[3], "rules[3]")
	if r3["inboundTag"] == nil || r3["protocol"] == nil || r3["outboundTag"] != "proxy" {
		t.Errorf("rule 3 RuleJSON mismatch: %v", r3)
	}
}

func TestValidateSettings_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		settings  *xray.InboundSettings
		network   string
		tlsType   string
		expectErr bool
		errMsg    string
	}{
		{
			name:      "nil settings with tcp and empty tls",
			settings:  nil,
			network:   "tcp",
			tlsType:   "",
			expectErr: false,
		},
		{
			name:      "ws network with nil ws settings",
			settings:  &xray.InboundSettings{},
			network:   "ws",
			tlsType:   "",
			expectErr: true,
			errMsg:    "ws 传输需要配置 path",
		},
		{
			name:      "ws network with empty path",
			settings:  &xray.InboundSettings{WS: &xray.WSSettings{Path: ""}},
			network:   "ws",
			tlsType:   "",
			expectErr: true,
			errMsg:    "ws 传输需要配置 path",
		},
		{
			name:      "ws network valid path",
			settings:  &xray.InboundSettings{WS: &xray.WSSettings{Path: "/ws"}},
			network:   "ws",
			tlsType:   "",
			expectErr: false,
		},
		{
			name:      "grpc network with nil grpc settings",
			settings:  &xray.InboundSettings{},
			network:   "grpc",
			tlsType:   "",
			expectErr: true,
			errMsg:    "grpc 传输需要配置 serviceName",
		},
		{
			name:      "grpc network with empty serviceName",
			settings:  &xray.InboundSettings{GRPC: &xray.GRPCSettings{ServiceName: ""}},
			network:   "grpc",
			tlsType:   "",
			expectErr: true,
			errMsg:    "grpc 传输需要配置 serviceName",
		},
		{
			name:      "grpc network valid serviceName",
			settings:  &xray.InboundSettings{GRPC: &xray.GRPCSettings{ServiceName: "my-service"}},
			network:   "grpc",
			tlsType:   "",
			expectErr: false,
		},
		{
			name:      "grpc network whitespace-only serviceName",
			settings:  &xray.InboundSettings{GRPC: &xray.GRPCSettings{ServiceName: "   "}},
			network:   "grpc",
			tlsType:   "",
			expectErr: true,
			errMsg:    "grpc 传输需要配置 serviceName",
		},
		{
			name:      "grpc network tab-newline-only serviceName",
			settings:  &xray.InboundSettings{GRPC: &xray.GRPCSettings{ServiceName: "\t\n"}},
			network:   "grpc",
			tlsType:   "",
			expectErr: true,
			errMsg:    "grpc 传输需要配置 serviceName",
		},
		{
			name:      "grpc network control-char serviceName",
			settings:  &xray.InboundSettings{GRPC: &xray.GRPCSettings{ServiceName: "svc\x00evil"}},
			network:   "grpc",
			tlsType:   "",
			expectErr: true,
			errMsg:    "grpc serviceName 不能包含控制字符",
		},
		{
			name:      "grpc network NUL-only serviceName",
			settings:  &xray.InboundSettings{GRPC: &xray.GRPCSettings{ServiceName: "\x00\x00"}},
			network:   "grpc",
			tlsType:   "",
			expectErr: true,
			errMsg:    "grpc serviceName 不能包含控制字符",
		},
		{
			name:      "grpc network padded valid serviceName",
			settings:  &xray.InboundSettings{GRPC: &xray.GRPCSettings{ServiceName: "  svc  "}},
			network:   "grpc",
			tlsType:   "",
			expectErr: false,
		},
		{
			name:      "xhttp network missing mode",
			settings:  &xray.InboundSettings{XHTTP: &xray.XHTTPSettings{Path: "/xhttp"}},
			network:   "xhttp",
			tlsType:   "",
			expectErr: true,
			errMsg:    "xhttp 传输需要配置 mode 和 path",
		},
		{
			name:      "xhttp network valid",
			settings:  &xray.InboundSettings{XHTTP: &xray.XHTTPSettings{Mode: "auto", Path: "/xhttp"}},
			network:   "xhttp",
			tlsType:   "",
			expectErr: false,
		},
		{
			name:      "unsupported network kcp",
			settings:  &xray.InboundSettings{},
			network:   "kcp",
			tlsType:   "",
			expectErr: true,
			errMsg:    `暂不支持传输层 "kcp"`,
		},
		{
			name:      "reality tls missing private_key",
			settings:  &xray.InboundSettings{Reality: &xray.RealitySettings{ServerName: "example.com", PublicKey: "pub", Dest: "1.1.1.1:443"}},
			network:   "tcp",
			tlsType:   "reality",
			expectErr: true,
			errMsg:    "reality 需要配置 server_name / public_key / private_key / dest",
		},
		{
			name:      "reality tls valid",
			settings:  &xray.InboundSettings{Reality: &xray.RealitySettings{ServerName: "example.com", PublicKey: "pub", PrivateKey: "priv", Dest: "1.1.1.1:443"}},
			network:   "tcp",
			tlsType:   "reality",
			expectErr: false,
		},
		{
			name:      "tls missing key_file",
			settings:  &xray.InboundSettings{TLS: &xray.TLSSettings{CertFile: "/etc/cert.pem"}},
			network:   "tcp",
			tlsType:   "tls",
			expectErr: true,
			errMsg:    "tls 需要配置 cert_file 和 key_file",
		},
		{
			name:      "tls valid",
			settings:  &xray.InboundSettings{TLS: &xray.TLSSettings{CertFile: "/etc/cert.pem", KeyFile: "/etc/key.pem"}},
			network:   "tcp",
			tlsType:   "tls",
			expectErr: false,
		},
		{
			name:      "unsupported tls type",
			settings:  &xray.InboundSettings{},
			network:   "tcp",
			tlsType:   "bogus_tls",
			expectErr: true,
			errMsg:    `暂不支持 TLS 类型 "bogus_tls"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := xray.ValidateSettings(tt.settings, tt.network, tt.tlsType)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errMsg)
				}
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestGenerateConfig_ErrorHandling(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-Err"}

	t.Run("invalid settings_json syntax", func(t *testing.T) {
		inbounds := []models.Inbound{
			{ID: 1, Tag: "in1", Protocol: "vless", Port: 443, Network: "tcp", TLSType: "none", SettingsJSON: "{invalid-json", Enabled: true},
		}
		users := []models.User{{ID: 1, UUID: "11111111-1111-1111-1111-111111111111", Status: models.StatusActive}}

		_, err := xray.Generate(srv, inbounds, nil, nil, users)
		if err == nil {
			t.Errorf("expected error for malformed settings_json, got nil")
		}
	})

	t.Run("unsupported inbound protocol", func(t *testing.T) {
		inbounds := []models.Inbound{
			{ID: 2, Tag: "in2", Protocol: "vmess", Port: 443, Network: "tcp", TLSType: "none", Enabled: true},
		}
		users := []models.User{{ID: 1, UUID: "11111111-1111-1111-1111-111111111111", Status: models.StatusActive}}

		_, err := xray.Generate(srv, inbounds, nil, nil, users)
		if err == nil {
			t.Errorf("expected error for unsupported protocol vmess, got nil")
		}
	})

	t.Run("no active users", func(t *testing.T) {
		inbounds := []models.Inbound{
			{ID: 3, Tag: "in3", Protocol: "vless", Port: 443, Network: "tcp", TLSType: "none", Enabled: true},
		}
		users := []models.User{
			{ID: 1, UUID: "", Status: models.StatusActive},        // empty UUID
			{ID: 2, UUID: "uuid2", Status: models.StatusDisabled}, // disabled user
		}

		_, err := xray.Generate(srv, inbounds, nil, nil, users)
		if err == nil {
			t.Errorf("expected error for no active users, got nil")
		}
	})
}

func genInbound(t *testing.T, srv *models.Server, inb models.Inbound, users []models.User) (map[string]any, error) {
	t.Helper()
	raw, err := xray.Generate(srv, []models.Inbound{inb}, nil, nil, users)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		t.Fatalf("expected non-nil config on success")
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("Unmarshal config JSON failed: %v", err)
	}
	return parsed, nil
}

func vlessTestUser() []models.User {
	return []models.User{{ID: 1, UUID: "11111111-1111-1111-1111-111111111111", Status: models.StatusActive}}
}

func TestGenerateConfig_GRPCRequiredServiceName(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-gRPC-Err"}

	cases := []struct {
		name     string
		settings string
		wantErr  string
	}{
		{name: "grpc-null", settings: `{"grpc":null}`, wantErr: "grpc 传输需要配置 serviceName"},
		{name: "grpc-empty-object", settings: `{"grpc":{}}`, wantErr: "grpc 传输需要配置 serviceName"},
		{name: "grpc-empty-service-name", settings: `{"grpc":{"serviceName":""}}`, wantErr: "grpc 传输需要配置 serviceName"},
		{name: "grpc-whitespace-service-name", settings: `{"grpc":{"serviceName":"   "}}`, wantErr: "grpc 传输需要配置 serviceName"},
		{name: "grpc-tab-newline-service-name", settings: `{"grpc":{"serviceName":"\t\n"}}`, wantErr: "grpc 传输需要配置 serviceName"},
		{name: "grpc-only-authority", settings: `{"grpc":{"authority":"grpc.example.com"}}`, wantErr: "grpc 传输需要配置 serviceName"},
		{name: "grpc-only-multi-mode", settings: `{"grpc":{"multiMode":true}}`, wantErr: "grpc 传输需要配置 serviceName"},
		{name: "grpc-only-snake-multi-mode", settings: `{"grpc":{"multi_mode":true}}`, wantErr: "grpc 传输需要配置 serviceName"},
		{name: "grpc-missing-settings", settings: `{}`, wantErr: "grpc 传输需要配置 serviceName"},
		{name: "grpc-empty-settings-json", settings: ``, wantErr: "grpc 传输需要配置 serviceName"},
		{name: "grpc-control-char-service-name", settings: `{"grpc":{"serviceName":"svc\u0000evil"}}`, wantErr: "grpc serviceName 不能包含控制字符"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inb := models.Inbound{ID: 1, ServerID: 1, Tag: "in-grpc", Protocol: "vless", Port: 443, Network: "grpc", TLSType: "none", SettingsJSON: tc.settings, Enabled: true}
			raw, err := xray.Generate(srv, []models.Inbound{inb}, nil, nil, vlessTestUser())
			if err == nil {
				t.Fatalf("expected error for settings %q, got nil", tc.settings)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %v", tc.wantErr, err)
			}
			if raw != nil {
				t.Errorf("expected no config emitted on error, got %d bytes", len(raw))
			}
		})
	}
}

func TestGenerateConfig_RealityMissingRequiredFields(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-REALITY-Err"}

	cases := []struct {
		name     string
		settings string
		wantErr  string
	}{
		{name: "reality-nil", settings: `{}`, wantErr: "reality 需要配置 server_name / public_key / private_key / dest"},
		{name: "missing-server-name", settings: `{"reality":{"public_key":"pk","private_key":"sk","dest":"1.1.1.1:443"}}`, wantErr: "reality 需要配置 server_name"},
		{name: "whitespace-server-name", settings: `{"reality":{"server_name":"   ","public_key":"pk","private_key":"sk","dest":"1.1.1.1:443"}}`, wantErr: "reality 需要配置 server_name"},
		{name: "missing-public-key", settings: `{"reality":{"server_name":"ex.com","private_key":"sk","dest":"1.1.1.1:443"}}`, wantErr: "reality 需要配置 public_key"},
		{name: "missing-private-key", settings: `{"reality":{"server_name":"ex.com","public_key":"pk","dest":"1.1.1.1:443"}}`, wantErr: "reality 需要配置 private_key"},
		{name: "missing-dest", settings: `{"reality":{"server_name":"ex.com","public_key":"pk","private_key":"sk"}}`, wantErr: "reality 需要配置 dest"},
		{name: "whitespace-dest", settings: `{"reality":{"server_name":"ex.com","public_key":"pk","private_key":"sk","dest":"   "}}`, wantErr: "reality 需要配置 dest"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inb := models.Inbound{ID: 1, ServerID: 1, Tag: "in-reality", Protocol: "vless", Port: 443, Network: "tcp", TLSType: "reality", SettingsJSON: tc.settings, Enabled: true}
			raw, err := xray.Generate(srv, []models.Inbound{inb}, nil, nil, vlessTestUser())
			if err == nil {
				t.Fatalf("expected error for settings %q, got nil", tc.settings)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %v", tc.wantErr, err)
			}
			if raw != nil {
				t.Errorf("expected no config emitted on error, got %d bytes", len(raw))
			}
		})
	}
}

func TestGenerateConfig_TLSMissingRequiredFiles(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-TLS-Err"}

	cases := []struct {
		name     string
		settings string
		wantErr  string
	}{
		{name: "tls-nil", settings: `{}`, wantErr: "tls 需要配置 cert_file 和 key_file"},
		{name: "cert-only", settings: `{"tls":{"cert_file":"/etc/cert.pem"}}`, wantErr: "tls 需要配置 key_file"},
		{name: "key-only", settings: `{"tls":{"key_file":"/etc/key.pem"}}`, wantErr: "tls 需要配置 cert_file"},
		{name: "whitespace-cert", settings: `{"tls":{"cert_file":"   ","key_file":"/etc/key.pem"}}`, wantErr: "tls 需要配置 cert_file"},
		{name: "whitespace-key", settings: `{"tls":{"cert_file":"/etc/cert.pem","key_file":"\t"}}`, wantErr: "tls 需要配置 key_file"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inb := models.Inbound{ID: 1, ServerID: 1, Tag: "in-tls", Protocol: "vless", Port: 443, Network: "tcp", TLSType: "tls", SettingsJSON: tc.settings, Enabled: true}
			raw, err := xray.Generate(srv, []models.Inbound{inb}, nil, nil, vlessTestUser())
			if err == nil {
				t.Fatalf("expected error for settings %q, got nil", tc.settings)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got %v", tc.wantErr, err)
			}
			if raw != nil {
				t.Errorf("expected no config emitted on error, got %d bytes", len(raw))
			}
		})
	}
}

func TestGRPCSettingsUnmarshalJSON(t *testing.T) {
	parse := func(t *testing.T, payload string) xray.GRPCSettings {
		t.Helper()
		var g xray.GRPCSettings
		if err := json.Unmarshal([]byte(payload), &g); err != nil {
			t.Fatalf("unmarshal %q failed: %v", payload, err)
		}
		return g
	}

	t.Run("snake-case service_name parsed", func(t *testing.T) {
		g := parse(t, `{"service_name":"snake-svc"}`)
		if g.ServiceName != "snake-svc" {
			t.Errorf("ServiceName = %q, want %q", g.ServiceName, "snake-svc")
		}
	})

	t.Run("snake-case multi_mode parsed", func(t *testing.T) {
		g := parse(t, `{"service_name":"svc","multi_mode":true}`)
		if !g.MultiMode {
			t.Errorf("MultiMode = false, want true (multi_mode snake_case must not be dropped)")
		}
	})

	t.Run("camelCase multiMode parsed", func(t *testing.T) {
		g := parse(t, `{"serviceName":"svc","multiMode":true}`)
		if !g.MultiMode {
			t.Errorf("MultiMode = false, want true")
		}
	})

	t.Run("both serviceName keys camelCase wins", func(t *testing.T) {
		g := parse(t, `{"serviceName":"camel-svc","service_name":"snake-svc"}`)
		if g.ServiceName != "camel-svc" {
			t.Errorf("ServiceName = %q, want %q", g.ServiceName, "camel-svc")
		}
	})

	t.Run("both serviceName keys snake wins when camel empty", func(t *testing.T) {
		g := parse(t, `{"serviceName":"","service_name":"snake-svc"}`)
		if g.ServiceName != "snake-svc" {
			t.Errorf("ServiceName = %q, want %q", g.ServiceName, "snake-svc")
		}
	})

	t.Run("both multiMode keys camelCase wins", func(t *testing.T) {
		g := parse(t, `{"serviceName":"svc","multiMode":false,"multi_mode":true}`)
		if g.MultiMode {
			t.Errorf("MultiMode = true, want false (camelCase false must win over snake_case true)")
		}
	})

	t.Run("both multiMode keys camelCase true wins", func(t *testing.T) {
		g := parse(t, `{"serviceName":"svc","multiMode":true,"multi_mode":false}`)
		if !g.MultiMode {
			t.Errorf("MultiMode = false, want true (camelCase true must win over snake_case false)")
		}
	})

	t.Run("authority parsed", func(t *testing.T) {
		g := parse(t, `{"serviceName":"svc","authority":"grpc.example.com"}`)
		if g.Authority != "grpc.example.com" {
			t.Errorf("Authority = %q, want %q", g.Authority, "grpc.example.com")
		}
	})

	t.Run("authority with snake multi_mode parsed", func(t *testing.T) {
		g := parse(t, `{"service_name":"svc","authority":"grpc.example.com","multi_mode":true}`)
		if g.ServiceName != "svc" || g.Authority != "grpc.example.com" || !g.MultiMode {
			t.Errorf("GRPCSettings = %+v, want svc/grpc.example.com/multi", g)
		}
	})

	t.Run("null input yields zero value", func(t *testing.T) {
		g := parse(t, `null`)
		if g.ServiceName != "" || g.Authority != "" || g.MultiMode {
			t.Errorf("expected zero-value GRPCSettings for null, got %+v", g)
		}
	})

	t.Run("malformed JSON rejected", func(t *testing.T) {
		var g xray.GRPCSettings
		if err := json.Unmarshal([]byte(`{"serviceName":`), &g); err == nil {
			t.Errorf("expected error for malformed JSON, got nil")
		}
	})
}

func TestGenerateConfig_TCPWithFallbacks(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-Fallbacks", NodeID: "node-fallbacks"}

	// TEST_INFRA.md §4.1 "VLESS + TCP + Fallbacks" payload
	settings := `{"tls":{"server_name":"main.example.com","cert_file":"/etc/ssl/certs/main.crt","key_file":"/etc/ssl/private/main.key"},"fallbacks":[{"dest":"8080","xver":1},{"path":"/web","dest":"8081","xver":1}]}`
	inb := models.Inbound{ID: 1, ServerID: 1, Tag: "vless-fallback-tcp", Protocol: "vless", Port: 50443, Network: "tcp", TLSType: "tls", SettingsJSON: settings, Enabled: true}

	parsed, err := genInbound(t, srv, inb, vlessTestUser())
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	inbounds := asArray(t, parsed["inbounds"], "inbounds")
	vlessIn := asObject(t, inbounds[0], "inbounds[0]")
	inSettings := asObject(t, vlessIn["settings"], "inbounds[0].settings")

	fbs := asArray(t, inSettings["fallbacks"], "settings.fallbacks")
	if len(fbs) != 2 {
		t.Fatalf("expected 2 fallbacks, got %d", len(fbs))
	}

	fb0 := asObject(t, fbs[0], "fallbacks[0]")
	if fb0["dest"] != "8080" {
		t.Errorf("fallbacks[0].dest = %v, want %q", fb0["dest"], "8080")
	}
	if fb0["xver"] != float64(1) {
		t.Errorf("fallbacks[0].xver = %v, want 1", fb0["xver"])
	}
	if _, has := fb0["path"]; has {
		t.Errorf("fallbacks[0] should not carry path, got %v", fb0["path"])
	}

	fb1 := asObject(t, fbs[1], "fallbacks[1]")
	if fb1["dest"] != "8081" || fb1["path"] != "/web" || fb1["xver"] != float64(1) {
		t.Errorf("fallbacks[1] = %v, want dest=8081 path=/web xver=1", fb1)
	}
}

func TestGenerateConfig_FallbacksOmittedWhenEmpty(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-NoFallbacks"}

	cases := []struct {
		name     string
		network  string
		tlsType  string
		settings string
	}{
		{name: "tcp-empty-array", network: "tcp", tlsType: "tls", settings: `{"tls":{"server_name":"main.example.com","cert_file":"/etc/cert.pem","key_file":"/etc/key.pem"},"fallbacks":[]}`},
		{name: "tcp-no-key", network: "tcp", tlsType: "tls", settings: `{"tls":{"server_name":"main.example.com","cert_file":"/etc/cert.pem","key_file":"/etc/key.pem"}}`},
		{name: "grpc-fallbacks-ignored", network: "grpc", tlsType: "none", settings: `{"grpc":{"serviceName":"svc"},"fallbacks":[{"dest":"8080","xver":1}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inb := models.Inbound{ID: 1, ServerID: 1, Tag: "in", Protocol: "vless", Port: 443, Network: tc.network, TLSType: tc.tlsType, SettingsJSON: tc.settings, Enabled: true}
			parsed, err := genInbound(t, srv, inb, vlessTestUser())
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}

			inbounds := asArray(t, parsed["inbounds"], "inbounds")
			vlessIn := asObject(t, inbounds[0], "inbounds[0]")
			inSettings := asObject(t, vlessIn["settings"], "inbounds[0].settings")

			if _, has := inSettings["fallbacks"]; has {
				t.Errorf("expected no fallbacks key for case %q, got %v", tc.name, inSettings["fallbacks"])
			}
		})
	}
}

// D1: GRPCSettings 存储层序列化必须输出 snake_case（service_name/multi_mode），
// 且 marshal → unmarshal 往返不丢字段（前端表单只解析 snake_case）。
func TestGRPCSettingsMarshalJSON_SnakeCaseRoundTrip(t *testing.T) {
	g := xray.GRPCSettings{ServiceName: "my-svc", Authority: "grpc.example.com", MultiMode: true}

	b, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Unmarshal raw failed: %v", err)
	}
	if raw["service_name"] != "my-svc" {
		t.Errorf("expected snake_case service_name, got %v", raw)
	}
	if raw["multi_mode"] != true {
		t.Errorf("expected snake_case multi_mode, got %v", raw)
	}
	if raw["authority"] != "grpc.example.com" {
		t.Errorf("expected authority, got %v", raw)
	}
	if _, has := raw["serviceName"]; has {
		t.Errorf("camelCase serviceName must not appear in stored JSON, got %v", raw)
	}
	if _, has := raw["multiMode"]; has {
		t.Errorf("camelCase multiMode must not appear in stored JSON, got %v", raw)
	}

	// 往返：marshal（snake_case）→ unmarshal（双写兼容）→ 字段保留
	var back xray.GRPCSettings
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("Unmarshal round-trip failed: %v", err)
	}
	if back.ServiceName != "my-svc" {
		t.Errorf("round-trip ServiceName = %q, want %q", back.ServiceName, "my-svc")
	}
	if back.Authority != "grpc.example.com" {
		t.Errorf("round-trip Authority = %q, want %q", back.Authority, "grpc.example.com")
	}
	if !back.MultiMode {
		t.Errorf("round-trip MultiMode = false, want true")
	}

	// 通过 InboundSettings 整体 marshal（marshalSettings 实际路径）同样输出 snake_case
	s := xray.InboundSettings{GRPC: &g}
	sb, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal InboundSettings failed: %v", err)
	}
	if !strings.Contains(string(sb), `"service_name":"my-svc"`) || !strings.Contains(string(sb), `"multi_mode":true`) {
		t.Errorf("InboundSettings marshal must carry snake_case grpc keys, got %s", sb)
	}
	if strings.Contains(string(sb), "serviceName") || strings.Contains(string(sb), "multiMode") {
		t.Errorf("InboundSettings marshal must not carry camelCase grpc keys, got %s", sb)
	}
}

// D2: SniffingSettings 双写解析（camelCase 前端发送 / snake_case 存储），camelCase 优先。
func TestSniffingSettingsUnmarshalJSON(t *testing.T) {
	parse := func(t *testing.T, payload string) xray.SniffingSettings {
		t.Helper()
		var s xray.SniffingSettings
		if err := json.Unmarshal([]byte(payload), &s); err != nil {
			t.Fatalf("unmarshal %q failed: %v", payload, err)
		}
		return s
	}

	t.Run("camelCase keys parsed", func(t *testing.T) {
		s := parse(t, `{"enabled":true,"destOverride":["http","tls"],"metadataOnly":true,"routeOnly":true}`)
		if !s.Enabled || len(s.DestOverride) != 2 || s.DestOverride[0] != "http" || !s.MetadataOnly || !s.RouteOnly {
			t.Errorf("camelCase parse mismatch: %+v", s)
		}
	})

	t.Run("snake_case keys parsed", func(t *testing.T) {
		s := parse(t, `{"enabled":true,"dest_override":["http","tls"],"metadata_only":true,"route_only":true}`)
		if !s.Enabled || len(s.DestOverride) != 2 || s.DestOverride[0] != "http" || !s.MetadataOnly || !s.RouteOnly {
			t.Errorf("snake_case parse mismatch: %+v", s)
		}
	})

	t.Run("camelCase wins when both present", func(t *testing.T) {
		s := parse(t, `{"enabled":true,"destOverride":["http"],"dest_override":["quic"],"routeOnly":true,"route_only":false}`)
		if len(s.DestOverride) != 1 || s.DestOverride[0] != "http" {
			t.Errorf("destOverride camelCase should win: %+v", s.DestOverride)
		}
		if !s.RouteOnly {
			t.Errorf("routeOnly camelCase true should win over snake false: %+v", s)
		}
	})

	t.Run("snake_case fallback when camel empty", func(t *testing.T) {
		s := parse(t, `{"enabled":true,"dest_override":["quic"],"route_only":true}`)
		if len(s.DestOverride) != 1 || s.DestOverride[0] != "quic" || !s.RouteOnly {
			t.Errorf("snake fallback mismatch: %+v", s)
		}
	})

	t.Run("empty object zero value", func(t *testing.T) {
		s := parse(t, `{}`)
		if s.Enabled || len(s.DestOverride) != 0 || s.MetadataOnly || s.RouteOnly {
			t.Errorf("expected zero value, got %+v", s)
		}
	})

	t.Run("null input yields zero value", func(t *testing.T) {
		s := parse(t, `null`)
		if s.Enabled || len(s.DestOverride) != 0 || s.MetadataOnly || s.RouteOnly {
			t.Errorf("expected zero value for null, got %+v", s)
		}
	})

	t.Run("malformed JSON rejected", func(t *testing.T) {
		var s xray.SniffingSettings
		if err := json.Unmarshal([]byte(`{"enabled":`), &s); err == nil {
			t.Errorf("expected error for malformed JSON, got nil")
		}
	})
}

// D2: 嗅探写线格式为入站顶层 sniffing 对象（camelCase wire keys）；未配置时不输出。
func TestGenerateConfig_Sniffing(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-Sniffing", NodeID: "node-sniffing"}

	t.Run("with sniffing emits camelCase wire keys", func(t *testing.T) {
		inb := models.Inbound{ID: 1, ServerID: 1, Tag: "in-sniff", Protocol: "vless", Port: 443,
			Network: "ws", TLSType: "none",
			SettingsJSON: `{"ws":{"path":"/ws"},"sniffing":{"enabled":true,"dest_override":["http","tls","quic"],"route_only":true}}`,
			Enabled:      true}
		parsed, err := genInbound(t, srv, inb, vlessTestUser())
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		inbounds := asArray(t, parsed["inbounds"], "inbounds")
		vlessIn := asObject(t, inbounds[0], "inbounds[0]")
		sniff, ok := vlessIn["sniffing"].(map[string]any)
		if !ok {
			t.Fatalf("expected inbound-level sniffing object, got %v", vlessIn["sniffing"])
		}
		if sniff["enabled"] != true {
			t.Errorf("sniffing.enabled = %v, want true", sniff["enabled"])
		}
		destOverride := asArray(t, sniff["destOverride"], "sniffing.destOverride")
		if len(destOverride) != 3 || destOverride[0] != "http" {
			t.Errorf("sniffing.destOverride = %v, want [http tls quic]", destOverride)
		}
		if sniff["routeOnly"] != true {
			t.Errorf("sniffing.routeOnly = %v, want true", sniff["routeOnly"])
		}
		if _, has := sniff["dest_override"]; has {
			t.Errorf("wire key must be camelCase destOverride, got %v", sniff)
		}
		if _, has := sniff["route_only"]; has {
			t.Errorf("wire key must be camelCase routeOnly, got %v", sniff)
		}
	})

	t.Run("sniffing omitted when nil", func(t *testing.T) {
		inb := models.Inbound{ID: 1, ServerID: 1, Tag: "in-no-sniff", Protocol: "vless", Port: 443,
			Network: "ws", TLSType: "none", SettingsJSON: `{"ws":{"path":"/ws"}}`, Enabled: true}
		parsed, err := genInbound(t, srv, inb, vlessTestUser())
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		inbounds := asArray(t, parsed["inbounds"], "inbounds")
		vlessIn := asObject(t, inbounds[0], "inbounds[0]")
		if _, has := vlessIn["sniffing"]; has {
			t.Errorf("expected no sniffing key, got %v", vlessIn["sniffing"])
		}
	})

	t.Run("camelCase frontend payload parsed", func(t *testing.T) {
		inb := models.Inbound{ID: 1, ServerID: 1, Tag: "in-sniff-camel", Protocol: "vless", Port: 443,
			Network: "tcp", TLSType: "none",
			SettingsJSON: `{"sniffing":{"enabled":true,"destOverride":["fakedns"],"metadataOnly":true,"routeOnly":false}}`,
			Enabled:      true}
		parsed, err := genInbound(t, srv, inb, vlessTestUser())
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		inbounds := asArray(t, parsed["inbounds"], "inbounds")
		vlessIn := asObject(t, inbounds[0], "inbounds[0]")
		sniff := asObject(t, vlessIn["sniffing"], "inbounds[0].sniffing")
		destOverride := asArray(t, sniff["destOverride"], "sniffing.destOverride")
		if len(destOverride) != 1 || destOverride[0] != "fakedns" || sniff["metadataOnly"] != true {
			t.Errorf("camelCase frontend payload mismatch: %v", sniff)
		}
	})
}

// D3: 路由规则 inbound_tag 写入 xray wire 键 inboundTag（数组）。
func TestGenerateConfig_RoutingInboundTag(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-RoutingInboundTag", NodeID: "node-routing-inbound-tag"}

	inbounds := []models.Inbound{
		{ID: 1, Tag: "vless-in", Protocol: "vless", Port: 443, Network: "tcp", TLSType: "none", Enabled: true},
	}
	users := []models.User{{ID: 1, UUID: "11111111-1111-1111-1111-111111111111", Status: models.StatusActive}}

	routingRules := []models.ServerRoutingRule{
		{
			ID:          1,
			ServerID:    1,
			OutboundTag: "direct",
			InboundTag:  "vless-in, api",
			Enabled:     true,
		},
		{
			ID:          2,
			ServerID:    1,
			OutboundTag: "blocked",
			InboundTag:  `["vless-in"]`,
			Enabled:     true,
		},
		{
			ID:          3,
			ServerID:    1,
			OutboundTag: "proxy",
			Enabled:     true,
		},
	}

	rawCfg, err := xray.Generate(srv, inbounds, nil, routingRules, users)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	rt := asObject(t, parsed["routing"], "routing")
	rules := asArray(t, rt["rules"], "routing.rules")
	if len(rules) != 4 {
		t.Fatalf("expected 4 routing rules (1 api + 3 custom), got %d", len(rules))
	}

	r1 := asObject(t, rules[1], "rules[1]")
	inbTags1 := asArray(t, r1["inboundTag"], "rules[1].inboundTag")
	if len(inbTags1) != 2 || inbTags1[0] != "vless-in" || inbTags1[1] != "api" {
		t.Errorf("rule 1 inboundTag = %v, want [vless-in api]", inbTags1)
	}

	r2 := asObject(t, rules[2], "rules[2]")
	inbTags2 := asArray(t, r2["inboundTag"], "rules[2].inboundTag")
	if len(inbTags2) != 1 || inbTags2[0] != "vless-in" {
		t.Errorf("rule 2 inboundTag = %v, want [vless-in]", inbTags2)
	}

	r3 := asObject(t, rules[3], "rules[3]")
	if _, has := r3["inboundTag"]; has {
		t.Errorf("rule 3 (no inbound_tag) must not carry inboundTag, got %v", r3["inboundTag"])
	}
}

// 加固回归：shortIds 必须始终输出。
// 实测 xray 26.6.27：realitySettings 缺失 shortIds → "empty shortIds" 拒绝（exit 23）；
// shortIds 含空字符串条目 ["", ...] → Configuration OK。因此即使 short_id 为空也不能省略。
func TestGenerateConfig_RealityShortIdsAlwaysEmitted(t *testing.T) {
	srv := &models.Server{ID: 1, Name: "Node-RealityNoShortID", NodeID: "node-reality-no-short-id"}

	cases := []struct {
		name     string
		shortID  string
		wantList []any
	}{
		{name: "empty short_id", shortID: "", wantList: []any{""}},
		{name: "with short_id", shortID: "6ba7b810", wantList: []any{"6ba7b810"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inb := models.Inbound{ID: 1, ServerID: 1, Tag: "in-reality", Protocol: "vless", Port: 443,
				Network: "tcp", TLSType: "reality",
				SettingsJSON: `{"reality":{"server_name":"example.com","public_key":"pk","private_key":"sk","dest":"1.1.1.1:443","short_id":"` + tc.shortID + `"}}`,
				Enabled:      true}

			parsed, err := genInbound(t, srv, inb, vlessTestUser())
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}
			inbounds := asArray(t, parsed["inbounds"], "inbounds")
			vlessIn := asObject(t, inbounds[0], "inbounds[0]")
			stream := asObject(t, vlessIn["streamSettings"], "inbounds[0].streamSettings")
			reality := asObject(t, stream["realitySettings"], "streamSettings.realitySettings")
			shortIds := asArray(t, reality["shortIds"], "realitySettings.shortIds")
			if len(shortIds) != 1 || shortIds[0] != tc.wantList[0] {
				t.Errorf("shortIds = %v, want %v", shortIds, tc.wantList)
			}
		})
	}
}
