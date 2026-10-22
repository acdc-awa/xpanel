package protocols_test

import (
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/protocols"
	"github.com/acdc-awa/xpanel/internal/models"
)

// TestHysteriaFinalMask_SalamanderGeckoAndHop 锁定 Hysteria 2 的客户端扩展参数投影：
// 服务端 streamSettings.finalmask 是 obfs/带宽/端口跳跃的唯一真源，订阅与机间出站
// 必须从它派生；映射错一处就是「服务端开混淆、客户端不带」→ 全体连接不通。
func TestHysteriaFinalMask_SalamanderGeckoAndHop(t *testing.T) {
	inb := models.Inbound{
		ID: 301, Tag: "hy2-obfs", Protocol: models.ProtocolHysteria, Port: 44321,
		StreamSettings: `{
			"network":"hysteria","security":"tls",
			"tlsSettings":{"serverName":"hy.example.com"},
			"finalmask":{
				"udp":[
					{"type":"salamander","settings":{"password":"obspw","packetSize":"1200-1400"}},
					{"type":"udphop","settings":{"mode":"intervalRemote","interval":"10-20","remotePorts":"20000-30000"}}
				],
				"quicParams":{"brutalUp":"100 mbps","brutalDown":"50 mbps"}
			}
		}`,
	}

	plugin := protocols.Find(models.ProtocolHysteria)
	dto, err := plugin.ProjectClient(&contracts.ClientProjectionInput{
		Role:     contracts.RoleSubscription,
		Name:     "hy2",
		Host:     "hy.example.com",
		Port:     44321,
		Spec:     contracts.DecodeInbound(&inb),
		UserUUID: "pass",
	})
	if err != nil || dto == nil {
		t.Fatalf("投影失败: %v", err)
	}

	opts := dto.Transport.Opts
	want := map[string]string{
		protocols.Hy2OptObfs:          "gecko", // packetSize 存在 → mihomo 侧必须是 gecko
		protocols.Hy2OptObfsPassword:  "obspw",
		protocols.Hy2OptObfsMinPacket: "1200",
		protocols.Hy2OptObfsMaxPacket: "1400",
		// 方向相对论：服务端 brutalUp（100）是客户端下行，brutalDown（50）是客户端上行。
		protocols.Hy2OptBwUp:        "50 mbps",
		protocols.Hy2OptBwDown:      "100 mbps",
		protocols.Hy2OptPorts:       "20000-30000",
		protocols.Hy2OptHopInterval: "10-20",
	}
	for k, v := range want {
		if got := opts[k]; got != v {
			t.Errorf("Opts[%s] = %q, want %q", k, got, v)
		}
	}

	// 机间出站必须能被 xray 接受，且 obfs/端口跳跃要回写成 xray 形态。
	out, err := protocols.RenderOutbound(dto)
	if err != nil {
		t.Fatalf("RenderOutbound: %v", err)
	}
	ss := out["streamSettings"].(map[string]any)
	fm, ok := ss["finalmask"].(map[string]any)
	if !ok {
		t.Fatal("出站缺少 finalmask（对端开了 obfs，缺了必然连不上）")
	}
	udp := fm["udp"].([]any)
	if len(udp) != 2 {
		t.Fatalf("finalmask.udp 条目数 = %d, want 2", len(udp))
	}
	sal := udp[0].(map[string]any)
	if sal["type"] != "salamander" {
		t.Errorf("第 1 条 mask = %v, want salamander（gecko 在 xray 侧仍是 salamander+packetSize）", sal["type"])
	}
	settings := sal["settings"].(map[string]any)
	// xray 的 Int32Range 只接受字符串，必须是 "1200-1400" 而不是 {from,to} 对象。
	if settings["packetSize"] != "1200-1400" {
		t.Errorf("packetSize = %v, want \"1200-1400\"", settings["packetSize"])
	}
	hop := udp[1].(map[string]any)
	if hop["type"] != "udphop" {
		t.Errorf("第 2 条 mask = %v, want udphop", hop["type"])
	}
	hopSettings := hop["settings"].(map[string]any)
	if hopSettings["mode"] != "intervalRemote" || hopSettings["remotePorts"] != "20000-30000" {
		t.Errorf("udphop settings = %v", hopSettings)
	}

	// 官方二进制金标准：生成结果必须能加载。
	runXrayTestHelper(t, out)
}

