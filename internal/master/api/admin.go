package api

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/db"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

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

	keyword := strings.TrimSpace(c.Query("keyword"))
	base := d.DB.Model(&models.User{})
	if keyword != "" {
		like := "%" + keyword + "%"
		base = base.Where("username LIKE ? OR email LIKE ? OR uuid LIKE ?", like, like, like)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	var users []models.User
	if err := base.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&users).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	// ISSUE-10：套餐信息一次批量预取，避免逐行查询 plan（用户流量仍按行统计）。
	planIDs := make([]uint64, 0, len(users))
	for _, u := range users {
		if u.PlanID > 0 {
			planIDs = append(planIDs, u.PlanID)
		}
	}
	planMap := make(map[uint64]models.Plan)
	if len(planIDs) > 0 {
		var plans []models.Plan
		if err := d.DB.Where("id IN ?", planIDs).Find(&plans).Error; err == nil {
			for _, p := range plans {
				planMap[p.ID] = p
			}
		}
	}

	list := make([]gin.H, 0, len(users))
	for _, u := range users {
		up, down, _ := d.Traffic.UserUsed(u.ID)
		used := up + down
		totalBytes := int64(0)
		effectiveGroupID := u.PermissionGroupID
		effectiveLimit, isCustomLimit := 0, false
		if u.DeviceLimit > 0 {
			effectiveLimit, isCustomLimit = u.DeviceLimit, true
		} else if plan, ok := planMap[u.PlanID]; ok && plan.Enabled {
			totalBytes = plan.TrafficGB * 1024 * 1024 * 1024
			effectiveLimit = plan.DeviceLimit
			if effectiveGroupID == 0 {
				effectiveGroupID = plan.PermissionGroupID
			}
		}
		list = append(list, gin.H{
			"id": u.ID, "username": u.Username, "email": u.Email,
			"uuid": u.UUID,
			"role": u.Role, "status": u.Status, "plan_id": u.PlanID,
			"permission_group_id":    u.PermissionGroupID,
			"effective_group_id":     effectiveGroupID,
			"device_limit":           u.DeviceLimit,
			"effective_device_limit": effectiveLimit,
			"is_custom_device_limit": isCustomLimit,
			"expire_at":              u.ExpireAt, "balance_cents": u.BalanceCents, "created_at": u.CreatedAt,
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
	util.OK(c, gin.H{"codes": codeStrs})
}

// AdminRevokeInvitation DELETE /api/v1/admin/invitations/:id —— 作废未使用的邀请码（ISSUE-17）。
func (d *Deps) AdminRevokeInvitation(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法邀请码 ID")
		return
	}
	var inv models.InvitationCode
	if err := d.DB.First(&inv, id).Error; err != nil {
		util.Fail(c, 404, "邀请码不存在")
		return
	}
	if inv.Status != models.InviteUnused {
		util.BadRequest(c, "仅未使用的邀请码可作废")
		return
	}
	if err := d.DB.Model(&inv).Update("status", models.InviteDisabled).Error; err != nil {
		util.ServerError(c, "作废失败")
		return
	}
	util.OK(c, gin.H{"id": id, "status": models.InviteDisabled})
}

// AdminCreateUser POST /api/v1/admin/users —— 管理员手动创建用户。
// 2026-08-14 方向④：余额只来自兑换码/调账，创建用户不再支持初始余额。
// validateUserRefs 校验套餐/权限组引用存在性（P1-5；值为 0 = 不使用该引用，跳过）。
// 返回错误文案，空字符串 = 通过。
func (d *Deps) validateUserRefs(planID, permGroupID uint64) string {
	if planID > 0 {
		var n int64
		if err := d.DB.Model(&models.Plan{}).Where("id = ?", planID).Count(&n).Error; err != nil || n == 0 {
			return fmt.Sprintf("套餐不存在: %d", planID)
		}
	}
	if permGroupID > 0 {
		var n int64
		if err := d.DB.Model(&models.PermissionGroup{}).Where("id = ?", permGroupID).Count(&n).Error; err != nil || n == 0 {
			return fmt.Sprintf("权限组不存在: %d", permGroupID)
		}
	}
	return ""
}

func (d *Deps) AdminCreateUser(c *gin.Context) {
	var req struct {
		Email             string     `json:"email" binding:"required,email"`
		Password          string     `json:"password" binding:"required,min=8"`
		PlanID            uint64     `json:"plan_id"`
		PermissionGroupID uint64     `json:"permission_group_id"`
		DeviceLimit       int        `json:"device_limit"`
		ExpireAt          *time.Time `json:"expire_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	// P1-5：套餐/权限组存在性校验——不存在则明确拒绝，避免"配额检查被跳过→无上限用量"
	if msg := d.validateUserRefs(req.PlanID, req.PermissionGroupID); msg != "" {
		util.BadRequest(c, msg)
		return
	}
	// 与 Register/AdminUpdateUser 对齐：邮箱小写化后作为用户名
	email := strings.ToLower(strings.TrimSpace(req.Email))
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
		Username:          email, // 用邮箱直接做用户名
		Email:             email,
		UUID:              uuid,
		PasswordHash:      hash,
		Role:              models.RoleUser,
		Status:            models.StatusActive,
		SubscribeToken:    token,
		PlanID:            req.PlanID,
		PermissionGroupID: req.PermissionGroupID,
		DeviceLimit:       req.DeviceLimit,
		ExpireAt:          req.ExpireAt,
	}
	if err := d.DB.Create(&user).Error; err != nil {
		if db.IsUniqueViolation(err, "users.username") {
			util.BadRequest(c, "该邮箱已用作用户名")
		} else {
			util.ServerError(c, "创建失败")
		}
		return
	}

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

// AdminUpdateUser PUT /api/v1/admin/users/:id —— 更新用户信息（角色/权限组/套餐/过期时间/密码/设备限制）。
// 2026-08-14 方向④：余额只走调账（AdminAdjustUserBalance，记流水），禁止直写 balance_cents。
func (d *Deps) AdminUpdateUser(c *gin.Context) {
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
	var req struct {
		Email             *string    `json:"email" binding:"omitempty,email,max=128"` // 管理员可改邮箱（=用户名，记审计）
		Role              *string    `json:"role"`                                    // "admin" | "user"
		PlanID            *uint64    `json:"plan_id"`
		PermissionGroupID *uint64    `json:"permission_group_id"` // 0=跟随套餐
		DeviceLimit       *int       `json:"device_limit"`        // 0=跟随套餐
		ExpireAt          *time.Time `json:"expire_at"`
		Status            *int       `json:"status"`
		Password          *string    `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	updates := map[string]any{}
	if req.Email != nil && *req.Email != "" {
		email := strings.ToLower(strings.TrimSpace(*req.Email))
		var cnt int64
		d.DB.Model(&models.User{}).Where("(username = ? OR email = ?) AND id != ?", email, email, id).Count(&cnt)
		if cnt > 0 {
			util.BadRequest(c, "邮箱已被使用")
			return
		}
		updates["email"] = email
		updates["username"] = email // 用户名=邮箱同值
	}
	if req.Role != nil {
		role := strings.ToLower(strings.TrimSpace(*req.Role))
		if role != models.RoleAdmin && role != models.RoleUser {
			util.BadRequest(c, "非法角色（仅支持 admin 或 user）")
			return
		}
		// 防死锁：如果试图将管理员降级为普通用户，必须确保系统中至少保留 1 名激活状态管理员
		if user.Role == models.RoleAdmin && role != models.RoleAdmin {
			var activeAdminCount int64
			d.DB.Model(&models.User{}).Where("role = ? AND status = ?", models.RoleAdmin, models.StatusActive).Count(&activeAdminCount)
			if activeAdminCount <= 1 {
				util.BadRequest(c, "系统必须至少保留一名处于激活状态的管理员，无法降级该账号")
				return
			}
		}
		updates["role"] = role
	}
	if req.PlanID != nil {
		if *req.PlanID > 0 {
			if msg := d.validateUserRefs(*req.PlanID, 0); msg != "" {
				util.BadRequest(c, msg)
				return
			}
		}
		updates["plan_id"] = *req.PlanID
	}
	if req.PermissionGroupID != nil {
		if *req.PermissionGroupID > 0 {
			if msg := d.validateUserRefs(0, *req.PermissionGroupID); msg != "" {
				util.BadRequest(c, msg)
				return
			}
		}
		updates["permission_group_id"] = *req.PermissionGroupID
	}
	if req.DeviceLimit != nil {
		updates["device_limit"] = *req.DeviceLimit
	}
	if req.ExpireAt != nil {
		updates["expire_at"] = req.ExpireAt
	}
	if req.Status != nil {
		status := *req.Status
		// 防死锁：如果试图禁用管理员，必须确保系统中至少保留 1 名激活状态管理员
		if user.Role == models.RoleAdmin && status != models.StatusActive {
			var activeAdminCount int64
			d.DB.Model(&models.User{}).Where("role = ? AND status = ?", models.RoleAdmin, models.StatusActive).Count(&activeAdminCount)
			if activeAdminCount <= 1 {
				util.BadRequest(c, "系统必须至少保留一名处于激活状态的管理员，无法禁用该账号")
				return
			}
		}
		updates["status"] = status
	}
	if req.Password != nil && *req.Password != "" {
		// 管理员改密：bump token_version 吊销该用户全部旧会话（J5）
		if err := d.Auth.AdminSetPassword(c.Request.Context(), id, *req.Password); err != nil {
			util.ServerError(c, "密码更新失败")
			return
		}
		delete(updates, "password_hash")
	}
	if len(updates) > 0 {
		if err := d.DB.Model(&user).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新失败")
			return
		}
	}
	d.enqueueForAllWithInbounds()
	if d.Hub != nil {
		d.Hub.SyncUsersToAll()
	}
	d.DB.First(&user, id)
	util.OK(c, gin.H{"user": user})
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
	if user.Status == models.StatusDisabled {
		newStatus = models.StatusActive
	} else {
		// 防死锁：如果试图禁用管理员，必须确保系统中至少保留 1 名激活状态管理员
		if user.Role == models.RoleAdmin {
			var activeAdminCount int64
			d.DB.Model(&models.User{}).Where("role = ? AND status = ?", models.RoleAdmin, models.StatusActive).Count(&activeAdminCount)
			if activeAdminCount <= 1 {
				util.BadRequest(c, "系统必须至少保留一名处于激活状态的管理员，无法禁用该账号")
				return
			}
		}
	}
	updates := map[string]any{"status": newStatus}
	if newStatus == models.StatusDisabled {
		// 封禁即吊销会话（J5：bump token_version，旧 refresh/access 立即失效）
		updates["token_version"] = gorm.Expr("token_version + 1")
	}
	if err := d.DB.Model(&user).Updates(updates).Error; err != nil {
		util.ServerError(c, "更新失败")
		return
	}
	d.enqueueForAllWithInbounds()
	util.OK(c, gin.H{"id": id, "status": newStatus})
}

// AdminResetUserTraffic POST /api/v1/admin/users/:id/reset-traffic —— 重置用户流量周期起点（J12）。
func (d *Deps) AdminResetUserTraffic(c *gin.Context) {
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
	if err := d.DB.Model(&user).Update("traffic_cycle_start", time.Now()).Error; err != nil {
		util.ServerError(c, "重置失败")
		return
	}
	d.enqueueForAllWithInbounds()
	if d.Hub != nil {
		d.Hub.SyncUsersToAll()
	}
	util.OK(c, gin.H{"ok": true})
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
	// 防死锁：如果试图删除管理员，必须确保系统中至少保留 1 名激活状态管理员
	if user.Role == models.RoleAdmin {
		var activeAdminCount int64
		d.DB.Model(&models.User{}).Where("role = ? AND status = ?", models.RoleAdmin, models.StatusActive).Count(&activeAdminCount)
		if activeAdminCount <= 1 {
			util.BadRequest(c, "系统必须至少保留一名处于激活状态的管理员，无法删除该账号")
			return
		}
	}
	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		return tx.Unscoped().Delete(&user).Error
	}); err != nil {
		util.ServerError(c, "删除失败")
		return
	}
	d.enqueueForAllWithInbounds()
	util.OK(c, gin.H{"deleted": id})
}

// TriggerUserChange 用户/权限相关变更统一出口（J17）：
// 热更新在线节点用户列表（SyncUsersToAll，秒级）+ 全量重推所有有入站的服务器配置（拉取型兜底）。
// 权限组节点集合/套餐绑定等变更后必须调用，消除「在线用户失效窗口不可控」。
func (d *Deps) TriggerUserChange() {
	if d.Hub != nil {
		d.Hub.SyncUsersToAll()
	}
	d.enqueueForAllWithInbounds()
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
