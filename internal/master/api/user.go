package api

import (
	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/middleware"
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

// UserUpdateProfile PUT /api/v1/user/profile —— 保存资料（邮箱）。
func (d *Deps) UserUpdateProfile(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req struct {
		Email string `json:"email" binding:"omitempty,email,max=128"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	var user models.User
	if err := d.DB.First(&user, uid).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	// 邮箱唯一性检查（排除自己）
	var cnt int64
	d.DB.Model(&models.User{}).Where("email = ? AND id != ?", req.Email, uid).Count(&cnt)
	if cnt > 0 {
		util.BadRequest(c, "邮箱已被使用")
		return
	}
	if err := d.DB.Model(&user).Update("email", req.Email).Error; err != nil {
		util.ServerError(c, "保存失败")
		return
	}
	util.OK(c, gin.H{"email": req.Email})
}
