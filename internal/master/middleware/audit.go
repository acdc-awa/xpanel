// Package middleware 提供 JWT 认证、RBAC 与限流中间件。
package middleware

import (
	"bytes"
	"io"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/models"
)

// sensitiveKeys 审计脱敏字段（Xboard RequestLog 同款：password/token/secret/key/code）。
var sensitiveKeys = []string{"password", "token", "secret", "key", "code", "turnstile_token"}

// redactBody 对请求体 JSON 做敏感字段脱敏 + 限长（保留审计可读性，不泄漏凭据）。
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

// Audit 自动审计中间件（J16，Xboard RequestLog 模式）：
// admin 组写操作（POST/PUT/DELETE）自动落 AuditLog——action 由路径推导、detail 为脱敏 body 摘要。
// 管理端手动打点已移除（避免双记），统一走本中间件。
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

		c.Next()

		uid := CurrentUser(c)
		action := strings.TrimPrefix(c.FullPath(), "/api/v1/admin/")
		action = strings.ReplaceAll(action, "/", ".")
		if action == "" {
			action = method
		}
		detail := redactBody(body)
		if detail == "" {
			detail = method + " " + c.FullPath() + " → " + strconv.Itoa(c.Writer.Status())
		}
		_ = db.Create(&models.AuditLog{
			OperatorType: "admin",
			OperatorID:   uid,
			Action:       action,
			Detail:       detail,
			IP:           c.ClientIP(),
		})
	}
}
