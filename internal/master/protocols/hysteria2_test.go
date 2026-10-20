package protocols_test

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/protocols"
	"github.com/acdc-awa/xpanel/internal/master/subscribe"
	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
)

func TestHysteriaPlugin_Descriptor(t *testing.T) {
	plugin := protocols.Find("hysteria")
	if plugin == nil {
		t.Fatal("未找到已注册的 hysteria 协议插件")
	}

	desc := plugin.Descriptor()
	if desc.Name != "hysteria" {
		t.Fatalf("Descriptor.Name 期望 hysteria, got %s", desc.Name)
	}
	if desc.ForceNetwork != "hysteria" {
		t.Fatalf("Descriptor.ForceNetwork 期望 hysteria, got %s", desc.ForceNetwork)
	}
	if desc.ForceSecurity != "tls" {
		t.Fatalf("Descriptor.ForceSecurity 期望 tls, got %s", desc.ForceSecurity)
	}
	if !desc.SupportsUDP {
		t.Fatal("Hysteria 2 必须声明 SupportsUDP = true")
	}
	if !desc.HotSync {
		t.Fatal("Hysteria 2 必须声明 HotSync = true")
	}
	if desc.AccountType != "hysteria" {
		t.Fatalf("Descriptor.AccountType 期望 hysteria, got %s", desc.AccountType)
	}
}

func TestHysteriaPlugin_ServerUsers(t *testing.T) {
	plugin := protocols.Find("hysteria")
	if plugin == nil {
		t.Fatal("未找到 hysteria 插件")
	}

	users := []protocol.User{
		{UUID: "pwd-user-1", Email: "u1.i10@panel.local", Level: 0},
		{UUID: "pwd-user-2", Email: "u2.i10@panel.local", Level: 1},
		{UUID: "", Email: "invalid@panel.local"}, // 空 auth 应忽略
	}

	key, items, err := plugin.ServerUsers(users, &contracts.InboundSpec{})
	if err != nil {
		t.Fatalf("ServerUsers 执行失败: %v", err)
	}
	if key != "users" {
		t.Fatalf("Hysteria 服务端挂载键必须为 users, got %s", key)
	}
	if len(items) != 2 {
		t.Fatalf("期望生成 2 个用户, got %d", len(items))
	}

	u0, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("用户条目类型错误: %T", items[0])
	}
	if u0["auth"] != "pwd-user-1" || u0["email"] != "u1.i10@panel.local" {
		t.Fatalf("用户 0 字段不匹配: %+v", u0)
	}
}

func TestHysteriaPlugin_ValidateInbound(t *testing.T) {
	plugin := protocols.Find("hysteria")
	if plugin == nil {
		t.Fatal("未找到 hysteria 插件")
	}

	// 合法组合
	err := plugin.ValidateInbound(&contracts.InboundSpec{
		Network:  "hysteria",
		Security: "tls",
	})
	if err != nil {
		t.Fatalf("合法组合不应报错: %v", err)
	}

	// 非法 network
	err = plugin.ValidateInbound(&contracts.InboundSpec{
		Network:  "tcp",
		Security: "tls",
	})
	if err == nil || !strings.Contains(err.Error(), "hysteria") {
		t.Fatalf("非法 network 应报错，got: %v", err)
	}

	// 非法 security
	err = plugin.ValidateInbound(&contracts.InboundSpec{
		Network:  "hysteria",
		Security: "reality",
	})
	if err == nil || !strings.Contains(err.Error(), "tls") {
		t.Fatalf("非法 security 应报错，got: %v", err)
	}
}

