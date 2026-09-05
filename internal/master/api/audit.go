package api

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// AdminAuditLogs GET /api/v1/admin/audit-logs —— 审计日志（分页）。
func (d *Deps) AdminAuditLogs(c *gin.Context) {
	page := atoiDefault(c.Query("page"), 1)
	size := atoiDefault(c.Query("size"), 20)
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	q := d.DB.Model(&models.AuditLog{})

	// 1. 精确 action 筛选
	if act := strings.TrimSpace(c.Query("action")); act != "" {
		q = q.Where("action = ?", act)
	}

	// 2. 模块分类筛选（按 action 前缀划分）
	switch strings.TrimSpace(c.Query("category")) {
	case "servers":
		q = q.Where("action LIKE 'servers.%' OR action = 'servers'")
	case "users":
		q = q.Where("action LIKE 'users.%' OR action = 'users' OR action LIKE 'invitations.%' OR action = 'invitations'")
	case "billing":
		q = q.Where("action LIKE 'plans.%' OR action = 'plans' OR action LIKE 'orders.%' OR action = 'orders' OR action LIKE 'gift-cards.%' OR action = 'gift-cards'")
	case "inbounds":
		q = q.Where("action LIKE 'inbounds.%' OR action = 'inbounds' OR action LIKE 'certs.%' OR action = 'certs' OR action LIKE 'access-points.%' OR action = 'access-points' OR action LIKE 'permission-groups.%' OR action = 'permission-groups'")
	case "settings":
		// notice.%（单数）为 2026-09-04 手动打点退役前的存量 action，保留可筛
		q = q.Where("action LIKE 'settings.%' OR action = 'settings' OR action LIKE 'notices.%' OR action = 'notices' OR action LIKE 'notice.%' OR action LIKE 'backup.%' OR action = 'backup' OR action LIKE 'topology.%' OR action LIKE 'topology-layout.%'")
	case "auth":
		q = q.Where("action LIKE 'auth.%' OR action = 'auth'")
	}

	// 3. 关键词跨字段模糊搜索 (action / detail / ip)
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		pat := "%" + kw + "%"
		q = q.Where("action LIKE ? OR detail LIKE ? OR ip LIKE ?", pat, pat, pat)
	}

	// 4. 操作人筛选
	if op := strings.TrimSpace(c.Query("operator_id")); op != "" {
		if opID := atoiDefault(op, 0); opID > 0 {
			q = q.Where("operator_id = ?", opID)
		}
	}
	if ot := strings.TrimSpace(c.Query("operator_type")); ot != "" {
		q = q.Where("operator_type = ?", ot)
	}

	// 5. 时间范围筛选
	if st := strings.TrimSpace(c.Query("start_time")); st != "" {
		if t, err := time.Parse(time.RFC3339, st); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if et := strings.TrimSpace(c.Query("end_time")); et != "" {
		if t, err := time.Parse(time.RFC3339, et); err == nil {
			q = q.Where("created_at <= ?", t)
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	var list []models.AuditLog
	if err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}

	// 批量补操作人用户名（admin/user 共用 users 表；system 类型 ID=0 跳过）。
	// 查询失败静默降级为裸编号（部分测试库未建 users 表），不影响审计列表本身。
	operatorIDs := make([]uint64, 0, len(list))
	seenOp := make(map[uint64]bool, len(list))
	for _, l := range list {
		if l.OperatorID != 0 && !seenOp[l.OperatorID] {
			seenOp[l.OperatorID] = true
			operatorIDs = append(operatorIDs, l.OperatorID)
		}
	}
	usernameByOp := make(map[uint64]string, len(operatorIDs))
	if len(operatorIDs) > 0 {
		var operators []models.User
		if err := d.DB.Select("id", "username").Where("id IN ?", operatorIDs).Find(&operators).Error; err == nil {
			for _, u := range operators {
				usernameByOp[u.ID] = u.Username
			}
		}
	}

	type view struct {
		ID               uint64    `json:"id"`
		OperatorType     string    `json:"operator_type"`
		OperatorID       uint64    `json:"operator_id"`
		OperatorUsername string    `json:"operator_username"`
		Action           string    `json:"action"`
		Detail           string    `json:"detail"`
		IP               string    `json:"ip"`
		CreatedAt        time.Time `json:"created_at"`
	}
	items := make([]view, 0, len(list))
	for _, l := range list {
		items = append(items, view{
			ID: l.ID, OperatorType: l.OperatorType, OperatorID: l.OperatorID,
			OperatorUsername: usernameByOp[l.OperatorID],
			Action: l.Action, Detail: l.Detail, IP: l.IP, CreatedAt: l.CreatedAt,
		})
	}
	util.OK(c, gin.H{"total": total, "page": page, "size": size, "items": items})
}
