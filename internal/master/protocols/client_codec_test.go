package protocols_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/protocols"
	"github.com/acdc-awa/xpanel/internal/models"
)

// runXrayTestHelper 构造一个包含待测 outbound 的完整配置，调用仓库随附的 xray.exe 进行校验。
func runXrayTestHelper(t *testing.T, outbound map[string]any) {
	t.Helper()
	outbound["tag"] = "test-out"

	config := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []any{
			map[string]any{
				"tag":      "in-dummy",
				"port":     54321,
				"protocol": "dokodemo-door",
				"settings": map[string]any{
					"address": "127.0.0.1",
					"port":    54321,
					"network": "tcp",
				},
			},
		},
		"outbounds": []any{
			outbound,
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("marshal config failed: %v", err)
	}

	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		t.Fatalf("write config file failed: %v", err)
	}

	// 查找随仓 xray.exe
	xrayBin := filepath.Join("..", "..", "..", "tools", "xray-windows-64", "xray.exe")
	if _, err := os.Stat(xrayBin); os.IsNotExist(err) {
		t.Skip("xray.exe not found, skipping gold standard binary test")
		return
	}

	cmd := exec.Command(xrayBin, "-test", "-c", configFile)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("xray -test 校验失败: %v\nOutput: %s\nConfig:\n%s", err, string(out), string(data))
	}
}

// TestVLESS_ClientCodec 测试 VLESS 协议的 URL 格式化、解析与 Xray 出站生成。
func TestVLESS_ClientCodec(t *testing.T) {
	// 1. Reality + TCP 测试
	rawReality := "vless://00000000-0000-0000-0000-000000000001@example.com:443?type=tcp&security=reality&pbk=1fX-n4WsBK43qAdhkvmHMvshe533A_Aj5fWcnSGcyzw&sid=1234abcd&sni=www.apple.com&fp=chrome&flow=xtls-rprx-vision#MyNode"
	dto1, err := protocols.ParseLink(rawReality)
	if err != nil {
		t.Fatalf("ParseLink reality failed: %v", err)
	}
	if dto1.Protocol != "vless" || dto1.ServerHost != "example.com" || dto1.ServerPort != 443 {
		t.Fatalf("DTO 基础参数不匹配: %+v", dto1)
	}
	if dto1.Auth.UUID != "00000000-0000-0000-0000-000000000001" || dto1.Auth.Flow != "xtls-rprx-vision" {
		t.Fatalf("DTO 认证参数不匹配: %+v", dto1.Auth)
	}
	if dto1.Security.Type != "reality" || dto1.Security.Reality.PublicKey != "1fX-n4WsBK43qAdhkvmHMvshe533A_Aj5fWcnSGcyzw" || dto1.Security.Reality.ShortID != "1234abcd" {
		t.Fatalf("DTO Reality 不匹配: %+v", dto1.Security)
	}
	// 链接未声明该参数时按面板默认（开启）解析。
	if !dto1.Security.Reality.SupportMLKEM {
		t.Fatalf("DTO REALITY 默认应开启 SupportMLKEM")
	}

	// 格式化回 URL
	link1, err := protocols.FormatLink(dto1)
	if err != nil {
		t.Fatalf("FormatLink reality failed: %v", err)
	}
	dto1Re, err := protocols.ParseLink(link1)
	if err != nil {
		t.Fatalf("ParseLink re-parse failed: %v", err)
	}
	if dto1Re.Auth.UUID != dto1.Auth.UUID || dto1Re.Security.Reality.PublicKey != dto1.Security.Reality.PublicKey {
		t.Fatalf("Round-trip DTO 不一致")
	}
	if !dto1Re.Security.Reality.SupportMLKEM {
		t.Fatalf("Round-trip 后 SupportMLKEM 丢失")
	}
	// Xray ≥26.9.8 的 REALITY 服务端强制要求 ClientHello 携带 X25519MLKEM768；
	// mihomo 默认剥离，链接必须显式声明该开关。
	if !strings.Contains(link1, "support-x25519mlkem768=1") {
		t.Errorf("reality 链接缺少 support-x25519mlkem768=1: %s", link1)
	}

	// 反向验证：开关来自基准模型，而非渲染器无条件硬编码。
	off := *dto1
	offSec := *dto1.Security
	offReality := *dto1.Security.Reality
	offReality.SupportMLKEM = false
	offSec.Reality = &offReality
	off.Security = &offSec
	linkOff, err := protocols.FormatLink(&off)
	if err != nil {
		t.Fatalf("FormatLink (mlkem off) failed: %v", err)
	}
	if strings.Contains(linkOff, "support-x25519mlkem768") {
		t.Errorf("SupportMLKEM=false 时链接不应输出该参数: %s", linkOff)
	}

	// 渲染出站并验证
	out1, err := protocols.RenderOutbound(dto1)
	if err != nil {
		t.Fatalf("RenderOutbound failed: %v", err)
	}
	if out1["protocol"] != "vless" {
		t.Fatalf("outbound protocol want vless, got %v", out1["protocol"])
	}
	runXrayTestHelper(t, out1)

	// 2. XHTTP + TLS 测试
	rawXHTTP := "vless://00000000-0000-0000-0000-000000000002@xh.example.com:8443?type=xhttp&security=tls&sni=xh.example.com&path=%2Fcustom&mode=stream-up#XHTTP-Node"
	dto2, err := protocols.ParseLink(rawXHTTP)
	if err != nil {
		t.Fatalf("ParseLink xhttp failed: %v", err)
	}
	if dto2.Transport.Network != "xhttp" || dto2.Transport.Path != "/custom" || dto2.Transport.Mode != "stream-up" {
		t.Fatalf("DTO xhttp 参数不匹配: %+v", dto2.Transport)
	}
	out2, err := protocols.RenderOutbound(dto2)
	if err != nil {
		t.Fatalf("RenderOutbound xhttp failed: %v", err)
	}
	runXrayTestHelper(t, out2)
}

