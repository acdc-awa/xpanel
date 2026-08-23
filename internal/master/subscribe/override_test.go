package subscribe

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/acdc/xray-panel/internal/models"
)

func TestBuildProxyItem_CaddyTLSReverseProxy(t *testing.T) {
	// 场景：服务端 Xray 本地监听 127.0.0.1:10086，明文 xhttp (security: none)
	// 外部 Caddy 监听 443，反代 /my-vless-xhttp 到 127.0.0.1:10086
	// 订阅需导出：端口 443，TLS 开启，SNI caddy.example.com，Path /my-vless-xhttp
	srv := &models.Server{
		Name: "香港01-Caddy反代",
		Host: "hk.node.local",
	}

	inb := &models.Inbound{
		Tag:                "vless-xhttp-caddy",
		Protocol:           "vless",
		Port:               10086,
		Listen:             "127.0.0.1",
		StreamSettings:     `{"network":"xhttp","security":"none","xhttpSettings":{"mode":"auto","path":"/xhttp"}}`,
		ShareAddrStrategy:  "custom",
		ShareAddr:          "caddy.example.com",
		SharePort:          443,
		ShareSecurity:      "tls",
		ShareSNI:           "caddy.example.com",
		ShareHost:          "caddy.example.com",
		SharePath:          "/my-vless-xhttp",
		ShareAllowInsecure: false,
	}

	uuid := "11111111-2222-3333-4444-555555555555"
	item := BuildProxyItem(srv, inb, uuid)

	// 1. 验证 ProxyItem 属性
	if item.Host != "caddy.example.com" {
		t.Fatalf("want Host caddy.example.com, got %s", item.Host)
	}
	if item.Port != 443 {
		t.Fatalf("want Port 443, got %d", item.Port)
	}
	if item.TLSType != "tls" {
		t.Fatalf("want TLSType tls, got %s", item.TLSType)
	}
	if item.TLS == nil || item.TLS.ServerName != "caddy.example.com" {
		t.Fatalf("want TLS SNI caddy.example.com, got %+v", item.TLS)
	}
	if item.XHTTP == nil || item.XHTTP.Path != "/my-vless-xhttp" || item.XHTTP.Host != "caddy.example.com" {
		t.Fatalf("want XHTTP Path /my-vless-xhttp and Host caddy.example.com, got %+v", item.XHTTP)
	}

	// 2. 验证 Clash YAML 输出格式
	user := &models.User{UUID: uuid}
	clashYAML := BuildClash(user, []ProxyItem{item})
	if !strings.Contains(clashYAML, "server: caddy.example.com") {
		t.Errorf("Clash YAML 缺少 server: caddy.example.com:\n%s", clashYAML)
	}
	if !strings.Contains(clashYAML, "port: 443") {
		t.Errorf("Clash YAML 缺少 port: 443:\n%s", clashYAML)
	}
	if !strings.Contains(clashYAML, "tls: true") {
		t.Errorf("Clash YAML 缺少 tls: true:\n%s", clashYAML)
	}
	if !strings.Contains(clashYAML, "network: xhttp") {
		t.Errorf("Clash YAML 缺少 network: xhttp:\n%s", clashYAML)
	}
	if !strings.Contains(clashYAML, "path: /my-vless-xhttp") {
		t.Errorf("Clash YAML 缺少 path: /my-vless-xhttp:\n%s", clashYAML)
	}

	// 3. 验证 Base64 输出格式
	b64 := BuildBase64(user, []ProxyItem{item})
	dec, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("Base64 解码失败: %v", err)
	}
	vlessURL := string(dec)
	if !strings.Contains(vlessURL, "security=tls") {
		t.Errorf("vless:// 缺少 security=tls: %s", vlessURL)
	}
	if !strings.Contains(vlessURL, "sni=caddy.example.com") {
		t.Errorf("vless:// 缺少 sni=caddy.example.com: %s", vlessURL)
	}
	if !strings.Contains(vlessURL, "type=xhttp") {
		t.Errorf("vless:// 缺少 type=xhttp: %s", vlessURL)
	}
	if !strings.Contains(vlessURL, "path=%2Fmy-vless-xhttp") {
		t.Errorf("vless:// 缺少 path=%%2Fmy-vless-xhttp: %s", vlessURL)
	}

	// 4. 验证转 DTO
	dto := item.ToDTO()
	if dto.ServerHost != "caddy.example.com" || dto.ServerPort != 443 || dto.TLSType != "tls" || dto.SNI != "caddy.example.com" || dto.Path != "/my-vless-xhttp" {
		t.Errorf("DTO 转换不匹配: %+v", dto)
	}
}
