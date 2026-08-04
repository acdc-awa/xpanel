package xray_test

import (
	"encoding/json"
	"testing"

	"github.com/zhx/xray-panel/internal/master/xray"
	"github.com/zhx/xray-panel/internal/models"
)

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
