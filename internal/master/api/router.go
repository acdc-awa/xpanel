package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/embed"
	"github.com/zhx/xray-panel/internal/master/middleware"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// agentDownloadHeaders 计算 /download/agent 下载响应头（version 为空则不发送版本头）。
// embed 与非 embed 路径共用；agent 升级（internal/agent/upgrade）以此为契约。
func agentDownloadHeaders(data []byte, version string) (versionHdr, shaHdr string) {
	if version != "" {
		versionHdr = version
	}
	sum := sha256.Sum256(data)
	shaHdr = hex.EncodeToString(sum[:])
	return
}

// NewRouter 组装全部路由。
func (d *Deps) NewRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")
	{
		// 公开配置（captcha site key 等）
		v1.GET("/config", d.PublicConfig)

		// 认证（登录/注册限流 5 次/分钟）
		auth := v1.Group("/auth", middleware.RateLimit(5, time.Minute))
		{
			auth.POST("/register", d.Register)
			auth.POST("/login", d.Login)
			auth.POST("/2fa/verify", d.TwoFAVerify)
			auth.POST("/forgot", d.ForgotPassword)
			auth.POST("/reset", d.ResetPassword)
			auth.POST("/refresh", d.Refresh)
			auth.POST("/logout", d.Logout)
		}

		// 节点 WebSocket 网关（节点 Agent 连接）
		v1.GET("/node/ws", d.Hub.ServeWS)

		// 订阅（公开，token 鉴权；J19-④ 按 IP+路径限流防盗刷）
		v1.GET("/sub/:token", middleware.RateLimit(120, time.Minute), d.Subscribe)

			// 用户端
			user := v1.Group("/user", middleware.AuthRequired(d.JWT), middleware.RequirePwdChanged(d.DB))
			{
				user.GET("/me", d.Me)
				user.GET("/servers", d.UserServers)
				user.GET("/orders", d.UserOrders)
				user.POST("/orders/pay-balance", d.UserPayOrderByBalance)
				user.POST("/gift-cards/redeem", d.UserRedeemGiftCard)
				user.GET("/balance-logs", d.UserBalanceLogs)
				user.POST("/password", d.UserChangePassword)
				user.PUT("/profile", d.UserUpdateProfile)
				user.POST("/2fa/setup", d.UserOTPSetup)
				user.POST("/2fa/confirm", d.UserOTPConfirm)
				user.POST("/2fa/disable", d.UserOTPDisable)
				user.POST("/subscribe/reset", d.UserResetSubscribe)
			}

		// 公开：上架套餐
		v1.GET("/plans", d.PublicPlans)

		// 节点一键安装脚本下载（部署用；Docker 镜像内置 /app/install-agent.sh）
		v1.GET("/download/install-agent.sh", d.DownloadInstallScript)

		// 节点 Agent 二进制下载（部署用；J19-④ 限流防盗刷）
		v1.GET("/download/agent", middleware.RateLimit(60, time.Minute), func(c *gin.Context) {
			var data []byte
			if len(embed.AgentBinary) > 0 {
				data = embed.AgentBinary
			} else {
				p := os.Getenv("AGENT_BIN_PATH")
				if p == "" {
					p = "/app/agent"
				}
				if _, err := os.Stat(p); err != nil {
					p = ""
					for _, cand := range []string{"agent-linux", "bin/agent-linux"} {
						if _, err := os.Stat(cand); err == nil {
							p = cand
							break
						}
					}
				}
				if p == "" {
					util.Fail(c, http.StatusNotFound, "agent 二进制未内置（请用 scripts/build.sh 或 build.ps1 构建，或设置 AGENT_BIN_PATH）")
					return
				}
				var err error
				if data, err = os.ReadFile(p); err != nil {
					util.Fail(c, http.StatusInternalServerError, "读取 agent 二进制失败: "+err.Error())
					return
				}
			}
			verHdr, shaHdr := agentDownloadHeaders(data, embed.AgentVersion)
			if verHdr != "" {
				c.Header("X-Agent-Version", verHdr)
			}
			c.Header("X-Agent-Sha256", shaHdr)
			c.Header("Content-Disposition", `attachment; filename="xray-agent"`)
			c.Data(http.StatusOK, "application/octet-stream", data)
		})

		// 管理端（需 admin 角色）
		admin := v1.Group("/admin",
			middleware.AuthRequired(d.JWT),
			middleware.RequireRole("admin"),
			middleware.Audit(d.DB),
			middleware.RequirePwdChanged(d.DB),
		)
		{
			admin.GET("/dashboard", d.AdminDashboard)
			admin.GET("/settings", d.AdminSettings)
			admin.PUT("/settings", d.AdminUpdateSettings)
			admin.GET("/users", d.AdminUsers)
			admin.GET("/invitations", d.AdminInvitations)
			admin.POST("/invitations", d.AdminCreateInvitations)
			admin.GET("/servers", d.AdminServers)
			admin.GET("/servers/:id/metrics", d.AdminServerMetrics)
			admin.POST("/servers", d.AdminCreateServer)
			admin.PUT("/servers/:id", d.AdminUpdateServer)
			admin.DELETE("/servers/:id", d.AdminDeleteServer)
			admin.POST("/servers/:id/reset-secret", d.AdminResetSecret)
			admin.POST("/servers/:id/command", d.AdminServerCommand)
			admin.POST("/servers/:id/generate-config", d.AdminGenerateConfig)
			admin.GET("/servers/:id/config-preview", d.AdminGetServerConfigPreview)
			admin.GET("/servers/:id/outbounds", d.AdminGetServerOutbounds)
			admin.POST("/servers/:id/outbounds", d.AdminCreateServerOutbound)
			admin.PUT("/servers/:id/outbounds/:outbound_id", d.AdminUpdateServerOutbound)
			admin.DELETE("/servers/:id/outbounds/:outbound_id", d.AdminDeleteServerOutbound)
			admin.GET("/servers/:id/routing", d.AdminGetServerRoutingRules)
			admin.POST("/servers/:id/routing", d.AdminCreateServerRoutingRule)
			admin.PUT("/servers/:id/routing/:rule_id", d.AdminUpdateServerRoutingRule)
			admin.DELETE("/servers/:id/routing/:rule_id", d.AdminDeleteServerRoutingRule)
			admin.GET("/xray/keys", d.AdminXrayKeys)
			admin.POST("/xray/preview-config", d.AdminPreviewConfig)
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
			admin.GET("/audit-logs", d.AdminAuditLogs)
			admin.POST("/backup", d.AdminCreateBackup)
			admin.GET("/backup", d.AdminListBackups)
			admin.GET("/backup/:file", d.AdminDownloadBackup)
			admin.POST("/users", d.AdminCreateUser)
			admin.PUT("/users/:id", d.AdminUpdateUser)
			admin.DELETE("/users/:id", d.AdminDeleteUser)
			admin.POST("/users/:id/toggle", d.AdminToggleUser)
			admin.POST("/users/:id/2fa/disable", d.AdminDisableOTP)
			admin.POST("/users/:id/reset-traffic", d.AdminResetUserTraffic)
			admin.POST("/users/:id/balance", d.AdminAdjustUserBalance)
			admin.GET("/gift-cards", d.AdminGiftCards)
			admin.POST("/gift-cards", d.AdminBatchCreateGiftCards)
			admin.DELETE("/gift-cards/:id", d.AdminDeleteGiftCard)
			// Phase T：内部账户指令 / 证书 / 权限组
			admin.POST("/inbounds/:id/setup-internal", d.AdminSetupInternal)
			admin.POST("/inbounds/:id/rotate-internal", d.AdminRotateInternal)
			admin.GET("/certs", d.AdminCerts)
			admin.POST("/certs", d.AdminCreateCert)
			admin.PUT("/certs/:id", d.AdminUpdateCert)
			admin.DELETE("/certs/:id", d.AdminDeleteCert)
			admin.GET("/permission-groups", d.AdminPermissionGroups)
			admin.POST("/permission-groups", d.AdminCreatePermissionGroup)
			admin.PUT("/permission-groups/:id", d.AdminUpdatePermissionGroup)
			admin.DELETE("/permission-groups/:id", d.AdminDeletePermissionGroup)
			admin.GET("/permission-groups/:id/inbounds", d.AdminGroupInbounds)
			admin.POST("/permission-groups/:id/inbounds", d.AdminSetGroupInbounds)
			admin.POST("/permission-groups/:id/preview-template", d.AdminPreviewPermissionGroupTemplate)
			admin.GET("/topology", d.AdminTopology)
			admin.GET("/topology-layout", d.AdminGetTopologyLayout)
			admin.PUT("/topology-layout", d.AdminSaveTopologyLayout)
		}
	}

	// 前端静态托管：web/dist 存在时托管（生产部署；开发用 vite dev 不需要）。
	// SPA fallback：非 API 路径返回 index.html，并注入 web base 供前端读取。
	dist := "web/dist"
	if _, err := os.Stat(dist); err == nil {
		r.Static("/assets", filepath.Join(dist, "assets"))
		indexHTML, _ := os.ReadFile(filepath.Join(dist, "index.html"))
		r.NoRoute(func(c *gin.Context) {
			p := c.Request.URL.Path
			if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/sub/") ||
				strings.HasPrefix(p, "/node/") || p == "/healthz" {
				util.Fail(c, http.StatusNotFound, "接口不存在")
				return
			}
			if len(indexHTML) == 0 {
				util.Fail(c, http.StatusNotFound, "前端未构建")
				return
			}
			base := ""
			if d.Site != nil {
				base = d.Site.WebBase()
			}
			html := strings.Replace(string(indexHTML), "</head>",
				fmt.Sprintf("<script>window.__PANEL_BASE__=%q</script></head>", base), 1)
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
		})
	}
	return r
}
