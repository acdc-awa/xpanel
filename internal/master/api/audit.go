package api

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

const (
	auditListDetailCap = 8 * 1024 // 列表接口单条 detail 截断（完整内容走 GET /:id）
	auditExportMaxRows = 50000    // 单次导出上限：超限提示缩小时间窗
	auditBackslash     = "\\"
)

// buildAuditQuery 组装列表/导出共用的筛选条件（唯一入口，避免两处口径漂移）。
func (d *Deps) buildAuditQuery(c *gin.Context) *gorm.DB {
	q := d.DB.Model(&models.AuditLog{})

	// 1. 精确 action 筛选
	if act := strings.TrimSpace(c.Query("action")); act != "" {
		q = q.Where("action = ?", act)
	}

	// 2. 模块分类筛选：按写入时落库的 category 列精确匹配（单源见 models.AuditCategory）。
	if cat := strings.TrimSpace(c.Query("category")); cat != "" {
		q = q.Where("category = ?", cat)
	}

	// 3. 关键词跨字段模糊搜索（action / detail / ip / 操作人快照 / 目标名快照）。
	// v5 起操作人邮箱与目标名是独立列，卡片上看到的名字因此「搜得到」；
	// 通配符按 SQL LIKE 语义转义，避免输入 % 或 _ 变成全表匹配。
	if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
		pat := "%" + escapeLike(kw) + "%"
		cond := "(action LIKE ? ESCAPE '\\' OR detail LIKE ? ESCAPE '\\' OR ip LIKE ? ESCAPE '\\'" +
			" OR operator_name LIKE ? ESCAPE '\\' OR target_name LIKE ? ESCAPE '\\')"
		q = q.Where(cond, pat, pat, pat, pat, pat)
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
	if name := strings.TrimSpace(c.Query("operator_name")); name != "" {
		q = q.Where("operator_name = ?", name)
	}

	// 5. 目标筛选（v5 新增）
	if tt := strings.TrimSpace(c.Query("target_type")); tt != "" {
		q = q.Where("target_type = ?", tt)
	}
	if tid := strings.TrimSpace(c.Query("target_id")); tid != "" {
		if v := atoiDefault(tid, 0); v > 0 {
			q = q.Where("target_id = ?", v)
		}
	}

	// 6. 结果筛选（v5 新增）：status 精确值，或用 result=success|fail 粗筛
	if st := strings.TrimSpace(c.Query("status")); st != "" {
		if v := atoiDefault(st, -1); v >= 0 {
			q = q.Where("status = ?", v)
		}
	}
	switch strings.ToLower(strings.TrimSpace(c.Query("result"))) {
	case "success":
		q = q.Where("status >= 200 AND status < 400")
	case "fail":
		q = q.Where("status >= 400")
	}

	// 7. 时间范围筛选（入库与查询统一为 UTC：进程本地时区已在 main 固定为 UTC，
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
	return q
}

// escapeLike 转义 LIKE 通配符（配合 ESCAPE '\'）。顺序必须是先反斜杠再 % 与 _。
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, auditBackslash, auditBackslash+auditBackslash)
	s = strings.ReplaceAll(s, "%", auditBackslash+"%")
	s = strings.ReplaceAll(s, "_", auditBackslash+"_")
	return s
}

// truncateAuditDetail 列表用截断：超过 cap 时按 rune 边界切，避免切出半个汉字。
func truncateAuditDetail(s string) (string, bool) {
	if len(s) <= auditListDetailCap {
		return s, false
	}
	cut := auditListDetailCap
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	if cut <= 0 {
		cut = auditListDetailCap
	}
	return s[:cut], true
}

// AuditLogView 审计列表/详情对外结构（列表与单条共用，避免字段漂移）。
type AuditLogView struct {
	ID              uint64    `json:"id"`
	OperatorType    string    `json:"operator_type"`
	OperatorID      uint64    `json:"operator_id"`
	OperatorName    string    `json:"operator_name"`
	Action          string    `json:"action"`
	Category        string    `json:"category"`
	CategoryLabel   string    `json:"category_label"`
	TargetType      string    `json:"target_type"`
	TargetID        uint64    `json:"target_id"`
	TargetName      string    `json:"target_name"`
	Status          int       `json:"status"`
	Detail          string    `json:"detail"`
	DetailTruncated bool      `json:"detail_truncated,omitempty"`
	IP              string    `json:"ip"`
	CreatedAt       time.Time `json:"created_at"`
}

// toAuditLogView 行 → 对外结构；truncate 控制是否对 detail 做列表级截断。
func (d *Deps) toAuditLogView(l models.AuditLog, operatorFallback string, truncate bool) AuditLogView {
	cat := auditCategoryOr(l.Category, l.Action)
	detail := l.Detail
	truncated := false
	if truncate {
		detail, truncated = truncateAuditDetail(detail)
	}
	return AuditLogView{
		ID: l.ID, OperatorType: l.OperatorType, OperatorID: l.OperatorID,
		OperatorName:  auditOperatorName(l.OperatorName, operatorFallback),
		Action:        l.Action,
		Category:      cat,
		CategoryLabel: models.AuditCategoryLabel(cat),
		TargetType:    l.TargetType, TargetID: l.TargetID, TargetName: l.TargetName,
		Status: l.Status, Detail: detail, DetailTruncated: truncated,
		IP: l.IP, CreatedAt: l.CreatedAt,
	}
}

