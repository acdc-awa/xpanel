// Package middleware 提供 JWT 认证、RBAC 与限流中间件。
package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// 审计 detail 结构化（v2 envelope）尺寸参数。
//
// v5（2026-11-01）把口径从「省空间」改成「可取证」：真实库全保真也只有 2.13 MB / 47 天
// （其中 1.38 MB 是 settings 里的 base64 logo，已改为 sha256 摘要），而旧的 2 KB 上限 +
// 200 字预览把 137 行正文（权限组 63、订阅模板 55、settings 8）变成了无法检索的摘要。
// 现在：文本原样保留、二进制转指纹、超限时按「最长的字段先省略」并留下省略标记。
const (
	auditNestedMax    = 32 * 1024 // 嵌套 JSON 字符串尝试解析的长度上限（超过只留省略标记，不落原文）
	auditArrayCap     = 200       // 数组字段落库条数上限
	auditDetailCap    = 32 * 1024 // detail 序列化后总长上限
	auditBinaryMinLen = 1024      // 判定 base64 的最小长度（logo/证书等）
	auditOmitMaxCount = 64        // 单条最多省略多少个字段（防止极端情况下反复序列化）
)

// sensitiveContains 包含匹配即脱敏（覆盖 turnstile_secret 等组合键名）；
// privatekey 覆盖 REALITY 驼峰 privateKey（入站编辑器经 stream_settings 提交，
// 下划线形态 private_key 由 sensitiveExact 兜底）；
// pass/auth 是 Hysteria（users[].auth）与 HTTP/SOCKS（accounts[].pass）的用户凭据；
// uuid 对应用户连接凭据；sensitiveExact 精确匹配脱敏（key/code 单列，
// 避免 sub_deny_code 之类误伤——keyword/keys 同理）。
var (
	sensitiveContains = []string{"password", "passwd", "secret", "token", "privatekey", "passphrase"}
	sensitiveExact    = map[string]bool{
		"key": true, "code": true, "key_pem": true, "private_key": true,
		"pass": true, "auth": true, "uuid": true,
	}
)

// sensitiveKeys 旧版正则脱敏字段（body_raw 回退路径沿用；无路径上下文，故不收 id）。
var sensitiveKeys = []string{
	"password", "passwd", "token", "secret", "key", "code", "turnstile_token",
	"pass", "auth", "uuid", "ca_key_pem", "private_key", "key_pem",
}

// isSensitiveKey 判断字段是否应脱敏。parent 是「包含该字段的对象所属的键名」，
// 用于路径感知规则（仅 clients[].id 这类上下文才把 id 当凭据）。
func isSensitiveKey(k, parent string) bool {
	lk := strings.ToLower(k)
	if sensitiveExact[lk] {
		return true
	}
	for _, w := range sensitiveContains {
		if strings.Contains(lk, w) {
			return true
		}
	}
	// 任意 *_key / *key（含 privateKey、api_key、public_key）：统一按密钥处理。
	// 宁可过度脱敏，也不让私钥片段进入审计（审计会被导出/备份/投递）。
	if strings.HasSuffix(lk, "key") {
		return true
	}
	// 私钥 PEM：键名同时含 key 与 pem（ca_key_pem / tls_key_pem）。
	// 证书本身（ca_pem / cert_pem）不含 key，保持可读。
	if strings.Contains(lk, "key") && strings.Contains(lk, "pem") {
		return true
	}
	// VLESS 客户端凭据：clients[].id 是用户 UUID。其它 id 是业务主键，脱敏会毁掉审计可读性。
	if lk == "id" && strings.EqualFold(parent, "clients") {
		return true
	}
	return false
}

// redactBody 对请求体做敏感字段脱敏 + 限长（仅用于 body 非 JSON 的回退路径）。
func redactBody(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	s := string(body)
	for _, k := range sensitiveKeys {
		s = redactKey(s, k)
	}
	if len(s) > 4096 {
		s = s[:4096] + "..."
	}
	return s
}