// TestHysteria_ClientCodec 测试 Hysteria 2 协议的 URL 格式化、解析与 Xray 出站生成。
func TestHysteria_ClientCodec(t *testing.T) {
	rawHy2 := "hysteria2://mySecretPass@hy.example.com:443/?sni=hy.example.com&insecure=1#Hy2Node"
	dto, err := protocols.ParseLink(rawHy2)
	if err != nil {
		t.Fatalf("ParseLink hy2 failed: %v", err)
	}
	if dto.Protocol != "hysteria" || dto.Auth.Password != "mySecretPass" || dto.ServerPort != 443 {
		t.Fatalf("DTO 参数不匹配: %+v", dto)
	}
	if dto.Security.SNI != "hy.example.com" || !dto.Security.AllowInsecure {
		t.Fatalf("DTO TLS 参数不匹配: %+v", dto.Security)
	}

	// 测试 hy2:// 别名
	rawHy2Alias := "hy2://mySecretPass2@hy2.example.com:8443/?sni=hy2.example.com#Hy2Alias"
	dtoAlias, err := protocols.ParseLink(rawHy2Alias)
	if err != nil {
		t.Fatalf("ParseLink hy2 alias failed: %v", err)
	}
	if dtoAlias.Auth.Password != "mySecretPass2" {
		t.Fatalf("DTO Alias password want mySecretPass2, got %v", dtoAlias.Auth.Password)
	}

	// 格式化回 URL
	link, err := protocols.FormatLink(dto)
	if err != nil {
		t.Fatalf("FormatLink failed: %v", err)
	}
	dtoRe, err := protocols.ParseLink(link)
	if err != nil {
		t.Fatalf("ParseLink re-parse failed: %v", err)
	}
	if dtoRe.Auth.Password != dto.Auth.Password {
		t.Fatalf("Round-trip password 不一致")
	}

	// 渲染出站并验证
	out, err := protocols.RenderOutbound(dto)
	if err != nil {
		t.Fatalf("RenderOutbound failed: %v", err)
	}
	if out["protocol"] != "hysteria" {
		t.Fatalf("protocol want hysteria, got %v", out["protocol"])
	}
	// 验证密码只在 streamSettings.hysteriaSettings.auth
	ss := out["streamSettings"].(map[string]any)
	hySettings := ss["hysteriaSettings"].(map[string]any)
	if hySettings["auth"] != "mySecretPass" {
		t.Fatalf("hysteriaSettings auth 不正确: %v", hySettings["auth"])
	}
	runXrayTestHelper(t, out)
}

