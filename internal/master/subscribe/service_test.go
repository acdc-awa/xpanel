package subscribe

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 构造 DTO 时 flow 已由协议插件决议（生成侧同源），导出器为哑渲染器。
func dtoVless(name, host string, port int, uuid, network string, sec *contracts.SecurityOptions, tr *contracts.TransportOptions, flow string) contracts.ProxyNodeDTO {
	if tr == nil {
		tr = &contracts.TransportOptions{Network: network}
	}
	return contracts.ProxyNodeDTO{
		Name:       name,
		ServerHost: host,
		ServerPort: port,
		Protocol:   "vless",
		Transport:  tr,
		Security:   sec,
		Auth:       &contracts.ClientCredentialDTO{UUID: uuid, Flow: flow},
	}
}

// A-G 差距回归（03 号文档 §4）：
// A tcp+tls+vision 缺 flow · B xhttp 缺 alpn:[h2] · C tls 缺 servername ·
// D xhttp 缺 host · E 缺 skip-cert-verify 透传 · F url-test 用 cloudflare ·
// G BuildBase64 同步（tcp+tls+flow / xhttp host / xhttp+tls 无 flow）
func TestBuildClash_Gaps(t *testing.T) {
	nodes := []contracts.ProxyNodeDTO{
		dtoVless("tcp-reality", "r.example.com", 443, "uuid-r", "tcp",
			&contracts.SecurityOptions{Type: "reality", SNI: "r.example.com",
				Reality: &contracts.RealityOptions{PublicKey: "pbk123", ShortID: "abcd"}},
			nil, "xtls-rprx-vision"), // tcp+reality 自动 vision（插件已决议）
		dtoVless("tcp-tls-vision", "t.example.com", 443, "uuid-t", "tcp",
			&contracts.SecurityOptions{Type: "tls", SNI: "t.example.com", AllowInsecure: true},
			nil, "xtls-rprx-vision"),
		dtoVless("xhttp-tls", "w.example.com", 8443, "uuid-w", "xhttp",
			&contracts.SecurityOptions{Type: "tls", SNI: "w.example.com"},
			&contracts.TransportOptions{Network: "xhttp", Mode: "auto", Path: "/xp-tls", Host: "cdn.example.com"}, ""),
		dtoVless("xhttp-reality", "x.example.com", 443, "uuid-x", "xhttp",
			&contracts.SecurityOptions{Type: "reality", SNI: "x.example.com",
				Reality: &contracts.RealityOptions{PublicKey: "pbk456", ShortID: "ef01"}},
			&contracts.TransportOptions{Network: "xhttp", Mode: "auto", Path: "/xp", Host: "h.example.com"}, ""),
		// reality 参数缺失（Reality=nil）应跳过不崩溃
		dtoVless("broken-reality", "b.example.com", 443, "uuid-b", "tcp",
			&contracts.SecurityOptions{Type: "reality"}, nil, ""),
	}

	yaml := BuildClashWithTemplate(nodes, "")

	for _, want := range []string{
		"flow: xtls-rprx-vision",        // reality+tcp 自动 vision
		"public-key: pbk123",            // reality 公钥
		"servername: t.example.com",     // C tls servername
		"skip-cert-verify: true",        // E allowInsecure 透传
		"network: xhttp",                // xhttp
		"alpn: [h2]",                    // B xhttp 显式 alpn
		"host: h.example.com",           // D xhttp host
		"cp.cloudflare.com/generate_204", // F url-test 地址
	} {
		if !strings.Contains(yaml, want) {
			t.Errorf("BuildClashWithTemplate 缺少 %q\n---\n%s", want, yaml)
		}
	}
	if strings.Contains(yaml, "broken-reality") {
		t.Error("reality 参数缺失的节点应被跳过")
	}
	// xhttp-tls 节点（无 flow）不应输出 flow
	xhIdx := strings.Index(yaml, "'xhttp-tls'")
	if xhIdx < 0 {
		xhIdx = strings.Index(yaml, `"xhttp-tls"`)
	}
	if xhIdx >= 0 {
		rest := yaml[xhIdx:]
		if idx := strings.Index(rest, "\n"); idx > 0 {
			rest = rest[:idx]
		}
		if strings.Contains(rest, "flow:") {
			t.Error("xhttp+tls 节点不应输出 flow")
		}
	}
	// xhttp-reality 节点也不应输出 flow
	xrIdx := strings.Index(yaml, "'xhttp-reality'")
	if xrIdx >= 0 {
		rest := yaml[xrIdx:]
		if idx := strings.Index(rest, "\n"); idx > 0 {
			rest = rest[:idx]
		}
		if strings.Contains(rest, "flow:") {
			t.Error("xhttp+reality 节点不应输出 flow")
		}
	}
}

