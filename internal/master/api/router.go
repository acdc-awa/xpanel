package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/middleware"
	"github.com/zhx/xray-panel/internal/pkg/util"
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

		// 节点 WebSocket 网关（节点 Agent 连接）
		v1.GET("/node/ws", d.Hub.ServeWS)

		// 订阅（公开，token 鉴权）
		v1.GET("/sub/:token", d.Subscribe)

		// 用户端
		user := v1.Group("/user", middleware.AuthRequired(d.JWT))
		{
			user.GET("/me", d.Me)
			user.POST("/orders", d.UserCreateOrder)
			user.GET("/orders", d.UserOrders)
			user.POST("/password", d.UserChangePassword)
			user.PUT("/profile", d.UserUpdateProfile)
		}

		// 公开：上架套餐
		v1.GET("/plans", d.PublicPlans)

		// 节点 Agent 二进制下载（部署用；Docker 镜像内置 /app/agent）
		v1.GET("/download/agent", func(c *gin.Context) {
			p := os.Getenv("AGENT_BIN_PATH")
			if p == "" {
				p = "/app/agent"
			}
			if _, err := os.Stat(p); err != nil {
				util.Fail(c, http.StatusNotFound, "agent 二进制未内置（开发环境请用 --agent-file 或本地编译）")
				return
			}
			c.FileAttachment(p, "xray-agent")
		})

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
			admin.GET("/servers", d.AdminServers)
			admin.POST("/servers", d.AdminCreateServer)
			admin.DELETE("/servers/:id", d.AdminDeleteServer)
			admin.POST("/servers/:id/command", d.AdminServerCommand)
			admin.POST("/servers/:id/generate-config", d.AdminGenerateConfig)
			admin.GET("/inbounds", d.AdminInbounds)
			admin.POST("/inbounds", d.AdminCreateInbound)
			admin.PUT("/inbounds/:id", d.AdminUpdateInbound)
			admin.DELETE("/inbounds/:id", d.AdminDeleteInbound)
			admin.POST("/inbounds/:id/toggle", d.AdminToggleInbound)
			admin.GET("/plans", d.AdminPlans)
			admin.POST("/plans", d.AdminCreatePlan)
			admin.PUT("/plans/:id", d.AdminUpdatePlan)
			admin.DELETE("/plans/:id", d.AdminDeletePlan)
			admin.GET("/orders", d.AdminOrders)
			admin.POST("/orders/:id/confirm", d.AdminConfirmOrder)
			admin.POST("/orders/:id/cancel", d.AdminCancelOrder)
			admin.GET("/audit-logs", d.AdminAuditLogs)
			admin.GET("/users/:id/inbounds", d.AdminUserInbounds)
			admin.POST("/users/:id/inbounds", d.AdminSetUserInbounds)
		}
	}

	// 前端静态托管：web/dist 存在时托管（生产部署；开发用 vite dev 不需要）。
	// SPA fallback：非 API 路径返回 index.html。
	dist := "web/dist"
	if _, err := os.Stat(dist); err == nil {
		r.Static("/assets", filepath.Join(dist, "assets"))
		r.NoRoute(func(c *gin.Context) {
			p := c.Request.URL.Path
			if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/sub/") ||
				strings.HasPrefix(p, "/node/") || p == "/healthz" {
				util.Fail(c, http.StatusNotFound, "接口不存在")
				return
			}
			c.File(filepath.Join(dist, "index.html"))
		})
	}
	return r
}