// TestMixed_ClientCodec 测试 SOCKS5 与 HTTP 协议的 URL 格式化、解析与 Xray 出站生成。
func TestRenderOutbound_TLSCAAndPin(t *testing.T) {
	// 测试 VLESS TLS 携带 CA 证书与 Pin SHA256 出站生成并送 xray.exe 校验
	rawVless := "vless://a0000000-0000-0000-0000-000000000001@1.2.3.4:443?security=tls&sni=proxy.example.com#VlessNode"
	dto, err := protocols.ParseLink(rawVless)
	if err != nil {
		t.Fatalf("ParseLink failed: %v", err)
	}
	dto.Security.PinnedPeerCertSha256 = "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff"
	dto.Security.DisableSystemRoot = true
	dto.Security.VerifyPeerCertByName = "proxy.example.com"
	dto.Security.CACertFile = "ca.crt"

	out, err := protocols.RenderOutbound(dto)
	if err != nil {
		t.Fatalf("RenderOutbound failed: %v", err)
	}

	stream := out["streamSettings"].(map[string]any)
	tlsSettings := stream["tlsSettings"].(map[string]any)
	if tlsSettings["pinnedPeerCertSha256"] != "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff" {
		t.Errorf("pinnedPeerCertSha256 不符合预期: %v", tlsSettings)
	}
	if tlsSettings["disableSystemRoot"] != true {
		t.Errorf("disableSystemRoot 不符合预期: %v", tlsSettings)
	}
	if tlsSettings["verifyPeerCertByName"] != "proxy.example.com" {
		t.Errorf("verifyPeerCertByName 不符合预期: %v", tlsSettings)
	}
	certs := tlsSettings["certificates"].([]any)
	if len(certs) != 1 || certs[0].(map[string]any)["usage"] != "verify" {
		t.Errorf("CA certificates 不符合预期: %v", certs)
	}
}