// redactKey 将 "key":"value" 形式（兼容空格）的敏感值替换为 "***"。
func redactKey(s, key string) string {
	patterns := []string{`"` + key + `":`, `"` + key + `" :`}
	for _, p := range patterns {
		searchFrom := 0
		for {
			idx := strings.Index(s[searchFrom:], p)
			if idx < 0 {
				break
			}
			idx += searchFrom
			rest := s[idx+len(p):]
			end := strings.IndexAny(rest, ",}")
			if end < 0 {
				s = s[:idx+len(p)] + `"***"`
				break
			}
			s = s[:idx+len(p)] + `"***"` + rest[end:]
			searchFrom = idx + len(p) + 6
		}
	}
	return s
}

// auditSanitize 递归脱敏 + 二进制摘要化，返回可 JSON 序列化结构。
// parent 是「包含当前映射的字段名」，仅用于路径感知规则（clients[].id）。
func auditSanitize(v any, depth int, parent string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if isSensitiveKey(k, parent) {
				out[k] = "***"
				continue
			}
			out[k] = auditSanitize(val, depth, k)
		}
		return out
	case []any:
		if len(t) > auditArrayCap {
			out := make([]any, auditArrayCap+1)
			for i := 0; i < auditArrayCap; i++ {
				out[i] = auditSanitize(t[i], depth, parent)
			}
			// 尾部留一条显式标记：既说明「还有多少项」，也不让条数静默丢失。
			out[auditArrayCap] = map[string]any{"__omitted": map[string]any{"items": len(t) - auditArrayCap}}
			return out
		}
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = auditSanitize(val, depth, parent)
		}
		return out
	case string:
		return auditSanitizeString(t, depth)
	default:
		return v
	}
}

// auditSanitizeString 对字符串值处理三步：
//  1. 二进制/base64（data: URI、长 base64）→ sha256 指纹，既压掉体积又保留「内容是否被改过」；
//  2. 字符串化 JSON 剥一层解析（供翻译层读协议/传输等子字段）；
//  3. 其余原样保留（v5 起不再摘要化文本，正文要可检索、可取证）。
func auditSanitizeString(s string, depth int) any {
	if digest, ok := auditBinaryDigest(s); ok {
		return digest
	}
	trimmed := strings.TrimSpace(s)
	jsonLike := len(trimmed) > 1 && (trimmed[0] == '{' || trimmed[0] == '[')
	// 最多剥三层字符串化 JSON（body → settings_json → 子配置），再深只做省略标记，
	// 避免恶意深层嵌套带来的解析递归。
	if depth < 3 && jsonLike && len(s) <= auditNestedMax {
		var parsed any
		if json.Unmarshal([]byte(s), &parsed) == nil {
			return auditSanitize(parsed, depth+1, "")
		}
	}
	// 超长 JSON 串不解析也不落原文：内部可能含 clients[].id / auth 等凭据，
	// 而按 key 脱敏只在解析成功时生效。
	if jsonLike && len(s) > auditNestedMax {
		return map[string]any{"__omitted": map[string]any{"chars": len(s), "json": true}}
	}
	return s
}

