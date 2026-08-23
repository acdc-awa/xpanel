// Package xray 负责从「服务器 + 入站 + 用户」生成 Xray 配置。
// 支持协议：VLESS（tcp+reality+vision / tcp+tls+fallbacks / xhttp+reality / xhttp+tls）；其他协议后续扩展。
package xray

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/protocols"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel-node/pkg/protocol"
)

// InboundSettings 对应 inbounds.settings_json（主控既用于生成服务端配置，也用于生成订阅）。
type InboundSettings struct {
	Reality *RealitySettings `json:"reality,omitempty"`
	XHTTP   *XHTTPSettings   `json:"xhttp,omitempty"`
	TLS     *TLSSettings     `json:"tls,omitempty"`
	// Fallbacks 仅应用于 tcp 传输（VLESS settings.fallbacks 线格式）。
	Fallbacks []FallbackSettings `json:"fallbacks,omitempty"`
	// Sniffing 入站流量嗅探（存储层 snake_case；前端表单以 camelCase 发送，UnmarshalJSON 双写兼容）。
	Sniffing *SniffingSettings `json:"sniffing,omitempty"`
}

// FallbackSettings 对应 xray-core VLESS settings.fallbacks 条目（infra/conf/vless.go 的 VLessInboundFallback）。
type FallbackSettings struct {
	Dest string `json:"dest"`
	Path string `json:"path,omitempty"`
	Xver int    `json:"xver,omitempty"`
}

type RealitySettings struct {
	ServerName string `json:"server_name"` // 客户端 SNI
	PublicKey  string `json:"public_key"`  // 客户端公钥
	ShortID    string `json:"short_id"`    // 客户端 shortId
	PrivateKey string `json:"private_key"` // 服务端私钥（不出现在订阅）
	Dest       string `json:"dest"`        // 服务端借壳目标 host:port
}

type XHTTPSettings struct {
	Mode string `json:"mode"`
	Path string `json:"path"`
	Host string `json:"host"`
}

// SniffingSettings 入站流量嗅探（存储层 snake_case；前端表单以 camelCase 发送，UnmarshalJSON 双写兼容）。
type SniffingSettings struct {
	Enabled      bool     `json:"enabled"`
	DestOverride []string `json:"dest_override,omitempty"`
	MetadataOnly bool     `json:"metadata_only,omitempty"`
	RouteOnly    bool     `json:"route_only,omitempty"`
}