// auditOperatorFallback 批量补历史行的操作人邮箱（仅快照为空时用；含软删用户）。
func (d *Deps) auditOperatorFallback(list []models.AuditLog) map[uint64]string {
	ids := make([]uint64, 0, len(list))
	seen := make(map[uint64]bool, len(list))
	for _, l := range list {
		if l.OperatorID != 0 && l.OperatorName == "" && !seen[l.OperatorID] {
			seen[l.OperatorID] = true
			ids = append(ids, l.OperatorID)
		}
	}
	out := make(map[uint64]string, len(ids))
	if len(ids) == 0 {
		return out
	}
	var operators []models.User
	if err := d.DB.Unscoped().Select("id", "email").Where("id IN ?", ids).Find(&operators).Error; err == nil {
		for _, u := range operators {
			out[u.ID] = u.Email
		}
	}
	return out
}

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
	q := d.buildAuditQuery(c)

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

	// 操作人显示名优先读写入时快照（operator_name 列）：改邮箱/删号都不会改写历史行。
	fallback := d.auditOperatorFallback(list)
	items := make([]AuditLogView, 0, len(list))
	for _, l := range list {
		items = append(items, d.toAuditLogView(l, fallback[l.OperatorID], true))
	}
	util.OK(c, gin.H{"total": total, "page": page, "size": size, "items": items})
}

// AdminAuditLogDetail GET /api/v1/admin/audit-logs/:id —— 单条完整 detail（列表截断后的全文）。
func (d *Deps) AdminAuditLogDetail(c *gin.Context) {
	id := uint64(atoiDefault(c.Param("id"), 0))
	if id == 0 {
		util.BadRequest(c, "无效的 ID")
		return
	}
	var row models.AuditLog
	if err := d.DB.First(&row, id).Error; err != nil {
		util.Fail(c, 404, "日志不存在")
		return
	}
	fallback := ""
	if row.OperatorName == "" && row.OperatorID > 0 {
		var u models.User
		if err := d.DB.Unscoped().Select("id", "email").First(&u, row.OperatorID).Error; err == nil {
			fallback = u.Email
		}
	}
	util.OK(c, d.toAuditLogView(row, fallback, false))
}

// AdminExportAuditLogs GET /api/v1/admin/audit-logs/export —— 按当前筛选导出 CSV。
// 上限 50000 行：超限不静默截断，直接提示缩小时间窗（导出一份不完整的取证材料比拒绝导出更危险）。
func (d *Deps) AdminExportAuditLogs(c *gin.Context) {
	q := d.buildAuditQuery(c)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	if total > auditExportMaxRows {
		util.BadRequest(c, fmt.Sprintf("匹配 %d 条，超过单次导出上限 %d 条，请缩小时间范围或补充筛选条件", total, auditExportMaxRows))
		return
	}
	var list []models.AuditLog
	if err := q.Order("id DESC").Limit(auditExportMaxRows).Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}

	fallback := d.auditOperatorFallback(list)
	filename := "audit-logs-" + time.Now().UTC().Format("20060102-150405") + ".csv"
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = c.Writer.Write([]byte("\ufeff")) // UTF-8 BOM：让 Excel 直接识别中文
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"时间(UTC)", "分类", "操作人", "操作", "目标类型", "目标", "结果", "IP", "详情"})
	for _, l := range list {
		_ = w.Write(auditCSVRow(l, fallback[l.OperatorID]))
	}
	w.Flush()

	// 导出本身也是敏感操作：留痕（正常写在本次查询之后，不进入本次导出）。
	d.audit(services.AuditEvent{
		OperatorType: "admin",
		OperatorID:   middleware.CurrentUser(c),
		Action:       "audit-logs.export",
		TargetType:   "",
		Status:       200,
		Message:      fmt.Sprintf("导出了 %d 条审计日志", len(list)),
		Extra:        map[string]any{"rows": len(list), "query": c.Request.URL.RawQuery},
		IP:           util.ClientIPFromContext(c),
	})
}

// auditCSVRow 单行 CSV。身份列沿用列表口径「名字 #id」：同名/同邮箱重建后只能靠 id 区分，
// 只写名字会让导出材料在「删了又建」的场景里失去唯一性。
func auditCSVRow(l models.AuditLog, operatorFallback string) []string {
	cat := auditCategoryOr(l.Category, l.Action)
	return []string{
		l.CreatedAt.UTC().Format("2006-01-02 15:04:05"),
		models.AuditCategoryLabel(cat),
		auditIdentityCell(auditOperatorName(l.OperatorName, operatorFallback), l.OperatorID),
		l.Action,
		l.TargetType,
		auditIdentityCell(l.TargetName, l.TargetID),
		strconv.Itoa(l.Status),
		l.IP,
		l.Detail,
	}
}

// auditIdentityCell 把「显示名 + 主键」拼成一个单元格；没有名字时至少留 #id。
func auditIdentityCell(name string, id uint64) string {
	switch {
	case name == "" && id > 0:
		return "#" + strconv.FormatUint(id, 10)
	case name != "" && id > 0:
		return name + " #" + strconv.FormatUint(id, 10)
	default:
		return name
	}
}

// auditCategoryOr 优先使用落库分类；空值（旧行/异常）按 action 现算，不返回空分类。
func auditCategoryOr(category, action string) string {
	if category != "" {
		return category
	}
	return models.AuditCategory(action)
}

// auditOperatorName 快照优先，缺失时用实时查询结果兜底（仅遗留行会走到）。
func auditOperatorName(snapshot, fallback string) string {
	if snapshot != "" {
		return snapshot
	}
	return fallback
}
