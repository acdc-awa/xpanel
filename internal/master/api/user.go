package api

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/middleware"
	"github.com/zhx/xray-panel/internal/master/services"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// Me GET /api/v1/user/me —— 当前用户资料 + 流量用量。
func (d *Deps) Me(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var user models.User
	if err := d.DB.First(&user, uid).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}

	totalBytes := int64(0)
	if user.PlanID > 0 {
		var plan models.Plan
		if err := d.DB.First(&plan, user.PlanID).Error; err == nil && plan.Enabled {
			totalBytes = plan.TrafficGB * 1024 * 1024 * 1024
		}
	}
	up, down, _ := d.Traffic.UserUsed(user.ID)

	util.OK(c, gin.H{
		"id":              user.ID,
		"username":        user.Username,
		"email":           user.Email,
		"role":            user.Role,
		"status":          user.Status,
		"plan_id":         user.PlanID,
		"expire_at":       user.ExpireAt,
		"balance_cents":   user.BalanceCents,
		"subscribe_token": user.SubscribeToken,
		"created_at":      user.CreatedAt,
		"up_bytes":        up,
		"down_bytes":      down,
		"used_bytes":      up + down,
		"total_bytes":     totalBytes,
		"must_change_pwd": user.MustChangePwd,
		"totp_enabled":    user.TotpEnabled,
	})
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
	d.Audit.Log("user", uid, "auth.change_password", "修改密码", c.ClientIP())
	util.OK(c, gin.H{"ok": true})
}

// UserUpdateProfile PUT /api/v1/user/profile —— 资料（用户名=邮箱，2026-08-14 起用户不可自助改邮箱，
// 仅管理员可改（AdminUpdateUser，记审计）。本接口保留为空操作以兼容旧客户端）。
func (d *Deps) UserUpdateProfile(c *gin.Context) {
	util.OK(c, gin.H{"ok": true})
}

// UserServers GET /api/v1/user/servers —— 用户可见节点可用性（J15：替换前端 mock）。
// 与订阅同源（权限组过滤）；在线 = last_seen_at 在心跳窗口（90s）内。
func (d *Deps) UserServers(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var user models.User
	if err := d.DB.First(&user, uid).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	granted := services.AuthorizedInboundSet(d.DB, &user)
	var inbounds []models.Inbound
	d.DB.Where("enabled = ? AND type = ?", true, models.InboundTypeUser).Find(&inbounds)

	serverSeen := make(map[uint64]bool)
	items := make([]gin.H, 0, 8)
	for i := range inbounds {
		inb := &inbounds[i]
		if !granted[inb.ID] || serverSeen[inb.ServerID] {
			continue
		}
		serverSeen[inb.ServerID] = true
		var srv models.Server
		if err := d.DB.First(&srv, inb.ServerID).Error; err != nil {
			continue
		}
		online := false
		if srv.LastSeenAt != nil {
			online = time.Since(*srv.LastSeenAt) < 90*time.Second
		}
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
