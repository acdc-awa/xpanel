package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/middleware"
)

// NewRouter 组装全部路由。
func (d *Deps) NewRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")
	{
		// 认证（登录/注册限流 5 次/分钟）
		auth := v1.Group("/auth", middleware.RateLimit(5, time.Minute))
		{
			auth.POST("/register", d.Register)
			auth.POST("/login", d.Login)
			auth.POST("/refresh", d.Refresh)
		}

		// 用户端
		user := v1.Group("/user", middleware.AuthRequired(d.JWT))
		{
			user.GET("/me", d.Me)
		}

		// 管理端（需 admin 角色）
		admin := v1.Group("/admin",
			middleware.AuthRequired(d.JWT),
			middleware.RequireRole("admin"),
		)
		{
			admin.GET("/dashboard", d.AdminDashboard)
			admin.GET("/users", d.AdminUsers)
			admin.GET("/invitations", d.AdminInvitations)
			admin.POST("/invitations", d.AdminCreateInvitations)
		}
	}
	return r
}