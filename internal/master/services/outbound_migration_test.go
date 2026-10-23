package services

import (
	"encoding/json"
	"testing"

	"github.com/acdc-awa/xpanel/internal/models"
)

// 旧版「外部代理节点」出站（protocol + settings/stream）一次性折叠为 outbound_json。
// 覆盖：vless TLS / vless REALITY / hysteria / socks / 完整对象形态，以及四类不该动的行。
func TestMigrateLegacyProxyOutbounds(t *testing.T) {
	db := equivDB(t)
	ref := uint64(9001)
	rows := []models.ServerOutbound{
		{
			ServerID: 1, Tag: "vless-tls", Protocol: "vless", Enabled: true,
			SettingsJSON:       `{"vnext":[{"address":"1.2.3.4","port":443,"users":[{"id":"u1"}]}]}`,
			StreamSettingsJSON: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"x.example.com","allowInsecure":true,"fingerprint":"chrome"}}`,
		},
		{
			ServerID: 1, Tag: "vless-reality", Protocol: "vless", Enabled: true,
			SettingsJSON:       `{"vnext":[{"address":"5.6.7.8","port":443,"users":[{"id":"u2","flow":"xtls-rprx-vision"}]}]}`,
			StreamSettingsJSON: `{"network":"tcp","security":"reality","realitySettings":{"serverName":"r.example.com","publicKey":"pk","password":"pw","shortId":"0123","fingerprint":"chrome","spiderX":"/"}}`,
		},
		{
			ServerID: 1, Tag: "hy2", Protocol: "hysteria", Enabled: true,
			SettingsJSON:       `{"version":2,"address":"9.9.9.9","port":8443}`,
			StreamSettingsJSON: `{"network":"hysteria","security":"tls","tlsSettings":{"serverName":"h.example.com"},"hysteriaSettings":{"version":2,"auth":"secret"}}`,
		},
		{
			ServerID: 1, Tag: "socks", Protocol: "socks", Enabled: true,
			SettingsJSON:       `{"servers":[{"address":"10.0.0.1","port":1080}]}`,
			StreamSettingsJSON: `{"network":"tcp","security":"none"}`,
		},
		{
			ServerID: 1, Tag: "full-object", Protocol: "vmess", Enabled: true,
			SettingsJSON: `{"protocol":"vmess","settings":{"vnext":[{"address":"vm.example.com","port":443,"users":[{"id":"u3"}]}]}}`,
		},
		{ServerID: 1, Tag: "direct", Protocol: "freedom", Enabled: true, SettingsJSON: `{"domainStrategy":"AsIs"}`},
		{ServerID: 1, Tag: "blocked", Protocol: "blackhole", Enabled: true, SettingsJSON: `{"response":{"type":"none"}}`},
		{ServerID: 1, Tag: "already-custom", Protocol: "custom", Enabled: true, OutboundJSON: `{"protocol":"vless","settings":{}}`},
		{ServerID: 1, Tag: "ref-row", Protocol: "vless", Enabled: true, InboundRef: &ref},
		{ServerID: 1, Tag: "owned-row", Protocol: "freedom", Enabled: true, OwnerInboundID: 77, SettingsJSON: `{"redirect":"x:1"}`},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("create fixtures: %v", err)
	}

	if err := MigrateLegacyProxyOutbounds(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	load := func(tag string) models.ServerOutbound {
		t.Helper()
		var ob models.ServerOutbound
		if err := db.Where("server_id = ? AND tag = ?", 1, tag).First(&ob).Error; err != nil {
			t.Fatalf("load %s: %v", tag, err)
		}
		return ob
	}
	outboundJSON := func(ob models.ServerOutbound) map[string]any {
		t.Helper()
		if ob.Protocol != "custom" {
			t.Fatalf("%s protocol = %q, want custom", ob.Tag, ob.Protocol)
		}
		if ob.SettingsJSON != "" || ob.StreamSettingsJSON != "" {
			t.Fatalf("%s legacy columns should be cleared", ob.Tag)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(ob.OutboundJSON), &m); err != nil {
			t.Fatalf("%s outbound_json invalid: %v", ob.Tag, err)
		}
		return m
	}

	// vless TLS：设置原样、流被出站清洗（allowInsecure 去掉，fingerprint 保留）
	vlessTLS := outboundJSON(load("vless-tls"))
	if vlessTLS["protocol"] != "vless" {
		t.Errorf("vless-tls protocol = %v", vlessTLS["protocol"])
	}
	settings := vlessTLS["settings"].(map[string]any)
	if settings["vnext"] == nil {
		t.Error("vless-tls settings.vnext lost")
	}
	tls := vlessTLS["streamSettings"].(map[string]any)["tlsSettings"].(map[string]any)
	if _, has := tls["allowInsecure"]; has {
		t.Error("vless-tls allowInsecure should be stripped")
	}
	if tls["fingerprint"] != "chrome" {
		t.Errorf("vless-tls fingerprint = %v, want chrome", tls["fingerprint"])
	}

	// vless REALITY：客户端字段全部保留
	reality := outboundJSON(load("vless-reality"))
	r := reality["streamSettings"].(map[string]any)["realitySettings"].(map[string]any)
	for _, k := range []string{"publicKey", "password", "spiderX", "fingerprint", "shortId"} {
		if _, ok := r[k]; !ok {
			t.Errorf("vless-reality %s should be kept", k)
		}
	}

	// hysteria：auth 是客户端凭据，必须保留（旧生成链路会把它删掉）
	hy := outboundJSON(load("hy2"))
	hyStream := hy["streamSettings"].(map[string]any)
	if auth := hyStream["hysteriaSettings"].(map[string]any)["auth"]; auth != "secret" {
		t.Errorf("hy2 auth = %v, want secret", auth)
	}

	// socks：裸 settings 进 settings
	socks := outboundJSON(load("socks"))
	if socks["settings"].(map[string]any)["servers"] == nil {
		t.Error("socks settings.servers lost")
	}

	// 完整对象形态：顶层 protocol 保留
	full := outboundJSON(load("full-object"))
	if full["protocol"] != "vmess" {
		t.Errorf("full-object protocol = %v, want vmess", full["protocol"])
	}
	if full["settings"].(map[string]any)["vnext"] == nil {
		t.Error("full-object settings.vnext lost")
	}

	// 不该动的四类
	if ob := load("direct"); ob.Protocol != "freedom" || ob.OutboundJSON != "" {
		t.Errorf("direct should be untouched, got protocol=%q json=%q", ob.Protocol, ob.OutboundJSON)
	}
	if ob := load("blocked"); ob.Protocol != "blackhole" || ob.OutboundJSON != "" {
		t.Errorf("blocked should be untouched")
	}
	if ob := load("already-custom"); ob.Protocol != "custom" || ob.OutboundJSON == "" {
		t.Errorf("already-custom should be untouched")
	}
	if ob := load("ref-row"); ob.InboundRef == nil || ob.OutboundJSON != "" {
		t.Errorf("ref-row should be untouched")
	}
	if ob := load("owned-row"); ob.OwnerInboundID != 77 || ob.OutboundJSON != "" {
		t.Errorf("owned-row should be untouched")
	}

	// 幂等：再跑一次不改变结果
	before := load("hy2").OutboundJSON
	if err := MigrateLegacyProxyOutbounds(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if after := load("hy2").OutboundJSON; after != before {
		t.Error("migration should be idempotent")
	}
}
