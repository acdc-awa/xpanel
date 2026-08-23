package protocols

import (
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
)

// TestVLESSResolveFlow flow 决议（生成侧盖戳与订阅侧同源）：
// 入站级 none 禁用自动注入 → 空 + tcp+reality 自动 vision → 其余透传。
func TestVLESSResolveFlow(t *testing.T) {
	p := VLESSPlugin{}
	cases := []struct {
		name     string
		network  string
		security string
		inbFlow  string
		want     string
	}{
		{"自动: tcp+reality", "tcp", "reality", "", "xtls-rprx-vision"},
		{"自动: 非 reality 不注入", "tcp", "tls", "", ""},
		{"自动: xhttp+reality 不注入", "xhttp", "reality", "", ""},
		{"入站级开启", "tcp", "reality", "xtls-rprx-vision", "xtls-rprx-vision"},
		{"入站级开启(tls 透传)", "tcp", "tls", "xtls-rprx-vision", "xtls-rprx-vision"},
		{"入站级 none 禁用", "tcp", "reality", "none", ""},
		{"入站级 none + 非 reality", "tcp", "tls", "none", ""},
	}
	for _, c := range cases {
		spec := &contracts.InboundSpec{Network: c.network, Security: c.security}
		if got := p.ResolveFlow(spec, c.inbFlow); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestVLESSServerClients 服务端 clients 注入（id/email/flow/limit，空 UUID 跳过）。
func TestVLESSServerClients(t *testing.T) {
	// 经由注册表分发，验证协议路由正确
	clients := ServerClients("vless", nil, &contracts.InboundSpec{})
	if len(clients) != 0 {
		t.Fatalf("空用户应产出空 clients，got %v", clients)
	}
	// 未注册协议回退最小通用注入（不含 flow）
	fallback := ServerClients("vmess", nil, &contracts.InboundSpec{})
	if len(fallback) != 0 {
		t.Fatalf("未注册协议空用户应产出空 clients，got %v", fallback)
	}
	// 未注册协议 ResolveFlow 返回空（flow 是 VLESS 语义）
	spec := &contracts.InboundSpec{Network: "tcp", Security: "reality"}
	if got := ResolveFlow("vmess", spec, ""); got != "" {
		t.Fatalf("未注册协议 flow 应为空，got %q", got)
	}
}

// TestVLESSBuildClientNode 订阅 DTO 组装：reality 缺 SNI 返回 nil；share 覆写生效。
func TestVLESSBuildClientNode(t *testing.T) {
	p := VLESSPlugin{}

	// reality 完整参数
	spec := contracts.DecodeStream(`{"network":"tcp","security":"reality","realitySettings":{
		"serverNames":["www.example.com"],"publicKey":"pbk","shortIds":["ab12"],"privateKey":"priv","dest":"www.example.com:443"}}`)
	dto := p.BuildClientNode(&contracts.ClientNodeInput{
		Name: "n1", Host: "h1", Port: 443, Spec: spec, UserUUID: "uuid-1",
	})
	if dto == nil {
		t.Fatal("reality 完整参数应产出 DTO")
	}
	if dto.Auth.UUID != "uuid-1" || dto.Auth.Flow != "xtls-rprx-vision" {
		t.Errorf("凭证/flow 错误: %+v", dto.Auth)
	}
	if dto.Security.SNI != "www.example.com" || dto.Security.Reality == nil ||
		dto.Security.Reality.PublicKey != "pbk" || dto.Security.Reality.ShortID != "ab12" {
		t.Errorf("reality 归一化错误: %+v", dto.Security)
	}

	// reality 缺 serverName → nil（缺 SNI/公钥不产出订阅节点）
	bad := contracts.DecodeStream(`{"network":"tcp","security":"reality","realitySettings":{"publicKey":"pbk"}}`)
	if dto := p.BuildClientNode(&contracts.ClientNodeInput{
		Name: "n2", Host: "h2", Port: 443, Spec: bad, UserUUID: "uuid-2",
	}); dto != nil {
		t.Errorf("reality 缺 SNI 应返回 nil，got %+v", dto)
	}

	// share 覆写：security none→tls + SNI/Path/Host（Caddy 反代场景）
	caddy := contracts.DecodeStream(`{"network":"xhttp","security":"none","xhttpSettings":{"mode":"auto","path":"/xhttp"}}`)
	dto = p.BuildClientNode(&contracts.ClientNodeInput{
		Name: "n3", Host: "caddy.example.com", Port: 443, Spec: caddy,
		Share: contracts.ShareOverride{
			Security: "tls", SNI: "caddy.example.com",
			Host: "caddy.example.com", Path: "/my-vless-xhttp",
		},
		UserUUID: "uuid-3",
	})
	if dto == nil {
		t.Fatal("Caddy 反代场景应产出 DTO")
	}
	if dto.Security.Type != "tls" || dto.Security.SNI != "caddy.example.com" {
		t.Errorf("share 安全覆写错误: %+v", dto.Security)
	}
	if dto.Transport.Mode != "auto" || dto.Transport.Path != "/my-vless-xhttp" || dto.Transport.Host != "caddy.example.com" {
		t.Errorf("share 传输覆写错误: %+v", dto.Transport)
	}
	if dto.Auth.Flow != "" {
		t.Errorf("xhttp 不应注入 flow，got %q", dto.Auth.Flow)
	}

	// 入站级 flow=none + tcp+reality → 不注入（与服务端 clients 一致）
	noneFlow := contracts.DecodeStream(`{"network":"tcp","security":"reality","realitySettings":{"serverName":"s.com","publicKey":"pbk"}}`)
	dto = p.BuildClientNode(&contracts.ClientNodeInput{
		Name: "n4", Host: "h4", Port: 443, Spec: noneFlow, InboundFlow: "none", UserUUID: "uuid-4",
	})
	if dto == nil || dto.Auth.Flow != "" {
		t.Errorf("flow=none 应产出空 flow，got %+v", dto)
	}
}
