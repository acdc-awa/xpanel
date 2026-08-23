// Package middleware 提供 JWT 认证、RBAC 与限流中间件。
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/master/services"
	"github.com/acdc/xray-panel/internal/models"
	"github.com/acdc/xray-panel/internal/pkg/util"
)

const CtxClaimsKey = "claims"

// AuthRequired 解析 Cookie access token、拒绝 2FA pending token，并按 DB 实时校验
// 用户存在、账号状态、token_version 与角色（ISSUE-03：封禁/改密/角色变更立即吊销旧 access）。
func AuthRequired(jwtMgr *services.JWTManager, db *gorm.DB) gin.HandlerFunc {
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
		// ISSUE-02：2FA pending token 只允许访问 /auth/2fa/verify（由 AuthPending2FA 放行），
		// 其余受保护接口一律拒绝。
		if claims.Pending {
			util.Unauthorized(c, "请先完成两步验证")
			c.Abort()
			return
		}

		var user models.User
		if err := db.First(&user, claims.UserID).Error; err != nil {
			util.Unauthorized(c, "用户不存在或已被删除")
			c.Abort()
			return
		}
		if user.Status != models.StatusActive {
			util.Unauthorized(c, "账号已被禁用")
			c.Abort()
			return
		}
		if user.TokenVersion != claims.Version {
			util.Unauthorized(c, "会话已失效，请重新登录")
			c.Abort()
			return
		}
		if user.Role != claims.Role {
			util.Unauthorized(c, "权限已变更，请重新登录")
			c.Abort()
			return
		}
		if user.TotpEnabled && !claims.TwoFA {
			util.Unauthorized(c, "请先完成两步验证")
			c.Abort()
			return
		}

		// 以 DB 实时值为准回写 claims，避免旧 token 中的过期角色/版本继续被下游消费。
		claims.Role = user.Role
		claims.Version = user.TokenVersion
		c.Set(CtxClaimsKey, claims)
		c.Next()
	}
}

// AuthPending2FA 解析并放行 2FA pending access（仅 /auth/2fa/verify 路由使用）。
// 同样按 DB 校验用户状态与 token_version，防止封禁/改密后的 pending token 换发完整令牌。
func AuthPending2FA(jwtMgr *services.JWTManager, db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenStr, err := c.Cookie("access_token")
		if err != nil || tokenStr == "" {
			util.Unauthorized(c, "缺少访问令牌")
			c.Abort()
			return
		}
		claims, err := jwtMgr.Parse(tokenStr)
		if err != nil || claims.Type != services.TokenAccess || !claims.Pending {
			util.Unauthorized(c, "请先完成密码登录")
			c.Abort()
			return
		}
		var user models.User
		if err := db.First(&user, claims.UserID).Error; err != nil {
			util.Unauthorized(c, "用户不存在或已被删除")
			c.Abort()
			return
		}
		if user.Status != models.StatusActive {
			util.Unauthorized(c, "账号已被禁用")
			c.Abort()
			return
		}
		if user.TokenVersion != claims.Version {
			util.Unauthorized(c, "会话已失效，请重新登录")
			c.Abort()
			return
		}
		if !user.TotpEnabled {
			util.BadRequest(c, "未开启两步验证")
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

// CurrentClaims 取当前 JWT claims（须在 AuthRequired 之后；未认证返回 nil）。
func CurrentClaims(c *gin.Context) *services.Claims {
	if v, ok := c.Get(CtxClaimsKey); ok {
		return v.(*services.Claims)
	}
	return nil
}

// CurrentRole 取当前角色。
func CurrentRole(c *gin.Context) string {
	if v, ok := c.Get(CtxClaimsKey); ok {
		return v.(*services.Claims).Role
	}
	return ""
}

// RequirePwdChanged 强制改密拦截（J8）：must_change_pwd=true 时仅放行改密/登出，
// 其余接口返回 403 引导先改密。须在 AuthRequired 之后挂载。
func RequirePwdChanged(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.FullPath() {
		case "/api/v1/user/password", "/api/v1/auth/logout", "/api/v1/user/me":
			c.Next()
			return
		}
		uid := CurrentUser(c)
		var u models.User
		if err := db.First(&u, uid).Error; err == nil && u.MustChangePwd {
			util.Fail(c, http.StatusForbidden, "首次登录请先修改初始密码")
			c.Abort()
			return
		}
		c.Next()
	}
}
