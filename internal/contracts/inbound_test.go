package contracts

import (
	"testing"

	"github.com/acdc/xray-panel/internal/models"
)

func TestDecodeInbound_Full(t *testing.T) {
	inb := &models.Inbound{
		ID:       7,
		Tag:      "vless-reality",
		Protocol: "vless",
		Port:     443,
		Listen:   "0.0.0.0",
		SettingsJSON: `{"decryption":"none","fallbacks":[{"dest":"8080"}]}`,
		StreamSettings: `{
			"network": "xhttp",
			"security": "reality",
			"realitySettings": {
				"serverNames": ["a.com", "b.com"],
				"password": "pbk-via-password",
				"shortIds": ["sid1", "sid2"],
				"privateKey": "priv",
				"dest": "a.com:443"
			},
			"xhttpSettings": {
				"mode": "stream-up",
				"path": "/xp",
				"host": "cdn.example.com",
				"xPaddingBytes": "100-1000",
				"scMaxEachPostBytes": 1000000,
				"header": {"type":"none"}
			}
		}`,
		Sniffing: `{"enabled":true,"destOverride":["http","tls"]}`,
	}
	spec := DecodeInbound(inb)

	if spec.ID != 7 || spec.Tag != "vless-reality" || spec.Protocol != "vless" || spec.Port != 443 {
		t.Errorf("基础字段错误: %+v", spec)
	}
	if spec.Network != "xhttp" || spec.Security != "reality" {
		t.Errorf("传输/安全错误: %s/%s", spec.Network, spec.Security)
	}
	// reality 归一化：数组兜底 + password 优先
	if spec.Reality == nil {
		t.Fatal("Reality 应解析")
	}
	if spec.Reality.ServerName != "a.com" || spec.Reality.ShortID != "sid1" ||
		spec.Reality.PublicKey != "pbk-via-password" || spec.Reality.PrivateKey != "priv" || spec.Reality.Dest != "a.com:443" {
		t.Errorf("Reality 归一化错误: %+v", spec.Reality)
	}
	// xhttp 一等字段 + Extra 高级键
	if spec.XHTTP == nil {
		t.Fatal("XHTTP 应解析")
	}
	if spec.XHTTP.Mode != "stream-up" || spec.XHTTP.Path != "/xp" || spec.XHTTP.Host != "cdn.example.com" {
		t.Errorf("XHTTP 字段错误: %+v", spec.XHTTP)
	}
	if spec.XHTTP.Extra["xPaddingBytes"] != "100-1000" {
		t.Errorf("Extra 字符串键错误: %+v", spec.XHTTP.Extra)
	}
	if spec.XHTTP.Extra["scMaxEachPostBytes"] != "1000000" {
		t.Errorf("Extra 数字键应字符串化: %+v", spec.XHTTP.Extra)
	}
	if spec.XHTTP.Extra["header"] != `{"type":"none"}` {
		t.Errorf("Extra 嵌套对象应 JSON 化: %+v", spec.XHTTP.Extra)
	}
	// settings / sniffing / stream 兜底通道
	if spec.Settings["decryption"] != "none" {
		t.Errorf("Settings 兜底通道错误: %+v", spec.Settings)
	}
	if spec.Sniffing["enabled"] != true {
		t.Errorf("Sniffing 兜底通道错误: %+v", spec.Sniffing)
	}
	if spec.Stream["network"] != "xhttp" {
		t.Errorf("Stream 兜底通道错误: %+v", spec.Stream)
	}
}

func TestDecodeInbound_RealityCompat(t *testing.T) {
	// 旧名 publicKey / 单数 serverName / 单数 shortId
	spec := DecodeStream(`{"security":"reality","realitySettings":{"serverName":"s.com","publicKey":"pk","shortId":"sid"}}`)
	if spec.Reality == nil || spec.Reality.ServerName != "s.com" || spec.Reality.PublicKey != "pk" || spec.Reality.ShortID != "sid" {
		t.Errorf("旧名兼容错误: %+v", spec.Reality)
	}
	// security 非 reality 时不解析 realitySettings
	spec = DecodeStream(`{"security":"tls","realitySettings":{"serverName":"s.com","publicKey":"pk"}}`)
	if spec.Reality != nil {
		t.Errorf("security=tls 不应解析 Reality: %+v", spec.Reality)
	}
}

func TestDecodeStream_Lenient(t *testing.T) {
	// 空串 / 非法 JSON → 零值不 panic
	if spec := DecodeStream(""); spec.Network != "" || spec.Security != "" || spec.Reality != nil {
		t.Errorf("空串应零值: %+v", spec)
	}
	if spec := DecodeStream(`{invalid`); spec.Network != "" || spec.Stream != nil {
		t.Errorf("非法 JSON 应零值: %+v", spec)
	}
	// tlsSettings 存在即解析（不限 security=tls）
	spec := DecodeStream(`{"security":"none","tlsSettings":{"serverName":"s.com","allowInsecure":true}}`)
	if spec.TLS == nil || spec.TLS.ServerName != "s.com" || !spec.TLS.AllowInsecure {
		t.Errorf("TLS 解析错误: %+v", spec.TLS)
	}
	// xhttpSettings mode 为空视为未配置（与历史 StreamXHTTP 语义一致）
	spec = DecodeStream(`{"network":"xhttp","xhttpSettings":{"path":"/p"}}`)
	if spec.XHTTP != nil {
		t.Errorf("mode 空应视为未配置: %+v", spec.XHTTP)
	}
}
