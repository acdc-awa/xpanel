package protocols_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/protocols"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 机间出站必须把落地入站的 xhttp 进阶参数逐项带出来：落地要求 sessionID/padding 时，
// 客户端少一项就是 502（2026-11-02 实测：参数齐全的落地 + 只带 mode/path 的出站 → 全 502）。
func TestRenderOutbound_XHTTPKeepsClientParams(t *testing.T) {
	inbound := models.Inbound{
		ID: 41, Tag: "xhttp-landing", Protocol: models.ProtocolVless, Port: 2053,
		SettingsJSON: `{"decryption":"none"}`,
		StreamSettings: `{"network":"xhttp","security":"tls",` +
			`"xhttpSettings":{` +
			`"mode":"stream-up","path":"/xp","host":"h.example.com",` +
			`"xPaddingBytes":"100-500","xPaddingPlacement":"header","xPaddingHeader":"X-Pad",` +
			`"xPaddingMethod":"repeat-x","sessionIDPlacement":"header","sessionIDKey":"X-Session",` +
			`"sessionIDLength":"8-16","sessionIDTable":"0123456789abcdef",` +
			`"scMaxEachPostBytes":"1000000","scMinPostsIntervalMs":"30",` +
			`"noGRPCHeader":true,"headers":{"X-Own":"1"},` +
			`"xmux":{"maxConnections":"4","hMaxReusableSecs":"1800-3000"},` +
			`"extra":{"uplinkDataPlacement":"body","uplinkDataKey":"u"},` +
			`"noSSEHeader":true,"scMaxBufferedPosts":128,` +
			`"scStreamUpServerSecs":"20-80","serverMaxHeaderBytes":8192},` +
			`"tlsSettings":{"serverName":"h.example.com"}}`,
	}
	spec := contracts.DecodeInbound(&inbound)
	if spec.XHTTP == nil {
		t.Fatal("夹具未解析出 xhttp 规格")
	}

	dto, err := protocols.VLESSPlugin{}.ProjectClient(&contracts.ClientProjectionInput{
		Role: contracts.RolePeer,
		RefTarget: &contracts.RefTarget{
			Inbound:    inbound,
			ServerHost: "127.0.0.1",
		},
		OutboundUUID: "relay-uuid",
	})
	if err != nil {
		t.Fatalf("互联投影失败: %v", err)
	}

	out, err := protocols.VLESSPlugin{}.RenderOutbound(dto)
	if err != nil {
		t.Fatalf("RenderOutbound 失败: %v", err)
	}
	ss, _ := out["streamSettings"].(map[string]any)
	xh, _ := ss["xhttpSettings"].(map[string]any)
	if xh == nil {
		t.Fatal("出站缺少 xhttpSettings")
	}

	// 基础三键 + 客户端进阶键必须齐全（值保持入站原文，区间仍是 "a-b" 字符串）。
	want := map[string]any{
		"mode":                 "stream-up",
		"path":                 "/xp",
		"host":                 "h.example.com",
		"xPaddingBytes":        "100-500",
		"xPaddingPlacement":    "header",
		"xPaddingHeader":       "X-Pad",
		"xPaddingMethod":       "repeat-x",
		"sessionIDPlacement":   "header",
		"sessionIDKey":         "X-Session",
		"sessionIDLength":      "8-16",
		"sessionIDTable":       "0123456789abcdef",
		"scMaxEachPostBytes":   "1000000",
		"scMinPostsIntervalMs": "30",
		"noGRPCHeader":         true,
		"uplinkDataPlacement":  "body",
		"uplinkDataKey":        "u",
	}
	for k, v := range want {
		if got, ok := xh[k]; !ok {
			t.Errorf("出站 xhttpSettings 缺少 %s（客户端参数必须与落地逐项一致）", k)
		} else if !reflect.DeepEqual(got, v) {
			t.Errorf("xhttpSettings.%s = %#v, want %#v", k, got, v)
		}
	}

	// 对象类键必须还原成 JSON 对象，否则 xray 反序列化整份失败。
	if h, ok := xh["headers"].(map[string]any); !ok || h["X-Own"] != "1" {
		t.Errorf("headers 未还原成对象: %#v", xh["headers"])
	}
	if xm, ok := xh["xmux"].(map[string]any); !ok || xm["maxConnections"] != "4" {
		t.Errorf("xmux 未还原成对象: %#v", xh["xmux"])
	}

	// 服务端专属键一个都不能出现在客户端出站里。
	for _, k := range []string{"noSSEHeader", "scMaxBufferedPosts", "scStreamUpServerSecs", "serverMaxHeaderBytes"} {
		if _, bad := xh[k]; bad {
			t.Errorf("出站 xhttpSettings 不应包含服务端专属键 %s", k)
		}
	}
	if _, bad := xh["extra"]; bad {
		t.Error("extra 必须展开，不能以 extra 键透传")
	}

	// 金标准：投影结果必须能被官方 xray 反序列化（对象键没还原、布尔写成字符串都会在这里红）。
	runXrayTestHelper(t, out)
}

// 反向验证：剥键来自服务端专属名单，而不是把参数整体丢掉。
func TestRenderOutbound_XHTTPStripsOnlyServerKeys(t *testing.T) {
	dto := &contracts.ProxyNodeDTO{
		ServerHost: "127.0.0.1", ServerPort: 2053, Protocol: "vless",
		Auth:      &contracts.ClientCredentialDTO{UUID: "u"},
		Transport: &contracts.TransportOptions{Network: "xhttp", Mode: "auto", Path: "/p"},
		Security:  &contracts.SecurityOptions{Type: "none"},
	}
	out, err := protocols.VLESSPlugin{}.RenderOutbound(dto)
	if err != nil {
		t.Fatalf("RenderOutbound 失败: %v", err)
	}
	ss, _ := out["streamSettings"].(map[string]any)
	b, _ := json.Marshal(ss["xhttpSettings"])
	if string(b) != `{"mode":"auto","path":"/p"}` {
		t.Errorf("无进阶参数时只应输出 mode/path，实际 %s", b)
	}
}
