// Package middleware 提供 JWT 认证、RBAC 与限流中间件。
package middleware

import (
	"bytes"
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
// 大文本字段（订阅模板/公告正文/PEM）落库时摘要化：只存预览 + 行数/字符数，
// 不再整段落库（旧版 500 字符硬截断既不可读也无法取证，两端都不讨好）。
const (
	auditTextThreshold = 256  // 字符串字段超过该长度（rune）视为大文本，落库摘要
	auditPreviewLen    = 200  // 大文本预览保留的字符数
	auditNestedMax     = 4096 // 嵌套 JSON 字符串（settings_json/stream_settings）尝试解析的长度上限
	auditArrayCap      = 20   // 数组字段落库条数上限
	auditDetailCap     = 2000 // detail 序列化后总长上限（超限降级为脱敏原文串）
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
	if len(s) > 500 {
		s = s[:500] + "..."
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

// auditSanitize 递归脱敏 + 大文本摘要化，返回可 JSON 序列化结构。
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
			t = t[:auditArrayCap]
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

// auditSanitizeString 对字符串值：嵌套 JSON 剥一层解析（供翻译层读协议/传输等子字段）；
// 超长文本摘要化（__text 标记：预览 + 行数 + 字符数）。
// 无法解析的超长 JSON 串不给预览——内部可能含 clients[].id / auth 等凭据，
// 而按 key 脱敏只在解析成功时生效。
func auditSanitizeString(s string, depth int) any {
	trimmed := strings.TrimSpace(s)
	jsonLike := len(trimmed) > 1 && (trimmed[0] == '{' || trimmed[0] == '[')
	// 最多剥三层字符串化 JSON（body → settings_json → 子配置），再深只做摘要，
	// 避免恶意深层嵌套带来的解析递归。
	if depth < 3 && jsonLike && len(s) <= auditNestedMax {
		var parsed any
		if json.Unmarshal([]byte(s), &parsed) == nil {
			return auditSanitize(parsed, depth+1, "")
		}
	}
	runes := []rune(s)
	if len(runes) <= auditTextThreshold {
		return s
	}
	if jsonLike {
		return map[string]any{
			"__text": map[string]any{
				"lines":         strings.Count(s, "\n") + 1,
				"chars":         len(runes),
				"redacted_blob": true,
			},
		}
	}
	return map[string]any{
		"__text": map[string]any{
			"preview": string(runes[:auditPreviewLen]),
			"lines":   strings.Count(s, "\n") + 1,
			"chars":   len(runes),
		},
	}
}

// auditTarget 注册表条目：目标实体模型 + 显示字段。
type auditTarget struct {
	model any
	field string
}

// auditTargets 目标名注册表：路由 FullPath → 目标实体。
// 只登记请求体里带不出目标上下文的操作（删除/启停/重置/指令等）：
// 中间件在 c.Next() 前按 :id 预读一次显示名（删除后记录即消失，必须提前读），
// 落进 envelope 的 target 字段；新增路由默认不登记，前端兜底只显示 #id。
// 业务代码不再手动打点（双记清理，2026-09-04 拍板方案 A）。
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
	"/api/v1/admin/servers/:id/command":                {model: &models.Server{}, field: "name"},
	"/api/v1/admin/servers/:id/reset-secret":           {model: &models.Server{}, field: "name"},
	"/api/v1/admin/servers/:id/generate-config":        {model: &models.Server{}, field: "name"},
	"/api/v1/admin/inbounds/:id":                       {model: &models.Inbound{}, field: "tag"},
	"/api/v1/admin/inbounds/:id/toggle":                {model: &models.Inbound{}, field: "tag"},
	"/api/v1/admin/inbounds/:id/setup-internal":        {model: &models.Inbound{}, field: "tag"},
	"/api/v1/admin/inbounds/:id/rotate-internal":       {model: &models.Inbound{}, field: "tag"},
	"/api/v1/admin/plans/:id":                          {model: &models.Plan{}, field: "name"},
	"/api/v1/admin/certs/:id":                          {model: &models.Cert{}, field: "domain"},
	"/api/v1/admin/permission-groups/:id":              {model: &models.PermissionGroup{}, field: "name"},
	"/api/v1/admin/permission-groups/:id/sub-template": {model: &models.PermissionGroup{}, field: "name"},
	"/api/v1/admin/sub-templates/:id":                  {model: &models.SubTemplate{}, field: "name"},
	"/api/v1/admin/access-points/:id":                  {model: &models.UserAccessPoint{}, field: "name"},
	"/api/v1/admin/invitations/:id":                    {model: &models.InvitationCode{}, field: "code"},
}

// loadAuditTarget 按注册表预读目标显示名（静默查询，不产生 GORM 日志噪音）。
func loadAuditTarget(db *gorm.DB, c *gin.Context) string {
	t, ok := auditTargets[c.FullPath()]
	if !ok || c.Param("id") == "" {
		return ""
	}
	var row map[string]any
	err := db.Session(&gorm.Session{Logger: logger.Discard}).
		Model(t.model).Select(t.field).Where("id = ?", c.Param("id")).Take(&row).Error
	if err != nil {
		return ""
	}
	s, _ := row[t.field].(string)
	return strings.TrimSpace(s)
}

// buildAuditDetail 生成结构化审计详情（v2）：
// {"v":2,"method","path"(真实 URL，含真实 ID),"params","status","target"(注册表预读显示名),"body"(脱敏+摘要)}。
// body 非 JSON 时降级为 body_raw（旧版正则脱敏原文）。
func buildAuditDetail(c *gin.Context, body []byte, status int, target string) string {
	env := map[string]any{
		"v":      2,
		"method": c.Request.Method,
		"path":   c.Request.URL.Path,
		"status": status,
	}
	if target != "" {
		env["target"] = target
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
	out, err := json.Marshal(env)
	if err != nil {
		return redactBody(body)
	}
	if len(out) <= auditDetailCap {
		return string(out)
	}
	// 超限降级：只保留 envelope 骨架 + body 顶层字段名，绝不回退原始 body。
	// 原始 body 里的嵌套 JSON 是转义形态（\"id\":），按 key 的正则脱敏匹配不到，
	// 回退原文会重新把凭据写进审计（2026-10-28 审查 P2-2）。
	delete(env, "body_raw")
	if b, ok := env["body"].(map[string]any); ok {
		keys := make([]string, 0, len(b))
		for k := range b {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		env["body"] = map[string]any{"__truncated": true, "keys": keys}
	} else {
		delete(env, "body")
	}
	if out2, err2 := json.Marshal(env); err2 == nil {
		return string(out2)
	}
	return ""
}

// Audit 自动审计中间件（J16，Xboard RequestLog 模式）：
// admin 组写操作（POST/PUT/DELETE）自动落 AuditLog——action 由路径推导、
// detail 为 v2 结构化信封（真实路径+参数+脱敏摘要 body+注册表目标名）。
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

		// 注册表命中：执行前预读目标显示名（删除类操作之后记录即不存在）
		target := loadAuditTarget(db, c)

		c.Next()

		uid := CurrentUser(c)
		action := strings.TrimPrefix(c.FullPath(), "/api/v1/admin/")
		action = strings.ReplaceAll(action, "/", ".")
		if action == "" {
			action = method
		}
		_ = db.Create(&models.AuditLog{
			OperatorType: "admin",
			OperatorID:   uid,
			Action:       action,
			Category:     models.AuditCategory(action),
			Detail:       buildAuditDetail(c, body, c.Writer.Status(), target),
			IP:           util.ClientIPFromContext(c),
		})
	}
}
