package api

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// reTitle 匹配 index.html 中的静态 <title>（含属性、跨行），供站点标题替换。
var reTitle = regexp.MustCompile(`(?is)<title[^>]*>.*?</title>`)

// maskSensitivePath 对访问日志中的敏感路径脱敏（订阅 token、节点 secret 等）。
func maskSensitivePath(p string) string {
	if strings.HasPrefix(p, "/api/v1/sub/") {
		return "/api/v1/sub/***"
	}
	return p
}

// accessLogFormatter 自定义 Gin 访问日志格式，确保订阅 token 不打全量进日志。
func accessLogFormatter(param gin.LogFormatterParams) string {
	return fmt.Sprintf("[GIN] %s | %3d | %13v | %15s | %-7s %s %#v\n",
		param.TimeStamp.Format("2006/01/02 - 15:04:05"),
		param.StatusCode,
		param.Latency,
		param.ClientIP,
		param.Method,
		maskSensitivePath(param.Path),
		param.ErrorMessage,
	)
}

// NewWebRouter 组装面板端口路由（三端口模型 2026-08-25 拍板：SPA 静态托管与后端 API 合并监听 APP_PORT）。
// 命中 /api/v1/* 与探针路径走 API 处理；其余路径托管前端 SPA（history fallback 注入站点设置）。
// 节点 WebSocket 网关归 NewWSRouter（APP_WS_PORT），订阅归独立订阅端口（APP_SUB_PORT）。
func NewWebRouter(d *Deps) *gin.Engine {
	r := gin.New()
	// ISSUE-12：访问日志对订阅 token 路径脱敏，避免完整 token 进入日志。
	// env=prod 静默运行（2026-09-03）：gin.SetMode(ReleaseMode) 只关 gin 自身 debug 输出，
	// Logger 中间件不受模式影响——按 env 决定是否挂载，否则 prod 仍会刷每请求 [GIN] 日志。
	if d.Cfg == nil || d.Cfg.App.Env != "prod" {
		r.Use(gin.LoggerWithFormatter(accessLogFormatter))
	}
	// P2-2：全局请求体上限 10MB（配置 JSON/拓扑布局/审计均远小于该值）。
	r.Use(gin.Recovery(), middleware.BodyLimit(10<<20))

	registerAPI(r, d)
	registerSPA(r, d)
	return r
}

