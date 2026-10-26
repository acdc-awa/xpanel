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

// SanitizeOutboundStreamSettings 规范化出站（客户端角色）streamSettings。
//
// 出站不能复用入站清洗器：入站会删除 fingerprint/publicKey/spiderX 与
// hysteriaSettings.auth 这些「只对客户端有意义」的字段，而出站正是客户端。
// 这里只清掉 xray 已移除或面板私有的键，客户端字段原样保留。
func SanitizeOutboundStreamSettings(raw string) string {
	if raw == "" {
		return raw
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(raw), &stream); err != nil {
		return raw // 解析失败则原样保留（让 xray -test 报错）
	}
	stream = SanitizeOutboundStreamMap(stream)

	result, err := json.Marshal(stream)
	if err != nil {
		return raw
	}
	return string(result)
}

// SanitizeOutboundStreamMap 就地规范化已解析的出站 stream map。
func SanitizeOutboundStreamMap(stream map[string]any) map[string]any {
	// 面板专用字段，xray 不识别
	delete(stream, "externalProxy")

	// 顶层 fingerprint 不是 xray streamSettings 的字段；客户端指纹在
	// tlsSettings.fingerprint / realitySettings.fingerprint 内，必须保留。
	delete(stream, "fingerprint")

	// 面板嵌套存储，不进 xray 配置
	deleteNestedKey(stream, "tlsSettings", "settings")
	deleteNestedKey(stream, "realitySettings", "settings")

	// cipherSuites 在 xray 里是「: 分隔的 string」；历史前端按数组提交会让整份配置反序列化失败。
	normalizeCipherSuites(stream)

	// xray v26.6.27 起已移除 tlsSettings.allowInsecure（Build 直接报错），
	// 客户端同样不能携带；验证改由 pinnedPeerCertSha256 / verifyPeerCertByName 承担。
	deleteNestedKey(stream, "tlsSettings", "allowInsecure")

	// REALITY + finalmask 组合会导致 xray panic（XTLS/Xray-core#6453）。
	if security, _ := stream["security"].(string); security == "reality" {
		delete(stream, "finalmask")
	}

	// 清理无效的 finalmask 数据（Hysteria 的 obfs/带宽/端口跳跃属于客户端能力，保留）
	dropEmptyRandPackets(stream["finalmask"])

	// xray-core #6258：session 旧键名提升
	liftXhttpSessionKeys(stream)

	return stream
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

	// Hysteria 走 QUIC/HTTP3，ALPN 只能是 h3。存量/手改配置若写成 h2,http/1.1，
	// 客户端（http3 强制 h3）会握手失败（no application protocol），这里兜底修正；
	// NormalizeServer 已在保存链路做过一次，生成链路再保一次。
	forceHysteriaTLS(stream)

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
	// 以下同样是客户端角色字段：uTLS 指纹、ECH 客户端配置、系统根池开关。
	delete(tls, "fingerprint")
	delete(tls, "echConfigList")
	delete(tls, "useSystemCA")

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

// stripInboundClientOnlyReality 删除 realitySettings 里只被客户端消费的字段。
//
// 服务端只读 privateKey / dest / serverNames / shortIds / minClientVer / maxClientVer /
// maxTimeDiff / mldsa65Seed / limitFallback*；publicKey、password、spiderX、单数
// serverName/shortId、fingerprint、mldsa65Verify 都属于客户端侧（DB 原文保留给订阅）。
func stripInboundClientOnlyReality(stream map[string]any) {
	r, ok := stream["realitySettings"].(map[string]any)
	if !ok {
		return
	}
	for _, k := range []string{
		"publicKey", "password", "spiderX",
		"serverName", "shortId", "fingerprint", "mldsa65Verify",
	} {
		delete(r, k)
	}
}

// forceHysteriaTLS 修正 Hysteria 传输的 TLS 配置。
//
// hysteria 传输建立在 HTTP/3 之上：客户端 http3 会无条件把 ALPN 覆盖为 h3，
// 服务端若显式声明其它值（例如常规 TLS 默认的 h2,http/1.1）会导致
// no application protocol。留空时内核本来也会补 h3，这里统一写死，
// 避免存量数据和「原始 JSON」入口把配置写坏。
//
// QUIC 只支持 TLS 1.3：minVersion/maxVersion 与 cipherSuites 对握手中不生效，
// 而 maxVersion 写成 1.2 还会直接失败，所以生成侧一并删除。
func forceHysteriaTLS(stream map[string]any) {
	network, _ := stream["method"].(string)
	if strings.TrimSpace(network) == "" {
		network, _ = stream["network"].(string)
	}
	if !strings.EqualFold(strings.TrimSpace(network), "hysteria") {
		return
	}
	tls, ok := stream["tlsSettings"].(map[string]any)
	if !ok {
		return
	}
	tls["alpn"] = []any{"h3"}
	delete(tls, "minVersion")
	delete(tls, "maxVersion")
	delete(tls, "cipherSuites")
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

// stripInboundClientOnlyMasks 剥离只对客户端有意义的 finalmask 条目。
//
// Hysteria 端口跳跃（udphop）只有客户端实现：服务端监听时
// finalmask/udphop 直接返回 "udphop: client only"，而 `xray -test` 不会发现，
// 结果就是配置「校验通过但起不来」。DB 原文保留供订阅/机间出站投影，生成服务端配置时剥离。
// 出站配置（客户端角色）不能剥离，所以本函数只允许在入站生成链路调用。
func stripInboundClientOnlyMasks(stream map[string]any) {
	fm, ok := stream["finalmask"].(map[string]any)
	if !ok {
		return
	}
	udp, ok := fm["udp"].([]any)
	if !ok {
		return
	}
	kept := make([]any, 0, len(udp))
	for _, item := range udp {
		if m, ok := item.(map[string]any); ok {
			if typ, _ := m["type"].(string); strings.EqualFold(strings.TrimSpace(typ), "udphop") {
				continue
			}
		}
		kept = append(kept, item)
	}
	if len(kept) == 0 {
		delete(fm, "udp")
	} else {
		fm["udp"] = kept
	}
}

// stripInboundClientOnlyXHTTP 删除 xhttpSettings 里只被客户端 dialer 消费的字段。
//
// 这些字段进入站 DB 是为了订阅 / 机间出站投影（客户端角色），但服务端 hub 完全不读；
// 透传只会让服务端配置出现无效键。服务端实际消费的是 host/path/mode/headers/
// xPadding*/noSSEHeader/scMaxEachPostBytes/scMaxBufferedPosts/scStreamUpServerSecs/
// sessionID*/seq*/uplinkData*/serverMaxHeaderBytes。
func stripInboundClientOnlyXHTTP(stream map[string]any) {
	x, ok := stream["xhttpSettings"].(map[string]any)
	if !ok {
		return
	}
	for _, k := range []string{
		"xmux",                 // 客户端连接复用参数
		"downloadSettings",     // 客户端下行独立参数
		"noGRPCHeader",         // 客户端上行请求体格式
		"uplinkHTTPMethod",     // 客户端上行 HTTP 方法
		"uplinkChunkSize",      // 客户端上行分片
		"scMinPostsIntervalMs", // 客户端上发间隔
	} {
		delete(x, k)
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
		xhttp["sessionIDKey"] = v
	}
}