func TestHysteriaPlugin_ProjectClient_SubscriptionAndClash(t *testing.T) {
	plugin := protocols.Find("hysteria")
	if plugin == nil {
		t.Fatal("未找到 hysteria 插件")
	}

	spec := &contracts.InboundSpec{
		ID:          101,
		Network:     "hysteria",
		Security:    "tls",
		Fingerprint: "firefox",
		TLS: &contracts.TLSSpec{
			ServerName:    "node1.example.com",
			AllowInsecure: true,
		},
	}

	in := &contracts.ClientProjectionInput{
		Role:     contracts.RoleSubscription,
		Name:     "HK-Hy2-01",
		Host:     "hk.example.com",
		Port:     44321,
		Spec:     spec,
		UserUUID: "secret-auth-key-999",
	}

	dto, err := plugin.ProjectClient(in)
	if err != nil {
		t.Fatalf("ProjectClient 失败: %v", err)
	}
	if dto.Protocol != "hysteria" {
		t.Fatalf("DTO 协议期望 hysteria, got %s", dto.Protocol)
	}
	if dto.Auth.Password != "secret-auth-key-999" {
		t.Fatalf("DTO 密码凭证不符合预期: %+v", dto.Auth)
	}
	if dto.Security.SNI != "node1.example.com" {
		t.Fatalf("DTO SNI 期望 node1.example.com, got %s", dto.Security.SNI)
	}
	if !dto.Security.AllowInsecure {
		t.Fatal("DTO AllowInsecure 期望 true")
	}
	// 回归锁：Hysteria 2 的 Fingerprint 必须是空。mihomo 的 hysteria2.fingerprint 是证书 pin，
	// 一旦导出浏览器名，adapter.ParseProxy 会报错并中止整份订阅配置加载。
	if dto.Security.Fingerprint != "" {
		t.Fatalf("Hysteria 2 DTO 不应携带 Fingerprint, got %q", dto.Security.Fingerprint)
	}

	// 1. 验证 Clash 节点 YAML 导出
	yamlBlock, names := subscribe.FormatNodesYAML([]contracts.ProxyNodeDTO{*dto})
	if len(names) != 1 || names[0] != "HK-Hy2-01" {
		t.Fatalf("FormatNodesYAML 节点名称异常: %v", names)
	}
	if !strings.Contains(yamlBlock, "type: hysteria2") {
		t.Fatalf("YAML 块应包含 type: hysteria2, got:\n%s", yamlBlock)
	}
	if !strings.Contains(yamlBlock, "password: 'secret-auth-key-999'") {
		t.Fatalf("YAML 块应包含密码, got:\n%s", yamlBlock)
	}
	if !strings.Contains(yamlBlock, "sni: node1.example.com") {
		t.Fatalf("YAML 块应包含 sni, got:\n%s", yamlBlock)
	}
	if !strings.Contains(yamlBlock, "skip-cert-verify: true") {
		t.Fatalf("YAML 块应包含 skip-cert-verify, got:\n%s", yamlBlock)
	}
	if !strings.Contains(yamlBlock, "alpn: [h3]") {
		t.Fatalf("YAML 块应包含 alpn: [h3], got:\n%s", yamlBlock)
	}
	// 回归锁：Clash YAML 不得输出 fingerprint（mihomo 会当成证书 pin 并拒绝加载）。
	if strings.Contains(yamlBlock, "fingerprint") {
		t.Fatalf("Hysteria 2 Clash 节点不应包含 fingerprint, got:\n%s", yamlBlock)
	}

	// 2. 验证 Base64 订阅链接导出
	b64 := subscribe.BuildBase64([]contracts.ProxyNodeDTO{*dto})
	rawBytes, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("Base64 解码失败: %v", err)
	}
	link := string(rawBytes)
	if !strings.HasPrefix(link, "hysteria2://secret-auth-key-999@hk.example.com:44321") {
		t.Fatalf("Base64 分享链接格式异常: %s", link)
	}
	if !strings.Contains(link, "sni=node1.example.com") || !strings.Contains(link, "#HK-Hy2-01") {
		t.Fatalf("分享链接参数或 fragment 缺失: %s", link)
	}
}

