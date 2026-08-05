package api

import (
	"fmt"
	"log"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

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
		up, down, _ := d.Traffic.UserUsed(u.ID)
		used := up + down
		totalBytes := int64(0)
		if u.PlanID > 0 {
			var plan models.Plan
			if err := d.DB.First(&plan, u.PlanID).Error; err == nil && plan.Enabled {
				totalBytes = plan.TrafficGB * 1024 * 1024 * 1024
			}
		}
		list = append(list, gin.H{
			"id": u.ID, "username": u.Username, "email": u.Email,
			"uuid": u.UUID,
			"role": u.Role, "status": u.Status, "plan_id": u.PlanID,
			"expire_at": u.ExpireAt, "created_at": u.CreatedAt,
			"up_bytes": up, "down_bytes": down,
			"used_bytes": used, "total_bytes": totalBytes,
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

// AdminCreateUser POST /api/v1/admin/users —— 管理员手动创建用户（自动生成 UUID 用于 Xray）。
func (d *Deps) AdminCreateUser(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,max=128"`
		Password string `json:"password" binding:"required,min=8,max=72"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	// 邮箱去重
	var cnt int64
	d.DB.Model(&models.User{}).Where("email = ?", req.Email).Count(&cnt)
	if cnt > 0 {
		util.BadRequest(c, "邮箱已被使用")
		return
	}

	hash, err := argon2id.CreateHash(req.Password, argon2id.DefaultParams)
	if err != nil {
		util.ServerError(c, "密码加密失败")
		return
	}
	uuid, err := util.NewUUID()
	if err != nil {
		util.ServerError(c, "生成 UUID 失败")
		return
	}
	token, err := util.NewSubscribeToken()
	if err != nil {
		util.ServerError(c, "生成订阅 Token 失败")
		return
	}

	user := models.User{
		Username:       req.Email, // 用邮箱直接做用户名
		Email:          req.Email,
		UUID:           uuid,
		PasswordHash:   hash,
		Role:           models.RoleUser,
		Status:         models.StatusActive,
		SubscribeToken: token,
	}
	if err := d.DB.Create(&user).Error; err != nil {
		if err.Error() == "UNIQUE constraint failed: users.username" {
			util.BadRequest(c, "该邮箱已用作用户名")
		} else {
			util.ServerError(c, "创建失败")
		}
		return
	}
	d.Audit.Log("admin", middleware.CurrentUser(c), "user.create", "创建用户 "+req.Email, c.ClientIP())

	// 用户变更 → 全量重推所有有入站的服务器的配置
	d.enqueueForAllWithInbounds()

	util.OK(c, gin.H{
		"id":       user.ID,
		"username": user.Username,
		"email":    user.Email,
		"uuid":     user.UUID,
		"role":     user.Role,
		"status":   user.Status,
	})
}

// AdminToggleUser POST /api/v1/admin/users/:id/toggle —— 封禁/解封用户。
func (d *Deps) AdminToggleUser(c *gin.Context) {
	id, err := parseUint(c.Param("id"))
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var user models.User
	if err := d.DB.First(&user, id).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	newStatus := models.StatusDisabled
	action := "封禁"
	if user.Status == models.StatusDisabled {
		newStatus = models.StatusActive
		action = "解封"
	}
	if err := d.DB.Model(&user).Update("status", newStatus).Error; err != nil {
		util.ServerError(c, "更新失败")
		return
	}
	d.Audit.Log("admin", middleware.CurrentUser(c), "user.toggle", action+"用户 "+user.Email, c.ClientIP())
	d.enqueueForAllWithInbounds()
	util.OK(c, gin.H{"id": id, "status": newStatus})
}

// AdminDeleteUser DELETE /api/v1/admin/users/:id —— 硬删除用户（清理授权后移除记录）。
func (d *Deps) AdminDeleteUser(c *gin.Context) {
	id, err := parseUint(c.Param("id"))
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var user models.User
	if err := d.DB.First(&user, id).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	if user.ID == middleware.CurrentUser(c) {
		util.BadRequest(c, "不能删除自己")
		return
	}
	email := user.Email
	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&models.UserInbound{}).Error; err != nil {
			return err
		}
		return tx.Delete(&user).Error
	}); err != nil {
		util.ServerError(c, "删除失败")
		return
	}
	d.Audit.Log("admin", middleware.CurrentUser(c), "user.delete", "删除用户 "+email, c.ClientIP())
	d.enqueueForAllWithInbounds()
	util.OK(c, gin.H{"deleted": id})
}

// enqueueForAllWithInbounds 对所有有启用入站的服务器触发配置重推。
func (d *Deps) enqueueForAllWithInbounds() {
	if d.Config == nil || d.Hub == nil {
		return
	}
	var serverIDs []uint64
	if err := d.DB.Model(&models.Inbound{}).Where("enabled = ?", true).
		Distinct("server_id").Pluck("server_id", &serverIDs).Error; err != nil {
		log.Printf("admin: 查询入站服务器失败: %v", err)
		return
	}
	for _, sid := range serverIDs {
		cfg, err := d.Config.Generate(sid)
		if err != nil {
			continue
		}
		if err := d.Config.SavePending(sid, cfg); err != nil {
			continue
		}
		go d.Hub.PushPending(sid)
	}
}

func parseUint(s string) (uint64, error) {
	var n uint64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	return n, nil
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