// auditBinaryDigest 识别 data: URI 与长 base64 串，返回 {"__binary":{sha256,bytes,type}}。
// type 只对 data: URI 有意义（其它留空）；bytes 是原始负载字节数（base64 解码前的长度）。
func auditBinaryDigest(s string) (any, bool) {
	if strings.HasPrefix(s, "data:") {
		if i := strings.Index(s, ";base64,"); i > 0 {
			payload := s[i+len(";base64,"):]
			return map[string]any{"__binary": map[string]any{
				"sha256": sha256Hex(payload),
				"bytes":  base64DecodedLen(payload),
				"type":   strings.TrimPrefix(s[:i], "data:"),
			}}, true
		}
		return nil, false
	}
	if len(s) >= auditBinaryMinLen && isBase64Like(s) {
		return map[string]any{"__binary": map[string]any{
			"sha256": sha256Hex(s),
			"bytes":  base64DecodedLen(s),
			"type":   "",
		}}, true
	}
	return nil, false
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// base64DecodedLen 估算 base64 字符串解码后的字节数（含换行/空白的标准与 URL-safe 变体）。
func base64DecodedLen(s string) int {
	n := 0
	padding := 0
	for _, r := range s {
		switch r {
		case '\r', '\n', ' ', '\t':
			continue
		case '=':
			padding++
		}
		n++
	}
	if n == 0 {
		return 0
	}
	return n/4*3 - padding
}

// isBase64Like 判定「整串只由 base64 字符组成」（允许换行/空白），用于识别未加 data: 前缀的
// 图片/密钥材料。必须是全量字符判定而不是比例判定：一段长 JSON 或长英文正文里字母数字占 95%
// 以上很常见，按比例会把它误判成二进制而只留一个摘要，正文就再也搜不到了。
func isBase64Like(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '+', r == '/', r == '=', r == '-', r == '_', r == 32, r == 9, r == 10, r == 13:
		default:
			return false
		}
	}
	return true
}

// auditTarget 注册表条目：目标实体模型 + 显示字段。
type auditTarget struct {
	model any
	field string
}

// auditTargets 目标名注册表：路由 FullPath → 目标实体。
// 中间件在 c.Next() 前按 :id 预读一次显示名（删除后记录即消失，必须提前读）。
// 未登记但有 :id 的路由仍会写入 target_type/target_id（类型由路径第一段推导），
// 只是没有显示名；创建类路由（无 :id）从 body 的 tag/name/email/... 推断显示名。
var auditTargets = map[string]auditTarget{
	"/api/v1/admin/notices/:id":                        {model: &models.Notice{}, field: "title"},
	"/api/v1/admin/notices/:id/toggle":                 {model: &models.Notice{}, field: "title"},
	"/api/v1/admin/users/:id":                          {model: &models.User{}, field: "email"},
	"/api/v1/admin/users/:id/toggle":                   {model: &models.User{}, field: "email"},
	"/api/v1/admin/users/:id/2fa/disable":              {model: &models.User{}, field: "email"},
	"/api/v1/admin/users/:id/reset-traffic":            {model: &models.User{}, field: "email"},
	"/api/v1/admin/users/:id/balance":                  {model: &models.User{}, field: "email"},
	"/api/v1/admin/users/:id/subscribe-token/reset":    {model: &models.User{}, field: "email"},
	"/api/v1/admin/servers/:id":                        {model: &models.Server{}, field: "name"},
	"/api/v1/admin/servers/:id/reset-secret":           {model: &models.Server{}, field: "name"},
	"/api/v1/admin/servers/:id/generate-config":        {model: &models.Server{}, field: "name"},
	"/api/v1/admin/inbounds/:id":                       {model: &models.Inbound{}, field: "tag"},
	"/api/v1/admin/inbounds/:id/toggle":                {model: &models.Inbound{}, field: "tag"},
	"/api/v1/admin/inbounds/:id/setup-internal":        {model: &models.Inbound{}, field: "tag"},
	"/api/v1/admin/inbounds/:id/rotate-internal":       {model: &models.Inbound{}, field: "tag"},
	"/api/v1/admin/plans/:id":                          {model: &models.Plan{}, field: "name"},
	"/api/v1/admin/plans/:id/restore":                  {model: &models.Plan{}, field: "name"},
	"/api/v1/admin/certs/:id":                          {model: &models.Cert{}, field: "domain"},
	"/api/v1/admin/certs/:id/reissue":                  {model: &models.Cert{}, field: "domain"},
	"/api/v1/admin/ca-certs/:id":                       {model: &models.CACert{}, field: "name"},
	"/api/v1/admin/ca-certs/:id/issue":                 {model: &models.CACert{}, field: "name"},
	"/api/v1/admin/gift-cards/:id":                     {model: &models.GiftCard{}, field: "name"},
	"/api/v1/admin/gift-cards/:id/purge":               {model: &models.GiftCard{}, field: "name"},
	"/api/v1/admin/permission-groups/:id":              {model: &models.PermissionGroup{}, field: "name"},
	"/api/v1/admin/permission-groups/:id/sub-template": {model: &models.PermissionGroup{}, field: "name"},
	"/api/v1/admin/permission-groups/:id/access-points": {
		model: &models.PermissionGroup{}, field: "name",
	},
	"/api/v1/admin/permission-groups/:id/preview-template": {model: &models.PermissionGroup{}, field: "name"},
	"/api/v1/admin/sub-templates/:id":                      {model: &models.SubTemplate{}, field: "name"},
	"/api/v1/admin/access-points/:id":                      {model: &models.UserAccessPoint{}, field: "name"},
	"/api/v1/admin/access-points/:id/target":               {model: &models.UserAccessPoint{}, field: "name"},
	"/api/v1/admin/invitations/:id":                        {model: &models.InvitationCode{}, field: "code"},
}

