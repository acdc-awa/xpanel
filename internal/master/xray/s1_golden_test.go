package xray_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
)

// S1 逐字节等价基线：P1 消费 InboundSpec 的改造不得改变任何生成结果。
// 覆盖 VLESS 的 reality / tls / xhttp / vlessenc / relay、Hysteria 2 的默认与部分配置、
// 直通入站（连线 / 手动目标 + PROXY Protocol）。
// 更新基线：UPDATE_GOLDEN=1 go test ./internal/master/xray -run TestS1GenerateGolden

type s1Scenario struct {
	name      string
	inbounds  []models.Inbound
	outbounds []models.ServerOutbound
	rules     []models.ServerRoutingRule
	users     map[string][]protocol.User
	ctx       *xray.GenerateContext
}

func s1ReferenceFlow(inboundTag string) *xray.GenerateContext {
	target := models.Inbound{ID: 901, ServerID: 2, Tag: "landing", Protocol: models.ProtocolVless, Port: 443}
	return &xray.GenerateContext{
		RefTargets: map[uint64]xray.RefTarget{
			target.ID: {Inbound: target, ServerHost: "10.0.0.9"},
		},
		ReferencingOutbounds: map[uint64][]models.ServerOutbound{
			101: {{ID: 55, Tag: "relay-out", Protocol: models.ProtocolVless, Enabled: true}},
		},
	}
}

