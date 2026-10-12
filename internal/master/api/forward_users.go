package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// forwardUserView 转发用户前端返回结构。
type forwardUserView struct {
	ID                uint64     `json:"id"`
	Username          string     `json:"username"`
	Password          string     `json:"password"`
	PermissionGroupID uint64     `json:"permission_group_id"`
	GroupName         string     `json:"group_name"`
	TrafficLimitGB    int64      `json:"traffic_limit_gb"`
	TrafficUsedBytes  int64      `json:"traffic_used_bytes"`
	UpBytes           int64      `json:"up_bytes"`
	DownBytes         int64      `json:"down_bytes"`
	TrafficReset      string     `json:"traffic_reset"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	Enabled           bool       `json:"enabled"`
	Remark            string     `json:"remark"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type forwardUserForm struct {
	Username          string     `json:"username" binding:"required,min=2,max=64"`
	Password          string     `json:"password" binding:"required,min=4,max=128"`
	PermissionGroupID uint64     `json:"permission_group_id" binding:"required"`
	TrafficLimitGB    int64      `json:"traffic_limit_gb" binding:"min=0"`
	TrafficReset      string     `json:"traffic_reset"`
	ExpiresAt         *time.Time `json:"expires_at"`
	Enabled           *bool      `json:"enabled"`
	Remark            string     `json:"remark"`
}

type forwardUserUpdateForm struct {
	Password          string     `json:"password" binding:"omitempty,min=4,max=128"`
	PermissionGroupID uint64     `json:"permission_group_id"`
	TrafficLimitGB    *int64     `json:"traffic_limit_gb" binding:"omitempty,min=0"`
	TrafficReset      string     `json:"traffic_reset"`
	ExpiresAt         *time.Time `json:"expires_at"`
	Enabled           *bool      `json:"enabled"`
	Remark            *string    `json:"remark"`
}

type forwardUsersSummaryView struct {
	TotalUsers     int64 `json:"total_users"`
	ActiveUsers    int64 `json:"active_users"`
	TotalLimitGB   int64 `json:"total_limit_gb"`
	TotalUsedBytes int64 `json:"total_used_bytes"`
}

// AdminSyncForwardUsersConfigs 手动将所有转发用户最新凭据配置下发到包含 Mixed/SOCKS/HTTP 入站的服务器。
func (d *Deps) AdminSyncForwardUsersConfigs(c *gin.Context) {
	var serverIDs []uint64
	if err := d.DB.Model(&models.Inbound{}).
		Where("enabled = ? AND (protocol = ? OR protocol = ? OR protocol = ?)", true, models.ProtocolMixed, models.ProtocolSocks, models.ProtocolHTTP).
		Distinct().
		Pluck("server_id", &serverIDs).Error; err != nil {
		util.ServerError(c, "查询转发入站服务器列表失败")
		return
	}

	for _, sid := range serverIDs {
		_ = d.enqueueConfig(sid)
	}

	util.OK(c, gin.H{
		"synced_servers": len(serverIDs),
	})
}

// AdminListForwardUsers 查询转发用户列表。
func (d *Deps) AdminListForwardUsers(c *gin.Context) {
	q := d.DB.Model(&models.ForwardUser{})

	if gidStr := c.Query("permission_group_id"); gidStr != "" {
		if gid, err := strconv.ParseUint(gidStr, 10, 64); err == nil && gid > 0 {
			q = q.Where("permission_group_id = ?", gid)
		}
	}
	if enStr := c.Query("enabled"); enStr != "" {
		if en, err := strconv.ParseBool(enStr); err == nil {
			q = q.Where("enabled = ?", en)
		}
	}
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		q = q.Where("username LIKE ? OR remark LIKE ?", "%"+kw+"%", "%"+kw+"%")
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		util.ServerError(c, "查询转发用户总数失败")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var users []models.ForwardUser
	if err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users).Error; err != nil {
		util.ServerError(c, "查询转发用户列表失败")
		return
	}

	// 批量取权限组名
	groupMap := make(map[uint64]string)
	if len(users) > 0 {
		var gids []uint64
		for _, u := range users {
			gids = append(gids, u.PermissionGroupID)
		}
		var groups []models.PermissionGroup
		d.DB.Where("id IN ?", gids).Find(&groups)
		for _, g := range groups {
			groupMap[g.ID] = g.Name
		}
	}

	items := make([]forwardUserView, 0, len(users))
	for _, u := range users {
		items = append(items, forwardUserView{
			ID:                u.ID,
			Username:          u.Username,
			Password:          u.Password,
			PermissionGroupID: u.PermissionGroupID,
			GroupName:         groupMap[u.PermissionGroupID],
			TrafficLimitGB:    u.TrafficLimitGB,
			TrafficUsedBytes:  u.TrafficUsedBytes,
			UpBytes:           u.UpBytes,
			DownBytes:         u.DownBytes,
			TrafficReset:      u.TrafficReset,
			ExpiresAt:         u.ExpiresAt,
			Enabled:           u.Enabled,
			Remark:            u.Remark,
			CreatedAt:         u.CreatedAt,
			UpdatedAt:         u.UpdatedAt,
		})
	}

	util.OK(c, gin.H{
		"items": items,
		"total": total,
	})
}

