package subscribe

import (
	"strings"
	"testing"

	"github.com/acdc/xray-panel/internal/contracts"
)

func TestBuildClashWithTemplate(t *testing.T) {
	uuid := "00000000-0000-0000-0000-000000000001"

	items := []contracts.ProxyNodeDTO{
		{
			Name:       "🇭🇰香港01 x1 | IEPL",
			ServerHost: "gz.perlica.cloud",
			ServerPort: 50000,
			Protocol:   "vless",
			Auth:       &contracts.ClientCredentialDTO{UUID: uuid, Flow: "xtls-rprx-vision"},
			Transport:  &contracts.TransportOptions{Network: "tcp"},
			Security: &contracts.SecurityOptions{
				Type: "reality",
				SNI:  "www.pcps.edu.hk",
				Reality: &contracts.RealityOptions{
					PublicKey: "pQDGvDURYEv8nxAVW9xsbBsQjOXzX0rCh5OWDW5q8kg",
					ShortID:   "e69c1c",
				},
			},
		},
		{
			Name:       "🇯🇵日本01 x0.7",
			ServerHost: "jp.perlica.cloud",
			ServerPort: 443,
			Protocol:   "vless",
			Auth:       &contracts.ClientCredentialDTO{UUID: uuid, Flow: "xtls-rprx-vision"},
			Transport:  &contracts.TransportOptions{Network: "tcp"},
			Security: &contracts.SecurityOptions{
				Type: "reality",
				SNI:  "eedu.jp",
				Reality: &contracts.RealityOptions{
					PublicKey: "KkXqOz9miGjBFekih0MbxURvX5CDghKLFGdooFhAFnA",
					ShortID:   "3745f10afac371",
				},
			},
		},
		{
			Name:       "🇹🇼台湾家宽 x1 | IEPL",
			ServerHost: "gz.perlica.cloud",
			ServerPort: 50003,
			Protocol:   "vless",
			Auth:       &contracts.ClientCredentialDTO{UUID: uuid, Flow: "xtls-rprx-vision"},
			Transport:  &contracts.TransportOptions{Network: "tcp"},
			Security: &contracts.SecurityOptions{
				Type: "reality",
				SNI:  "www.twnic.tw",
				Reality: &contracts.RealityOptions{
					PublicKey: "LZecf_K9Njv1FqU2RlcDs2z2lxaOqxfXKwLQFPpwsg0",
					ShortID:   "4fb7e1d145",
				},
			},
		},
	}

	t.Run("DefaultFallbackWhenTemplateEmpty", func(t *testing.T) {
		res := BuildClashWithTemplate(items, "")
		if !strings.Contains(res, "proxies:") || !strings.Contains(res, "name: 节点选择") {
			t.Fatalf("expected standard fallback, got:\n%s", res)
		}
		if !strings.Contains(res, "🇭🇰香港01 x1 | IEPL") || !strings.Contains(res, "🇯🇵日本01 x0.7") {
			t.Fatalf("expected all nodes in fallback output")
		}
	})

	t.Run("InlineArrayTemplateMatchExampleYAML", func(t *testing.T) {
		tmpl := `mixed-port: 7890
mode: rule

proxies:
$PROXIES$

proxy-groups:
    - { name: 节点选择, type: select, proxies: [DIRECT, $ALL_PROXIES$] }
    - { name: 香港节点, type: select, proxies: [$FILTER_PROXIES(HK|香港)$] }
    - { name: 日本节点, type: select, proxies: [$FILTER_PROXIES(JP|日本)$] }
    - { name: Anthropic, type: select, proxies: [节点选择, $FILTER_PROXIES(家宽|台湾|日本)$] }
    - { name: 美国节点(无匹配), type: select, proxies: [$FILTER_PROXIES(US|美国)$] }

rules:
    - 'DOMAIN,$PANEL_HOST$,DIRECT'
    - 'MATCH,节点选择'
`
		res := BuildClashWithTemplate(items, tmpl, "clash.perlica.cloud")

		// 验证单行 flow 映射（已清理冗余 alterId/cipher/encryption/skip-cert-verify）
		if !strings.Contains(res, "- { name: '🇭🇰香港01 x1 | IEPL', type: vless, server: gz.perlica.cloud, port: 50000, uuid: 00000000-0000-0000-0000-000000000001, udp: true, flow: xtls-rprx-vision, tls: true, servername: www.pcps.edu.hk, reality-opts: { public-key: pQDGvDURYEv8nxAVW9xsbBsQjOXzX0rCh5OWDW5q8kg, short-id: e69c1c }, client-fingerprint: chrome, network: tcp }") {
			t.Errorf("flow mapping format mismatch:\n%s", res)
		}

		// 验证 $PANEL_HOST$ 替换
		if !strings.Contains(res, "DOMAIN,clash.perlica.cloud,DIRECT") {
			t.Errorf("panel host placeholder replacement failed")
		}

		// 验证行内 $ALL_PROXIES$
		// 验证行内 $ALL_PROXIES$
		if !strings.Contains(res, "proxies: [DIRECT, '🇭🇰香港01 x1 | IEPL', '🇯🇵日本01 x0.7', '🇹🇼台湾家宽 x1 | IEPL']") {
			t.Errorf("inline ALL_PROXIES expansion failed:\n%s", res)
		}

		// 验证行内 $FILTER_PROXIES(HK|香港)$
		if !strings.Contains(res, "proxies: ['🇭🇰香港01 x1 | IEPL']") {
			t.Errorf("inline FILTER_PROXIES HK expansion failed:\n%s", res)
		}

		// 验证行内 $FILTER_PROXIES(家宽|台湾|日本)$
		if !strings.Contains(res, "proxies: [节点选择, '🇯🇵日本01 x0.7', '🇹🇼台湾家宽 x1 | IEPL']") {
			t.Errorf("inline Anthropic expansion failed:\n%s", res)
		}

		// 验证行内无匹配安全兜底为 DIRECT
		if !strings.Contains(res, "proxies: [DIRECT]") {
			t.Errorf("empty inline array safe fallback failed:\n%s", res)
		}
	})
}

