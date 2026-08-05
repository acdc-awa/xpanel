// Package middleware 提供 JWT 认证、RBAC 与限流中间件。
package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/services"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

const CtxClaimsKey = "claims"

// AuthRequired 解析 Bearer access token 并注入 claims。
func AuthRequired(jwtMgr *services.JWTManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, err := c.Cookie("access_token")
		if err != nil || tokenStr == "" {
			util.Unauthorized(c, "缺少访问令牌")
			c.Abort()
			return
		}
		claims, err := jwtMgr.Parse(tokenStr)
		if err != nil || claims.Type != services.TokenAccess {
			util.Unauthorized(c, "访问令牌无效或已过期")
			c.Abort()
			return
		}
		c.Set(CtxClaimsKey, claims)
		c.Next()
	}
}

// RequireRole 限制角色访问（须在 AuthRequired 之后使用）。
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		claims, ok := c.Get(CtxClaimsKey)
		if !ok {
			util.Unauthorized(c, "未认证")
			c.Abort()
			return
		}
		cl := claims.(*services.Claims)
		if !allowed[cl.Role] {
			util.Forbidden(c, "无权限访问")
			c.Abort()
			return
		}
		c.Next()
	}
}

// CurrentUser 取当前用户 ID（须在 AuthRequired 之后）。
func CurrentUser(c *gin.Context) uint64 {
	if v, ok := c.Get(CtxClaimsKey); ok {
		return v.(*services.Claims).UserID
	}
	return 0
}

// CurrentRole 取当前角色。
func CurrentRole(c *gin.Context) string {
	if v, ok := c.Get(CtxClaimsKey); ok {
		return v.(*services.Claims).Role
	}
	return ""
}
