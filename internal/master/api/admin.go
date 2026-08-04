package api

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/middleware"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// AdminDashboard GET /api/v1/admin/dashboard —— 仪表盘统计（P0 先提供真实计数，
// 卡片数据后续由 stats 模块补全）。
func (d *Deps) AdminDashboard(c *gin.Context) {
	var users, servers, plans, orders, pendingOrders int64
	db := d.DB
	db.Model(&models.User{}).Count(&users)
	db.Model(&models.Server{}).Count(&servers)
	db.Model(&models.Plan{}).Count(&plans)
	db.Model(&models.Order{}).Count(&orders)
	db.Model(&models.Order{}).Where("status = ?", models.OrderPending).Count(&pendingOrders)

	util.OK(c, gin.H{
		"total_users":    users,
		"online_servers": servers, // P1 接入心跳后改为在线数
		"total_plans":    plans,
		"total_orders":   orders,
		"pending_orders": pendingOrders,
	})
}

// AdminUsers GET /api/v1/admin/users —— 用户列表（分页）。
func (d *Deps) AdminUsers(c *gin.Context) {
	page := atoiDefault(c.Query("page"), 1)
	size := atoiDefault(c.Query("size"), 20)
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	var total int64
	if err := d.DB.Model(&models.User{}).Count(&total).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	var users []models.User
	if err := d.DB.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&users).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	list := make([]gin.H, 0, len(users))
	for _, u := range users {
		list = append(list, gin.H{
			"id": u.ID, "username": u.Username, "email": u.Email,
			"role": u.Role, "status": u.Status, "plan_id": u.PlanID,
			"expire_at": u.ExpireAt, "created_at": u.CreatedAt,
		})
	}
	util.OK(c, gin.H{"total": total, "page": page, "size": size, "items": list})
}

// AdminInvitations GET /api/v1/admin/invitations —— 邀请码列表。
func (d *Deps) AdminInvitations(c *gin.Context) {
	var list []models.InvitationCode
	if err := d.DB.Order("id DESC").Limit(200).Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	util.OK(c, gin.H{"items": list})
}

// AdminCreateInvitations POST /api/v1/admin/invitations —— 批量生成邀请码。
func (d *Deps) AdminCreateInvitations(c *gin.Context) {
	var req struct {
		Count   int    `json:"count" binding:"required,min=1,max=100"`
		Expires string `json:"expires"` // RFC3339 可选；空 = 永不过期
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	adminID := middleware.CurrentUser(c)

	var expires *time.Time
	if req.Expires != "" {
		t, err := time.Parse(time.RFC3339, req.Expires)
		if err != nil {
			util.BadRequest(c, "expires 需为 RFC3339 格式")
			return
		}
		expires = &t
	}

	codes := make([]models.InvitationCode, 0, req.Count)
	codeStrs := make([]string, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		code, err := util.NewInviteCode()
		if err != nil {
			util.ServerError(c, "生成邀请码失败")
			return
		}
		codes = append(codes, models.InvitationCode{
			Code:      code,
			CreatedBy: adminID,
			ExpiresAt: expires,
			Status:    models.InviteUnused,
		})
		codeStrs = append(codeStrs, code)
	}
	if err := d.DB.Create(&codes).Error; err != nil {
		util.ServerError(c, "保存邀请码失败")
		return
	}
	util.OK(c, gin.H{"count": len(codes), "codes": codeStrs})
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return def
	}
	return n
}