func TestBuildBase64_Gaps(t *testing.T) {
	nodes := []contracts.ProxyNodeDTO{
		dtoVless("tcp-tls-vision", "t.example.com", 443, "uuid-t", "tcp",
			&contracts.SecurityOptions{Type: "tls", SNI: "t.example.com", AllowInsecure: true},
			nil, "xtls-rprx-vision"),
		dtoVless("xhttp-host", "x.example.com", 443, "uuid-x", "xhttp",
			&contracts.SecurityOptions{Type: "reality", SNI: "x.example.com",
				Reality: &contracts.RealityOptions{PublicKey: "pbk456", ShortID: "ef01"}},
			&contracts.TransportOptions{Network: "xhttp", Mode: "auto", Path: "/xp", Host: "h.example.com"}, ""),
		dtoVless("xhttp-tls", "w.example.com", 8443, "uuid-w", "xhttp",
			&contracts.SecurityOptions{Type: "tls", SNI: "w.example.com"},
			&contracts.TransportOptions{Network: "xhttp", Mode: "auto", Path: "/xp-tls", Host: "cdn.example.com"}, ""),
	}
	b64 := BuildBase64(nodes)
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("Base64 解码失败: %v", err)
	}
	links := strings.Split(string(raw), "\n")
	if len(links) != 3 {
		t.Fatalf("期望 3 条链接，实际 %d: %s", len(links), raw)
	}
	for _, want := range []string{
		"flow=xtls-rprx-vision", // A/G tcp+tls+vision 缺 flow
		"sni=t.example.com",     // C tls servername
		"allowInsecure=1",       // E
		"host=h.example.com",    // D/G xhttp host
		"type=xhttp",            // xhttp
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("BuildBase64 缺少 %q\n---\n%s", want, raw)
		}
	}
	// xhttp+tls 不应有 flow
	for _, l := range links {
		if strings.Contains(l, "xhttp-tls") && strings.Contains(l, "flow=") {
			t.Errorf("xhttp+tls 链接不应带 flow: %s", l)
		}
	}
}

func TestBuildClash_SkipsBrokenReality(t *testing.T) {
	nodes := []contracts.ProxyNodeDTO{
		dtoVless("ok", "a.com", 443, "u", "tcp",
			&contracts.SecurityOptions{Type: "reality", SNI: "a.com",
				Reality: &contracts.RealityOptions{PublicKey: "pk", ShortID: "sid"}},
			nil, "xtls-rprx-vision"),
		dtoVless("broken", "b.com", 443, "u2", "tcp",
			&contracts.SecurityOptions{Type: "reality"}, nil, ""),
	}
	yaml := BuildClashWithTemplate(nodes, "")
	if strings.Contains(yaml, "broken") {
		t.Errorf("broken 节点不应输出: %s", yaml)
	}
	b64 := BuildBase64(nodes)
	raw, _ := base64.StdEncoding.DecodeString(b64)
	if strings.Contains(string(raw), "u2@") {
		t.Errorf("broken 节点不应输出链接: %s", raw)
	}
}

