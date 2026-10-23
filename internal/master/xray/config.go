// Package xray 负责从「服务器 + 入站 + 用户」生成 Xray 配置。
// 支持协议：VLESS（tcp+reality+vision / tcp+tls+fallbacks / xhttp+reality / xhttp+tls）；其他协议后续扩展。
package xray

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/protocols"
	"github.com/acdc-awa/xpanel/internal/models"
)

// UserEmailFor 用户在指定入站的 Xray 统计键 email：u<uid>.i<iid>@panel.local（UUID 不变，
// 仅是统计键）。xray 用户级计数器（user>>>email）与在线地图均无入站维度，这里把入站
// 维度直接注入统计键，流量回收即可精确归账 (用户, 入站) 并按该入站倍率计费（2026-09-06）。
// 约束与保障：
//   - xray 仅要求 email 在单个入站内唯一（vless validator 按 ToLower 建键，跨入站可重复），
//     同一用户多入站多键、不同用户不同键天然满足；
//   - 全小写数字格式，不受 xray 大小写折叠影响（旧方案注入真实邮箱时会被 ToLower，
//     回收端大小写敏感查库导致静默丢流量）；
//   - 不含 ">"（counter 名裸拼接 user>>>email>>>traffic>>>uplink，email 不得含分隔符）。
//
// 撞名豁免：若用户注册了形如 u1.i2@panel.local 的真实邮箱，其流量会按键解析归到
// 用户 1——与旧 fallback 格式 user-<id>@panel.local 同类既有怪癖，解析优先于查库。
func UserEmailFor(u *models.User, inboundID uint64) string {
	return fmt.Sprintf("u%d.i%d@panel.local", u.ID, inboundID)
}

var (
	userEmailForRe     = regexp.MustCompile(`^u(\d+)\.i(\d+)@panel\.local$`)
	userEmailLegacyRe  = regexp.MustCompile(`^user-(\d+)@panel\.local$`)
	relayEmailRe       = regexp.MustCompile(`^relay\.in_(\d+)\.out_(\d+)@panel\.local$`)
	relayEmailLegacyRe = regexp.MustCompile(`^relay\.out_(\d+)@panel\.local$`)
)

// ParseRelayEmail 反解 RelayEmail 格式 → (inboundID, outboundID, ok)。
func ParseRelayEmail(email string) (inboundID, outboundID uint64, ok bool) {
	e := strings.ToLower(email)
	if m := relayEmailRe.FindStringSubmatch(e); m != nil {
		iid, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			return 0, 0, false
		}
		oid, err := strconv.ParseUint(m[2], 10, 64)
		if err != nil {
			return 0, 0, false
		}
		return iid, oid, true
	}
	if m := relayEmailLegacyRe.FindStringSubmatch(e); m != nil {
		oid, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			return 0, 0, false
		}
		return 0, oid, true
	}
	return 0, 0, false
}

// ParseUserEmailFor 反解 UserEmailFor 格式 → (用户ID, 入站ID)；非该格式返回 false。
// 先小写化：xray validator 按 ToLower 建键，防手改配置注入大写后对不上。
func ParseUserEmailFor(email string) (userID, inboundID uint64, ok bool) {
	uid, iid, ok := ParseUserEmailAny(email)
	if !ok || iid == 0 {
		return 0, 0, false
	}
	return uid, iid, true
}

// ParseUserEmailAny 兼容解析面板生成的用户统计键：新格式 u<uid>.i<iid>@panel.local →
// (uid, iid, true)；旧格式 user-<uid>@panel.local → (uid, 0, true)——存量节点在配置
// 重生成前仍按旧键上报，回收端必须继续认得。其余（真实邮箱/relay/自定义/数字溢出）→ false。
func ParseUserEmailAny(email string) (userID, inboundID uint64, ok bool) {
	e := strings.ToLower(email)
	if m := userEmailForRe.FindStringSubmatch(e); m != nil {
		uid, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			return 0, 0, false
		}
		iid, err := strconv.ParseUint(m[2], 10, 64)
		if err != nil {
			return 0, 0, false
		}
		return uid, iid, true
	}
	if m := userEmailLegacyRe.FindStringSubmatch(e); m != nil {
		uid, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			return 0, 0, false
		}
		return uid, 0, true
	}
	return 0, 0, false
}

