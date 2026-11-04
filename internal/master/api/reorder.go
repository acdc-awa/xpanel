package api

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// 全量重排（卡片拖拽排序）的唯一后端实现。
//
// 契约：PUT .../order，body {"ids":[...]}，数组下标即新 sort_order。
// 只接受「整表重排」——ids 必须恰好覆盖 scope 内全部行；缺行/多行/重复都说明
// 页面数据已过期（期间有人新建/删除/归档），直接 400 拒绝且不落库。
//
// scope 限定参与排序的行集：nil 表示模型默认查询（含软删除语义，
// 例如 Plan 默认就排除已归档）；需要显式 Unscoped 时传 func(q) { return q.Unscoped() }。

// validOrder 判断 ids 是否是 all 的一个完整排列（无缺、无多、无重复）。
func validOrder(ids, all []uint64) bool {
	if len(ids) == 0 || len(ids) != len(all) {
		return false
	}
	seen := make(map[uint64]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return false
		}
		seen[id] = true
	}
	for _, id := range all {
		if !seen[id] {
			return false
		}
	}
	return true
}

// reorderAll 全量重排的公共实现：校验 + 事务内按下标写 sort_order + 统一响应。
func reorderAll(c *gin.Context, db *gorm.DB, model any, scope func(*gorm.DB) *gorm.DB, staleMsg string) {
	var req struct {
		IDs []uint64 `json:"ids" binding:"required,min=1,max=500"`
	}
	if !util.BindJSON(c, &req) {
		return
	}
	q := db.Model(model)
	if scope != nil {
		q = scope(q)
	}
	var allIDs []uint64
	if err := q.Order("id ASC").Pluck("id", &allIDs).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	if !validOrder(req.IDs, allIDs) {
		util.Fail(c, 400, staleMsg)
		return
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		for i, id := range req.IDs {
			// UpdateColumn 跳过钩子与 updated_at：调整排序不算业务修改。
			if err := tx.Model(model).Where("id = ?", id).UpdateColumn("sort_order", i).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		util.ServerError(c, "保存排序失败")
		return
	}
	util.OK(c, gin.H{"ordered": len(req.IDs)})
}

// nextSortOrder 新建行的默认排序 = scope 内当前最大值 + 1（追加末尾）。
// 存量重复值下同样安全：MAX 之后的序号一定排在所有已有序号之后。
func nextSortOrder(db *gorm.DB, model any, scope func(*gorm.DB) *gorm.DB) int {
	q := db.Model(model)
	if scope != nil {
		q = scope(q)
	}
	var maxOrder int
	if err := q.Select("COALESCE(MAX(sort_order), -1)").Scan(&maxOrder).Error; err != nil {
		return 0
	}
	return maxOrder + 1
}