// TestResolveAPSubscription_TunnelPipeline 验证 AP 指向四层直通管道时自动向后寻路组装落地机 VLESS 参数
func TestResolveAPSubscription_TunnelPipeline(t *testing.T) {
	edgeServer := models.Server{ID: 1, Host: "edge.example.com", Name: "国内前置机"}
	exitServer := models.Server{ID: 2, Host: "exit.example.com", Name: "日本落地机"}

	targetInbID := uint64(201)
	tunnelInb := models.Inbound{
		ID:                101,
		ServerID:          edgeServer.ID,
		Tag:               "tunnel-10001",
		Port:              10001,
		Type:              models.InboundTypeTunnel,
		Protocol:          models.ProtocolDokodemo,
		TargetInboundID:   &targetInbID,
		ShareAddrStrategy: "node",
		Enabled:           true,
	}

	exitInb := models.Inbound{
		ID:                targetInbID,
		ServerID:          exitServer.ID,
		Tag:               "vless-in-443",
		Port:              443,
		Type:              models.InboundTypeUser,
		Protocol:          "vless",
		StreamSettings:    `{"network":"tcp","security":"reality","realitySettings":{"serverName":"www.apple.com","publicKey":"MklFWFDOJqoe6ONT9a5vvOsyp1lKqYzmSOgDgmSV_j8","shortId":"0123456789abcdef"}}`,
		ShareAddrStrategy: "node",
		Enabled:           true,
	}

	apTunnel := models.UserAccessPoint{
		ID:              1,
		Name:            "🇯🇵 日本 01 [专线中转]",
		TargetType:      "inbound",
		TargetInboundID: &tunnelInb.ID,
		Enabled:         true,
	}

	srvMap := map[uint64]models.Server{edgeServer.ID: edgeServer, exitServer.ID: exitServer}
	inbMap := map[uint64]models.Inbound{tunnelInb.ID: tunnelInb, exitInb.ID: exitInb}

	userUUID := "a6a0e69e-5c62-4b2a-89a7-8f5b82143719"
	dto := ResolveAPSubscription(&apTunnel, srvMap, inbMap, nil, userUUID)
	if dto == nil {
		t.Fatalf("ResolveAPSubscription returned nil")
	}

	// 1. 连接地址和端口必须取自首跳前置机
	if dto.ServerHost != "edge.example.com" {
		t.Errorf("dto.ServerHost = %s, want edge.example.com", dto.ServerHost)
	}
	if dto.ServerPort != 10001 {
		t.Errorf("dto.ServerPort = %d, want 10001", dto.ServerPort)
	}

	// 2. 协议与安全参数必须取自终结落地机
	if dto.Protocol != "vless" {
		t.Errorf("dto.Protocol = %s, want vless", dto.Protocol)
	}
	if dto.Security == nil || dto.Security.Type != "reality" {
		t.Fatalf("dto.Security = %v, want reality", dto.Security)
	}
	if dto.Security.SNI != "www.apple.com" {
		t.Errorf("dto.Security.SNI = %s, want www.apple.com", dto.Security.SNI)
	}
	if dto.Security.Reality.PublicKey != "MklFWFDOJqoe6ONT9a5vvOsyp1lKqYzmSOgDgmSV_j8" {
		t.Errorf("dto.Security.Reality.PublicKey = %s", dto.Security.Reality.PublicKey)
	}

	// 3. 用户 UUID 与节点名
	if dto.Auth.UUID != userUUID {
		t.Errorf("dto.Auth.UUID = %s, want %s", dto.Auth.UUID, userUUID)
	}
	if dto.Name != "🇯🇵 日本 01 [专线中转]" {
		t.Errorf("dto.Name = %s", dto.Name)
	}

	// 4. 测试管道悬空草稿（TargetInboundID 为 nil）
	tunnelDraft := models.Inbound{
		ID:       102,
		ServerID: edgeServer.ID,
		Tag:      "tunnel-draft",
		Port:     10002,
		Type:     models.InboundTypeTunnel,
		Protocol: models.ProtocolDokodemo,
	}
	apDraft := models.UserAccessPoint{
		ID:              2,
		Name:            "草稿节点",
		TargetType:      "inbound",
		TargetInboundID: &tunnelDraft.ID,
		Enabled:         true,
	}
	inbMap[tunnelDraft.ID] = tunnelDraft
	if dtoDraft := ResolveAPSubscription(&apDraft, srvMap, inbMap, nil, userUUID); dtoDraft != nil {
		t.Errorf("悬空管道 AP 不应产出节点 DTO: %v", dtoDraft)
	}
}