func TestProjectPeer_CAAndPinVerification(t *testing.T) {
	vlessPlugin := protocols.Find("vless")
	inb := models.Inbound{
		ID:             10,
		Protocol:       "vless",
		Port:           443,
		InternalUUID:   "peer-uuid-1234",
		StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"peer.example.com"}}`,
	}

	targetWithCA := contracts.RefTarget{
		Inbound:          inb,
		ServerHost:       "node-b.example.com",
		TargetCertDomain: "peer.example.com",
		CertPin:          "1122334455667788",
		CACert:           "-----BEGIN CERTIFICATE-----\nMIICA...\n-----END CERTIFICATE-----",
	}

	// 1. 自动模式（优先 CA）
	dtoAuto, err := vlessPlugin.ProjectClient(&contracts.ClientProjectionInput{
		Role:         contracts.RolePeer,
		RefTarget:    &targetWithCA,
		OutboundUUID: "out-peer-uuid",
	})
	if err != nil {
		t.Fatalf("ProjectClient auto failed: %v", err)
	}
	if dtoAuto.Security.CACert != targetWithCA.CACert {
		t.Errorf("CACert want %s, got %s", targetWithCA.CACert, dtoAuto.Security.CACert)
	}
	if !dtoAuto.Security.DisableSystemRoot {
		t.Errorf("DisableSystemRoot should be true")
	}
	if dtoAuto.Security.VerifyPeerCertByName != "peer.example.com" {
		t.Errorf("VerifyPeerCertByName want peer.example.com, got %s", dtoAuto.Security.VerifyPeerCertByName)
	}
	if _, ok := dtoAuto.Auth.Extra["CertPin"]; ok {
		t.Errorf("CertPin should be cleared in CA mode")
	}

	// 2. 强制 Pin 模式
	dtoPin, err := vlessPlugin.ProjectClient(&contracts.ClientProjectionInput{
		Role:          contracts.RolePeer,
		RefTarget:     &targetWithCA,
		OutboundUUID:  "out-peer-uuid",
		TlsVerifyMode: "pin",
	})
	if err != nil {
		t.Fatalf("ProjectClient pin failed: %v", err)
	}
	if dtoPin.Security.CACert != "" {
		t.Errorf("CACert should be empty in Pin mode")
	}
	if dtoPin.Auth.Extra["CertPin"] != "1122334455667788" {
		t.Errorf("CertPin want 1122334455667788, got %v", dtoPin.Auth.Extra["CertPin"])
	}

	// 3. 渲染出站测试（CA 模式）
	rendered, err := protocols.RenderOutbound(dtoAuto)
	if err != nil {
		t.Fatalf("RenderOutbound failed: %v", err)
	}
	runXrayTestHelper(t, rendered)
}

// TestProjectPeer_CAFileWhenAgentSupports：源节点声明 push_ca 后，机间 CA 验签
// 必须写成 certificateFile（文件由 push_ca 落盘），而不是内联 PEM。
func TestProjectPeer_CAFileWhenAgentSupports(t *testing.T) {
	vlessPlugin := protocols.Find("vless")
	inb := models.Inbound{
		ID:             11,
		Protocol:       "vless",
		Port:           443,
		InternalUUID:   "peer-uuid-5678",
		StreamSettings: `{"network":"tcp","security":"tls","tlsSettings":{"serverName":"peer.example.com"}}`,
	}
	target := contracts.RefTarget{
		Inbound:          inb,
		ServerHost:       "node-b.example.com",
		TargetCertDomain: "peer.example.com",
		CertPin:          "1122334455667788",
		CACertID:         7,
		CACertFile:       "/etc/xray/certs/ca/7.crt",
	}
	dto, err := vlessPlugin.ProjectClient(&contracts.ClientProjectionInput{
		Role:         contracts.RolePeer,
		RefTarget:    &target,
		OutboundUUID: "out-peer-uuid",
	})
	if err != nil {
		t.Fatalf("ProjectClient failed: %v", err)
	}
	if dto.Security.CACertFile != target.CACertFile {
		t.Errorf("CACertFile want %s, got %s", target.CACertFile, dto.Security.CACertFile)
	}
	if dto.Security.CACert != "" {
		t.Errorf("CACert 应留空（文件模式），got %s", dto.Security.CACert)
	}
	if !dto.Security.DisableSystemRoot || dto.Security.VerifyPeerCertByName != "peer.example.com" {
		t.Errorf("CA 模式字段不完整: %+v", dto.Security)
	}

	rendered, err := protocols.RenderOutbound(dto)
	if err != nil {
		t.Fatalf("RenderOutbound failed: %v", err)
	}
	raw, _ := json.Marshal(rendered)
	s := string(raw)
	if !strings.Contains(s, `"certificateFile":"/etc/xray/certs/ca/7.crt"`) {
		t.Errorf("出站未引用 CA 文件: %s", s)
	}
	if strings.Contains(s, `"certificate":[`) {
		t.Errorf("文件模式不应再内联 CA PEM: %s", s)
	}
	if !strings.Contains(s, `"disableSystemRoot":true`) || !strings.Contains(s, `"verifyPeerCertByName":"peer.example.com"`) {
		t.Errorf("出站缺少 CA 验签字段: %s", s)
	}
}
