package protocols

import (
	"reflect"
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
)

// S2 契约锁：同一入站的订阅投影（P2）与互联投影（P4）必须只在
// 身份/地址/凭证上不同；传输层与安全层语义必须一致。
// 若某次改动只修了一条投影的字段（如 reality 公钥、xhttp 参数），这里会先红。
func TestProjectionSemantics_SubscriptionPeerEquivalent(t *testing.T) {
	cases := []struct {
		name    string
		inbound models.Inbound
	}{
		{
			name: "vless tcp reality",
			inbound: models.Inbound{
				ID: 11, Tag: "vless-r", Protocol: models.ProtocolVless, Port: 443,
				StreamSettings: `{"network":"tcp","security":"reality","fingerprint":"firefox",` +
					`"realitySettings":{"serverNames":["r.example.com"],"privateKey":"sk","shortIds":["abcd"],"publicKey":"pbk"}}`,
			},
		},
		{
			name: "vless tcp tls",
			inbound: models.Inbound{
				ID: 12, Tag: "vless-t", Protocol: models.ProtocolVless, Port: 8443,
				StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"t.example.com","allowInsecure":true}}`,
			},
		},
		{
			name: "vless xhttp reality",
			inbound: models.Inbound{
				ID: 13, Tag: "vless-xr", Protocol: models.ProtocolVless, Port: 2053,
				StreamSettings: `{"network":"xhttp","security":"reality",` +
					`"xhttpSettings":{"mode":"stream-up","path":"/xp","host":"h.example.com","xPaddingBytes":"100"},` +
					`"realitySettings":{"serverNames":["r.example.com"],"privateKey":"sk","shortIds":["abcd"],"publicKey":"pbk"}}`,
			},
		},
		{
			name: "vless xhttp tls",
			inbound: models.Inbound{
				ID: 14, Tag: "vless-xt", Protocol: models.ProtocolVless, Port: 2054,
				StreamSettings: `{"network":"xhttp","security":"tls",` +
					`"xhttpSettings":{"mode":"auto","path":"/p","host":"h.example.com"},` +
					`"tlsSettings":{"serverName":"t.example.com"}}`,
			},
		},
		{
			name: "hysteria tls",
			inbound: models.Inbound{
				ID: 21, Tag: "hy2", Protocol: models.ProtocolHysteria, Port: 44321,
				StreamSettings: `{"network":"hysteria","security":"tls","tlsSettings":{"serverName":"hy.example.com"}}`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := Find(tc.inbound.Protocol)
			if plugin == nil {
				t.Fatalf("协议 %s 未注册", tc.inbound.Protocol)
			}
			spec := contracts.DecodeInbound(&tc.inbound)
			sub, err := plugin.ProjectClient(&contracts.ClientProjectionInput{
				Role:     contracts.RoleSubscription,
				Name:     "node",
				Host:     "edge.example.com",
				Port:     20000,
				Spec:     spec,
				UserUUID: "user-uuid",
			})
			if err != nil {
				t.Fatalf("订阅投影失败: %v", err)
			}
			peer, err := plugin.ProjectClient(&contracts.ClientProjectionInput{
				Role: contracts.RolePeer,
				RefTarget: &contracts.RefTarget{
					Inbound:          tc.inbound,
					ServerHost:       "real-host.internal",
					TargetCertDomain: tlsServerName(spec),
				},
				OutboundUUID: "peer-uuid",
			})
			if err != nil {
				t.Fatalf("互联投影失败: %v", err)
			}
			if sub == nil || peer == nil {
				t.Fatal("两条投影都不应返回 nil")
			}

			if !reflect.DeepEqual(sub.Transport, peer.Transport) {
				t.Errorf("传输层语义不一致:\n sub = %+v\npeer = %+v", sub.Transport, peer.Transport)
			}
			if !reflect.DeepEqual(sub.Security, peer.Security) {
				t.Errorf("安全层语义不一致:\n sub = %+v\npeer = %+v", sub.Security, peer.Security)
			}
		})
	}
}

func tlsServerName(spec *contracts.InboundSpec) string {
	if spec.TLS != nil {
		return spec.TLS.ServerName
	}
	return ""
}
