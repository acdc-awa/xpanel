package protocols_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/acdc-awa/xpanel/internal/master/protocols"
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
func TestMixed_ClientCodec(t *testing.T) {
	// 1. SOCKS5 链接解析与出站
	rawSocks := "socks5://user1:pass1@1.2.3.4:1080#SocksNode"
	dto1, err := protocols.ParseLink(rawSocks)
	if err != nil {
		t.Fatalf("ParseLink socks5 failed: %v", err)
	}
	if dto1.Protocol != "socks" || dto1.Auth.User != "user1" || dto1.Auth.Password != "pass1" {
		t.Fatalf("DTO socks5 参数不匹配: %+v", dto1)
	}
	out1, err := protocols.RenderOutbound(dto1)
	if err != nil {
		t.Fatalf("RenderOutbound socks5 failed: %v", err)
	}
	if out1["protocol"] != "socks" {
		t.Fatalf("protocol want socks, got %v", out1["protocol"])
	}
	runXrayTestHelper(t, out1)

	// 2. HTTP 链接解析与出站
	rawHTTP := "http://admin:secret@1.2.3.4:8080#HttpNode"
	dto2, err := protocols.ParseLink(rawHTTP)
	if err != nil {
		t.Fatalf("ParseLink http failed: %v", err)
	}
	if dto2.Protocol != "http" || dto2.Auth.User != "admin" || dto2.Auth.Password != "secret" {
		t.Fatalf("DTO http 参数不匹配: %+v", dto2)
	}
	out2, err := protocols.RenderOutbound(dto2)
	if err != nil {
		t.Fatalf("RenderOutbound http failed: %v", err)
	}
	if out2["protocol"] != "http" {
		t.Fatalf("protocol want http, got %v", out2["protocol"])
	}
	runXrayTestHelper(t, out2)

	// 3. HTTPS 与 SOCKS5S (带 TLS)
	rawHTTPS := "https://admin:secret@1.2.3.4:8443?sni=proxy.example.com#HttpsNode"
	dto3, err := protocols.ParseLink(rawHTTPS)
	if err != nil {
		t.Fatalf("ParseLink https failed: %v", err)
	}
	if dto3.Security.Type != "tls" || dto3.Security.SNI != "proxy.example.com" {
		t.Fatalf("DTO https TLS 不匹配: %+v", dto3.Security)
	}
	out3, err := protocols.RenderOutbound(dto3)
	if err != nil {
		t.Fatalf("RenderOutbound https failed: %v", err)
	}
	runXrayTestHelper(t, out3)
}
