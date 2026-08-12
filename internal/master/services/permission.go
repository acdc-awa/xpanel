package services

import (
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/models"
)

// GroupInboundIDs 返回套餐（Plan）绑定权限组后覆盖的入站 ID 集合（T4 动态授权）。
// planID=0 或未绑定权限组时返回空。不入库授权记录，订阅/生成时动态计算，避免授权表膨胀。
func GroupInboundIDs(db *gorm.DB, planID uint64) []uint64 {
	if planID == 0 {
		return nil
	}
	var plan models.Plan
	if err := db.First(&plan, planID).Error; err != nil || plan.PermissionGroupID == 0 {
		return nil
	}
	var links []models.PermissionGroupInbound
	if err := db.Where("permission_group_id = ?", plan.PermissionGroupID).Find(&links).Error; err != nil {
		return nil
	}
	ids := make([]uint64, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.InboundID)
	}
	return ids
}

// AuthorizedInboundSet 用户当前可用入站集合（UserInbound 授权 ∪ Plan→权限组集合）。
// 订阅过滤与热更新共用同一计算，保证口径一致。
func AuthorizedInboundSet(db *gorm.DB, user *models.User) map[uint64]bool {
	set := make(map[uint64]bool)
	var grants []models.UserInbound
	_ = db.Where("user_id = ? AND enabled = ?", user.ID, true).Find(&grants)
	for _, g := range grants {
		set[g.InboundID] = true
	}
	for _, id := range GroupInboundIDs(db, user.PlanID) {
		set[id] = true
	}
	return set
}
