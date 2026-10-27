package xray_test

// 出站级 PROXY Protocol：开关只存在于 server_outbounds.send_proxy_protocol 上，生成期
// 内联派生一个 freedom（pp-<tag>）承担 PP 报文，本体通过 sockopt.dialerProxy 走它。
// 这里只断言「配置长什么样」；PP 头是否真的发出、落地是否按 PP 解析由
// tools/protocol-verify/out/pp-protocol-pp 的链路用例证明（xray -test 对未知字段静默忽略）。

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
)

func ppRefFixture(t *testing.T, targetNetwork string) (models.Inbound, *xray.GenerateContext, []models.ServerOutbound, uint64) {
	t.Helper()
	target := models.Inbound{
		ID: 99, ServerID: 2, Tag: "landing", Protocol: models.ProtocolVless, Port: 443,
		Type:           models.InboundTypeRelay,
		InternalUUID:   "33333333-3333-3333-3333-333333333333",
		StreamSettings: `{"network":"` + targetNetwork + `","security":"none"}`,
		Enabled:        true,
	}
	ref := target.ID
	ctx := &xray.GenerateContext{
		RefTargets: map[uint64]xray.RefTarget{
			target.ID: {Inbound: target, ServerHost: "10.0.0.5"},
		},
	}
	outs := []models.ServerOutbound{{
		ID: 1, ServerID: 1, Tag: "to-landing", Protocol: "vless", InboundRef: &ref,
		Enabled: true, SendProxyProtocol: true,
	}}
	return target, ctx, outs, ref
}

func outboundByTag(t *testing.T, parsed map[string]any, tag string) map[string]any {
	t.Helper()
	for _, o := range asArray(t, parsed["outbounds"], "outbounds") {
		om, _ := o.(map[string]any)
		if om["tag"] == tag {
			return om
		}
	}
	return nil
}

