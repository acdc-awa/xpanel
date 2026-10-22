package contracts

import (
	"testing"

	"github.com/acdc-awa/xpanel/internal/models"
)

// decodeStream 测试辅助：仅解码 streamSettings（生产统一入口为 DecodeInbound）。
func decodeStream(raw string) *InboundSpec {
	return DecodeInbound(&models.Inbound{StreamSettings: raw})
}

func TestDecodeInbound_Full(t *testing.T) {
	inb := &models.Inbound{
		ID:           7,
		Tag:          "vless-reality",
		Protocol:     "vless",
		Port:         443,
		Listen:       "0.0.0.0",
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
	spec := decodeStream(`{"security":"reality","realitySettings":{"serverName":"s.com","publicKey":"pk","shortId":"sid"}}`)
	if spec.Reality == nil || spec.Reality.ServerName != "s.com" || spec.Reality.PublicKey != "pk" || spec.Reality.ShortID != "sid" {
		t.Errorf("旧名兼容错误: %+v", spec.Reality)
	}
	// security 非 reality 时不解析 realitySettings
	spec = decodeStream(`{"security":"tls","realitySettings":{"serverName":"s.com","publicKey":"pk"}}`)
	if spec.Reality != nil {
		t.Errorf("security=tls 不应解析 Reality: %+v", spec.Reality)
	}
	// 顶层 fingerprint（客户端角色字段）解码，供订阅消费
	spec = decodeStream(`{"security":"reality","fingerprint":"firefox","realitySettings":{"serverName":"s.com"}}`)
	if spec.Fingerprint != "firefox" {
		t.Errorf("fingerprint 解码错误: %q", spec.Fingerprint)
	}
	if spec = decodeStream(`{"security":"reality","realitySettings":{"serverName":"s.com"}}`); spec.Fingerprint != "" {
		t.Errorf("缺省 fingerprint 应为空: %q", spec.Fingerprint)
	}
}

func TestDecodeStream_Lenient(t *testing.T) {
	// 空串 / 非法 JSON → 零值不 panic
	if spec := decodeStream(""); spec.Network != "" || spec.Security != "" || spec.Reality != nil {
		t.Errorf("空串应零值: %+v", spec)
	}
	if spec := decodeStream(`{invalid`); spec.Network != "" || spec.Stream != nil {
		t.Errorf("非法 JSON 应零值: %+v", spec)
	}
	// tlsSettings 存在即解析（不限 security=tls）
	spec := decodeStream(`{"security":"none","tlsSettings":{"serverName":"s.com","allowInsecure":true}}`)
	if spec.TLS == nil || spec.TLS.ServerName != "s.com" || !spec.TLS.AllowInsecure {
		t.Errorf("TLS 解析错误: %+v", spec.TLS)
	}
	// xhttpSettings mode 为空视为未配置 xhttp
	spec = decodeStream(`{"network":"xhttp","xhttpSettings":{"path":"/p"}}`)
	if spec.XHTTP != nil {
		t.Errorf("mode 空应视为未配置: %+v", spec.XHTTP)
	}
}

func TestDecodeStream_Aliases(t *testing.T) {
	// method/rawSettings 是 network/tcpSettings 的正式别名；raw 归一为内部名 tcp。
	spec := decodeStream(`{"method":"raw","rawSettings":{"acceptProxyProtocol":true}}`)
	if spec.Network != "tcp" {
		t.Fatalf("method=raw 应归一为 tcp, got %q", spec.Network)
	}
	if _, ok := spec.Stream["tcpSettings"]; !ok {
		t.Fatalf("rawSettings 应补一份 tcpSettings 别名: %+v", spec.Stream)
	}
	// network 优先于 method。
	spec = decodeStream(`{"network":"xhttp","method":"raw"}`)
	if spec.Network != "xhttp" {
		t.Fatalf("network 应优先, got %q", spec.Network)
	}
}

func TestDecodeInboundStrict(t *testing.T) {
	// 合法输入与宽松解码结果一致
	raw := &models.Inbound{
		ID:             3,
		Tag:            "in",
		Protocol:       "vless",
		Port:           443,
		SettingsJSON:   `{"decryption":"none"}`,
		StreamSettings: `{"network":"tcp","security":"reality"}`,
		Sniffing:       `{"enabled":true}`,
	}
	strict, err := DecodeInboundStrict(raw)
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	lenient := DecodeInbound(raw)
	if strict.Settings["decryption"] != lenient.Settings["decryption"] || strict.Network != lenient.Network || strict.Sniffing["enabled"] != lenient.Sniffing["enabled"] {
		t.Fatalf("严格/宽松解码结果不一致: %+v vs %+v", strict, lenient)
	}

	// settings 非法 → 报错（生成链路必须显式失败，不能静默降级为空对象）
	if _, err := DecodeInboundStrict(&models.Inbound{SettingsJSON: `{bad`}); err == nil {
		t.Error("非法 settings 应报错")
	}
	// streamSettings 非法 → 报错
	if _, err := DecodeInboundStrict(&models.Inbound{StreamSettings: `{bad`}); err == nil {
		t.Error("非法 streamSettings 应报错")
	}
	// sniffing 非法 → 保持历史容错（忽略）
	if spec, err := DecodeInboundStrict(&models.Inbound{Sniffing: `{bad`}); err != nil || spec.Sniffing != nil {
		t.Errorf("非法 sniffing 应被忽略: spec=%+v err=%v", spec, err)
	}
}

func TestDecodeStream_SockoptAndTLSExtended(t *testing.T) {
	raw := `{
		"security": "tls",
		"tlsSettings": {
			"serverName": "test.com",
			"disableSystemRoot": true,
			"rejectUnknownSni": true,
			"pinnedPeerCertSha256": "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff",
			"verifyPeerCertByName": "alt.com",
			"certificates": [
				{
					"certificateFile": "/etc/ca.crt",
					"usage": "verify"
				}
			]
		},
		"sockopt": {
			"mark": 100,
			"tcpFastOpen": true,
			"tcpCongestion": "bbr",
			"tcpMptcp": true,
			"tcpKeepAliveIdle": 60,
			"tcpKeepAliveInterval": 15,
			"tcpUserTimeout": 5000,
			"acceptProxyProtocol": true,
			"tproxy": "tproxy"
		}
	}`
	spec := decodeStream(raw)
	if spec.TLS == nil {
		t.Fatal("TLS 应解析")
	}
	if !spec.TLS.DisableSystemRoot || !spec.TLS.RejectUnknownSni {
		t.Errorf("TLS 布尔标志解析失败: %+v", spec.TLS)
	}
	if spec.TLS.PinnedPeerCertSha256 != "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff" {
		t.Errorf("pinnedPeerCertSha256 解析失败: %s", spec.TLS.PinnedPeerCertSha256)
	}
	if spec.TLS.VerifyPeerCertByName != "alt.com" {
		t.Errorf("verifyPeerCertByName 解析失败: %s", spec.TLS.VerifyPeerCertByName)
	}
	if len(spec.TLS.Certificates) != 1 || spec.TLS.Certificates[0].Usage != "verify" || spec.TLS.Certificates[0].CertificateFile != "/etc/ca.crt" {
		t.Errorf("Certificates 解析失败: %+v", spec.TLS.Certificates)
	}

	if spec.Sockopt == nil {
		t.Fatal("Sockopt 应解析")
	}
	if spec.Sockopt.Mark != 100 || spec.Sockopt.TCPCongestion != "bbr" || !spec.Sockopt.TCPMptcp || !spec.Sockopt.AcceptProxyProtocol {
		t.Errorf("Sockopt 字段解析失败: %+v", spec.Sockopt)
	}
	if spec.Sockopt.TCPKeepAliveIdle != 60 || spec.Sockopt.TCPKeepAliveInterval != 15 || spec.Sockopt.TCPUserTimeout != 5000 || spec.Sockopt.TProxy != "tproxy" {
		t.Errorf("Sockopt 细节字段解析失败: %+v", spec.Sockopt)
	}
}
