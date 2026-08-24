package subscribe

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
)

func TestBuildNodeDTO_CaddyTLSReverseProxy(t *testing.T) {
	// 场景：服务端 Xray 本地监听 127.0.0.1:10086，明文 xhttp (security: none)
	// 外部 Caddy 监听 443，反代 /my-vless-xhttp 到 127.0.0.1:10086
	// 未挂层路径：入站 share_* 自持覆写（订阅导出 443/TLS/SNI/Path）
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
	dto := BuildNodeDTO(srv, inb, uuid, nil)
	if dto == nil {
		t.Fatalf("BuildNodeDTO returned nil")
	}

	// 1. 验证 DTO 属性
	if dto.ServerHost != "caddy.example.com" {
		t.Fatalf("want Host caddy.example.com, got %s", dto.ServerHost)
	}
	if dto.ServerPort != 443 {
		t.Fatalf("want Port 443, got %d", dto.ServerPort)
	}
	if dto.Security == nil || dto.Security.Type != "tls" {
		t.Fatalf("want Security Type tls, got %+v", dto.Security)
	}
	if dto.Security.SNI != "caddy.example.com" {
		t.Fatalf("want TLS SNI caddy.example.com, got %s", dto.Security.SNI)
	}
	if dto.Transport == nil || dto.Transport.Path != "/my-vless-xhttp" || dto.Transport.Host != "caddy.example.com" {
		t.Fatalf("want XHTTP Path /my-vless-xhttp and Host caddy.example.com, got %+v", dto.Transport)
	}

	// 2. 验证 Clash YAML 输出格式
	clashYAML := BuildClash([]contracts.ProxyNodeDTO{*dto})
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
	b64 := BuildBase64([]contracts.ProxyNodeDTO{*dto})
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
}

func TestBuildNodeDTO_AccessLayer(t *testing.T) {
	// 挂层路径：入站挂对外接入层（内部实现不可见），对外 host/port/security 由层决议；
	// SNI 缺省回落层 Host，path 沿用入站 share_path；share_security/share_sni 显式覆写优先。
	srv := &models.Server{Name: "香港01-Layer", Host: "hk.node.local"}
	lid := uint64(9)
	inb := &models.Inbound{
		Tag:            "vless-xhttp-layer",
		Protocol:       "vless",
		Port:           10086,
		Listen:         "127.0.0.1",
		StreamSettings: `{"network":"xhttp","security":"none","xhttpSettings":{"mode":"auto","path":"/xhttp"}}`,
		LayerID:        &lid,
		SharePath:      "/my-vless-xhttp",
	}
	layer := &models.AccessLayer{ID: 9, ServerID: 1, Name: "HK 443 反代层", Host: "caddy.example.com", Port: 443, Security: "tls"}

	uuid := "11111111-2222-3333-4444-555555555555"
	dto := BuildNodeDTO(srv, inb, uuid, layer)
	if dto == nil {
		t.Fatalf("BuildNodeDTO returned nil")
	}
	if dto.ServerHost != "caddy.example.com" {
		t.Fatalf("挂层 host 应由层决议: got %s", dto.ServerHost)
	}
	if dto.ServerPort != 443 {
		t.Fatalf("挂层 port 应由层决议: got %d", dto.ServerPort)
	}
	if dto.Security == nil || dto.Security.Type != "tls" {
		t.Fatalf("挂层 security 应由层决议 tls: got %+v", dto.Security)
	}
	if dto.Security.SNI != "caddy.example.com" {
		t.Fatalf("SNI 缺省应回落层 Host: got %s", dto.Security.SNI)
	}
	if dto.Transport == nil || dto.Transport.Path != "/my-vless-xhttp" {
		t.Fatalf("Path 应沿用入站 share_path: got %+v", dto.Transport)
	}

	// share_security 显式覆写优先于层；share_sni 显式覆写优先于层 Host（security=tls 才有 SNI 层）
	inb.ShareSecurity = "tls"
	inb.ShareSNI = "override.example.com"
	dto2 := BuildNodeDTO(srv, inb, uuid, layer)
	if dto2.Security == nil || dto2.Security.Type != "tls" {
		t.Fatalf("share_security=tls 应覆写层（层也是 tls）: got %+v", dto2.Security)
	}
	if dto2.Security.SNI != "override.example.com" {
		t.Fatalf("share_sni 应覆写层 Host: got %s", dto2.Security.SNI)
	}

	// security=none 覆写生效（无 TLS 层，SNI 自然不产出）
	inb.ShareSecurity = "none"
	inb.ShareSNI = ""
	dto3 := BuildNodeDTO(srv, inb, uuid, layer)
	if dto3.Security == nil || dto3.Security.Type != "none" {
		t.Fatalf("share_security=none 应覆写层 tls: got %+v", dto3.Security)
	}
}
