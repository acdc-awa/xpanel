package subscribe

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
)

func TestBuildNodeDTO_NoLayer_FollowsTransport(t *testing.T) {
	// 无接入层 = 直连入站：对外安全层就是入站自己的 streamSettings.security（这里 none）；
	// path / host 也只取传输设置。旧模型里用 share_security 声明「外部 Caddy 做了 TLS」
	// 已经移除——这种入口必须建成接入层（见 TestBuildNodeDTO_AccessLayer）。
	srv := &models.Server{
		Name: "香港01-直连",
		Host: "hk.node.local",
	}

	inb := &models.Inbound{
		Tag:               "vless-xhttp-direct",
		Protocol:          "vless",
		Port:              10086,
		Listen:            "127.0.0.1",
		StreamSettings:    `{"network":"xhttp","security":"none","xhttpSettings":{"mode":"auto","path":"/my-vless-xhttp","host":"cdn.host.example"}}`,
		ShareAddrStrategy: "custom",
		ShareAddr:         "caddy.example.com",
		SharePort:         443,
		ShareSNI:          "caddy.example.com",
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
	if dto.Security == nil || dto.Security.Type != "none" {
		t.Fatalf("无层时应跟随传输安全层 none: got %+v", dto.Security)
	}
	if dto.Transport == nil || dto.Transport.Path != "/my-vless-xhttp" || dto.Transport.Host != "cdn.host.example" {
		t.Fatalf("path/host 应只取传输设置: got %+v", dto.Transport)
	}

	// 2. 验证 Clash YAML 输出格式
	clashYAML := BuildClashWithTemplate([]contracts.ProxyNodeDTO{*dto}, "")
	if !strings.Contains(clashYAML, "server: caddy.example.com") {
		t.Errorf("Clash YAML 缺少 server: caddy.example.com:\n%s", clashYAML)
	}
	if !strings.Contains(clashYAML, "port: 443") {
		t.Errorf("Clash YAML 缺少 port: 443:\n%s", clashYAML)
	}
	if !strings.Contains(clashYAML, "tls: false") {
		t.Errorf("Clash YAML 缺少 tls: false:\n%s", clashYAML)
	}
	if !strings.Contains(clashYAML, "network: xhttp") {
		t.Errorf("Clash YAML 缺少 network: xhttp:\n%s", clashYAML)
	}
	if !strings.Contains(clashYAML, "path: '/my-vless-xhttp'") {
		t.Errorf("Clash YAML 缺少 path: '/my-vless-xhttp':\n%s", clashYAML)
	}
	if !strings.Contains(clashYAML, "host: 'cdn.host.example'") {
		t.Errorf("Clash YAML 缺少 host: 'cdn.host.example':\n%s", clashYAML)
	}

	// 3. 验证 Base64 输出格式
	b64 := BuildBase64([]contracts.ProxyNodeDTO{*dto})
	dec, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("Base64 解码失败: %v", err)
	}
	vlessURL := string(dec)
	if !strings.Contains(vlessURL, "security=none") {
		t.Errorf("vless:// 缺少 security=none: %s", vlessURL)
	}
	if !strings.Contains(vlessURL, "host=cdn.host.example") {
		t.Errorf("vless:// 缺少 host=cdn.host.example: %s", vlessURL)
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
	// SNI 缺省回落层 Host，path 只取值入站 xhttpSettings.path；share_sni 显式覆写优先。
	srv := &models.Server{Name: "香港01-Layer", Host: "hk.node.local"}
	lid := uint64(9)
	inb := &models.Inbound{
		Tag:            "vless-xhttp-layer",
		Protocol:       "vless",
		Port:           10086,
		Listen:         "127.0.0.1",
		StreamSettings: `{"network":"xhttp","security":"none","xhttpSettings":{"mode":"auto","path":"/my-vless-xhttp"}}`,
		LayerID:        &lid,
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
		t.Fatalf("Path 应取入站 xhttpSettings.path: got %+v", dto.Transport)
	}

	// share_sni 显式覆写优先于层 Host（安全层为 tls 时才有 SNI 字段）
	inb.ShareSNI = "override.example.com"
	dto2 := BuildNodeDTO(srv, inb, uuid, layer)
	if dto2.Security.SNI != "override.example.com" {
		t.Fatalf("share_sni 应覆写层 Host: got %s", dto2.Security.SNI)
	}

	// 安全层不再有客户端覆写：层是 tls 就一定下发 tls（存量 share_security 列被忽略）
	inb.ShareSecurity = "none"
	inb.ShareSNI = ""
	dto3 := BuildNodeDTO(srv, inb, uuid, layer)
	if dto3.Security == nil || dto3.Security.Type != "tls" {
		t.Fatalf("安全层应恒由接入层决议: got %+v", dto3.Security)
	}
}
