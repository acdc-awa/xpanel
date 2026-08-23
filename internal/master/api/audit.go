package api

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc/xray-panel/internal/models"
	"github.com/acdc/xray-panel/internal/pkg/util"
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
	if act := c.Query("action"); act != "" {
		q = q.Where("action = ?", act)
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
	type view struct {
		ID           uint64    `json:"id"`
		OperatorType string    `json:"operator_type"`
		OperatorID   uint64    `json:"operator_id"`
		Action       string    `json:"action"`
		Detail       string    `json:"detail"`
		IP           string    `json:"ip"`
		CreatedAt    time.Time `json:"created_at"`
	}
	items := make([]view, 0, len(list))
	for _, l := range list {
		items = append(items, view{
			ID: l.ID, OperatorType: l.OperatorType, OperatorID: l.OperatorID,
			Action: l.Action, Detail: l.Detail, IP: l.IP, CreatedAt: l.CreatedAt,
		})
	}
	util.OK(c, gin.H{"total": total, "page": page, "size": size, "items": items})
}