// auditTargetsByType 由路由注册表反推「目标类型 → 实体模型 + 名称列」，
// 供创建类操作（路径里没有 :id）成功后回查主键。
var auditTargetsByType = func() map[string]auditTarget {
	m := make(map[string]auditTarget, len(auditTargets))
	for path, t := range auditTargets {
		typ := models.AuditTargetTypeFromPath(path)
		if typ == "" {
			continue
		}
		if _, ok := m[typ]; !ok {
			m[typ] = t
		}
	}
	return m
}()

// auditTargetSnapshot 目标对象的类型/主键/显示名快照。
type auditTargetSnapshot struct {
	Type string
	ID   uint64
	Name string
}

// loadAuditTarget 执行前预读目标快照。
//   - 有 :id 且登记过显示字段 → 查库取名字（删除类操作之后记录即不存在，必须提前读）；
//   - 有 :id 未登记 → 只有类型与主键；
//   - 无 :id（创建类）→ 类型来自路径，显示名从 body 推断。
func loadAuditTarget(db *gorm.DB, c *gin.Context, body []byte) auditTargetSnapshot {
	snap := auditTargetSnapshot{Type: models.AuditTargetTypeFromPath(c.FullPath())}
	if snap.Type == "" {
		snap.Type = models.AuditTargetTypeFromPath(c.Request.URL.Path)
	}
	if raw := strings.TrimSpace(c.Param("id")); raw != "" {
		if id, err := parseUintParam(raw); err == nil {
			snap.ID = id
		}
	}
	if t, ok := auditTargets[c.FullPath()]; ok && c.Param("id") != "" {
		var row map[string]any
		err := db.Session(&gorm.Session{Logger: logger.Discard}).
			Model(t.model).Select(t.field).Where("id = ?", c.Param("id")).Take(&row).Error
		if err == nil {
			if s, ok := row[t.field].(string); ok {
				snap.Name = strings.TrimSpace(s)
			}
		}
	}
	if snap.Name == "" && snap.ID == 0 {
		snap.Name = auditNameFromBody(body)
	}
	return snap
}

// fillCreatedTargetID 创建类操作（路径里没有 :id）成功后按显示名回查主键。
//
// 新建行的主键只出现在响应体里，而本中间件不缓冲响应，所以用「类型 → 名称列」反查。
// 名称不唯一（同名套餐/服务器/公告）时不猜：宁可留空，也不把这条日志挂到别人的主键上。
func fillCreatedTargetID(db *gorm.DB, snap auditTargetSnapshot, status int) uint64 {
	if snap.ID != 0 || snap.Name == "" || status < 200 || status >= 300 {
		return snap.ID
	}
	t, ok := auditTargetsByType[snap.Type]
	if !ok {
		return 0
	}
	var ids []uint64
	err := db.Session(&gorm.Session{Logger: logger.Discard}).
		Model(t.model).Where(t.field+" = ?", snap.Name).
		Order("id DESC").Limit(2).Pluck("id", &ids).Error
	if err != nil || len(ids) != 1 {
		return 0
	}
	return ids[0]
}

