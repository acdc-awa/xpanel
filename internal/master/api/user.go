package api

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/master/nodegate"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// Me GET /api/v1/user/me —— 当前用户资料 + 流量用量。
func (d *Deps) Me(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var user models.User
	if err := d.DB.First(&user, uid).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	util.OK(c, d.userView(&user))
}

// userView 构造用户资料视图（登录/注册响应与 /user/me 同源，避免首屏数据失真）。
// ISSUE-16：补齐余额/流量/订阅 token/设备限制/权限组字段。
// 订阅对外根地址/入口路径仅在登录态下发（前端拼接订阅链接用；公开面不暴露订阅端点）。
func (d *Deps) userView(user *models.User) gin.H {
	totalBytes := int64(0)
	planName := ""
	if user.PlanID > 0 {
		var plan models.Plan
		if err := d.DB.First(&plan, user.PlanID).Error; err == nil && plan.Enabled {
			totalBytes = plan.TrafficGB * 1024 * 1024 * 1024
			planName = plan.Name
		}
	}
	var up, down int64
	if d.Traffic != nil {
		up, down, _ = d.Traffic.UserUsed(user.ID)
	}
	effectiveGroupID := services.UserEffectiveGroupID(d.DB, user)
	effectiveLimit, isCustomLimit := services.UserEffectiveDeviceLimit(d.DB, user)

	site := map[string]string{}
	if d.Site != nil {
		site = d.Site.SiteGroup()
	}

	return gin.H{
		"id":                     user.ID,
		"username":               user.Username,
		"email":                  user.Email,
		"role":                   user.Role,
		"status":                 user.Status,
		"plan_id":                user.PlanID,
		"plan_name":              planName,
		"expire_at":              user.ExpireAt,
		"balance_cents":          user.BalanceCents,
		"subscribe_token":        user.SubscribeToken,
		"subscribe_url":          site[services.SettingSubscribeURL],
		"subscribe_path":         site[services.SettingSubscribePath],
		"created_at":             user.CreatedAt,
		"up_bytes":               up,
		"down_bytes":             down,
		"used_bytes":             up + down,
		"total_bytes":            totalBytes,
		"must_change_pwd":        user.MustChangePwd,
		"totp_enabled":           user.TotpEnabled,
		"device_limit":           user.DeviceLimit,
		"effective_device_limit": effectiveLimit,
		"is_custom_device_limit": isCustomLimit,
		"permission_group_id":    user.PermissionGroupID,
		"effective_group_id":     effectiveGroupID,
	}
}

// UserChangePassword POST /api/v1/user/password —— 修改密码。
func (d *Deps) UserChangePassword(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8,max=72"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := d.Auth.ChangePassword(c.Request.Context(), uid, req.OldPassword, req.NewPassword); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	d.Audit.Log("user", uid, "auth.change_password", "修改密码", util.ClientIPFromContext(c))
	util.OK(c, gin.H{"ok": true})
}

// UserServers GET /api/v1/user/servers —— 用户可见节点可用性（J15：替换前端 mock）。
// 与订阅同源（AP 单点授权派生）：列出用户可见接入点的入口服务器
// （入口 = 目标入站所在服务器）。在线 = last_seen_at 在心跳窗口（90s）内。
func (d *Deps) UserServers(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var user models.User
	if err := d.DB.First(&user, uid).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	granted := services.AuthorizedEntryServerIDs(d.DB, &user)
	if len(granted) == 0 {
		util.OK(c, gin.H{"items": []gin.H{}})
		return
	}

	ids := make([]uint64, 0, len(granted))
	for id := range granted {
		ids = append(ids, id)
	}
	var servers []models.Server
	d.DB.Where("id IN ?", ids).Order("id ASC").Find(&servers)

	items := make([]gin.H, 0, len(servers))
	for i := range servers {
		srv := &servers[i]
		// 在线口径与网关一致：复用 nodegate.HeartbeatTimeout（90s），不再写死魔法数字
		online := srv.LastSeenAt != nil && time.Since(*srv.LastSeenAt) < nodegate.HeartbeatTimeout
		items = append(items, gin.H{
			"id":           srv.ID,
			"name":         srv.Name,
			"host":         srv.Host,
			"location":     srv.Location,
			"online":       online,
			"last_seen_at": srv.LastSeenAt,
		})
	}
	util.OK(c, gin.H{"items": items})
}
