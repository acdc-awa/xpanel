package xray

import (
	"encoding/json"
	"strings"
)

// sanitizeStreamSettings 规范化入站 streamSettings：删除面板专用字段、修复旧版键名、
// 拦截已知的 xray panic 组合。输出前调用，确保 xray-core 收到的配置是干净的。
func sanitizeStreamSettings(raw string) string {
	if raw == "" {
		return raw
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(raw), &stream); err != nil {
		return raw // 解析失败则原样保留（让 xray -test 报错）
	}
	stream = sanitizeStreamMap(stream)

	result, err := json.Marshal(stream)
	if err != nil {
		return raw
	}
	return string(result)
}

// sanitizeStreamMap 就地规范化已解析的 stream map（生成链路直接消费 InboundSpec.Stream 时使用）。
func sanitizeStreamMap(stream map[string]any) map[string]any {

	// 面板专用字段，xray 不识别
	delete(stream, "externalProxy")

	// 客户端角色字段，不属于服务端 streamSettings（xray 静默忽略未知字段，但透传属于脏配置）。
	// DB 原文保留：fingerprint 供订阅 client-fingerprint / vless fp= 消费，
	// publicKey 供订阅 reality public-key / pbk= 消费；spiderX 无消费方仅随存。
	delete(stream, "fingerprint")
	deleteNestedKey(stream, "realitySettings", "publicKey")
	deleteNestedKey(stream, "realitySettings", "spiderX")

	// tlsSettings / realitySettings 下的 "settings" 是面板嵌套存储，不进 xray 配置
	deleteNestedKey(stream, "tlsSettings", "settings")
	deleteNestedKey(stream, "realitySettings", "settings")

	// cipherSuites 在 xray 里是「: 分隔的 string」；历史前端按数组提交会让整份配置反序列化失败。
	normalizeCipherSuites(stream)

	// xray v26.6.27 起已移除 tlsSettings.allowInsecure（迁移到 pinnedPeerCertSha256 /
	// verifyPeerCertByName），携带会直接拒绝加载；面板保留 DB 标志仅供订阅 skip-cert-verify 透传
	deleteNestedKey(stream, "tlsSettings", "allowInsecure")

	// Hysteria: 服务端 streamSettings 不保留客户端 auth 共享密码
	deleteNestedKey(stream, "hysteriaSettings", "auth")

	// REALITY + finalmask 组合会导致 xray panic（XTLS/Xray-core#6453）
	if security, _ := stream["security"].(string); security == "reality" {
		delete(stream, "finalmask")
	}

	// 清理无效的 finalmask 数据
	dropEmptyRandPackets(stream["finalmask"])

	// xray-core #6258：session 旧键名提升
	liftXhttpSessionKeys(stream)

	return stream
}

// normalizeCipherSuites 把 tlsSettings.cipherSuites 归一为 xray 要求的字符串形态。
// 历史数据可能是数组（旧前端按数组提交），直接透传会导致 xray 整份配置反序列化失败：
//   - 数组 → ":" 连接；
//   - 字符串原样保留（已是正确形态）；
//   - 空数组 / 空白字符串 → 删除，交给 xray 默认套件。
func normalizeCipherSuites(stream map[string]any) {
	tls, ok := stream["tlsSettings"].(map[string]any)
	if !ok {
		return
	}
	switch v := tls["cipherSuites"].(type) {
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
		if len(parts) == 0 {
			delete(tls, "cipherSuites")
			return
		}
		tls["cipherSuites"] = strings.Join(parts, ":")
	case string:
		if strings.TrimSpace(v) == "" {
			delete(tls, "cipherSuites")
		} else {
			tls["cipherSuites"] = strings.TrimSpace(v)
		}
	}
}

// stripInboundClientOnlyTLS 删除入站 TLS 配置里「只对客户端有意义」的字段。
//
// 这三类字段在服务端入站上没有任何消费方：
//   - usage=verify 的证书：服务端只做 encipherment，verify 是客户端校验远端用的；
//   - disableSystemRoot / pinnedPeerCertSha256 / verifyPeerCertByName：都是客户端校验远端的参数。
//
// 机间出站的验证参数由 protocols.ApplyPeerTLSValidation 按目标证书派生（有托管 CA 走 CA
// 验签，否则走证书指纹 Pin），不读取入站上的这些值；用户订阅也不透传它们。
// DB 原文保留（历史配置不丢数据），仅在生成侧剥离。
func stripInboundClientOnlyTLS(stream map[string]any) {
	tls, ok := stream["tlsSettings"].(map[string]any)
	if !ok {
		return
	}
	delete(tls, "disableSystemRoot")
	delete(tls, "pinnedPeerCertSha256")
	delete(tls, "verifyPeerCertByName")

	certs, ok := tls["certificates"].([]any)
	if !ok {
		return
	}
	kept := make([]any, 0, len(certs))
	for _, c := range certs {
		if cm, ok := c.(map[string]any); ok {
			if usage, _ := cm["usage"].(string); usage == "verify" {
				continue
			}
		}
		kept = append(kept, c)
	}
	if len(kept) == 0 {
		delete(tls, "certificates")
		return
	}
	tls["certificates"] = kept
}

// deleteNestedKey 删除 m[key] 下的 subKey（如果 m[key] 是一个 map）。
func deleteNestedKey(m map[string]any, key, subKey string) {
	if inner, ok := m[key].(map[string]any); ok {
		delete(inner, subKey)
	}
}

// dropEmptyRandPackets 删除 finalmask 下空的 rand 包条目。
func dropEmptyRandPackets(v any) {
	fm, ok := v.(map[string]any)
	if !ok {
		return
	}
	rand, ok := fm["rand"].([]any)
	if !ok {
		return
	}
	filtered := make([]any, 0, len(rand))
	for _, item := range rand {
		if m, ok := item.(map[string]any); ok {
			if len(m) == 0 {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		delete(fm, "rand")
	} else {
		fm["rand"] = filtered
	}
}

// liftXhttpSessionKeys 将 xhttpSettings 下的旧键名提升为新键名（xray-core #6258）。
func liftXhttpSessionKeys(stream map[string]any) {
	xhttp, ok := stream["xhttpSettings"].(map[string]any)
	if !ok {
		return
	}
	if v, ok := xhttp["sessionPlacement"]; ok {
		delete(xhttp, "sessionPlacement")
		xhttp["sessionIDPlacement"] = v
	}
	if v, ok := xhttp["sessionKey"]; ok {
		delete(xhttp, "sessionKey")
		xhttp["sessionID"] = v
	}
}