// UnmarshalJSON 支持 camelCase (destOverride/metadataOnly/routeOnly) 与 snake_case (dest_override/
// metadata_only/route_only) 双写；enabled 两写法相同。前端表单按 camelCase 发送，存储按 snake_case。
// 两套键同时出现时显式 camelCase 优先（与 GRPCSettings 一致）。
func (s *SniffingSettings) UnmarshalJSON(data []byte) error {
	var raw struct {
		Enabled         bool     `json:"enabled"`
		DestOverride    []string `json:"destOverride"`
		DestOverrideAlt []string `json:"dest_override"`
		MetadataOnly    *bool    `json:"metadataOnly"`
		MetadataOnlyAlt *bool    `json:"metadata_only"`
		RouteOnly       *bool    `json:"routeOnly"`
		RouteOnlyAlt    *bool    `json:"route_only"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	s.Enabled = raw.Enabled
	s.DestOverride = raw.DestOverride
	if len(s.DestOverride) == 0 {
		s.DestOverride = raw.DestOverrideAlt
	}
	s.MetadataOnly = false
	if raw.MetadataOnly != nil {
		s.MetadataOnly = *raw.MetadataOnly
	} else if raw.MetadataOnlyAlt != nil {
		s.MetadataOnly = *raw.MetadataOnlyAlt
	}
	s.RouteOnly = false
	if raw.RouteOnly != nil {
		s.RouteOnly = *raw.RouteOnly
	} else if raw.RouteOnlyAlt != nil {
		s.RouteOnly = *raw.RouteOnlyAlt
	}
	return nil
}

type TLSSettings struct {
	ServerName    string `json:"server_name"`
	CertFile      string `json:"cert_file"`
	KeyFile       string `json:"key_file"`
	AllowInsecure bool   `json:"allowInsecure"` // stream_settings 线格式 camelCase；订阅 skip-cert-verify 透传
}

// UserEmail 用户在 Xray 中的 email（2026-08-14 方向①：同步用户真实邮箱，stats 上报按此回查 user_id；
// 空邮箱回退固定格式 user-<id>@panel.local 兼容存量）。注册起 username=email 且必填，新用户均为真邮箱。
func UserEmail(u *models.User) string {
	if u.Email != "" {
		return u.Email
	}
	return fmt.Sprintf("user-%d@panel.local", u.ID)
}

// RelayEmail relay 入站在 Xray 中的 email（不入用户体系，仅 stats 标识）。
func RelayEmail(tag string) string {
	return fmt.Sprintf("relay-%s@panel.local", tag)
}

// certDomainFor 返回入站绑定证书的域名（ctx 为 nil / 未绑定 / 域名非法时返回 false）。
func certDomainFor(inb *models.Inbound, ctx *GenerateContext) (string, bool) {
	if inb.CertID == nil || ctx == nil || ctx.CertDomains == nil {
		return "", false
	}
	domain, ok := ctx.CertDomains[*inb.CertID]
	if !ok || domain == "" || strings.ContainsAny(domain, "/\\") || domain == "." || domain == ".." {
		return "", false
	}
	return domain, true
}

// certFilePath 托管证书固定路径（与 agent push_cert 落盘一致，见 05 号文档 §4）。
func certFilePath(domain, file string) string {
	return "/etc/xray/certs/" + domain + "/" + file
}

// ParseSettings 解析入站 settings_json。
func ParseSettings(inb *models.Inbound) (*InboundSettings, error) {
	s := &InboundSettings{}
	if inb.SettingsJSON == "" {
		return s, nil
	}
	if err := json.Unmarshal([]byte(inb.SettingsJSON), s); err != nil {
		return nil, fmt.Errorf("入站 %s settings_json 解析失败: %w", inb.Tag, err)
	}
	return s, nil
}

// ValidateInbound 校验入站 JSON 基本有效性（透传模式：仅检查 JSON 可解析）。
func ValidateInbound(settingsJSON, streamSettings, sniffing string) error {
	if settingsJSON != "" {
		var v any
		if err := json.Unmarshal([]byte(settingsJSON), &v); err != nil {
			return fmt.Errorf("settings JSON 无效: %w", err)
		}
	}
	if streamSettings != "" {
		var v any
		if err := json.Unmarshal([]byte(streamSettings), &v); err != nil {
			return fmt.Errorf("streamSettings JSON 无效: %w", err)
		}
	}
	if sniffing != "" {
		var v any
		if err := json.Unmarshal([]byte(sniffing), &v); err != nil {
			return fmt.Errorf("sniffing JSON 无效: %w", err)
		}
	}
	// REALITY 密钥格式预检（非法 base64/长度推送到 agent 端 -test 才暴露）
	return ValidateRealityStream(streamSettings)
}

// ValidateOutbound 校验出站 settings/streamSettings JSON 有效性，
// 并对 REALITY 密钥做格式预检（01 号文档 §4 第 6 项）。
func ValidateOutbound(settingsJSON, streamSettings string) error {
	for _, s := range []string{settingsJSON, streamSettings} {
		if s == "" {
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			return fmt.Errorf("出站 JSON 无效: %w", err)
		}
	}
	return ValidateRealityStream(streamSettings)
}

// ValidateRealityStream 校验 streamSettings 中 REALITY 密钥格式：x25519 密钥须为
// base64 RawURL 解码 32 字节（xray x25519 输出格式）。非法直接报错，避免配置
// 推送到 agent 端 -test 才暴露（01 号文档 §4 第 6 项）。
func ValidateRealityStream(streamSettings string) error {
	if streamSettings == "" {
		return nil
	}
	var s struct {
		Security string `json:"security"`
		Reality  *struct {
			PrivateKey string `json:"privateKey"`
			PublicKey  string `json:"publicKey"` // 兼容旧名
			Password   string `json:"password"`  // 标准名（出站）
		} `json:"realitySettings"`
	}
	if err := json.Unmarshal([]byte(streamSettings), &s); err != nil {
		return fmt.Errorf("streamSettings JSON 无效: %w", err)
	}
	if s.Security != "reality" || s.Reality == nil {
		return nil
	}
	if s.Reality.PrivateKey != "" {
		if err := checkX25519Key(s.Reality.PrivateKey); err != nil {
			return fmt.Errorf("REALITY privateKey 非法: %w", err)
		}
	}
	pk := s.Reality.Password
	if pk == "" {
		pk = s.Reality.PublicKey
	}
	if pk != "" {
		if err := checkX25519Key(pk); err != nil {
			return fmt.Errorf("REALITY 公钥(password/publicKey) 非法: %w", err)
		}
	}
	return nil
}

// checkX25519Key x25519 密钥格式：base64 RawURL 解码后须为 32 字节（xray x25519 输出格式）。
func checkX25519Key(k string) error {
	dec, err := base64.RawURLEncoding.DecodeString(k)
	if err != nil {
		return fmt.Errorf("base64 解码失败: %w", err)
	}
	if len(dec) != 32 {
		return fmt.Errorf("解码后 %d 字节，须为 32 字节", len(dec))
	}
	return nil
}

// ValidateSettings 保留旧签名（向后兼容，委托给 ValidateInbound）。
func ValidateSettings(s *InboundSettings, network, tlsType string) error {
	return nil
}

// parseStringList 解析 JSON 数组字符串或逗号/换行/分号分隔字符串。
func parseStringList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		var arr []string
		if err := json.Unmarshal([]byte(s), &arr); err == nil {
			return arr
		}
	}
	lines := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';'
	})
	var res []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			res = append(res, l)
		}
	}
	return res
}

// RefTarget / GenerateContext 已上提为契约类型（Stage 4），此处保留别名以兼容既有代码与测试。
type RefTarget = contracts.RefTarget

// GenerateContext 拓扑化上下文（Phase T）：跨服务器引用与证书映射，由调用方从 DB 组装。
// 传 nil 时行为与旧版完全一致（无 InboundRef / CertID 注入）。
type GenerateContext = contracts.TopologyContext

// Generate 生成完整 Xray 配置（模板驱动：不可变段来自模板，动态段由代码填空）。
// usersByTag 为「入站 tag → 已按权限组过滤的用户列表」（服务层 GetValidUsers 计算结果，
// 与热更新 SyncUsers 同一数据源；12 号文档：订阅/热更新/配置生成消费同一组计算结果）。
// defaultOutboundTag 为空时使用 outbounds 第一个；routingDomainStrategy 为空时使用模板默认值。
// 2026-08-14 方向①/批2：UserInbound 冻结删除，per-user 覆盖不再存在（仅入站级 Flow 三态）。
func Generate(inbounds []models.Inbound, outbounds []models.ServerOutbound, routingRules []models.ServerRoutingRule, usersByTag map[string][]protocol.User, ctx *GenerateContext, defaultOutboundTag string, routingDomainStrategy string, defaultOutboundDS ...string) ([]byte, error) {
	cfg := LoadTemplate()

	// 旧数据/测试夹具可能无 Type（DB 默认 user）——归一化避免"未知类型"
	for i := range inbounds {
		if inbounds[i].Type == "" {
			inbounds[i].Type = models.InboundTypeUser
		}
	}

	// Phase T 预检：InboundRef 出站的落地入站必须已完成 setup（InternalUUID 非空）
	for _, ob := range outbounds {
		if !ob.Enabled || ob.InboundRef == nil {
			continue
		}
		target, ok := ctxTarget(ctx, *ob.InboundRef)
		if !ok {
			return nil, fmt.Errorf("出站 %s 引用的入站 %d 不存在", ob.Tag, *ob.InboundRef)
		}
		if target.Inbound.InternalUUID == "" {
			return nil, fmt.Errorf("出站 %s 引用的落地入站 %s 未完成内部账户 setup（InternalUUID 为空）", ob.Tag, target.Inbound.Tag)
		}
	}

	// 1. 出站：模板基础出站 + 节点自定义出站叠加，然后按默认出口排序
	cfg["outbounds"] = mergeOutbounds(cfg["outbounds"], outbounds, ctx, defaultOutboundTag)
	// 默认出口（freedom）出站解析策略注入：AsIs/UseIP/UseIPv4/UseIPv6（作用于出站连接阶段，
	// 与 routing.domainStrategy 语义不同；模板/DB 已显式配置非 AsIs 时不覆盖）
	if len(defaultOutboundDS) > 0 && defaultOutboundDS[0] != "" && defaultOutboundDS[0] != "AsIs" {
		if list, ok := cfg["outbounds"].([]any); ok {
			for _, item := range list {
				m, ok := item.(map[string]any)
				if !ok || m["tag"] != defaultOutboundTag {
					continue
				}
				if settings, ok := m["settings"].(map[string]any); ok {
					if cur, _ := settings["domainStrategy"].(string); cur == "" || cur == "AsIs" {
						settings["domainStrategy"] = defaultOutboundDS[0]
					}
				}
			}
		}
	}

	// 2. 路由规则：保留模板 routing 顶层字段 + api 保护规则 + 节点规则叠加
	routing := map[string]any{}
	if tmplRouting, ok := cfg["routing"].(map[string]any); ok {
		for k, v := range tmplRouting {
			routing[k] = v
		}
	}
	blockCN := checkBlockCN(outbounds, defaultOutboundTag)
	routing["rules"] = mergeRoutingRules(routingRules, blockCN)
	if routingDomainStrategy != "" {
		routing["domainStrategy"] = routingDomainStrategy
	}
	cfg["routing"] = routing

	// 3. 入站：全部启用的入站 + api 内建入站
	inboundList := []any{}
	for _, inb := range inbounds {
		if !inb.Enabled {
			continue
		}
		item, err := buildInbound(&inb, usersByTag, ctx)
		if err != nil {
			return nil, fmt.Errorf("生成入站 %s 失败: %w", inb.Tag, err)
		}
		inboundList = append(inboundList, item)
	}
	// api 入站：内部 gRPC 通信
	inboundList = append(inboundList, map[string]any{
		"tag": "api", "listen": "127.0.0.1", "port": 10085,
		"protocol": "dokodemo-door", "settings": map[string]any{"address": "127.0.0.1"},
	})
	cfg["inbounds"] = inboundList

	return json.MarshalIndent(cfg, "", "  ")
}

// mergeOutbounds 合并模板出站与节点自定义出站。模板提供基础（freedom/blackhole），
// 节点 ServerOutbound 叠加在模板之上。同 tag 时节点覆盖模板（允许管理员自定义基础策略）。
// InboundRef 出站忽略手填 settings/streamSettings，vnext 由生成器按目标入站自动构造。
// defaultTag 非空时将该标签的出站移至数组首位（xray 将数组首个出站作为路由默认出口）。
func mergeOutbounds(tmpl any, outs []models.ServerOutbound, ctx *GenerateContext, defaultTag string) []any {
	seen := make(map[string]int) // tag → index in list
	list := make([]any, 0)
	if arr, ok := tmpl.([]any); ok {
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				if tag, _ := m["tag"].(string); tag != "" {
					seen[tag] = len(list)
				}
			}
			list = append(list, item)
		}
	}
	for _, ob := range outs {
		if !ob.Enabled {
			continue
		}
		if ob.Protocol == "vless" && ob.InboundRef == nil && strings.TrimSpace(ob.SettingsJSON) == "" {
			// 草稿未连线出站（等待管理员在拓扑画布中拖线关联目标入站）：安全跳过注入，避免 Xray 配置校验失败
			continue
		}
		item := make(map[string]any)
		if ob.InboundRef != nil && ctx != nil {
			// InboundRef 出站：vnext / streamSettings 全自动构造（不手填）
			if target, ok := ctx.RefTargets[*ob.InboundRef]; ok {
				buildRefOutbound(item, target)
			}
		} else if ob.SettingsJSON != "" {
			var parsed map[string]any
			if err := json.Unmarshal([]byte(ob.SettingsJSON), &parsed); err == nil {
				_, hasProto := parsed["protocol"]
				_, hasSettings := parsed["settings"]
				switch {
				case hasProto || hasSettings:
					// 完整出站配置（含 protocol / settings 键）：透传到顶层
					for k, v := range parsed {
						item[k] = v
					}
				default:
					// 裸 settings（如 {"vnext": [...]}）：只进 settings，
					// 避免顶层 + settings 双写脏数据（01 号文档 §4 附注）
					item["settings"] = parsed
				}
			}
		}
		if ob.Tag != "" {
			item["tag"] = ob.Tag
		}
		if ob.Protocol != "" {
			item["protocol"] = ob.Protocol
		}
		if ob.StreamSettingsJSON != "" {
			var stream map[string]any
			if err := json.Unmarshal([]byte(sanitizeStreamSettings(ob.StreamSettingsJSON)), &stream); err == nil {
				item["streamSettings"] = stream
			}
		}
		if ob.SendThrough != "" {
			item["sendThrough"] = ob.SendThrough
		}
		// vless 出站 users 兜底注入 encryption:"none"（xray 校验强制，见 01 号文档 §2.2）
		normalizeVlessOutbound(item)
		tag, _ := item["tag"].(string)
		if tag != "" {
			if idx, exists := seen[tag]; exists {
				list[idx] = item // 覆盖模板中的同 tag 出站
				continue
			}
			seen[tag] = len(list)
		}
		list = append(list, item)
	}

	// 默认出口置顶：xray 路由未命中时使用 outbounds[0] 作为默认出口
	if defaultTag != "" {
		for i, item := range list {
			if m, ok := item.(map[string]any); ok {
				if t, _ := m["tag"].(string); t == defaultTag {
					// 移到首位
					copy(list[1:i+1], list[0:i])
					list[0] = item
					break
				}
			}
		}
	}

	return list
}

// ctxTarget 从 GenerateContext 取引用目标（nil ctx 安全）。
func ctxTarget(ctx *GenerateContext, id uint64) (RefTarget, bool) {
	if ctx == nil || ctx.RefTargets == nil {
		return RefTarget{}, false
	}
	t, ok := ctx.RefTargets[id]
	return t, ok
}

// buildRefOutbound 按目标入站自动构造中转出站（vless）：vnext address/port/uuid/flow
// 与 streamSettings（reality 公钥/SNI/shortId 派生、tls serverName）全部由生成器填充。
func buildRefOutbound(item map[string]any, target RefTarget) {
	inb := target.Inbound
	spec := contracts.DecodeInbound(&inb)
	net := spec.Network
	sec := spec.Security

	flow := ""
	if net == "tcp" && (sec == "reality" || sec == "tls") {
		flow = "xtls-rprx-vision"
	}
	user := map[string]any{
		"id":         inb.InternalUUID,
		"encryption": "none",
	}
	if flow != "" {
		user["flow"] = flow
	}
	item["protocol"] = "vless"
	item["settings"] = map[string]any{
		"vnext": []any{map[string]any{
			"address": target.ServerHost,
			"port":    inb.Port,
			"users":   []any{user},
		}},
	}

	// streamSettings：传输参数透传目标 + 安全参数自动填充
	ss := map[string]any{"network": net, "security": sec}
	if net == "xhttp" {
		if x := spec.XHTTP; x != nil {
			xh := map[string]any{"mode": x.Mode, "path": x.Path}
			if x.Host != "" {
				xh["host"] = x.Host
			}
			ss["xhttpSettings"] = xh
		}
	}
	switch sec {
	case "reality":
		// 出站 REALITY：公钥标准名 password、SNI/shortId 单数（01 号文档 §2.2）
		if r := spec.Reality; r != nil && r.ServerName != "" {
			ss["realitySettings"] = map[string]any{
				"serverName":  r.ServerName,
				"password":    r.PublicKey,
				"shortId":     r.ShortID,
				"fingerprint": "chrome",
				"spiderX":     "/",
			}
		}
	case "tls":
		serverName := target.ServerHost
		if t := spec.TLS; t != nil && t.ServerName != "" {
			serverName = t.ServerName
		}
		// 注意：v26.6.27 已移除 allowInsecure（迁移 pinnedPeerCertSha256/verifyPeerCertByName），不输出
		tlsSettings := map[string]any{"serverName": serverName}
		// 链式代理证书固定：目标入站绑定的证书带 pin（面板生成/上传时自动计算入库）时注入，
		// pin 命中即验证通过——自签证书链路亦防 MITM（v26.6.27 实测字段为 leaf DER 的 64 位小写 hex）。
		if target.CertPin != "" {
			tlsSettings["pinnedPeerCertSha256"] = target.CertPin
		}
		ss["tlsSettings"] = tlsSettings
	}
	item["streamSettings"] = ss
}

// normalizeVlessOutbound 对 vless 出站的 settings.vnext[].users[] 兜底注入
// encryption:"none"（缺该字段 xray -test 直接报错；入站相反，禁止 encryption）。
func normalizeVlessOutbound(item map[string]any) {
	proto, _ := item["protocol"].(string)
	if proto != "vless" {
		return
	}
	settings, _ := item["settings"].(map[string]any)
	if settings == nil {
		return
	}
	vnext, _ := settings["vnext"].([]any)
	for _, vn := range vnext {
		vm, ok := vn.(map[string]any)
		if !ok {
			continue
		}
		users, _ := vm["users"].([]any)
		for _, u := range users {
			um, ok := u.(map[string]any)
			if !ok {
				continue
			}
			if _, has := um["encryption"]; !has {
				um["encryption"] = "none"
			}
		}
	}
}

// checkBlockCN 检查启用的 direct 出站或默认出口是否开启了 block_cn 开关。
func checkBlockCN(outbounds []models.ServerOutbound, defaultOutboundTag string) bool {
	for _, ob := range outbounds {
		if !ob.Enabled {
			continue
		}
		if ob.Tag == "direct" || (defaultOutboundTag != "" && ob.Tag == defaultOutboundTag) || ob.Protocol == "freedom" {
			if ob.SettingsJSON != "" {
				var s struct {
					BlockCN bool `json:"block_cn"`
				}
				if err := json.Unmarshal([]byte(ob.SettingsJSON), &s); err == nil && s.BlockCN {
					return true
				}
			}
		}
	}
	return false
}

// mergeRoutingRules 合并模板路由规则与节点路由规则。
// 顺序：api 保护规则（最前）→ 默认规则（BT 屏蔽 / 内网直连 / 阻断回国）→ 节点规则（按 Priority ASC, id ASC 排序）。
func mergeRoutingRules(rules []models.ServerRoutingRule, blockCN ...bool) []any {
	list := []any{
		map[string]any{
			"type":        "field",
			"inboundTag":  []string{"api"},
			"outboundTag": "api",
		},
	}

	// 默认内置规则（3x-ui 风格）：BT 流量屏蔽、内网 IP 直连、阻断回国流量（若开启）
	// 仅当 DB 中不存在同类型规则时才注入（允许用户自定义覆盖）
	hasBT := false
	hasPrivate := false
	hasCN := false
	for _, rule := range rules {
		if rule.Enabled {
			if rule.Protocol == "bittorrent" {
				hasBT = true
			}
			if rule.IP == "geoip:private" {
				hasPrivate = true
			}
			if strings.Contains(rule.Domain, "geosite:cn") || strings.Contains(rule.IP, "geoip:cn") {
				hasCN = true
			}
		}
	}
	if !hasBT {
		list = append(list, map[string]any{
			"type":        "field",
			"protocol":    []string{"bittorrent"},
			"outboundTag": "blocked",
		})
	}
	if !hasPrivate {
		list = append(list, map[string]any{
			"type":        "field",
			"ip":          []string{"geoip:private"},
			"outboundTag": "direct",
		})
	}
	if len(blockCN) > 0 && blockCN[0] && !hasCN {
		list = append(list, map[string]any{
			"type":        "field",
			"domain":      []string{"geosite:cn"},
			"ip":          []string{"geoip:cn"},
			"outboundTag": "blocked",
		})
	}

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if rule.RuleJSON != "" {
			var rmap map[string]any
			if err := json.Unmarshal([]byte(rule.RuleJSON), &rmap); err == nil {
				if rmap["type"] == nil {
					rmap["type"] = "field"
				}
				if rule.OutboundTag != "" && rmap["outboundTag"] == nil {
					rmap["outboundTag"] = rule.OutboundTag
				}
				list = append(list, rmap)
				continue
			}
		}
		rmap := map[string]any{
			"type":        "field",
			"outboundTag": rule.OutboundTag,
		}
		if domains := parseStringList(rule.Domain); len(domains) > 0 {
			rmap["domain"] = domains
		}
		if ips := parseStringList(rule.IP); len(ips) > 0 {
			rmap["ip"] = ips
		}
		if rule.Port != "" {
			rmap["port"] = rule.Port
		}
		if rule.Network != "" {
			rmap["network"] = rule.Network
		}
		if protocols := parseStringList(rule.Protocol); len(protocols) > 0 {
			rmap["protocol"] = protocols
		}
		if rule.InboundTag != "" {
			rmap["inboundTag"] = parseStringList(rule.InboundTag)
		}
		list = append(list, rmap)
	}
	return list
}

// buildInbound 透传模式：解析 SettingsJSON / StreamSettings / Sniffing 原文，
// 动态注入 clients 列表（按入站二态分流——user 动态用户 / relay 内部 UUID）。其余字段完全透传。
func buildInbound(inb *models.Inbound, usersByTag map[string][]protocol.User, ctx *GenerateContext) (map[string]any, error) {
	spec := contracts.DecodeInbound(inb)

	// 1. 解析协议 settings JSON → 注入 clients
	settings := map[string]any{}
	if inb.SettingsJSON != "" {
		if err := json.Unmarshal([]byte(inb.SettingsJSON), &settings); err != nil {
			return nil, fmt.Errorf("入站 %s settings 解析失败: %w", inb.Tag, err)
		}
	}
	// VLESS: xray 拒绝入站级别的 encryption 字段（运行时固定为 "none"）
	if inb.Protocol == "vless" {
		delete(settings, "encryption")
		settings["decryption"] = "none"
	}

	// Phase T：入站三态分流
	var clients []any
	switch inb.Type {
	case models.InboundTypeRelay:
		if inb.InternalUUID == "" {
			return nil, fmt.Errorf("relay 入站 %s 缺少内部 UUID（请先执行 setup-internal）", inb.Tag)
		}
		flow := ""
		if spec.Network == "tcp" &&
			(spec.Security == "reality" || spec.Security == "tls") {
			flow = "xtls-rprx-vision"
		}
		c := map[string]any{
			"id":    inb.InternalUUID,
			"email": RelayEmail(inb.Tag),
		}
		if flow != "" {
			c["flow"] = flow
		}
		clients = []any{c}
	default:
		// user 入站：动态用户列表（已由服务层 GetValidUsers 按接入点白名单派生过滤，无接入点指向返回空列表）；
		// clients 结构由协议插件生成（未注册协议走最小通用注入）
		userList := usersByTag[inb.Tag]
		clients = protocols.ServerClients(inb.Protocol, userList, spec)
	}
	settings["clients"] = clients

	// 2. 解析 streamSettings JSON（完全透传）
	item := map[string]any{
		"tag":      inb.Tag,
		"protocol": inb.Protocol,
		"port":     inb.Port,
		"settings": settings,
	}
	if inb.Listen != "" && inb.Listen != "0.0.0.0" {
		item["listen"] = inb.Listen
	}
	if inb.StreamSettings != "" {
		cleaned := sanitizeStreamSettings(inb.StreamSettings)
		var stream map[string]any
		if err := json.Unmarshal([]byte(cleaned), &stream); err != nil {
			return nil, fmt.Errorf("入站 %s streamSettings 解析失败: %w", inb.Tag, err)
		}
		// 证书路径动态注入（Phase T）：当关联了 CertID 且为 TLS 入站时，覆盖 certificates 路径
		if spec.Security == "tls" {
			if domain, ok := certDomainFor(inb, ctx); ok {
				stream["tlsSettings"] = map[string]any{
					"certificates": []any{
						map[string]any{
							"certificateFile": certFilePath(domain, "fullchain.pem"),
							"keyFile":         certFilePath(domain, "key.pem"),
						},
					},
				}
			}
		}
		item["streamSettings"] = stream
	}
	// 3. 解析 sniffing JSON（完全透传）
	if inb.Sniffing != "" {
		var sniffing map[string]any
		if err := json.Unmarshal([]byte(inb.Sniffing), &sniffing); err == nil {
			item["sniffing"] = sniffing
		}
	}
	return item, nil
}

// StreamHasReality 判断 streamSettings JSON 是否启用了 REALITY。
// 以下为统一解码层（contracts.InboundSpec）的兼容包装：保留旧签名供既有测试与调用方，
// 新代码应直接使用 contracts.DecodeInbound/DecodeStream。
func StreamHasReality(raw string) bool {
	return StreamSecurity(raw) == "reality"
}

// StreamNetwork 从 streamSettings JSON 中提取 network 字段。
func StreamNetwork(raw string) string {
	return contracts.DecodeStream(raw).Network
}

// StreamSecurity 从 streamSettings JSON 中提取 security 字段。
func StreamSecurity(raw string) string {
	return contracts.DecodeStream(raw).Security
}

// StreamReality 从 streamSettings JSON 中提取 realitySettings。
// 与历史行为一致：security 非 reality、解析失败或缺 ServerName 时返回 nil。
func StreamReality(raw string) *RealitySettings {
	r := contracts.DecodeStream(raw).Reality
	if r == nil || r.ServerName == "" {
		return nil
	}
	return &RealitySettings{
		ServerName: r.ServerName,
		PublicKey:  r.PublicKey,
		ShortID:    r.ShortID,
		PrivateKey: r.PrivateKey,
		Dest:       r.Dest,
	}
}

// StreamTLS 从 streamSettings JSON 中提取 tlsSettings（订阅 servername / skip-cert-verify 透传）。
func StreamTLS(raw string) *TLSSettings {
	t := contracts.DecodeStream(raw).TLS
	if t == nil {
		return nil
	}
	return &TLSSettings{
		ServerName:    t.ServerName,
		AllowInsecure: t.AllowInsecure,
	}
}

// StreamXHTTP 从 streamSettings JSON 中提取 xhttpSettings。
func StreamXHTTP(raw string) *XHTTPSettings {
	x := contracts.DecodeStream(raw).XHTTP
	if x == nil {
		return nil
	}
	return &XHTTPSettings{Mode: x.Mode, Path: x.Path, Host: x.Host}
}