// auditNameFromBody 创建类操作从请求体推断目标显示名（用户=email、服务器/入站/套餐=name/tag…）。
func auditNameFromBody(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(trimmed, &m) != nil {
		return ""
	}
	for _, k := range []string{"email", "name", "tag", "title", "domain", "common_name"} {
		if v, ok := m[k].(string); ok {
			if s := strings.TrimSpace(v); s != "" && !isSensitiveKey(k, "") {
				return s
			}
		}
	}
	return ""
}

// parseUintParam 解析路径里的十进制主键。
func parseUintParam(s string) (uint64, error) {
	var n uint64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errInvalidParam
		}
		n = n*10 + uint64(r-'0')
	}
	return n, nil
}

// errInvalidParam 非数字路径参数（如 notifications/:key）视为「无目标主键」。
var errInvalidParam = &paramError{}

type paramError struct{}

func (*paramError) Error() string { return "invalid param" }

// operatorName 操作人显示名快照（users.email）。查询静默且只在写请求上发生一次；
// 查不到（system / 用户已硬删 / 测试库无 users 表）留空。
func operatorName(db *gorm.DB, uid uint64) string {
	if uid == 0 {
		return ""
	}
	var user struct{ Email string }
	err := db.Session(&gorm.Session{Logger: logger.Discard}).
		Model(&models.User{}).Select("email").Where("id = ?", uid).Take(&user).Error
	if err != nil {
		return ""
	}
	return strings.TrimSpace(user.Email)
}

// buildAuditDetail 生成结构化审计详情（v2）：
// {"v":2,"method","path"(真实 URL，含真实 ID),"params","status","body"(脱敏)}。
// 目标对象的类型/主键/显示名不再重复写进 detail，它们已经是 audit_logs 的独立列。
// body 非 JSON 时降级为 body_raw（旧版正则脱敏原文）。
func buildAuditDetail(c *gin.Context, body []byte, status int) string {
	env := map[string]any{
		"v":      2,
		"method": c.Request.Method,
		"path":   c.Request.URL.Path,
		"status": status,
	}
	if len(c.Params) > 0 {
		params := make(map[string]string, len(c.Params))
		for _, p := range c.Params {
			params[p.Key] = p.Value
		}
		env["params"] = params
	}
	if trimmed := bytes.TrimSpace(body); len(trimmed) > 0 {
		var parsed any
		if json.Unmarshal(trimmed, &parsed) == nil {
			env["body"] = auditSanitize(parsed, 0, "")
		} else {
			env["body_raw"] = redactBody(trimmed)
		}
	}
	return marshalAuditEnvelope(env)
}

