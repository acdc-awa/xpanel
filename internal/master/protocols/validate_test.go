package protocols

import (
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
)

func hasIssue(issues []contracts.ValidationIssue, path, code string) bool {
	for _, is := range issues {
		if is.Path == path && is.Code == code {
			return true
		}
	}
	return false
}

func TestValidateInboundFields_VLESS(t *testing.T) {
	t.Run("合法 reality 无问题", func(t *testing.T) {
		inb := &models.Inbound{
			Protocol: models.ProtocolVless,
			StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"dest":"1.1.1.1:443",` +
				`"serverNames":["r.example.com"],"privateKey":"sk","shortIds":["abcd"]}}`,
			Flow: "",
		}
		if issues := ValidateInboundFields(inb); len(issues) != 0 {
			t.Fatalf("不应有问题: %+v", issues)
		}
	})

	t.Run("reality 缺必填项", func(t *testing.T) {
		inb := &models.Inbound{
			Protocol:       models.ProtocolVless,
			StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{}}`,
		}
		issues := ValidateInboundFields(inb)
		for _, path := range []string{
			"stream.realitySettings.privateKey",
			"stream.realitySettings.dest",
			"stream.realitySettings.serverNames",
			"stream.realitySettings.shortIds",
		} {
			if !hasIssue(issues, path, "required") {
				t.Errorf("缺少 required 问题: %s（got %+v）", path, issues)
			}
		}
	})

	t.Run("security=none 携带 TLS 残留", func(t *testing.T) {
		inb := &models.Inbound{
			Protocol:       models.ProtocolVless,
			StreamSettings: `{"network":"tcp","security":"none","tlsSettings":{"serverName":"x.com"}}`,
		}
		if issues := ValidateInboundFields(inb); !hasIssue(issues, "stream.tlsSettings", "forbidden") {
			t.Errorf("应拒绝 TLS 残留: %+v", issues)
		}
	})

	t.Run("xhttp 不支持 vision", func(t *testing.T) {
		inb := &models.Inbound{
			Protocol:       models.ProtocolVless,
			StreamSettings: `{"network":"xhttp","security":"tls","xhttpSettings":{"mode":"auto","path":"/p"}}`,
			Flow:           "xtls-rprx-vision",
		}
		if issues := ValidateInboundFields(inb); !hasIssue(issues, "flow", "not_allowed") {
			t.Errorf("xhttp + vision 应被拒绝: %+v", issues)
		}
	})

	t.Run("入站 encryption 必须为空", func(t *testing.T) {
		inb := &models.Inbound{
			Protocol:       models.ProtocolVless,
			SettingsJSON:   `{"encryption":"legacy"}`,
			StreamSettings: `{"network":"tcp","security":"none"}`,
		}
		if issues := ValidateInboundFields(inb); !hasIssue(issues, "settings.encryption", "forbidden") {
			t.Errorf("入站 encryption 应被拒绝: %+v", issues)
		}
	})

	t.Run("splithttpSettings 别名满足 xhttp 规则", func(t *testing.T) {
		inb := &models.Inbound{
			Protocol:       models.ProtocolVless,
			StreamSettings: `{"network":"xhttp","security":"none","splithttpSettings":{"mode":"auto","path":"/p"}}`,
		}
		if issues := ValidateInboundFields(inb); len(issues) != 0 {
			t.Errorf("别名应被识别: %+v", issues)
		}
	})
}