func TestHysteria_ConfigGenerate_And_XrayTest(t *testing.T) {
	certID := uint64(55)
	hyInbound := models.Inbound{
		ID:             201,
		ServerID:       1,
		Tag:            "hy2-landing",
		Protocol:       models.ProtocolHysteria,
		Port:           44322,
		Type:           models.InboundTypeUser,
		InternalUUID:   "internal-peer-uuid-5555",
		CertID:         &certID,
		Enabled:        true,
		StreamSettings: `{"network":"hysteria","security":"tls","tlsSettings":{"serverName":"hy.test.com"}}`,
	}

	usersByTag := map[string][]protocol.User{
		"hy2-landing": {
			{UUID: "client-auth-111", Email: "u1.i201@panel.local", Level: 0},
			{UUID: "client-auth-222", Email: "u2.i201@panel.local", Level: 0},
		},
	}

	ctx := &xray.GenerateContext{
		CertDomains: map[uint64]string{certID: "hy.test.com"},
	}

	// 1. 生成服务端入站配置
	cfgBytes, err := xray.Generate([]models.Inbound{hyInbound}, nil, nil, usersByTag, ctx, "", "")
	if err != nil {
		t.Fatalf("xray.Generate 失败: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(cfgBytes, &root); err != nil {
		t.Fatalf("生成的 JSON 配置解析失败: %v", err)
	}

	inbounds, ok := root["inbounds"].([]any)
	if !ok || len(inbounds) < 1 {
		t.Fatalf("inbounds 结构异常: %+v", root["inbounds"])
	}

	var foundHyInb map[string]any
	for _, inb := range inbounds {
		m := inb.(map[string]any)
		if m["tag"] == "hy2-landing" {
			foundHyInb = m
			break
		}
	}
	if foundHyInb == nil {
		t.Fatalf("未在生成配置中找到 hy2-landing 入站: %+v", inbounds)
	}

	settings := foundHyInb["settings"].(map[string]any)
	if settings["version"] != float64(2) {
		t.Fatalf("Hysteria settings.version 必须为 2, got: %v", settings["version"])
	}
	users, ok := settings["users"].([]any)
	if !ok || len(users) != 2 {
		t.Fatalf("Hysteria settings.users 必须注入 2 个用户, got: %+v", settings["users"])
	}

	streamSettings := foundHyInb["streamSettings"].(map[string]any)
	if streamSettings["network"] != "hysteria" || streamSettings["security"] != "tls" {
		t.Fatalf("streamSettings 传输与安全协议不符合预期: %+v", streamSettings)
	}
	hySettings := streamSettings["hysteriaSettings"].(map[string]any)
	if hySettings["version"] != float64(2) {
		t.Fatalf("hysteriaSettings.version 必须为 2, got: %v", hySettings["version"])
	}
	if _, hasAuth := hySettings["auth"]; hasAuth {
		t.Fatal("服务端 streamSettings.hysteriaSettings 绝不应包含 auth 共享密码")
	}

	// 2. 节点互联出站配置生成（InboundRef 引用该入站）
	peerTargetInbound := hyInbound
	peerTargetInbound.Type = models.InboundTypeRelay
	refID := peerTargetInbound.ID
	peerOutbound := models.ServerOutbound{
		Tag:        "relay-to-hy2",
		Protocol:   models.ProtocolHysteria,
		Enabled:    true,
		InboundRef: &refID,
	}

	peerCtx := &xray.GenerateContext{
		RefTargets: map[uint64]contracts.RefTarget{
			refID: {
				Inbound:    peerTargetInbound,
				ServerHost: "10.0.0.88",
				CertPin:    "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff",
			},
		},
	}

	peerCfgBytes, err := xray.Generate(nil, []models.ServerOutbound{peerOutbound}, nil, nil, peerCtx, "", "")
	if err != nil {
		t.Fatalf("节点出站配置生成失败: %v", err)
	}

	var peerRoot map[string]any
	if err := json.Unmarshal(peerCfgBytes, &peerRoot); err != nil {
		t.Fatalf("出站 JSON 解析失败: %v", err)
	}
	outbounds := peerRoot["outbounds"].([]any)
	var foundHyOut map[string]any
	for _, ob := range outbounds {
		m := ob.(map[string]any)
		if m["tag"] == "relay-to-hy2" {
			foundHyOut = m
			break
		}
	}
	if foundHyOut == nil {
		t.Fatalf("未找到 relay-to-hy2 出站: %+v", outbounds)
	}
	outSettings := foundHyOut["settings"].(map[string]any)
	if outSettings["version"] != float64(2) || outSettings["address"] != "10.0.0.88" || outSettings["port"] != float64(44322) {
		t.Fatalf("出站 settings 不符合预期: %+v", outSettings)
	}
	outStream := foundHyOut["streamSettings"].(map[string]any)
	outHyS := outStream["hysteriaSettings"].(map[string]any)
	if outHyS["auth"] != "internal-peer-uuid-5555" || outHyS["version"] != float64(2) {
		t.Fatalf("出站 hysteriaSettings 不符合预期: %+v", outHyS)
	}
	outTLS := outStream["tlsSettings"].(map[string]any)
	if outTLS["pinnedPeerCertSha256"] != "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff" {
		t.Fatalf("出站 tlsSettings pinnedPeerCertSha256 注入失败: %+v", outTLS)
	}

	// 3. 金标准真实 xray.exe -test 运行期校验
	xrayBin := filepath.Join("..", "..", "..", "tools", "xray-windows-64", "xray.exe")
	if _, err := os.Stat(xrayBin); err == nil {
		// 服务端配置校验（证书在测试中替换为内存 PEM）
		streamSettings["tlsSettings"] = map[string]any{
			"serverName": "hy.test.com",
			"certificates": []any{
				map[string]any{
					"certificate": []string{"-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"},
					"key":         []string{"-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----"},
				},
			},
		}
		serverTestFile := filepath.Join(t.TempDir(), "hy2_server.json")
		b, _ := json.Marshal(root)
		if err := os.WriteFile(serverTestFile, b, 0644); err != nil {
			t.Fatalf("写入测试文件失败: %v", err)
		}
		cmd := exec.Command(xrayBin, "-test", "-c", serverTestFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("xray.exe -test 服务端配置校验失败:\n%s", string(out))
		}

		// 客户端/中转出站配置校验
		clientTestFile := filepath.Join(t.TempDir(), "hy2_peer.json")
		bClient, _ := json.Marshal(peerRoot)
		if err := os.WriteFile(clientTestFile, bClient, 0644); err != nil {
			t.Fatalf("写入测试文件失败: %v", err)
		}
		cmdClient := exec.Command(xrayBin, "-test", "-c", clientTestFile)
		outClient, errClient := cmdClient.CombinedOutput()
		if errClient != nil {
			t.Fatalf("xray.exe -test 客户端/节点出站配置校验失败:\n%s", string(outClient))
		}
	}
}