// registerAPI 注册后端 HTTP API 路由（/healthz /readyz 探针 + /api/v1/*）。
func registerAPI(r *gin.Engine, d *Deps) {

	// P2-1：/healthz 为进程存活探针；/readyz 额外检查数据库连通性。
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		ready := d.DB != nil
		latency := int64(0)
		if d.DB != nil {
			if sqlDB, err := d.DB.DB(); err == nil {
				ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
				start := time.Now()
				ready = sqlDB.PingContext(ctx) == nil
				latency = time.Since(start).Milliseconds()
				cancel()
			} else {
				ready = false
			}
		}
		if !ready {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready", "db_latency_ms": latency})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready", "db_latency_ms": latency})
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
			// ISSUE-01：verify 单独挂 pending-token 认证中间件，注入 claims 供 handler 使用。
			auth.POST("/2fa/verify", middleware.AuthPending2FA(d.JWT, d.DB), d.TwoFAVerify)
			auth.POST("/forgot", d.ForgotPassword)
			auth.POST("/reset", d.ResetPassword)
			auth.POST("/refresh", d.Refresh)
			auth.POST("/logout", d.Logout)
		}

		// 用户端
		user := v1.Group("/user", middleware.AuthRequired(d.JWT, d.DB), middleware.RequirePwdChanged(d.DB))
		{
			user.GET("/me", d.Me)
			user.GET("/servers", d.UserServers)
			user.GET("/orders", d.UserOrders)
			user.POST("/orders/pay-balance", d.UserPayOrderByBalance)
			user.POST("/gift-cards/redeem", middleware.RateLimit(10, time.Minute), d.UserRedeemGiftCard)
			user.GET("/balance-logs", d.UserBalanceLogs)
			user.POST("/password", d.UserChangePassword)
			user.POST("/auto-renew", d.UserAutoRenew)
			user.POST("/2fa/setup", d.UserOTPSetup)
			user.POST("/2fa/confirm", d.UserOTPConfirm)
			user.POST("/2fa/disable", d.UserOTPDisable)
			user.POST("/subscribe/reset", d.UserResetSubscribe)
			user.GET("/notices", d.UserListNotices)
		}

		// 商店套餐：公开可读，但挂可选鉴权——登录身份用于「可新购/可续费」感知过滤
		// （非持有者只见可新购；持有者额外见自己可续费的当前套餐）
		v1.GET("/plans", middleware.AuthOptional(d.JWT, d.DB), d.PublicPlans)

		// 管理端（需 admin 角色）
		admin := v1.Group("/admin",
			middleware.AuthRequired(d.JWT, d.DB),
			middleware.RequireRole("admin"),
			middleware.Audit(d.DB),
			middleware.RequirePwdChanged(d.DB),
			middleware.RateLimitWrite(120, time.Minute),
		)
		{
			admin.GET("/dashboard", d.AdminDashboard)
			admin.GET("/system/status", d.AdminSystemStatus)
			admin.GET("/update/check", d.AdminUpdateCheck)
			admin.GET("/update/status", d.AdminUpdateStatus)
			admin.POST("/update/apply", d.AdminUpdateApply)
			admin.GET("/settings", d.AdminSettings)
			admin.PUT("/settings", d.AdminUpdateSettings)
			admin.GET("/users", d.AdminUsers)
			admin.GET("/invitations", d.AdminInvitations)
			admin.POST("/invitations", d.AdminCreateInvitations)
			admin.DELETE("/invitations/:id", d.AdminRevokeInvitation)
			admin.GET("/servers", d.AdminServers)
			admin.GET("/servers/agent-version", d.AdminGetAgentVersion)
			admin.GET("/servers/:id/metrics", d.AdminServerMetrics)
			admin.GET("/servers/:id/online-ips", d.AdminServerOnlineIPs)
			admin.POST("/servers", d.AdminCreateServer)
			admin.PUT("/servers/:id", d.AdminUpdateServer)
			admin.DELETE("/servers/:id", d.AdminDeleteServer)
			admin.POST("/servers/:id/reset-secret", d.AdminResetSecret)
			admin.POST("/servers/:id/command", d.AdminServerCommand)
			admin.GET("/servers/:id/upgrade-status", d.AdminGetServerUpgradeStatus)
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
			admin.GET("/servers/:id/layers", d.AdminGetLayers)
			admin.POST("/servers/:id/layers", d.AdminCreateLayer)
			admin.PUT("/servers/:id/layers/:layer_id", d.AdminUpdateLayer)
			admin.DELETE("/servers/:id/layers/:layer_id", d.AdminDeleteLayer)
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
			admin.GET("/users/:id/subscribe-token", d.AdminGetUserSubscribeToken)
			admin.POST("/users/:id/subscribe-token/reset", d.AdminResetUserSubscribeToken)
			admin.GET("/gift-cards", d.AdminGiftCards)
			admin.POST("/gift-cards", d.AdminBatchCreateGiftCards)
			admin.DELETE("/gift-cards/:id", d.AdminDeleteGiftCard)
			// Phase T：内部账户指令 / 证书 / 权限组
			admin.POST("/inbounds/:id/setup-internal", d.AdminSetupInternal)
			admin.POST("/inbounds/:id/rotate-internal", d.AdminRotateInternal)
			admin.GET("/certs", d.AdminCerts)
			admin.POST("/certs", d.AdminCreateCert)
			admin.POST("/certs/self-signed", d.AdminGenerateSelfSignedCert)
			admin.PUT("/certs/:id", d.AdminUpdateCert)
			admin.DELETE("/certs/:id", d.AdminDeleteCert)
			admin.GET("/permission-groups", d.AdminPermissionGroups)
			admin.POST("/permission-groups", d.AdminCreatePermissionGroup)
			admin.PUT("/permission-groups/:id", d.AdminUpdatePermissionGroup)
			admin.PUT("/permission-groups/:id/access-points", d.AdminSetPermissionGroupAccessPoints)
			admin.DELETE("/permission-groups/:id", d.AdminDeletePermissionGroup)
			admin.POST("/permission-groups/:id/preview-template", d.AdminPreviewPermissionGroupTemplate)
			admin.GET("/sub-templates", d.AdminListSubTemplates)
			admin.POST("/sub-templates", d.AdminCreateSubTemplate)
			admin.PUT("/sub-templates/:id", d.AdminUpdateSubTemplate)
			admin.DELETE("/sub-templates/:id", d.AdminDeleteSubTemplate)
			admin.GET("/access-points", d.AdminGetAccessPoints)
			admin.POST("/access-points", d.AdminCreateAccessPoint)
			admin.PUT("/access-points/:id", d.AdminUpdateAccessPoint)
			admin.PUT("/access-points/:id/target", d.AdminSetAccessPointTarget)
			admin.DELETE("/access-points/:id", d.AdminDeleteAccessPoint)
			admin.GET("/topology", d.AdminTopology)
			admin.GET("/topology-layout", d.AdminGetTopologyLayout)
			admin.PUT("/topology-layout", d.AdminSaveTopologyLayout)
			admin.GET("/notices", d.AdminListNotices)
			admin.POST("/notices", d.AdminCreateNotice)
			admin.PUT("/notices/:id", d.AdminUpdateNotice)
			admin.DELETE("/notices/:id", d.AdminDeleteNotice)
			admin.POST("/notices/:id/toggle", d.AdminToggleNotice)
		}
	}

	// registerAPI 不设 NoRoute：未匹配路径统一由 registerSPA 的 fallback 处理（守卫拒绝 API/WS 路径）
}

