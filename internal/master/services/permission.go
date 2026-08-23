package services

import (
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/models"
)

// UserEffectiveGroupID 计算用户生效的权限组 ID。
// 规则：若 user.PermissionGroupID > 0（管理员显式指定）则优先使用；
// 否则若 user.PlanID > 0，则继承套餐绑定的 plan.PermissionGroupID；
// 否则返回 0。
func UserEffectiveGroupID(db *gorm.DB, user *models.User) uint64 {
	if user == nil {
		return 0
	}
	if user.PermissionGroupID > 0 {
		return user.PermissionGroupID
	}
	if user.PlanID > 0 {
		var plan models.Plan
		if err := db.Select("id, permission_group_id").First(&plan, user.PlanID).Error; err == nil {
			return plan.PermissionGroupID
		}
	}
	return 0
}

// UserEffectiveDeviceLimit 计算用户生效的最大在线设备限制。
// 规则：
// 1. 若 user.DeviceLimit > 0（管理员单独为该用户指定），则优先使用并标记 custom=true；
// 2. 否则若 user.PlanID > 0，则继承当前套餐 plan.DeviceLimit（custom=false）；
// 3. 否则返回 0（不限，custom=false）。
func UserEffectiveDeviceLimit(db *gorm.DB, user *models.User) (limit int, custom bool) {
	if user == nil {
		return 0, false
	}
	if user.DeviceLimit > 0 {
		return user.DeviceLimit, true
	}
	if user.PlanID > 0 {
		var plan models.Plan
		if err := db.Select("id, device_limit").First(&plan, user.PlanID).Error; err == nil {
			return plan.DeviceLimit, false
		}
	}
	return 0, false
}

// InboundIDsByGroupID 返回属于指定权限组的所有入站 ID 集合。
func InboundIDsByGroupID(db *gorm.DB, groupID uint64) []uint64 {
	if groupID == 0 {
		return nil
	}
	var links []models.PermissionGroupInbound
	if err := db.Where("permission_group_id = ?", groupID).Find(&links).Error; err != nil {
		return nil
	}
	ids := make([]uint64, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.InboundID)
	}
	return ids
}

// GroupInboundIDs 保持向后兼容：根据套餐 ID 返回绑定权限组覆盖的入站 ID 集合。
func GroupInboundIDs(db *gorm.DB, planID uint64) []uint64 {
	if planID == 0 {
		return nil
	}
	var plan models.Plan
	if err := db.Select("id, permission_group_id").First(&plan, planID).Error; err != nil || plan.PermissionGroupID == 0 {
		return nil
	}
	return InboundIDsByGroupID(db, plan.PermissionGroupID)
}

// AuthorizedInboundSet 用户当前可用入站集合（纯净权限组模式，Xboard 架构）。
// 计算逻辑：根据用户有效权限组（User.PermissionGroupID 覆盖 -> Plan.PermissionGroupID 继承）匹配入站。
func AuthorizedInboundSet(db *gorm.DB, user *models.User) map[uint64]bool {
	set := make(map[uint64]bool)
	if user == nil {
		return set
	}
	groupID := UserEffectiveGroupID(db, user)
	if groupID > 0 {
		for _, id := range InboundIDsByGroupID(db, groupID) {
			set[id] = true
		}
	}
	return set
}

// InboundPermissionGroupIDs 查询某个入站绑定的所有权限组 ID 集合。
func InboundPermissionGroupIDs(db *gorm.DB, inboundID uint64) []uint64 {
	var links []models.PermissionGroupInbound
	if err := db.Where("inbound_id = ?", inboundID).Find(&links).Error; err != nil {
		return nil
	}
	ids := make([]uint64, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.PermissionGroupID)
	}
	return ids
}

// BatchInboundPermissionGroupIDs 批量查询入站绑定的权限组映射 (inboundID -> []permissionGroupID)。
func BatchInboundPermissionGroupIDs(db *gorm.DB, inboundIDs []uint64) map[uint64][]uint64 {
	res := make(map[uint64][]uint64)
	if len(inboundIDs) == 0 {
		return res
	}
	var links []models.PermissionGroupInbound
	if err := db.Where("inbound_id IN ?", inboundIDs).Find(&links).Error; err != nil {
		return res
	}
	for _, l := range links {
		res[l.InboundID] = append(res[l.InboundID], l.PermissionGroupID)
	}
	return res
}

// SyncInboundPermissionGroups 同步入站的开放权限组（原子替换）。
func SyncInboundPermissionGroups(tx *gorm.DB, inboundID uint64, groupIDs []uint64) error {
	if err := tx.Where("inbound_id = ?", inboundID).Delete(&models.PermissionGroupInbound{}).Error; err != nil {
		return err
	}
	if len(groupIDs) == 0 {
		return nil
	}
	rows := make([]models.PermissionGroupInbound, 0, len(groupIDs))
	for _, gid := range groupIDs {
		if gid > 0 {
			rows = append(rows, models.PermissionGroupInbound{PermissionGroupID: gid, InboundID: inboundID})
		}
	}
	if len(rows) > 0 {
		return tx.Create(&rows).Error
	}
	return nil
}