// TestHysteriaFinalMask_LocalHopNotExposed 反向验证：intervalLocal 只跳源端口，
// mihomo 的 ports 表达不了，不能硬映射（否则语义被悄悄改成目标端口跳跃）。
func TestHysteriaFinalMask_LocalHopNotExposed(t *testing.T) {
	inb := models.Inbound{
		ID: 302, Tag: "hy2-localhop", Protocol: models.ProtocolHysteria, Port: 44321,
		StreamSettings: `{"network":"hysteria","security":"tls",
			"finalmask":{"udp":[{"type":"udphop","settings":{"mode":"intervalLocal","interval":"10","remotePorts":"20000-30000"}}]}}`,
	}
	plugin := protocols.Find(models.ProtocolHysteria)
	dto, err := plugin.ProjectClient(&contracts.ClientProjectionInput{
		Role: contracts.RoleSubscription, Name: "hy2", Host: "h", Port: 44321,
		Spec: contracts.DecodeInbound(&inb), UserUUID: "p",
	})
	if err != nil || dto == nil {
		t.Fatalf("投影失败: %v", err)
	}
	if ports := dto.Transport.Opts[protocols.Hy2OptPorts]; ports != "" {
		t.Errorf("intervalLocal 不应投影 ports，got %q", ports)
	}
}

// TestHysteriaFormatLink_Extras 链接必须携带 obfs/带宽/端口跳跃，否则 base64 订阅用户断连。
func TestHysteriaFormatLink_Extras(t *testing.T) {
	dto := &contracts.ProxyNodeDTO{
		Name: "hy2", ServerHost: "hy.example.com", ServerPort: 44321, Protocol: "hysteria",
		Transport: &contracts.TransportOptions{Network: "hysteria", Opts: map[string]string{
			protocols.Hy2OptObfs:         "salamander",
			protocols.Hy2OptObfsPassword: "obspw",
			protocols.Hy2OptBwUp:         "100 mbps",
			protocols.Hy2OptBwDown:       "100000000", // xray 无单位写法 = 12.5MB/s
			protocols.Hy2OptPorts:        "20000-30000",
		}},
		Security: &contracts.SecurityOptions{Type: "tls", SNI: "hy.example.com"},
		Auth:     &contracts.ClientCredentialDTO{Password: "secret"},
	}
	link, err := protocols.FormatLink(dto)
	if err != nil {
		t.Fatalf("FormatLink: %v", err)
	}
	for _, want := range []string{"obfs=salamander", "obfs-password=obspw", "up=100+Mbps", "down=12500000+Bps", "hy.example.com:20000-30000"} {
		if !strings.Contains(link, want) {
			t.Errorf("链接缺少 %q: %s", want, link)
		}
	}

	// 回环解析：端口跳跃与 obfs 必须能读回来。
	back, err := protocols.ParseLink(link)
	if err != nil {
		t.Fatalf("ParseLink: %v", err)
	}
	if back.ServerPort != 20000 {
		t.Errorf("端口跳跃回环 ServerPort = %d, want 20000", back.ServerPort)
	}
	if back.Transport.Opts[protocols.Hy2OptPorts] != "20000-30000" {
		t.Errorf("回环 ports = %q", back.Transport.Opts[protocols.Hy2OptPorts])
	}
	if back.Transport.Opts[protocols.Hy2OptObfs] != "salamander" {
		t.Errorf("回环 obfs = %q", back.Transport.Opts[protocols.Hy2OptObfs])
	}
}