func TestGenerateConfig_ProxyProtocolHopInjected(t *testing.T) {
	_, ctx, outs, _ := ppRefFixture(t, "tcp")
	raw, err := xray.Generate(
		[]models.Inbound{{ID: 1, Tag: "in", Protocol: "vless", Port: 10080, Type: models.InboundTypeUser, Enabled: true}},
		outs, nil, vlessUsers("in"), ctx, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	body := outboundByTag(t, parsed, "to-landing")
	if body == nil {
		t.Fatal("to-landing 出站缺失")
	}
	stream := asObject(t, body["streamSettings"], "streamSettings")
	if stream["network"] != "tcp" {
		t.Errorf("network = %v, want tcp", stream["network"])
	}
	sockopt := asObject(t, stream["sockopt"], "sockopt")
	if sockopt["dialerProxy"] != "pp-to-landing" {
		t.Errorf("sockopt.dialerProxy = %v, want pp-to-landing", sockopt["dialerProxy"])
	}
	// PP 只能由派生 freedom 发出：本体 settings 里不该出现 proxyProtocol。
	if s, _ := body["settings"].(map[string]any); s != nil && s["proxyProtocol"] != nil {
		t.Errorf("出站本体不应携带 proxyProtocol: %v", s["proxyProtocol"])
	}

	hop := outboundByTag(t, parsed, "pp-to-landing")
	if hop == nil {
		t.Fatal("派生 pp-to-landing 缺失")
	}
	if hop["protocol"] != "freedom" {
		t.Errorf("派生协议 = %v, want freedom", hop["protocol"])
	}
	hopSettings := asObject(t, hop["settings"], "hop settings")
	if hopSettings["proxyProtocol"] != float64(2) {
		t.Errorf("proxyProtocol = %v, want 2", hopSettings["proxyProtocol"])
	}
	rules := asArray(t, hopSettings["finalRules"], "finalRules")
	if len(rules) == 0 {
		t.Fatal("派生 freedom 必须带 finalRules（否则私网/回环落地会被内建策略阻断）")
	}
	if r0 := asObject(t, rules[0], "finalRules[0]"); r0["action"] != "allow" {
		t.Errorf("finalRules[0].action = %v, want allow", r0["action"])
	}
}

func TestGenerateConfig_ProxyProtocolHopAbsentWhenOff(t *testing.T) {
	_, ctx, outs, _ := ppRefFixture(t, "tcp")
	outs[0].SendProxyProtocol = false
	raw, err := xray.Generate(
		[]models.Inbound{{ID: 1, Tag: "in", Protocol: "vless", Port: 10080, Type: models.InboundTypeUser, Enabled: true}},
		outs, nil, vlessUsers("in"), ctx, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var parsed map[string]any
	json.Unmarshal(raw, &parsed)
	if outboundByTag(t, parsed, "pp-to-landing") != nil {
		t.Error("关闭开关时不应派生 pp- 出站")
	}
	body := outboundByTag(t, parsed, "to-landing")
	stream := asObject(t, body["streamSettings"], "streamSettings")
	if sockopt, ok := stream["sockopt"].(map[string]any); ok && sockopt["dialerProxy"] != nil {
		t.Errorf("关闭开关时不应注入 dialerProxy: %v", sockopt["dialerProxy"])
	}
}

func TestGenerateConfig_ProxyProtocolHopRejectsNonTCP(t *testing.T) {
	_, ctx, outs, _ := ppRefFixture(t, "xhttp")
	_, err := xray.Generate(
		[]models.Inbound{{ID: 1, Tag: "in", Protocol: "vless", Port: 10080, Type: models.InboundTypeUser, Enabled: true}},
		outs, nil, vlessUsers("in"), ctx, "", "")
	if err == nil {
		t.Fatal("XHTTP 落地开启 PP 应报错（XHTTP 不解析 PP 头）")
	}
	if !strings.Contains(err.Error(), "仅支持 TCP") {
		t.Errorf("错误信息未点明 TCP 限制: %v", err)
	}
}

func TestGenerateConfig_ProxyProtocolHopRequiresRef(t *testing.T) {
	outs := []models.ServerOutbound{{
		ID: 1, ServerID: 1, Tag: "standalone", Protocol: "freedom",
		SettingsJSON: models.DefaultFreedomDirectSettingsJSON,
		Enabled:      true, SendProxyProtocol: true,
	}}
	_, err := xray.Generate(nil, outs, nil, nil, nil, "", "")
	if err == nil {
		t.Fatal("未引用落地入站的出站开启 PP 应报错")
	}
	if !strings.Contains(err.Error(), "未引用落地入站") {
		t.Errorf("错误信息未点明缺少引用: %v", err)
	}
}

func TestGenerateConfig_ProxyProtocolHopRejectsTagCollision(t *testing.T) {
	_, ctx, outs, _ := ppRefFixture(t, "tcp")
	// 存量库若已有同名出站（正常路径已被 pp- 保留前缀挡住），生成期必须报错而不是静默覆盖。
	outs = append(outs, models.ServerOutbound{
		ID: 2, ServerID: 1, Tag: "pp-to-landing", Protocol: "freedom",
		SettingsJSON: models.DefaultFreedomDirectSettingsJSON, Enabled: true,
	})
	_, err := xray.Generate(
		[]models.Inbound{{ID: 1, Tag: "in", Protocol: "vless", Port: 10080, Type: models.InboundTypeUser, Enabled: true}},
		outs, nil, vlessUsers("in"), ctx, "", "")
	if err == nil {
		t.Fatal("派生出站 tag 撞名应报错")
	}
	if !strings.Contains(err.Error(), "已被占用") {
		t.Errorf("错误信息未点明撞名: %v", err)
	}
}

// TestGenerateConfig_ProxyProtocolHopXrayTestValidation 用随仓的 xray.exe 做金标准校验：
// 派生 freedom 的 proxyProtocol 与本体 sockopt.dialerProxy 都必须被内核接受。
// 注意配置路径必须以 .json 结尾——xray 纯按扩展名判格式，.tmp 必然校验失败。
func TestGenerateConfig_ProxyProtocolHopXrayTestValidation(t *testing.T) {
	xrayBin := `..\..\..\tools\xray-windows-64\xray.exe`
	if _, err := os.Stat(xrayBin); err != nil {
		t.Skip("跳过 xray 二进制实测（文件不存在）")
	}
	_, ctx, outs, _ := ppRefFixture(t, "tcp")
	raw, err := xray.Generate(
		[]models.Inbound{{ID: 1, Tag: "in", Protocol: "vless", Port: 10080, Type: models.InboundTypeUser, Enabled: true}},
		outs, nil, vlessUsers("in"), ctx, "", "")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	path := filepath.Join(t.TempDir(), "pp-hop.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if out, err := exec.Command(xrayBin, "-test", "-config", path).CombinedOutput(); err != nil {
		t.Fatalf("xray -test 校验失败: %v, out: %s", err, string(out))
	}
}