func TestValidateInboundFields_Hysteria(t *testing.T) {
	base := func(settings, stream string) *models.Inbound {
		return &models.Inbound{Protocol: models.ProtocolHysteria, SettingsJSON: settings, StreamSettings: stream}
	}

	if issues := ValidateInboundFields(base(`{"version":2}`, `{"network":"hysteria","security":"tls"}`)); len(issues) != 0 {
		t.Fatalf("合法 Hysteria 2 不应有问题: %+v", issues)
	}
	if issues := ValidateInboundFields(base(`{"clients":[]}`, `{"network":"hysteria","security":"tls"}`)); len(issues) != 0 {
		t.Errorf("空 clients 视为不存在: %+v", issues)
	}
	if issues := ValidateInboundFields(base(`{"clients":[{"id":"x"}]}`, `{"network":"hysteria","security":"tls"}`)); !hasIssue(issues, "settings.clients", "forbidden") {
		t.Errorf("settings.clients 应被拒绝: %+v", issues)
	}
	if issues := ValidateInboundFields(base(`{"version":2}`,
		`{"network":"hysteria","security":"tls","hysteriaSettings":{"version":2,"auth":"shared"}}`)); !hasIssue(issues, "stream.hysteriaSettings.auth", "forbidden") {
		t.Errorf("服务端共享密码应被拒绝: %+v", issues)
	}
	if issues := ValidateInboundFields(base(`{"version":2}`, `{"network":"tcp","security":"tls"}`)); !hasIssue(issues, "stream.network", "not_allowed") {
		t.Errorf("非法 network 应被拒绝: %+v", issues)
	}
}

func TestValidateInboundFields_Dokodemo(t *testing.T) {
	targetID := uint64(99)

	if issues := ValidateInboundFields(&models.Inbound{
		Protocol: models.ProtocolTunnel, TargetInboundID: &targetID,
	}); len(issues) != 0 {
		t.Fatalf("连线模式不应有问题: %+v", issues)
	}
	if issues := ValidateInboundFields(&models.Inbound{
		Protocol: models.ProtocolTunnel, TargetAddress: "1.2.3.4", TargetPort: 8443,
	}); len(issues) != 0 {
		t.Fatalf("手动模式不应有问题: %+v", issues)
	}
	if issues := ValidateInboundFields(&models.Inbound{
		Protocol: models.ProtocolTunnel, TargetInboundID: &targetID, TargetAddress: "1.2.3.4",
	}); !hasIssue(issues, "target.address", "forbidden") {
		t.Errorf("连线模式不应再填地址: %+v", issues)
	}
	if issues := ValidateInboundFields(&models.Inbound{
		Protocol: models.ProtocolTunnel, TargetAddress: "1.2.3.4", TargetPort: 0,
	}); len(issues) != 0 {
		// 无有效目标 = 草稿；mode 为空时目标规则不触发
		t.Errorf("草稿不应被目标规则误报: %+v", issues)
	}
	if issues := ValidateInboundFields(&models.Inbound{
		Protocol: models.ProtocolTunnel, TargetAddress: "1.2.3.4", TargetPort: 8443,
		SettingsJSON: `{"address":"9.9.9.9"}`,
	}); !hasIssue(issues, "settings.address", "forbidden") {
		t.Errorf("入站不得再写 address: %+v", issues)
	}
}

func TestExplainInbound(t *testing.T) {
	inb := &models.Inbound{
		Protocol: models.ProtocolVless,
		Flow:     "",
		StreamSettings: `{"network":"tcp","security":"reality","unknownKey":{"a":1},` +
			`"realitySettings":{"dest":"1.1.1.1:443","serverNames":["r.example.com"],"privateKey":"sk","shortIds":["abcd"]}}`,
	}
	entries := ExplainInbound(inb)

	byPath := map[string]contracts.ProvenanceEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}
	if e, ok := byPath["stream.network"]; !ok || !e.Present || e.Origin != contracts.OriginAdmin {
		t.Errorf("stream.network 来源错误: %+v", e)
	}
	if e, ok := byPath["stream.realitySettings.serverNames"]; !ok || !e.Present || e.Role != contracts.RoleServerOnly {
		t.Errorf("serverNames 来源错误: %+v", e)
	}
	// 客户端派生字段：已声明、未存储
	if e, ok := byPath["stream.realitySettings.publicKey"]; !ok || e.Present || e.Origin != contracts.OriginDerived {
		t.Errorf("publicKey 应为未存储的派生字段: %+v", e)
	}
	// 未声明字段必须被标出来
	if e, ok := byPath["stream.unknownKey.a"]; !ok || e.Declared {
		t.Errorf("未声明字段应标记 Declared=false: %+v", e)
	}
}
