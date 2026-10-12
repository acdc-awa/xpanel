package protocols_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/protocols"
	"github.com/acdc-awa/xpanel/internal/master/subscribe"
	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
)

// TestProtocolContracts_FailClosed 反向验证：未注册协议必须显式硬报错阻断（Fail-Closed 铁律）。
func TestProtocolContracts_FailClosed(t *testing.T) {
	// 1. ServerUsers 必须报错
	_, _, err := protocols.ServerUsers("fake-unregistered-protocol", nil, &contracts.InboundSpec{})
	if err == nil {
		t.Fatal("未注册协议调用 protocols.ServerUsers 必须报错，但得到了 nil")
	}

	// 2. 生成器在遇到未知协议的 user 入站时必须拒绝生成并返回错误
	unregisteredInbound := models.Inbound{
		ID:       999,
		Tag:      "unregistered-inb",
		Protocol: "fake-unregistered-protocol",
		Port:     12345,
		Enabled:  true,
		Type:     models.InboundTypeUser,
	}
	_, err = xray.Generate([]models.Inbound{unregisteredInbound}, nil, nil, map[string][]protocol.User{
		"unregistered-inb": {
			{UUID: "00000000-0000-0000-0000-000000000001", Email: "test@panel.local"},
		},
	}, nil, "", "")
	if err == nil {
		t.Fatal("Generate 遇到未注册协议入站必须报错拒绝，但生成成功了")
	}
	if !strings.Contains(err.Error(), "未注册协议插件") {
		t.Fatalf("报错信息应指出未注册协议插件，got: %v", err)
	}

	// 3. 订阅层遇到未注册协议必须返回 nil 并记录，不得伪造节点
	srv := &models.Server{ID: 1, Host: "test.example.com"}
	dto := subscribe.BuildNodeDTO(srv, &unregisteredInbound, "00000000-0000-0000-0000-000000000001", nil)
	if dto != nil {
		t.Fatalf("未注册协议在订阅转换时应返回 nil，got: %+v", dto)
	}
}

// TestProtocolContracts_VLESSEquivalence 验证重构后 VLESS 协议的服务端配置生成与订阅输出。
func TestProtocolContracts_VLESSEquivalence(t *testing.T) {
	ctx := context.Background()

	// 1. 落地入站 (Relay)
	landInb := models.Inbound{
		ID:           201,
		ServerID:     2,
		Tag:          "vless-land",
		Protocol:     models.ProtocolVless,
		Port:         443,
		Enabled:      true,
		Type:         models.InboundTypeRelay,
		InternalUUID: "b8045e7f-71ad-4674-8b65-66778899aabb",
		StreamSettings: `{"network":"tcp","security":"reality","fingerprint":"chrome","realitySettings":{
			"serverNames":["land.example.com"],"publicKey":"land-pubkey","shortIds":["1234abcd"],"privateKey":"priv"}}`,
	}

	// 2. 用户直连入站 (User)
	userInb := models.Inbound{
		ID:             101,
		ServerID:       1,
		Tag:            "vless-user",
		Protocol:       models.ProtocolVless,
		Port:           8443,
		Enabled:        true,
		Type:           models.InboundTypeUser,
		StreamSettings: `{"network":"tcp","security":"reality","fingerprint":"firefox","realitySettings":{"serverNames":["user.example.com"],"publicKey":"user-pubkey","shortIds":["abcd1234"]}}`,
	}

	// 3. 中转出站 (引用落地入站 201)
	targetRef := uint64(201)
	relayOut := models.ServerOutbound{
		ID:         301,
		ServerID:   1,
		Tag:        "out-relay",
		Protocol:   models.ProtocolVless,
		Enabled:    true,
		InboundRef: &targetRef,
	}

	topo := &contracts.TopologyContext{
		RefTargets: map[uint64]contracts.RefTarget{
			201: {
				Inbound:    landInb,
				ServerHost: "landing.example.com",
				CertPin:    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		},
	}

	usersByTag := map[string][]protocol.User{
		"vless-user": {
			{UUID: "c1111111-2222-3333-4444-555555555555", Email: "u1.i101@panel.local"},
		},
	}

	rawJSON, err := xray.Generate([]models.Inbound{userInb}, []models.ServerOutbound{relayOut}, nil, usersByTag, topo, "out-relay", "")
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}

	// 解析校验 JSON 结构
	var root map[string]any
	if err := json.Unmarshal(rawJSON, &root); err != nil {
		t.Fatalf("生成配置 JSON 无效: %v", err)
	}

	// 断言入站正确注入 clients
	inbounds, ok := root["inbounds"].([]any)
	if !ok || len(inbounds) < 1 {
		t.Fatalf("inbounds 长度不符合预期: %v", root["inbounds"])
	}
	var userInb0 map[string]any
	for _, inb := range inbounds {
		m := inb.(map[string]any)
		if m["tag"] == "vless-user" {
			userInb0 = m
			break
		}
	}
	if userInb0 == nil {
		t.Fatalf("未在生成配置中找到 vless-user 入站: %+v", inbounds)
	}
	settings := userInb0["settings"].(map[string]any)
	clients, ok := settings["clients"].([]any)
	if !ok || len(clients) != 1 {
		t.Fatalf("clients 注入不符合预期: %+v", settings)
	}
	cl0 := clients[0].(map[string]any)
	if cl0["id"] != "c1111111-2222-3333-4444-555555555555" {
		t.Fatalf("用户凭证不符合预期: %+v", cl0)
	}

	// 断言出站正确由 Peer 投影渲染出 vnext
	outbounds, ok := root["outbounds"].([]any)
	if !ok || len(outbounds) < 1 {
		t.Fatalf("outbounds 不符合预期: %v", root["outbounds"])
	}
	var peerOut map[string]any
	for _, o := range outbounds {
		om := o.(map[string]any)
		if om["tag"] == "out-relay" {
			peerOut = om
			break
		}
	}
	if peerOut == nil {
		t.Fatal("未找到 out-relay 出站")
	}
	if peerOut["protocol"] != "vless" {
		t.Fatalf("peerOut 协议不符合预期: %v", peerOut["protocol"])
	}
	outSettings := peerOut["settings"].(map[string]any)
	vnext := outSettings["vnext"].([]any)
	v0 := vnext[0].(map[string]any)
	if v0["address"] != "landing.example.com" || v0["port"] != float64(443) {
		t.Fatalf("vnext 目标不符合预期: %+v", v0)
	}
	outUsers := v0["users"].([]any)[0].(map[string]any)
	if outUsers["id"] != "b8045e7f-71ad-4674-8b65-66778899aabb" || outUsers["flow"] != "xtls-rprx-vision" {
		t.Fatalf("outUsers 不符合预期: %+v", outUsers)
	}

	// 4. 金标准：通过 xray -test 校验
	driver := xray.NewDriver()
	if err := driver.ValidateConfig(ctx, rawJSON); err != nil {
		t.Fatalf("生成的 Xray 配置未能通过 xray.exe -test 校验: %v", err)
	}
}