// marshalAuditEnvelope 序列化并按需裁剪到 auditDetailCap。
//
// 裁剪策略（v5）：按字符串值长度降序，把最长的逐个替换成 {"__omitted":{"chars":N}}，
// 直到压进上限，并记录 truncated_fields。绝不再走「整份 body 只剩字段名」的旧路径——
// 那样连没超限的小字段也一起丢了，反而最难取证。
func marshalAuditEnvelope(env map[string]any) string {
	out, err := json.Marshal(env)
	if err != nil {
		return "{}"
	}
	// 预留标记的空间，避免加了标记又超限
	const reserve = 512
	if len(out) <= auditDetailCap-reserve {
		return string(out)
	}

	type ref struct {
		path []any
		size int
	}
	var refs []ref
	var collect func(v any, path []any)
	collect = func(v any, path []any) {
		switch t := v.(type) {
		case map[string]any:
			for k, val := range t {
				child := append(append([]any{}, path...), k)
				collect(val, child)
			}
		case []any:
			for i, val := range t {
				child := append(append([]any{}, path...), i)
				collect(val, child)
			}
		case string:
			refs = append(refs, ref{path: append([]any{}, path...), size: len(t)})
		}
	}
	collect(env, nil)
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].size > refs[j].size })

	var omitted []string
	for i, r := range refs {
		if i >= auditOmitMaxCount {
			break
		}
		if r.size < 64 {
			break // 剩下的都是小字段，省略它们既无用也不可读
		}
		setAuditPath(env, r.path, map[string]any{"__omitted": map[string]any{"chars": r.size}})
		if p := auditPathString(r.path); p != "" {
			omitted = append(omitted, p)
		}
		if out, err = json.Marshal(env); err == nil && len(out) <= auditDetailCap-reserve {
			env["__truncated"] = true
			env["truncated_fields"] = omitted
			if out2, err2 := json.Marshal(env); err2 == nil {
				return string(out2)
			}
			return string(out)
		}
	}
	// 兜底：只剩信封骨架（不含脱敏原文，避免嵌套转义绕过按 key 脱敏）
	skeleton := map[string]any{"v": 2, "method": env["method"], "path": env["path"], "status": env["status"]}
	if p, ok := env["params"]; ok {
		skeleton["params"] = p
	}
	skeleton["__truncated"] = true
	if b, ok := env["body"].(map[string]any); ok {
		keys := make([]string, 0, len(b))
		for k := range b {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		skeleton["body"] = map[string]any{"__truncated": true, "keys": keys}
	}
	if out, err := json.Marshal(skeleton); err == nil {
		return string(out)
	}
	return "{}"
}

// setAuditPath 按收集到的路径写入替换值。
func setAuditPath(root map[string]any, path []any, val any) {
	var cur any = root
	for i, seg := range path {
		last := i == len(path)-1
		switch node := cur.(type) {
		case map[string]any:
			k, ok := seg.(string)
			if !ok {
				return
			}
			if last {
				node[k] = val
				return
			}
			cur = node[k]
		case []any:
			idx, ok := seg.(int)
			if !ok || idx < 0 || idx >= len(node) {
				return
			}
			if last {
				node[idx] = val
				return
			}
			cur = node[idx]
		default:
			return
		}
	}
}

func auditPathString(path []any) string {
	var b strings.Builder
	for _, seg := range path {
		switch t := seg.(type) {
		case string:
			if b.Len() > 0 {
				b.WriteByte('.')
			}
			b.WriteString(t)
		case int:
			b.WriteString("[")
			b.WriteString(itoa(t))
			b.WriteString("]")
		}
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Audit 自动审计中间件（J16，Xboard RequestLog 模式）：
// admin 组写操作（POST/PUT/DELETE）自动落 AuditLog——action 由路径推导、
// detail 为 v2 结构化信封（真实路径+参数+脱敏 body），并把身份/目标/结果写成独立列：
// operator_name（写入时快照）、target_type/target_id/target_name、status。
//
// 审计唯一入口：业务代码不手动打点（2026-09-04 双记清理拍板，方案 A）。
func Audit(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		if method == "GET" || method == "OPTIONS" || method == "HEAD" {
			c.Next()
			return
		}
		// 读取并还原 body（供 handler 继续绑定）
		body, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))

		// 执行前预读目标（删除类操作之后记录即不存在）
		target := loadAuditTarget(db, c, body)

		c.Next()

		uid := CurrentUser(c)
		action := strings.TrimPrefix(c.FullPath(), "/api/v1/admin/")
		action = strings.ReplaceAll(action, "/", ".")
		if action == "" {
			action = method
		}
		status := c.Writer.Status()
		target.ID = fillCreatedTargetID(db, target, status)
		_ = db.Create(&models.AuditLog{
			OperatorType: "admin",
			OperatorID:   uid,
			OperatorName: operatorName(db, uid),
			Action:       action,
			Category:     models.AuditCategory(action),
			TargetType:   target.Type,
			TargetID:     target.ID,
			TargetName:   target.Name,
			Status:       status,
			Detail:       buildAuditDetail(c, body, status),
			IP:           util.ClientIPFromContext(c),
		})
	}
}
