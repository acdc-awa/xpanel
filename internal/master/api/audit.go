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

	// 2. 模块分类筛选：按写入时落库的 category 列精确匹配（单源见 models.AuditCategory）。
	// 旧版按 action 前缀 LIKE（每个分类一长串 OR）已退役；存量行由启动迁移回填分类。
	if cat := strings.TrimSpace(c.Query("category")); cat != "" {
		q = q.Where("category = ?", cat)
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

	// 5. 时间范围筛选（入库与查询统一为 UTC：进程本地时区已在 main 固定为 UTC，
	//    存量非 UTC 时间戳由 models.NormalizeTimesToUTC 在启动迁移时归一）
	if st := strings.TrimSpace(c.Query("start_time")); st != "" {
		if t, err := time.Parse(time.RFC3339, st); err == nil {
			q = q.Where("created_at >= ?", t.UTC())
		}
	}
	if et := strings.TrimSpace(c.Query("end_time")); et != "" {
		if t, err := time.Parse(time.RFC3339, et); err == nil {
			q = q.Where("created_at <= ?", t.UTC())
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
		Category         string    `json:"category"`
		CategoryLabel    string    `json:"category_label"`
		Detail           string    `json:"detail"`
		IP               string    `json:"ip"`
		CreatedAt        time.Time `json:"created_at"`
	}
	items := make([]view, 0, len(list))
	for _, l := range list {
		// 防御：回填前/异常行为空分类时按 action 现算，保证前端分类角标不缺失。
		cat := auditCategoryOr(l.Category, l.Action)
		items = append(items, view{
			ID: l.ID, OperatorType: l.OperatorType, OperatorID: l.OperatorID,
			OperatorUsername: usernameByOp[l.OperatorID],
			Action:           l.Action,
			Category:         cat,
			CategoryLabel:    models.AuditCategoryLabel(cat),
			Detail:           l.Detail, IP: l.IP, CreatedAt: l.CreatedAt,
		})
	}
	util.OK(c, gin.H{"total": total, "page": page, "size": size, "items": items})
}

// auditCategoryOr 优先使用落库分类；空值（旧行/异常）按 action 现算，不返回空分类。
func auditCategoryOr(category, action string) string {
	if category != "" {
		return category
	}
	return models.AuditCategory(action)
}
