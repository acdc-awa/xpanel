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
		"subscribe_token": user.SubscribeToken,
		"created_at":      user.CreatedAt,
		"up_bytes":        up,
		"down_bytes":      down,
		"used_bytes":      up + down,
		"total_bytes":     totalBytes,
	})
}