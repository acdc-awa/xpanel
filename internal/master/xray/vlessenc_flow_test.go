package xray_test

// vlessenc 与 XTLS 不搭配：**机间中继**是本项目里唯一会自动补 flow 的地方
// （用户侧只对 tcp+reality 自动注入），此前把 vlessenc 也算了进去，导致落地侧的
// 内部账号与前置侧的参照出站都带上 flow=xtls-rprx-vision。本用例锁定两端都不带。
// 反向验证：把 ResolveRelayFlow / ProjectClient 里的 vlessEncActive 判断去掉，本用例必红。

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
)

const testValidDecryption = "mlkem768x25519plus.native.600s.AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

func TestGenerateConfig_VlessEncRelayHasNoFlow(t *testing.T) {
	landing := models.Inbound{
		ID: 77, ServerID: 2, Tag: "landing-enc", Protocol: models.ProtocolVless, Port: 443,
		Type:           models.InboundTypeRelay,
		InternalUUID:   "77777777-7777-7777-7777-777777777777",
		SettingsJSON:   `{"decryption":"` + testValidDecryption + `"}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Enabled:        true,
	}
	ref := landing.ID
	transit := models.ServerOutbound{
		ID: 9, ServerID: 1, Tag: "to-enc", Protocol: models.ProtocolVless,
		InboundRef: &ref, InternalUUID: "88888888-8888-8888-8888-888888888888", Enabled: true,
	}

	// 1. 落地机：内部中继账号不得带 flow（decryption 必须保留）
	rawLanding, err := xray.Generate(
		[]models.Inbound{landing}, nil, nil, nil,
		&xray.GenerateContext{ReferencingOutbounds: map[uint64][]models.ServerOutbound{landing.ID: {transit}}},
		"", "")
	if err != nil {
		t.Fatalf("Generate landing failed: %v", err)
	}
	var parsedLanding map[string]any
	if err := json.Unmarshal(rawLanding, &parsedLanding); err != nil {
		t.Fatalf("unmarshal landing: %v", err)
	}
	var landingInb map[string]any
	for _, it := range parsedLanding["inbounds"].([]any) {
		if m, _ := it.(map[string]any); m["tag"] == "landing-enc" {
			landingInb = m
		}
	}
	if landingInb == nil {
		t.Fatal("落地机配置里找不到 landing-enc 入站")
	}
	settings := asObject(t, landingInb["settings"], "landing settings")
	if dec, _ := settings["decryption"].(string); dec == "" || dec == "none" {
		t.Fatalf("落地机应保留 decryption（vlessenc 服务端载体），got %q", dec)
	}
	clients := asArray(t, settings["clients"], "landing clients")
	if len(clients) != 1 {
		t.Fatalf("落地机应只有 1 个中继账号，got %d", len(clients))
	}
	if flow := asObject(t, clients[0], "relay client")["flow"]; flow != nil {
		t.Errorf("vlessenc 落地的中继账号不应带 flow，got %v", flow)
	}

	// 2. 前置机：参照出站派生 encryption，但同样不得带 flow
	rawFront, err := xray.Generate(
		nil, []models.ServerOutbound{transit}, nil, nil,
		&xray.GenerateContext{RefTargets: map[uint64]xray.RefTarget{
			landing.ID: {Inbound: landing, ServerHost: "node2.example.com"},
		}}, "", "")
	if err != nil {
		t.Fatalf("Generate front failed: %v", err)
	}
	var parsedFront map[string]any
	if err := json.Unmarshal(rawFront, &parsedFront); err != nil {
		t.Fatalf("unmarshal front: %v", err)
	}
	body := outboundByTag(t, parsedFront, "to-enc")
	if body == nil {
		t.Fatal("前置机配置里找不到 to-enc 出站")
	}
	vnext := asArray(t, asObject(t, body["settings"], "out settings")["vnext"], "vnext")
	user := asObject(t, asArray(t, asObject(t, vnext[0], "vnext[0]")["users"], "users")[0], "users[0]")
	if flow := user["flow"]; flow != nil {
		t.Errorf("vlessenc 落地的参照出站不应带 flow，got %v", flow)
	}
	if enc, _ := user["encryption"].(string); enc == "" || enc == "none" {
		t.Errorf("vlessenc 落地应派生客户端 encryption，got %q", enc)
	}

	// 3. 金标准校验（配置路径必须以 .json 结尾）
	xrayBin := `..\..\..\tools\xray-windows-64\xray.exe`
	if _, err := os.Stat(xrayBin); err != nil {
		t.Skip("跳过 xray 二进制实测（文件不存在）")
	}
	for name, raw := range map[string][]byte{"landing": rawLanding, "front": rawFront} {
		path := filepath.Join(t.TempDir(), name+".json")
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
		if out, err := exec.Command(xrayBin, "-test", "-config", path).CombinedOutput(); err != nil {
			t.Fatalf("%s 配置 xray -test 失败: %v, out: %s", name, err, string(out))
		}
	}
}