// AdminForwardUsersSummary 统计概览。
func (d *Deps) AdminForwardUsersSummary(c *gin.Context) {
	var summary forwardUsersSummaryView
	d.DB.Model(&models.ForwardUser{}).Count(&summary.TotalUsers)
	d.DB.Model(&models.ForwardUser{}).Where("enabled = true").Count(&summary.ActiveUsers)

	type sumResult struct {
		LimitTotal int64
		UsedTotal  int64
	}
	var sr sumResult
	d.DB.Model(&models.ForwardUser{}).Select("COALESCE(SUM(traffic_limit_gb), 0) as limit_total, COALESCE(SUM(traffic_used_bytes), 0) as used_total").Scan(&sr)
	summary.TotalLimitGB = sr.LimitTotal
	summary.TotalUsedBytes = sr.UsedTotal

	util.OK(c, summary)
}

// AdminCreateForwardUser 创建转发用户。
func (d *Deps) AdminCreateForwardUser(c *gin.Context) {
	var req forwardUserForm
	if !util.BindJSON(c, &req) {
		return
	}

	// 校验权限组存在
	var pg models.PermissionGroup
	if err := d.DB.First(&pg, req.PermissionGroupID).Error; err != nil {
		util.BadRequest(c, "指定的权限组不存在")
		return
	}

	// 检查用户名唯一
	var cnt int64
	d.DB.Model(&models.ForwardUser{}).Where("username = ?", req.Username).Count(&cnt)
	if cnt > 0 {
		util.BadRequest(c, "用户名已存在，请更换")
		return
	}

	resetCycle := strings.ToLower(strings.TrimSpace(req.TrafficReset))
	if resetCycle == "" {
		resetCycle = "never"
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	fu := models.ForwardUser{
		Username:          req.Username,
		Password:          req.Password,
		PermissionGroupID: req.PermissionGroupID,
		TrafficLimitGB:    req.TrafficLimitGB,
		TrafficReset:      resetCycle,
		ExpiresAt:         req.ExpiresAt,
		Enabled:           enabled,
		Remark:            strings.TrimSpace(req.Remark),
	}

	if err := d.DB.Create(&fu).Error; err != nil {
		util.ServerError(c, "创建转发用户失败")
		return
	}

	util.OK(c, fu)
}

// AdminUpdateForwardUser 更新转发用户。
func (d *Deps) AdminUpdateForwardUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的用户 ID")
		return
	}

	var req forwardUserUpdateForm
	if !util.BindJSON(c, &req) {
		return
	}

	var fu models.ForwardUser
	if err := d.DB.First(&fu, id).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "转发用户不存在")
		return
	}

	updates := map[string]any{
		"updated_at": time.Now(),
	}

	if req.Password != "" {
		updates["password"] = req.Password
	}
	if req.PermissionGroupID > 0 && req.PermissionGroupID != fu.PermissionGroupID {
		var pg models.PermissionGroup
		if err := d.DB.First(&pg, req.PermissionGroupID).Error; err != nil {
			util.BadRequest(c, "指定的权限组不存在")
			return
		}
		updates["permission_group_id"] = req.PermissionGroupID
	}
	if req.TrafficLimitGB != nil {
		updates["traffic_limit_gb"] = *req.TrafficLimitGB
	}
	if req.TrafficReset != "" {
		updates["traffic_reset"] = strings.ToLower(strings.TrimSpace(req.TrafficReset))
	}
	if req.ExpiresAt != nil {
		updates["expires_at"] = req.ExpiresAt
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Remark != nil {
		updates["remark"] = strings.TrimSpace(*req.Remark)
	}

	if err := d.DB.Model(&fu).Updates(updates).Error; err != nil {
		util.ServerError(c, "更新转发用户失败")
		return
	}

	util.OK(c, gin.H{"updated": true})
}

// AdminToggleForwardUser 切换启用/禁用状态。
func (d *Deps) AdminToggleForwardUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的用户 ID")
		return
	}

	var fu models.ForwardUser
	if err := d.DB.First(&fu, id).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "转发用户不存在")
		return
	}

	fu.Enabled = !fu.Enabled
	if err := d.DB.Model(&fu).Update("enabled", fu.Enabled).Error; err != nil {
		util.ServerError(c, "更新转发用户状态失败")
		return
	}

	util.OK(c, gin.H{
		"enabled": fu.Enabled,
	})
}

// AdminResetForwardUserTraffic 流量归零并尝试解除熔断。
func (d *Deps) AdminResetForwardUserTraffic(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的用户 ID")
		return
	}

	var fu models.ForwardUser
	if err := d.DB.First(&fu, id).Error; err != nil {
		util.Fail(c, http.StatusNotFound, "转发用户不存在")
		return
	}

	updates := map[string]any{
		"traffic_used_bytes": 0,
		"enabled":            true,
		"updated_at":         time.Now(),
	}
	if err := d.DB.Model(&fu).Updates(updates).Error; err != nil {
		util.ServerError(c, "重置转发用户流量失败")
		return
	}

	util.OK(c, gin.H{"reset": true})
}

// AdminDeleteForwardUser 删除转发用户。
func (d *Deps) AdminDeleteForwardUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的用户 ID")
		return
	}

	var fu models.ForwardUser
	if err := d.DB.First(&fu, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			util.OK(c, gin.H{"deleted": true})
			return
		}
		util.ServerError(c, "查询转发用户失败")
		return
	}

	if err := d.DB.Delete(&fu).Error; err != nil {
		util.ServerError(c, "删除转发用户失败")
		return
	}

	util.OK(c, gin.H{"deleted": true})
}
