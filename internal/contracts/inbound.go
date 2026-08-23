package contracts

import (
	"encoding/json"
	"strconv"

	"github.com/acdc-awa/xpanel/internal/models"
)

// InboundSpec 入站统一类型化解码结果：settings/streamSettings/sniffing 三段 JSON 一次解析，
// 生成（CoreDriver）、订阅（ProtocolPlugin）、校验共用同一份解码，
// 替代历史上 StreamXxx 系列对同一段 JSON 的多次散点解析。
// 解析容错：非法 JSON 对应字段留零值（保存入口已做严格校验，此处面向只读消费）。
type InboundSpec struct {
	ID       uint64
	Tag      string
	Protocol string
	Listen   string
	Port     int

	Network  string       // streamSettings.network（tcp / xhttp / ...，原样透传）
	Security string       // streamSettings.security（none / tls / reality，原样透传）
	TLS      *TLSSpec     // tlsSettings 存在时非 nil（不限于 security=tls）
	Reality  *RealitySpec // security=reality 且 realitySettings 存在时非 nil
	XHTTP    *XHTTPSpec   // network=xhttp 且 xhttpSettings.mode 非空时非 nil

	Settings map[string]any // 协议 settings 解码（clients 由后端注入，不经此结构）
	Stream   map[string]any // streamSettings 原始 map（高级/新增字段兜底通道）
	Sniffing map[string]any // sniffing 原始 map
}

// TLSSpec TLS 安全层（订阅/生成消费的最小字段集；其余经 InboundSpec.Stream 兜底）。
type TLSSpec struct {
	ServerName    string
	AllowInsecure bool
}

// RealitySpec REALITY 安全层。ServerName/ShortID 已做「单数优先、数组兜底」归一；
// PublicKey 兼容线格式 password/publicKey 双写。
// PrivateKey/Dest 为服务端私有字段，订阅导出不得输出。
type RealitySpec struct {
	ServerName string
	PublicKey  string
	ShortID    string
	PrivateKey string
	Dest       string
}

// XHTTPSpec XHTTP 传输层。Mode/Path/Host 提升为一等字段；
// 其余高级键（xPadding*/session*/seq*/xmux 等）收进 Extra 供导出器按需取用。
type XHTTPSpec struct {
	Mode  string
	Path  string
	Host  string
	Extra map[string]string
}

// DecodeInbound 将入站模型三段 JSON 一次解码为 InboundSpec。
func DecodeInbound(inb *models.Inbound) *InboundSpec {
	spec := &InboundSpec{
		ID:       inb.ID,
		Tag:      inb.Tag,
		Protocol: inb.Protocol,
		Listen:   inb.Listen,
		Port:     inb.Port,
	}
	if inb.SettingsJSON != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(inb.SettingsJSON), &m); err == nil {
			spec.Settings = m
		}
	}
	decodeStreamInto(spec, inb.StreamSettings)
	if inb.Sniffing != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(inb.Sniffing), &m); err == nil {
			spec.Sniffing = m
		}
	}
	return spec
}

// DecodeStream 仅解码 streamSettings（只有原始 JSON 而非完整入站模型时使用）。
func DecodeStream(raw string) *InboundSpec {
	spec := &InboundSpec{}
	decodeStreamInto(spec, raw)
	return spec
}

func decodeStreamInto(spec *InboundSpec, raw string) {
	if raw == "" {
		return
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return
	}
	spec.Stream = m
	spec.Network, _ = m["network"].(string)
	spec.Security, _ = m["security"].(string)

	if tls, ok := m["tlsSettings"].(map[string]any); ok {
		spec.TLS = &TLSSpec{
			ServerName:    strOf(tls, "serverName"),
			AllowInsecure: boolOf(tls, "allowInsecure"),
		}
	}
	if spec.Security == "reality" {
		if r, ok := m["realitySettings"].(map[string]any); ok {
			spec.Reality = decodeRealitySpec(r)
		}
	}
	if xh, ok := m["xhttpSettings"].(map[string]any); ok {
		x := &XHTTPSpec{
			Mode: strOf(xh, "mode"),
			Path: strOf(xh, "path"),
			Host: strOf(xh, "host"),
		}
		// 与历史 StreamXHTTP 语义一致：mode 为空视为未配置 xhttp
		if x.Mode != "" {
			for k, v := range xh {
				switch k {
				case "mode", "path", "host":
					continue
				}
				if x.Extra == nil {
					x.Extra = make(map[string]string)
				}
				x.Extra[k] = scalarToString(v)
			}
			spec.XHTTP = x
		}
	}
}

// decodeRealitySpec 解析 realitySettings，归一化单数/数组与 password/publicKey 双写。
func decodeRealitySpec(m map[string]any) *RealitySpec {
	r := &RealitySpec{
		PrivateKey: strOf(m, "privateKey"),
		Dest:       strOf(m, "dest"),
		PublicKey:  strOf(m, "password"), // 标准名（出站）；兼容旧名 publicKey
		ServerName: strOf(m, "serverName"),
		ShortID:    strOf(m, "shortId"),
	}
	if r.PublicKey == "" {
		r.PublicKey = strOf(m, "publicKey")
	}
	if r.ServerName == "" {
		if names := strListOf(m, "serverNames"); len(names) > 0 {
			r.ServerName = names[0]
		}
	}
	if r.ShortID == "" {
		if ids := strListOf(m, "shortIds"); len(ids) > 0 {
			r.ShortID = ids[0]
		}
	}
	return r
}

func strOf(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func boolOf(m map[string]any, key string) bool {
	b, _ := m[key].(bool)
	return b
}

func strListOf(m map[string]any, key string) []string {
	arr, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// scalarToString 将标量转为字符串（嵌套对象/数组序列化为 JSON），供 Extra 兜底通道使用。
func scalarToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return ""
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return ""
	}
}