// registerSPA 注册前端 SPA 静态托管与 history 路由 fallback。
// 未构建前端（开发由 vite dev server 承担）：明确提示而非白屏。
func registerSPA(r *gin.Engine, d *Deps) {
	dist := "web/dist"
	if _, err := os.Stat(dist); err != nil {
		r.NoRoute(func(c *gin.Context) {
			util.Fail(c, http.StatusNotFound, "前端未构建（开发请用 vite dev server）")
		})
		return
	}

	r.Static("/assets", filepath.Join(dist, "assets"))
	indexHTML, _ := os.ReadFile(filepath.Join(dist, "index.html"))
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		// 前缀守卫：未匹配的 API/WS/探针路径绝不返回 index.html（防反代误分流落到 SPA fallback）
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/node/") ||
			p == "/healthz" || p == "/readyz" {
			util.Fail(c, http.StatusNotFound, "接口不存在")
			return
		}
		if len(indexHTML) == 0 {
			util.Fail(c, http.StatusNotFound, "前端未构建")
			return
		}
		// 站点设置注入（17 号 P0 ②）：app_name 替换静态 <title>（DB 优先，静态兜底）；
		// favicon → <link rel="icon">；全量 site 分组 → window.__PANEL_SETTINGS__（前端读取标题/LOGO/注册开关等）。
		site := map[string]string{}
		if d.Site != nil {
			site = d.Site.SiteGroup()
		}
		html := injectSiteHead(string(indexHTML), site)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
	})
}

// injectSiteHead 向 index.html 注入站点设置：DB 配置的 app_name 替换静态 <title>
// （避免出现双 title——浏览器只取第一个，静态默认会永远盖过 DB 配置）；
// 公开白名单站点设置（不含订阅端点/UA 规则等内部信息）与 favicon 追加到 </head> 前。
func injectSiteHead(indexHTML string, site map[string]string) string {
	doc := indexHTML
	if title := site[services.SettingAppName]; title != "" {
		newTitle := "<title>" + html.EscapeString(title) + "</title>"
		if reTitle.MatchString(doc) {
			doc = reTitle.ReplaceAllString(doc, newTitle)
		} else {
			doc = strings.Replace(doc, "</head>", newTitle+"</head>", 1)
		}
	}
	head := ""
	if icon := site[services.SettingFavicon]; icon != "" {
		head += `<link rel="icon" href="` + html.EscapeString(icon) + `">`
	}
	settingsJSON, _ := json.Marshal(publicSiteSettings(site))
	head += fmt.Sprintf("<script>window.__PANEL_SETTINGS__=%s</script>", settingsJSON)
	return strings.Replace(doc, "</head>", head+"</head>", 1)
}

// publicSiteSettings 公开站点设置白名单：仅品牌/展示/注册开关/条款/货币——订阅端点、
// UA 规则与拒绝码等内部信息不下发（与 PublicConfig 口径一致，防未登录探测订阅入口）。
func publicSiteSettings(site map[string]string) map[string]string {
	out := make(map[string]string, 8)
	for _, k := range []string{
		services.SettingAppName, services.SettingAppDesc, services.SettingLogo,
		services.SettingFavicon, services.SettingTOSURL, services.SettingStopRegister,
		services.SettingCurrency, services.SettingCurrencySymbol,
	} {
		out[k] = site[k]
	}
	return out
}

// NewWSRouter 组装节点 WebSocket 网关（三端口模型下仅监听 app.ws_port）。
// 端口专用：任意路径都交给 WS 网关——对外路径（默认 /node/ws，或 config.yaml 的 app.ws_public_url
// 指定的任意路径/域名）由反代裁决后原样转发，本端口无需感知具体路径。
func NewWSRouter(d *Deps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.NoRoute(d.Hub.ServeWS)
	return r
}
