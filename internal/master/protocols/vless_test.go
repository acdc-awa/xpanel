package protocols

import (
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
)

// decodeStream 测试辅助：仅解码 streamSettings（生产统一入口为 contracts.DecodeInbound）。
func decodeStream(raw string) *contracts.InboundSpec {
	return contracts.DecodeInbound(&models.Inbound{StreamSettings: raw})
}

// TestVLESSDescriptor 验证描述符属性。
func TestVLESSDescriptor(t *testing.T) {
	p := VLESSPlugin{}
	desc := p.Descriptor()
	if desc.Name != "vless" {
		t.Fatalf("expected vless, got %s", desc.Name)
	}
	if !desc.HotSync || !desc.SupportsUDP {
		t.Fatalf("vless descriptor capabilities mismatch: %+v", desc)
	}
}

// TestVLESSResolveFlow flow 决议（生成侧盖戳与订阅侧同源）：
// 入站级 none 禁用自动注入 → 空 + tcp+reality 自动 vision → vlessenc 一律不产出 → 其余透传。
func TestVLESSResolveFlow(t *testing.T) {
	const validDec = "mlkem768x25519plus.native.600s.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	p := VLESSPlugin{}
	cases := []struct {
		name     string
		network  string
		security string
		dec      string
		inbFlow  string
		want     string
	}{
		{"自动: tcp+reality", "tcp", "reality", "", "", "xtls-rprx-vision"},
		{"自动: 非 reality 不注入", "tcp", "tls", "", "", ""},
		{"自动: xhttp+reality 不注入", "xhttp", "reality", "", "", ""},
		{"入站级开启", "tcp", "reality", "", "xtls-rprx-vision", "xtls-rprx-vision"},
		{"入站级开启(tls 透传)", "tcp", "tls", "", "xtls-rprx-vision", "xtls-rprx-vision"},
		{"入站级 none 禁用", "tcp", "reality", "", "none", ""},
		{"入站级 none + 非 reality", "tcp", "tls", "", "none", ""},
		// vlessenc 与 XTLS 不搭配：即使显式开了 vision 也按空处理（两端同源，不会失配）
		{"vlessenc 不自动注入", "tcp", "none", validDec, "", ""},
		{"vlessenc 覆盖显式 vision", "tcp", "none", validDec, "xtls-rprx-vision", ""},
		{"vlessenc + reality 不注入", "tcp", "reality", validDec, "", ""},
	}
	for _, c := range cases {
		spec := &contracts.InboundSpec{Network: c.network, Security: c.security, Settings: map[string]any{"decryption": c.dec}}
		if got := p.ResolveFlow(spec, c.inbFlow); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestVLESSServerUsers 服务端用户注入与反向验证。
func TestVLESSServerUsers(t *testing.T) {
	// 经由注册表分发，验证协议路由正确
	key, clients, err := ServerUsers("vless", nil, &contracts.InboundSpec{})
	if err != nil {
		t.Fatalf("vless ServerUsers 失败: %v", err)
	}
	if key != "clients" || len(clients) != 0 {
		t.Fatalf("空用户应产出 key=clients 空列表，got key=%s, len=%d", key, len(clients))
	}

	// 反向验证：未注册协议必须显式硬报错（Fail-Closed 铁律）
	_, _, err = ServerUsers("vmess", nil, &contracts.InboundSpec{})
	if err == nil {
		t.Fatal("未注册协议应显式报错，但 err 为 nil")
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
	spec := decodeStream(`{"network":"tcp","security":"reality","realitySettings":{
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
	bad := decodeStream(`{"network":"tcp","security":"reality","realitySettings":{"publicKey":"pbk"}}`)
	if dto := p.BuildClientNode(&contracts.ClientNodeInput{
		Name: "n2", Host: "h2", Port: 443, Spec: bad, UserUUID: "uuid-2",
	}); dto != nil {
		t.Errorf("reality 缺 SNI 应返回 nil，got %+v", dto)
	}

	// 入口安全层由接入层传入（EntrySecurity）；客户端只覆写 SNI。
	// xhttp 的 path/host 全部来自传输设置，没有第二来源。
	caddy := decodeStream(`{"network":"xhttp","security":"none","xhttpSettings":{"mode":"auto","path":"/my-vless-xhttp","host":"caddy.example.com"}}`)
	dto = p.BuildClientNode(&contracts.ClientNodeInput{
		Name: "n3", Host: "caddy.example.com", Port: 443, Spec: caddy,
		EntrySecurity: "tls",
		Share:         contracts.ShareOverride{SNI: "caddy.example.com"},
		UserUUID:      "uuid-3",
	})
	if dto == nil {
		t.Fatal("接入层安全层应产出 DTO")
	}
	if dto.Security.Type != "tls" || dto.Security.SNI != "caddy.example.com" {
		t.Errorf("接入层安全层/SNI 未生效: %+v", dto.Security)
	}
	if dto.Transport.Path != "/my-vless-xhttp" || dto.Transport.Host != "caddy.example.com" || dto.Transport.Mode != "auto" {
		t.Errorf("xhttp 传输参数未生效: %+v", dto.Transport)
	}
}

// TestVLESSProjectPeer 节点互联端点投影。
func TestVLESSProjectPeer(t *testing.T) {
	p := VLESSPlugin{}
	inb := models.Inbound{
		ID:           101,
		Tag:          "relay-land",
		Protocol:     "vless",
		Port:         8443,
		InternalUUID: "internal-uuid-peer",
		StreamSettings: `{"network":"tcp","security":"reality","fingerprint":"chrome","realitySettings":{
			"serverNames":["peer.example.com"],"publicKey":"peer-pbk","shortIds":["cd34"],"privateKey":"priv"}}`,
	}
	target := &contracts.RefTarget{
		Inbound:    inb,
		ServerHost: "peer-exit.example.com",
		CertPin:    "aabbccddeeff",
	}

	dto, err := p.ProjectClient(&contracts.ClientProjectionInput{
		Role:      contracts.RolePeer,
		RefTarget: target,
	})
	if err != nil {
		t.Fatalf("ProjectClient(peer) 失败: %v", err)
	}
	if dto == nil {
		t.Fatal("ProjectClient(peer) 返回了 nil")
	}

	if dto.ServerHost != "peer-exit.example.com" || dto.ServerPort != 8443 {
		t.Errorf("目标地址端口不符合预期: host=%s, port=%d", dto.ServerHost, dto.ServerPort)
	}
	if dto.Auth.UUID != "internal-uuid-peer" || dto.Auth.Flow != "xtls-rprx-vision" {
		t.Errorf("peer auth 不符合预期: %+v", dto.Auth)
	}
	if dto.Auth.Extra["CertPin"] != "aabbccddeeff" {
		t.Errorf("CertPin 未正确透传: %+v", dto.Auth.Extra)
	}
	if dto.Security.Reality == nil || dto.Security.Reality.PublicKey != "peer-pbk" || dto.Security.Reality.ShortID != "cd34" {
		t.Errorf("reality 选项未正确提取: %+v", dto.Security.Reality)
	}
}

// TestVLESSProjectPeer_VlessEncNoFlow vlessenc 落地：参照出站必须派生 encryption 但**不带 flow**。
// 反向验证：若把 vlessEncActive 判断从 ProjectClient 里去掉，本用例会拿到 xtls-rprx-vision。
func TestVLESSProjectPeer_VlessEncNoFlow(t *testing.T) {
	const validDec = "mlkem768x25519plus.native.600s.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	p := VLESSPlugin{}
	inb := models.Inbound{
		ID:             102,
		Tag:            "relay-enc",
		Protocol:       "vless",
		Port:           8443,
		InternalUUID:   "internal-uuid-enc",
		SettingsJSON:   `{"decryption":"` + validDec + `"}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
	}
	dto, err := p.ProjectClient(&contracts.ClientProjectionInput{
		Role:      contracts.RolePeer,
		RefTarget: &contracts.RefTarget{Inbound: inb, ServerHost: "peer-exit.example.com"},
	})
	if err != nil {
		t.Fatalf("ProjectClient(peer) 失败: %v", err)
	}
	if dto == nil || dto.Auth == nil {
		t.Fatal("ProjectClient(peer) 返回空 DTO")
	}
	if dto.Auth.Encryption == "" || dto.Auth.Encryption == "none" {
		t.Errorf("vlessenc 落地应派生客户端 encryption，got %q", dto.Auth.Encryption)
	}
	if dto.Auth.Flow != "" {
		t.Errorf("vlessenc 落地不应带 flow，got %q", dto.Auth.Flow)
	}
}