// CountDistinctOnlineUsers 按去重用户口径统计在线快照人数：面板用户按统计键反解去重
// （按入站区分统计键后，同一商业用户若跨入站/双协议连接，只计一人）；
// relay 内部账户与自定义 email 各计一。
func CountDistinctOnlineUsers(users []protocol.OnlineUserIPs) int {
	if len(users) == 0 {
		return 0
	}
	seenUsers := make(map[uint64]struct{}, len(users))
	n := 0
	for _, u := range users {
		if uid, _, ok := ParseUserEmailAny(u.Email); ok {
			if _, dup := seenUsers[uid]; !dup {
				seenUsers[uid] = struct{}{}
				n++
			}
		} else {
			n++
		}
	}
	return n
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

// ValidateTLSCertSource 保存期校验：TLS 入站必须有可用的服务端证书来源。
// 来源二选一：绑定面板托管证书（certID），或 certificates 里成对的证书/私钥。
// 否则节点加载配置时直接失败（xray -test 不校验这一点，只有真正握手才暴露）。
// Hysteria 2 无论 streamSettings 是否为空都强制 TLS（协议插件会补 security=tls），因此单独判定。
func ValidateTLSCertSource(streamSettings string, certID *uint64, protocol string) error {
	if certID != nil && *certID != 0 {
		return nil
	}
	isTLS := protocol == models.ProtocolHysteria
	if strings.TrimSpace(streamSettings) == "" {
		if !isTLS {
			return nil
		}
	} else {
		var s struct {
			Security string `json:"security"`
			TLS      *struct {
				Certificates []struct {
					Usage           string   `json:"usage"`
					CertificateFile string   `json:"certificateFile"`
					KeyFile         string   `json:"keyFile"`
					Certificate     []string `json:"certificate"`
					Key             []string `json:"key"`
				} `json:"certificates"`
			} `json:"tlsSettings"`
		}
		if err := json.Unmarshal([]byte(streamSettings), &s); err != nil {
			return fmt.Errorf("streamSettings JSON 无效: %w", err)
		}
		if s.Security != "tls" {
			return nil
		}
		if s.TLS != nil {
			for _, c := range s.TLS.Certificates {
				if c.Usage == "verify" {
					continue // 纯客户端语义，服务端忽略
				}
				if c.CertificateFile != "" && c.KeyFile != "" {
					return nil
				}
				if len(c.Certificate) > 0 && len(c.Key) > 0 {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("TLS 入站必须绑定托管证书，或填写成对的证书/私钥文件路径")
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

// ParseStringList 解析 JSON 数组字符串或逗号/换行/分号分隔字符串。
func ParseStringList(s string) []string {
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
// defaultOutboundTag 只决定 outbounds[0]（Xray 路由未命中时的兜底出口），不携带解析策略：
// freedom 出站的 settings.domainStrategy 一律取自该出站在库内的自有配置（唯一入口 = 出站编辑器）。
// 2026-08-14 方向①/批2：UserInbound 冻结删除，per-user 覆盖不再存在（仅入站级 Flow 三态）。
func Generate(inbounds []models.Inbound, outbounds []models.ServerOutbound, routingRules []models.ServerRoutingRule, usersByTag map[string][]protocol.User, ctx *GenerateContext, defaultOutboundTag string, routingDomainStrategy string) ([]byte, error) {
	cfg := LoadTemplate()

	// 旧数据/测试夹具可能无 Type（DB 默认 user）——归一化避免"未知类型"
	for i := range inbounds {
		if inbounds[i].Type == "" {
			inbounds[i].Type = models.InboundTypeUser
		}
	}

	// 预检：InboundRef 出站或其引用的落地入站必须持有认证凭据
	for _, ob := range outbounds {
		if !ob.Enabled || ob.InboundRef == nil {
			continue
		}
		target, ok := ctxTarget(ctx, *ob.InboundRef)
		if !ok {
			return nil, fmt.Errorf("出站 %s 引用的入站 %d 不存在", ob.Tag, *ob.InboundRef)
		}
		if (target.Inbound.Protocol == "vless" || target.Inbound.Protocol == "hysteria") &&
			target.Inbound.InternalUUID == "" && ob.InternalUUID == "" {
			return nil, fmt.Errorf("出站 %s 引用的落地入站 %s 未完成内部账户 setup（InternalUUID 为空）", ob.Tag, target.Inbound.Tag)
		}
	}

	// 1. 入站：全部启用的入站（四层直通只注入监听，派生出站与转发规则是库内真行，见 C2）
	inboundList := []any{}
	for _, inb := range inbounds {
		if !inb.Enabled {
			continue
		}
		if inb.Type == models.InboundTypeTunnel ||
			(inb.Type == models.InboundTypeChannel && inb.Protocol == models.ProtocolDokodemo) ||
			(inb.Type == models.InboundTypeForward && inb.Protocol == models.ProtocolDokodemo) {
			// C2（2026-09-28）：直通的派生 freedom 出站（`out-<tag>`）与转发规则已落库为真行
			// （owner_inbound_id = 该入站），本函数不再现场派生——出站随 Outbounds 参数、规则随
			// RoutingRules 参数进来，顺序由 Priority 决定（见 mergeRoutingRules）。
			item, err := buildTunnelInbound(&inb, contracts.DecodeInbound(&inb), ctx)
			if err != nil {
				return nil, fmt.Errorf("生成直通管道 %s 失败: %w", inb.Tag, err)
			}
			if item == nil {
				// 未连线或目标缺失，作为草稿跳过注入 Xray
				continue
			}
			inboundList = append(inboundList, item)
			continue
		}
		item, err := buildInbound(&inb, usersByTag, ctx)
		if err != nil {
			return nil, fmt.Errorf("生成入站 %s 失败: %w", inb.Tag, err)
		}
		if item == nil {
			// 协议插件判定为不可生成（如无有效认证账号），物理跳过注入 Xray
			continue
		}
		inboundList = append(inboundList, item)
	}
	// api 入站：内部 gRPC 通信
	inboundList = append(inboundList, map[string]any{
		"tag": "api", "listen": "127.0.0.1", "port": 10085,
		"protocol": "dokodemo-door", "settings": map[string]any{"address": "127.0.0.1"},
	})
	cfg["inbounds"] = inboundList

	// 2. 出站：模板基础出站 + 节点自定义出站（含直通落库的 out-<tag>）叠加，然后按默认出口排序
	obs, err := mergeOutbounds(cfg["outbounds"], outbounds, ctx, defaultOutboundTag)
	if err != nil {
		return nil, err
	}
	cfg["outbounds"] = obs

	// 3. 路由规则：保留模板 routing 顶层字段 + api 保护规则 + 节点规则叠加（含直通落库的转发规则）
	routing := map[string]any{}
	if tmplRouting, ok := cfg["routing"].(map[string]any); ok {
		for k, v := range tmplRouting {
			routing[k] = v
		}
	}
	routing["rules"] = mergeRoutingRules(routingRules)
	if routingDomainStrategy != "" {
		routing["domainStrategy"] = routingDomainStrategy
	}
	cfg["routing"] = routing

	// 4. 规则库热重载：顶层 geodata.cron 让 xray 按周期重读磁盘上的 geoip.dat/geosite.dat。
	// assets 留空表示只重读本地文件（不联网）；这样 agent 的规则库更新落盘后无需重启内核。
	// 模板若已显式给出 geodata（自定义 cron/assets），以模板为准。
	if _, exists := cfg["geodata"]; !exists {
		cfg["geodata"] = map[string]any{"cron": "0 5 * * *"}
	}

	return json.MarshalIndent(cfg, "", "  ")
}

// mergeOutbounds 合并模板出站与节点自定义出站。模板提供基础（freedom/blackhole），
// 节点 ServerOutbound 叠加在模板之上。同 tag 时节点覆盖模板（允许管理员自定义基础策略）。
// InboundRef 出站忽略手填 settings/streamSettings，vnext 由生成器按目标入站自动构造。
// defaultTag 非空时将该标签的出站移至数组首位（xray 将数组首个出站作为路由默认出口）。
func mergeOutbounds(tmpl any, outs []models.ServerOutbound, ctx *GenerateContext, defaultTag string) ([]any, error) {
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
		if ob.InboundRef == nil && strings.TrimSpace(ob.SettingsJSON) == "" && strings.TrimSpace(ob.OutboundJSON) == "" &&
			ob.Protocol != "freedom" && ob.Protocol != "blackhole" {
			// 草稿未连线出站（等待管理员在拓扑画布/出站编辑器中选择目标入站）：
			// 安全跳过注入，避免 Xray 配置校验失败。
			continue
		}
		item := make(map[string]any)

		if strings.TrimSpace(ob.OutboundJSON) != "" {
			// 原生完整出站 JSON 优先模式
			var parsed map[string]any
			if err := json.Unmarshal([]byte(ob.OutboundJSON), &parsed); err == nil {
				item = parsed
			}
		} else if ob.InboundRef != nil && ctx != nil && ob.Protocol != "freedom" {
			// InboundRef 节点间出站：由对应目标协议插件统一投影全自动构造（支持 vless / hysteria / mixed 等）
			if target, ok := ctx.RefTargets[*ob.InboundRef]; ok {
				if err := buildRefOutbound(item, target, ob); err != nil {
					return nil, err
				}
			}
		} else if ob.InboundRef != nil && ctx != nil && ob.Protocol == "freedom" {
			// InboundRef freedom 出站（四层直通派生链路）：redirect 目标跟随落地入站，保留已配置的 proxyProtocol 等高级项
			var parsed map[string]any
			if ob.SettingsJSON != "" {
				_ = json.Unmarshal([]byte(ob.SettingsJSON), &parsed)
			}
			if parsed == nil {
				parsed = make(map[string]any)
			}
			if target, ok := ctx.RefTargets[*ob.InboundRef]; ok {
				parsed["redirect"] = fmt.Sprintf("%s:%d", target.ServerHost, target.Inbound.Port)
			}
			item["settings"] = parsed
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
		if ob.Protocol != "" && item["protocol"] == nil {
			item["protocol"] = ob.Protocol
		}
		if ob.StreamSettingsJSON != "" {
			var stream map[string]any
			if err := json.Unmarshal([]byte(SanitizeOutboundStreamSettings(ob.StreamSettingsJSON)), &stream); err == nil {
				item["streamSettings"] = stream
			}
		}
		if ob.SendThrough != "" {
			item["sendThrough"] = ob.SendThrough
		}
		// vless 出站 users 兜底注入 encryption:"none"（xray 校验强制，见 01 号文档 §2.2）
		normalizeVlessOutbound(item)
		// freedom 出站面板开关（block_cn）拆分为 finalRules，面板键不透传给 xray
		translateFreedomPanelKeys(item)
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

	return list, nil
}

// ctxTarget 从 GenerateContext 取引用目标（nil ctx 安全）。
func ctxTarget(ctx *GenerateContext, id uint64) (RefTarget, bool) {
	if ctx == nil || ctx.RefTargets == nil {
		return RefTarget{}, false
	}
	t, ok := ctx.RefTargets[id]
	return t, ok
}

// buildRefOutbound 按目标入站自动构造中转出站：由目标入站协议插件的 Peer 角色投影决议端点 DTO，
// 再由 protocols.RenderOutbound 统一渲染为对应核心的出站结构。
func buildRefOutbound(item map[string]any, target RefTarget, ob models.ServerOutbound) error {
	inb := target.Inbound
	plugin := protocols.Find(inb.Protocol)
	if plugin == nil {
		return fmt.Errorf("出站 %s 引用的目标入站协议 %q 未注册协议插件", item["tag"], inb.Protocol)
	}

	dto, err := plugin.ProjectClient(&contracts.ClientProjectionInput{
		Role:          contracts.RolePeer,
		RefTarget:     &target,
		OutboundUUID:  ob.InternalUUID,
		TlsVerifyMode: ob.TlsVerifyMode,
	})
	if err != nil {
		return fmt.Errorf("出站 %s 投影目标入站 %s 失败: %w", item["tag"], inb.Tag, err)
	}

	rendered, err := protocols.RenderOutbound(dto)
	if err != nil {
		return fmt.Errorf("出站 %s 渲染中转出站失败: %w", item["tag"], err)
	}
	for k, v := range rendered {
		item[k] = v
	}
	return nil
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

// translateFreedomPanelKeys 把 freedom 出站 settings 中的面板专用开关拆分为真实 xray 表达，
// 面板键本身不透传（xray freedom settings 无 block_cn 字段，静默忽略属于脏配置）。
// block_cn（阻止回国流量）→ 出站级 finalRules 注入 {"action":"block","ip":["geoip:cn"]}：
// 路由层规则是全局分发（会误伤中转等其他出口的合法回国链路），finalRules 只拦走该出站的
// 流量，且在解析出最终 IP 后、拨号前匹配，域名会被强制解析，比 geosite 更彻底；
// 域名级拦截无此字段，故仅支持 IP 级。
// blockDelay 有意不设置（保留官方默认 30-90s 黑洞）：回国拦截历史上一直是出站级 finalRules，
// 黑洞延迟起到抗探测作用；而私网拦截原先由路由层的 blocked 出站「立即断开」承担，改用
// finalRules 后必须显式 blockDelay:"0" 才能维持同样的快速失败（见 models 的 canonical 默认值）。
func translateFreedomPanelKeys(item map[string]any) {
	proto, _ := item["protocol"].(string)
	if proto != "freedom" {
		return
	}
	settings, _ := item["settings"].(map[string]any)
	if settings == nil {
		return
	}
	blockCN, _ := settings["block_cn"].(bool)
	delete(settings, "block_cn")
	if !blockCN {
		return
	}
	rules, _ := settings["finalRules"].([]any)
	for _, r := range rules {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		if action, _ := m["action"].(string); action == "block" {
			if ips, ok := m["ip"].([]any); ok && len(ips) > 0 && ips[0] == "geoip:cn" {
				return // 已有同类规则（存量数据），不重复注入
			}
		}
	}
	newRule := map[string]any{"action": "block", "ip": []string{"geoip:cn"}}
	for i, r := range rules {
		if m, ok := r.(map[string]any); ok {
			if action, _ := m["action"].(string); action == "allow" {
				// 插在 allow 兜底之前（兜底后的规则永不匹配）
				updated := append(append(rules[:i:i], newRule), rules[i:]...)
				settings["finalRules"] = updated
				return
			}
		}
	}
	settings["finalRules"] = append(rules, newRule)
}

// mergeRoutingRules 合并模板路由规则与节点路由规则。
// 顺序：api 保护规则（最前）→ BT 屏蔽规则（等价于「Priority = 0 的第一条」）→ 节点规则（按 Priority ASC, id ASC 排序）。
// 私网阻断不在路由层（改由 freedom 出站 finalRules 承担，见函数内注释）。
//
// 四层直通规则自 C2（2026-09-28）起也是库内真行（Priority = -100），随 rules 一并排序，
// 因此默认位置与历史行为一致（全部节点规则之前），而管理员把某条规则的优先级设成 < -100
// 时就能排到它之前——这正是「给直通做来源限制」的唯一途径。
func mergeRoutingRules(rules []models.ServerRoutingRule) []any {
	list := []any{
		map[string]any{
			"type":        "field",
			"inboundTag":  []string{"api"},
			"outboundTag": "api",
		},
	}

	// 先按 Priority 稳定排序（同优先级保持调用方给的顺序，生产端是 id ASC）。
	// 排序放在生成器里而不是只依赖调用方的取数顺序：BT 规则的插入位置是按优先级判定的，
	// 若规则未经排序，插入点与命中顺序会互相矛盾（顺序 = 谁先命中，必须与优先级一致）。
	sorted := append([]models.ServerRoutingRule(nil), rules...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Priority < sorted[j].Priority })
	rules = sorted

	// 默认内置规则：BT 流量屏蔽。BT 属服务器级滥用防护（P2P 分发不因出站不同而改变性质，
	// freedom 的 finalRules 也无对应能力），故仍在路由层注入。
	// 仅当 DB 中不存在同类型规则时才注入（允许用户自定义覆盖）。
	//
	// 位置语义（C2 起显式化）：BT 规则等价于「Priority = 0 的第一条」——优先级为负的规则
	// （含 Priority = -100 的直通规则）越过它，优先级 >= 0 的规则排在它之后。这与历史行为
	// 逐字节一致（历史上直通规则固定注入在此处、节点规则全在其后），同时让"想把某条规则
	// 排到直通之前"变得可表达。
	hasBT := false
	for _, rule := range rules {
		if rule.Enabled && rule.Protocol == "bittorrent" {
			hasBT = true
		}
	}
	btRule := map[string]any{
		"type":        "field",
		"protocol":    []string{"bittorrent"},
		"outboundTag": "blocked",
	}
	btEmitted := false

	// 私网阻断不再在路由层注入（2026-09-17）：路由规则自上而下首个命中生效，且该规则不带
	// inboundTag/outbound 维度限定，会抢在管理员「入站 → 其他出站」的规则之前把目标为私网的
	// 连接全部丢给 blocked——即使那条链路本来需要访问私网（且注入规则在 UI 中不可见，管理员
	// 无从排查）。私网拦截改由各 freedom 出站的 finalRules 承担：作用域限于真正走该出站的流量，
	// 且在最终 IP 解析后、拨号前匹配（目标根本不会被拨号），UDP 还逐包匹配，比路由层更严。
	// 详见 translateFreedomPanelKeys / models.DefaultFreedomDirectSettingsJSON。

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if !btEmitted && !hasBT && rule.Priority >= 0 {
			list = append(list, btRule)
			btEmitted = true
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
		if domains := ParseStringList(rule.Domain); len(domains) > 0 {
			rmap["domain"] = domains
		}
		if ips := ParseStringList(rule.IP); len(ips) > 0 {
			rmap["ip"] = ips
		}
		if rule.Port != "" {
			rmap["port"] = rule.Port
		}
		if rule.Network != "" {
			rmap["network"] = rule.Network
		}
		if protocols := ParseStringList(rule.Protocol); len(protocols) > 0 {
			rmap["protocol"] = protocols
		}
		if rule.InboundTag != "" {
			rmap["inboundTag"] = ParseStringList(rule.InboundTag)
		}
		list = append(list, rmap)
	}
	// 全部规则的优先级都为负（或有规则被禁用而没触发上面的插入点）：BT 规则落到末尾
	if !btEmitted && !hasBT {
		list = append(list, btRule)
	}
	return list
}

// buildInbound 服务端入站投影（P1）：从 InboundSpec（settings/stream/sniffing 一次解码）
// 组装 xray 入站。协议专属语义全部由协议插件的 NormalizeServer 提供，本函数不再认识具体协议；
// 动态注入 clients 列表（按入站二态分流——user 动态用户 / relay 内部 UUID）。
func buildInbound(inb *models.Inbound, usersByTag map[string][]protocol.User, ctx *GenerateContext) (map[string]any, error) {
	spec, err := contracts.DecodeInboundStrict(inb)
	if err != nil {
		return nil, fmt.Errorf("入站 %s %w", inb.Tag, err)
	}
	// 协议级规范化：强约束与派生默认（VLESS decryption、Hysteria 2 network/security/version …）。
	overrides, err := protocols.NormalizeServer(inb.Protocol, spec)
	if err != nil {
		return nil, fmt.Errorf("入站 %s %w", inb.Tag, err)
	}
	settings, stream := spec.Settings, spec.Stream
	if overrides != nil {
		if overrides.Settings != nil {
			settings = overrides.Settings
		}
		if len(overrides.Stream) > 0 {
			if stream == nil {
				stream = make(map[string]any, len(overrides.Stream))
			}
			for k, v := range overrides.Stream {
				stream[k] = v
			}
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}
	if stream != nil {
		stream = sanitizeStreamMap(stream)
		// 服务端入站不消费客户端校验证书/根池的字段，生成前剥离，避免脏配置透传。
		stripInboundClientOnlyTLS(stream)
		stripInboundClientOnlyReality(stream)
		// udphop 只存在于客户端实现；服务端带它会在监听阶段报 "udphop: client only"。
		stripInboundClientOnlyMasks(stream)
		// xmux / downloadSettings 等只被客户端 dialer 消费；入站 DB 保留给订阅，生成侧剥离。
		stripInboundClientOnlyXHTTP(stream)
	}

	// 入站认证用户/凭据动态装配
	switch {
	case inb.IsForwardProxy():
		// 四层直通 / 独立通道：没有用户维度，settings 原样透传（不注入 clients）。
	default:
		// 通用入站：多源动态汇聚（商业用户 + 连入出站的机间中继凭据）
		var aggregatedUsers []protocol.User

		// 1. 商业用户（由服务层 GetValidUsers 按接入点白名单派生过滤）
		if commercialUsers, ok := usersByTag[inb.Tag]; ok {
			aggregatedUsers = append(aggregatedUsers, commercialUsers...)
		}

		// 2. 机间中继凭据（来自所有连入该入站的前置 ServerOutbound）
		if ctx != nil && ctx.ReferencingOutbounds != nil {
			for _, refOut := range ctx.ReferencingOutbounds[inb.ID] {
				refUUID := refOut.InternalUUID
				if refUUID == "" && inb.InternalUUID != "" {
					refUUID = inb.InternalUUID
				}
				if refUUID != "" {
					aggregatedUsers = append(aggregatedUsers, protocol.User{
						UUID:  refUUID,
						Email: fmt.Sprintf("relay.in_%d.out_%d@panel.local", inb.ID, refOut.ID),
						Flow:  protocols.ResolveRelayFlow(inb.Protocol, spec, inb.Flow),
					})
				}
			}
		}

		// 兼容存量单测与遗留数据：若显式标为 relay
		if inb.Type == models.InboundTypeRelay {
			if inb.InternalUUID == "" {
				return nil, fmt.Errorf("relay 入站 %s 缺少内部 UUID（请先执行 setup-internal）", inb.Tag)
			}
			if len(aggregatedUsers) == 0 {
				aggregatedUsers = append(aggregatedUsers, protocol.User{
					UUID:  inb.InternalUUID,
					Email: RelayEmail(inb.Tag),
					Flow:  protocols.ResolveRelayFlow(inb.Protocol, spec, inb.Flow),
				})
			}
		}

		// 3. 严格零客户端物理跳过防御（杜绝 Xray -test 因 clients 为空报错）
		// 在真实编排上下文（ctx != nil）下执行跳过；在 ctx == nil 的底层单元测试中保留空列表以支持透传断言
		if ctx != nil && len(aggregatedUsers) == 0 {
			log.Printf("[xray-generate] 入站 %s (id=%d) 无有效认证用户或连入出站，物理跳过以防 Xray 启动报错", inb.Tag, inb.ID)
			return nil, nil
		}

		key, items, err := protocols.ServerUsers(inb.Protocol, aggregatedUsers, spec)
		if err != nil {
			return nil, fmt.Errorf("入站 %s 注入用户失败: %w", inb.Tag, err)
		}
		settings[key] = items
	}

	item := map[string]any{
		"tag":      spec.Tag,
		"protocol": spec.Protocol,
		"port":     spec.Port,
		"settings": settings,
	}
	if spec.Listen != "" && spec.Listen != "0.0.0.0" {
		item["listen"] = spec.Listen
	}
	if stream != nil {
		// 证书路径动态注入（Phase T）：当关联了 CertID 且为 TLS 入站时，注入托管证书路径并保留已有 CA 证书
		if spec.Security == "tls" {
			if domain, ok := certDomainFor(inb, ctx); ok {
				var tlsSettings map[string]any
				if ts, ok := stream["tlsSettings"].(map[string]any); ok && ts != nil {
					tlsSettings = ts
				} else {
					tlsSettings = make(map[string]any)
				}
				serverCert := map[string]any{
					"certificateFile": certFilePath(domain, "fullchain.pem"),
					"keyFile":         certFilePath(domain, "key.pem"),
				}
				// 绑定托管证书时以托管路径为准；usage=verify 项已在 sanitize 阶段剥离。
				tlsSettings["certificates"] = []any{serverCert}
				stream["tlsSettings"] = tlsSettings
			}
		}
		// PROXY Protocol 规范化：若开启了 acceptProxyProtocol，确保注入 tcpSettings，并清理顶层私有键
		if ap, _ := stream["acceptProxyProtocol"].(bool); ap {
			if tcpS, ok := stream["tcpSettings"].(map[string]any); ok {
				tcpS["acceptProxyProtocol"] = true
			} else {
				stream["tcpSettings"] = map[string]any{"acceptProxyProtocol": true}
			}
			delete(stream, "acceptProxyProtocol")
		}
		item["streamSettings"] = stream
	}
	// 解析 sniffing JSON（完全透传）
	if spec.Sniffing != nil {
		item["sniffing"] = spec.Sniffing
	}
	return item, nil
}

// TunnelOutboundTag 直通派生 freedom 出站的 tag（唯一命名来源，改名级联也用它）。
func TunnelOutboundTag(inboundTag string) string { return "out-" + inboundTag }

// TunnelTarget 解析直通入站的目标：连线目标（TargetInboundID → 目标服务器 Host + 目标入站端口）
// 优先，其次手动外部地址。ok=false = 草稿（既未连线也无有效手动目标）。
//
// 唯一实现：生成器的草稿判定与落库 reconcile 都用它，避免"什么算有目标"出现两份口径。
func TunnelTarget(inb *models.Inbound, ctx *GenerateContext) (host string, port int, ok bool) {
	if inb.TargetInboundID != nil && *inb.TargetInboundID > 0 {
		if target, found := ctxTarget(ctx, *inb.TargetInboundID); found {
			return target.ServerHost, target.Inbound.Port, target.ServerHost != "" && target.Inbound.Port > 0
		}
		return "", 0, false
	}
	if inb.TargetAddress != "" && inb.TargetPort > 0 {
		return inb.TargetAddress, inb.TargetPort, true
	}
	return "", 0, false
}

// TunnelSendProxyProtocol 直通出站是否向目标发送 PROXY Protocol v2（发送侧语义）：
// 沿用领域内的既有约定——**只有显式写 false 才关闭**，其余（缺省 / true）都开启。
// 私有键两种写法都认（proxy_protocol 为面板通道表单写的键，proxyProtocol 为手写配置）。
func TunnelSendProxyProtocol(inb *models.Inbound) bool {
	if inb.StreamSettings == "" {
		return true
	}
	var ss map[string]any
	if err := json.Unmarshal([]byte(inb.StreamSettings), &ss); err != nil {
		return true
	}
	for _, key := range []string{"proxy_protocol", "proxyProtocol"} {
		if v, present := ss[key].(bool); present && !v {
			return false
		}
	}
	return true
}

// TunnelOutboundSettings 直通派生 freedom 出站的 settings（目标由 redirect 承担）。
// C1 起目标住在出站，C2 起该字符串落库（server_outbounds.settings_json）。
func TunnelOutboundSettings(host string, port int, sendProxyProtocol bool) (string, error) {
	obSettings := map[string]any{"redirect": fmt.Sprintf("%s:%d", host, port)}
	if sendProxyProtocol {
		obSettings["proxyProtocol"] = 2
	}
	raw, err := json.Marshal(obSettings)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// buildTunnelInbound 生成四层直通管道入站（dokodemo-door，**只监听**）。
// 目标不写进入站：目标由该直通的 freedom 出站（tag = TunnelOutboundTag(tag)）的 `redirect` 承担
// （入站 = 哪里听、出站 = 转到哪，两者解耦；该出站自 C2 起是库内真行）。
// 若既无连线目标又无有效手动外部目标，返回 nil（草稿状态，跳过注入 Xray，防启动报错）。
func buildTunnelInbound(inb *models.Inbound, spec *contracts.InboundSpec, ctx *GenerateContext) (map[string]any, error) {
	if _, _, ok := TunnelTarget(inb, ctx); !ok {
		// 目标未配置或未连线，作为草稿状态跳过注入
		return nil, nil
	}

	network := "tcp,udp"
	wantAcceptPP := false
	// settings.network 是 xray dokodemo-door 的正式位置；stream.network 仅为兼容
	// 旧版面板把转发网络写进 streamSettings 的存量数据。
	if spec.Settings != nil {
		if n, _ := spec.Settings["network"].(string); n != "" {
			network = n
		}
	}
	if spec.Stream != nil {
		if n, _ := spec.Stream["network"].(string); n != "" && (spec.Settings == nil || spec.Settings["network"] == nil) {
			network = n
		}
		// 接收侧 PROXY Protocol（发送侧见 TunnelSendProxyProtocol）：
		// - ensureTargetAcceptProxyProtocol 写入 tcpSettings.acceptProxyProtocol（新形态）；
		// - 存量数据可能还带顶层 acceptProxyProtocol；
		// - 通道托管入站的「透传 PROXY Protocol」勾选写的是面板私有键 proxy_protocol。
		// 任一为 true → 生成的 dokodemo 带上 tcpSettings.acceptProxyProtocol，链式管道才能
		// 「上游 freedom 注入 → 本层消费 → freedom 重注入」逐跳透传；首跳管道（无上游指向）
		// 两键皆缺省，保持关闭——普通用户客户端不发 PROXY 头，开了会拒绝直连。
		if ap, _ := spec.Stream["acceptProxyProtocol"].(bool); ap {
			wantAcceptPP = true
		}
		if tcpS, ok := spec.Stream["tcpSettings"].(map[string]any); ok {
			if ap, _ := tcpS["acceptProxyProtocol"].(bool); ap {
				wantAcceptPP = true
			}
		}
		if pp, _ := spec.Stream["proxy_protocol"].(bool); pp {
			wantAcceptPP = true
		}
		if pp, _ := spec.Stream["proxyProtocol"].(bool); pp {
			wantAcceptPP = true
		}
	}

	settings := map[string]any{
		"network":        network,
		"followRedirect": false,
	}

	item := map[string]any{
		"tag":      spec.Tag,
		"protocol": "dokodemo-door",
		"port":     spec.Port,
		"settings": settings,
	}
	if wantAcceptPP {
		item["streamSettings"] = map[string]any{
			"tcpSettings": map[string]any{"acceptProxyProtocol": true},
		}
	}
	if spec.Listen != "" && spec.Listen != "0.0.0.0" {
		item["listen"] = spec.Listen
	}
	return item, nil
}