func s1Scenarios() []s1Scenario {
	const vlessUUID = "11111111-1111-1111-1111-111111111111"
	vlessDec := "mlkem768x25519plus.native.600s." + goldenMlkemSeed

	return []s1Scenario{
		{
			name: "vless_tcp_reality",
			inbounds: []models.Inbound{{
				ID: 101, ServerID: 1, Tag: "vless-443", Protocol: models.ProtocolVless, Port: 443,
				Listen: "0.0.0.0",
				// fingerprint / publicKey / spiderX / realitySettings.settings 均为客户端角色或面板私有键，
				// 生成时必须被 sanitize 剥掉。
				StreamSettings: `{"network":"tcp","security":"reality","fingerprint":"chrome",` +
					`"realitySettings":{"dest":"1.2.3.4:443","serverNames":["r.example.com"],"privateKey":"sk",` +
					`"shortIds":["abcd","ef01"],"publicKey":"pk","spiderX":"/","settings":{"x":1}}}`,
				Sniffing: `{"enabled":true,"destOverride":["http","tls"],"routeOnly":true}`,
				Enabled:  true,
			}},
			users: map[string][]protocol.User{
				"vless-443": {{UUID: vlessUUID, Email: "u1.i101@panel.local"}},
			},
		},
		{
			name: "vless_tcp_tls_cert",
			inbounds: []models.Inbound{{
				ID: 102, ServerID: 1, Tag: "vless-tls", Protocol: models.ProtocolVless, Port: 8443,
				CertID: func() *uint64 { v := uint64(7); return &v }(),
				StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"t.example.com",` +
					`"allowInsecure":true,"settings":{"nested":true},"certificates":[{"usage":"verify",` +
					`"certificateFile":"/etc/ca.pem"},{"usage":"encipherment","certificateFile":"/old.pem","keyFile":"/old.key"}]},` +
					`"externalProxy":{"x":1}}`,
				Enabled: true,
			}},
			users: map[string][]protocol.User{
				"vless-tls": {{UUID: vlessUUID, Email: "u1.i102@panel.local", Flow: "none"}},
			},
			ctx: &xray.GenerateContext{CertDomains: map[uint64]string{7: "t.example.com"}},
		},
		{
			name: "vless_xhttp_reality",
			inbounds: []models.Inbound{{
				ID: 103, ServerID: 1, Tag: "vless-xhttp", Protocol: models.ProtocolVless, Port: 2053,
				StreamSettings: `{"network":"xhttp","security":"reality",` +
					`"xhttpSettings":{"mode":"auto","path":"/xp","host":"h.example.com","sessionPlacement":"path",` +
					`"sessionKey":"k1","xPaddingBytes":"100-1000","xmux":{"maxConcurrency":"16-32"}},` +
					`"realitySettings":{"dest":"1.2.3.4:443","serverNames":["r.example.com"],"privateKey":"sk","shortIds":["abcd"]},` +
					`"finalmask":{"udp":[{"type":"salamander","settings":{"password":"p"}}]}}`,
				Enabled: true,
			}},
			users: map[string][]protocol.User{
				"vless-xhttp": {{UUID: vlessUUID, Email: "u1.i103@panel.local"}},
			},
		},
		{
			name: "vless_xhttp_tls",
			inbounds: []models.Inbound{{
				ID: 104, ServerID: 1, Tag: "vless-xhttp-tls", Protocol: models.ProtocolVless, Port: 8443,
				StreamSettings: `{"network":"xhttp","security":"tls","xhttpSettings":{"mode":"stream-up","path":"/xp"},` +
					`"tlsSettings":{"serverName":"x.example.com","allowInsecure":true,` +
					`"certificates":[{"certificateFile":"/c.pem","keyFile":"/k.pem"}]},"sockopt":{"tcpFastOpen":true,"mark":255}}`,
				Enabled: true,
			}},
			users: map[string][]protocol.User{
				"vless-xhttp-tls": {{UUID: vlessUUID, Email: "u1.i104@panel.local"}},
			},
		},
		{
			name: "vless_vlessenc_relay",
			inbounds: []models.Inbound{{
				ID: 105, ServerID: 1, Tag: "ve-relay", Protocol: models.ProtocolVless, Port: 9443,
				Type: models.InboundTypeRelay, InternalUUID: "22222222-2222-2222-2222-222222222222",
				SettingsJSON:   `{"decryption":"` + vlessDec + `","encryption":"should-be-removed"}`,
				StreamSettings: `{"network":"tcp","security":"none"}`,
				Enabled:        true,
			}},
		},
		{
			name: "vless_encryption_stripped",
			inbounds: []models.Inbound{{
				ID: 106, ServerID: 1, Tag: "vless-plain", Protocol: models.ProtocolVless, Port: 443,
				SettingsJSON:   `{"encryption":"legacy","clients":[{"id":"stale"}]}`,
				StreamSettings: `{"network":"tcp","security":"none"}`,
				Enabled:        true,
			}},
			users: map[string][]protocol.User{
				"vless-plain": {{UUID: vlessUUID, Email: "u1.i106@panel.local"}},
			},
		},
		{
			name: "relay_ref_user_tcp_tls",
			inbounds: []models.Inbound{{
				ID: 101, ServerID: 1, Tag: "vless-443", Protocol: models.ProtocolVless, Port: 443,
				StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"r.example.com"}}`,
				Enabled:        true,
			}},
			users: map[string][]protocol.User{
				"vless-443": {{UUID: vlessUUID, Email: "u1.i101@panel.local"}},
			},
			ctx: s1ReferenceFlow("vless-443"),
		},
		{
			name: "hysteria_defaults",
			inbounds: []models.Inbound{{
				ID: 201, ServerID: 1, Tag: "hy2", Protocol: models.ProtocolHysteria, Port: 44321,
				Enabled: true,
			}},
			users: map[string][]protocol.User{
				"hy2": {
					{UUID: "auth-1", Email: "u1.i201@panel.local"},
					{UUID: "auth-2", Email: "u2.i201@panel.local"},
				},
			},
		},
		{
			name: "hysteria_partial",
			inbounds: []models.Inbound{{
				ID: 202, ServerID: 1, Tag: "hy2-partial", Protocol: models.ProtocolHysteria, Port: 44322,
				StreamSettings: `{"network":"hysteria","security":"tls",` +
					`"hysteriaSettings":{"version":1,"auth":"shared-secret","udpIdleTimeout":30},` +
					`"tlsSettings":{"serverName":"hy.example.com"}}`,
				Enabled: true,
			}},
			users: map[string][]protocol.User{
				"hy2-partial": {{UUID: "auth-1", Email: "u1.i202@panel.local"}},
			},
		},
		{
			name: "tunnel_linked",
			inbounds: []models.Inbound{{
				ID: 301, ServerID: 1, Tag: "tunnel-10001", Protocol: models.ProtocolTunnel, Port: 10001,
				Type: models.InboundTypeTunnel, TargetInboundID: func() *uint64 { v := uint64(901); return &v }(),
				Enabled: true,
			}},
			ctx: &xray.GenerateContext{
				RefTargets: map[uint64]xray.RefTarget{
					901: {Inbound: models.Inbound{ID: 901, Tag: "landing", Protocol: models.ProtocolVless, Port: 443}, ServerHost: "exit.example.com"},
				},
			},
		},
		{
			name: "tunnel_send_pp_only",
			inbounds: []models.Inbound{{
				ID: 302, ServerID: 1, Tag: "tunnel-send-pp", Protocol: models.ProtocolTunnel, Port: 10002,
				Type: models.InboundTypeTunnel, TargetAddress: "203.0.113.5", TargetPort: 8443,
				// 发送侧私有键：只影响派生 freedom 出站，不得落进入站的 streamSettings。
				StreamSettings: `{"network":"tcp","proxy_protocol":true,"proxyProtocol":true}`,
				Enabled:        true,
			}},
		},
		{
			name: "tunnel_accept_pp",
			inbounds: []models.Inbound{{
				ID: 303, ServerID: 1, Tag: "tunnel-accept-pp", Protocol: models.ProtocolTunnel, Port: 10003,
				Type: models.InboundTypeTunnel, TargetAddress: "203.0.113.5", TargetPort: 8443,
				// 接收侧唯一入口：tcpSettings.acceptProxyProtocol。
				StreamSettings: `{"network":"tcp","tcpSettings":{"acceptProxyProtocol":true}}`,
				Enabled:        true,
			}},
		},
	}
}

func TestS1GenerateGolden(t *testing.T) {
	update := os.Getenv("UPDATE_GOLDEN") == "1"
	dir := filepath.Join("testdata", "s1")

	for _, sc := range s1Scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			got, err := xray.Generate(sc.inbounds, sc.outbounds, sc.rules, sc.users, sc.ctx, "", "")
			if err != nil {
				t.Fatalf("Generate 失败: %v", err)
			}
			path := filepath.Join(dir, sc.name+".json")
			if update {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("创建基线目录失败: %v", err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatalf("写入基线失败: %v", err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("读取基线失败（首次运行请带 UPDATE_GOLDEN=1）: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("生成结果与基线不一致（P1 改造必须逐字节等价）\n--- got ---\n%s", got)
			}
		})
	}
}
