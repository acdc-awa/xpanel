package services

import (
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/models"
)

// 访问控制单点化（2026-08-23）：
// 用户可见性/可注入性的唯一权威来源 = 用户接入点（UserAccessPoint）的权限组白名单。
// - 订阅生成：只从「用户生效权限组命中的启用 AP」产出节点（直连入站 / 经 L4 转发管道继承）。
// - 配置注入：入站 clients = 所有「解析到该入站的启用 AP」的权限组并集命中的用户。
// 零信任：AP 未绑定权限组 = 对全员不可见；用户无生效权限组 = 无任何节点。

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

// AccessPointPermissionGroupIDs 获取用户接入点绑定的开放权限组 ID 列表。
func AccessPointPermissionGroupIDs(db *gorm.DB, apID uint64) []uint64 {
	var links []models.PermissionGroupAccessPoint
	if err := db.Where("access_point_id = ?", apID).Find(&links).Error; err != nil {
		return nil
	}
	ids := make([]uint64, 0, len(links))
	for _, l := range links {
		ids = append(ids, l.PermissionGroupID)
	}
	return ids
}

// BatchAccessPointPermissionGroupIDs 批量查询用户接入点绑定的权限组映射 (apID -> []permissionGroupID)。
func BatchAccessPointPermissionGroupIDs(db *gorm.DB, apIDs []uint64) map[uint64][]uint64 {
	res := make(map[uint64][]uint64)
	if len(apIDs) == 0 {
		return res
	}
	var links []models.PermissionGroupAccessPoint
	if err := db.Where("access_point_id IN ?", apIDs).Find(&links).Error; err != nil {
		return res
	}
	for _, l := range links {
		res[l.AccessPointID] = append(res[l.AccessPointID], l.PermissionGroupID)
	}
	return res
}

// SyncAccessPointPermissionGroups 同步用户接入点的开放权限组（原子替换；空列表 = 全部不可见）。
func SyncAccessPointPermissionGroups(tx *gorm.DB, apID uint64, groupIDs []uint64) error {
	if err := tx.Where("access_point_id = ?", apID).Delete(&models.PermissionGroupAccessPoint{}).Error; err != nil {
		return err
	}
	if len(groupIDs) == 0 {
		return nil
	}
	rows := make([]models.PermissionGroupAccessPoint, 0, len(groupIDs))
	for _, gid := range groupIDs {
		if gid > 0 {
			rows = append(rows, models.PermissionGroupAccessPoint{PermissionGroupID: gid, AccessPointID: apID})
		}
	}
	if len(rows) > 0 {
		return tx.Create(&rows).Error
	}
	return nil
}

// ResolveAccessPointInboundID 解析接入点最终落地（消费）的用户入站 ID：
// 直连 = TargetInboundID；L4 中转 = 规则的目标入站。无法解析返回 0。
func ResolveAccessPointInboundID(ap *models.UserAccessPoint, l4Map map[uint64]models.L4PortRule) uint64 {
	if ap == nil {
		return 0
	}
	switch ap.TargetType {
	case "inbound":
		if ap.TargetInboundID != nil {
			return *ap.TargetInboundID
		}
	case "l4_rule":
		if ap.TargetL4RuleID != nil {
			if rule, ok := l4Map[*ap.TargetL4RuleID]; ok {
				return rule.TargetInboundID
			}
		}
	}
	return 0
}

// loadEnabledAPsAndL4 加载全部启用接入点与 L4 规则映射（AP 派生计算共用取数段）。
func loadEnabledAPsAndL4(db *gorm.DB) ([]models.UserAccessPoint, map[uint64]models.L4PortRule, map[uint64][]uint64) {
	var aps []models.UserAccessPoint
	_ = db.Where("enabled = ?", true).Find(&aps).Error
	var rules []models.L4PortRule
	_ = db.Where("enabled = ?", true).Find(&rules).Error
	l4Map := make(map[uint64]models.L4PortRule, len(rules))
	for _, r := range rules {
		l4Map[r.ID] = r
	}
	apIDs := make([]uint64, 0, len(aps))
	for _, ap := range aps {
		apIDs = append(apIDs, ap.ID)
	}
	return aps, l4Map, BatchAccessPointPermissionGroupIDs(db, apIDs)
}

// AuthorizedInboundSet 用户当前可用入站集合（AP 单点授权派生）。
// 计算逻辑：用户生效权限组命中的启用 AP → 解析落地入站（直连 / 经 L4 转发）。
func AuthorizedInboundSet(db *gorm.DB, user *models.User) map[uint64]bool {
	set := make(map[uint64]bool)
	if user == nil {
		return set
	}
	groupID := UserEffectiveGroupID(db, user)
	if groupID == 0 {
		return set
	}
	aps, l4Map, apGroupMap := loadEnabledAPsAndL4(db)
	for i := range aps {
		ap := &aps[i]
		if !groupHit(apGroupMap[ap.ID], groupID) {
			continue
		}
		if inbID := ResolveAccessPointInboundID(ap, l4Map); inbID > 0 {
			set[inbID] = true
		}
	}
	return set
}

// BatchInboundAuthorizedGroupIDs 批量计算入站的授权权限组映射（inboundID -> []permissionGroupID），
// 由启用 AP 白名单派生：AP 直连入站 / 经 L4 规则解析到目标入站，AP 的开放组并入该入站的授权组集。
// 配置生成（GetValidUsers）与用户注入的唯一权威来源。
func BatchInboundAuthorizedGroupIDs(db *gorm.DB, inboundIDs []uint64) map[uint64][]uint64 {
	res := make(map[uint64][]uint64)
	if len(inboundIDs) == 0 {
		return res
	}
	wanted := make(map[uint64]bool, len(inboundIDs))
	for _, id := range inboundIDs {
		wanted[id] = true
	}
	aps, l4Map, apGroupMap := loadEnabledAPsAndL4(db)
	sets := make(map[uint64]map[uint64]bool)
	for i := range aps {
		ap := &aps[i]
		inbID := ResolveAccessPointInboundID(ap, l4Map)
		if inbID == 0 || !wanted[inbID] {
			continue
		}
		if sets[inbID] == nil {
			sets[inbID] = make(map[uint64]bool)
		}
		for _, gid := range apGroupMap[ap.ID] {
			sets[inbID][gid] = true
		}
	}
	for inbID, gs := range sets {
		ids := make([]uint64, 0, len(gs))
		for gid := range gs {
			ids = append(ids, gid)
		}
		res[inbID] = ids
	}
	return res
}

// groupHit 判断权限组集合是否命中目标组。
func groupHit(groupIDs []uint64, groupID uint64) bool {
	for _, gid := range groupIDs {
		if gid == groupID {
			return true
		}
	}
	return false
}

// AuthorizedEntryServerIDs 用户可见接入点的「入口服务器」集合（用户端节点可用性展示用）：
// 直连 = 目标入站所在服务器；L4 中转 = 中转机服务器（用户实际连接的入口）。
func AuthorizedEntryServerIDs(db *gorm.DB, user *models.User) map[uint64]bool {
	set := make(map[uint64]bool)
	if user == nil {
		return set
	}
	groupID := UserEffectiveGroupID(db, user)
	if groupID == 0 {
		return set
	}
	aps, l4Map, apGroupMap := loadEnabledAPsAndL4(db)
	var inbs []models.Inbound
	_ = db.Where("enabled = ? AND type = ?", true, models.InboundTypeUser).Find(&inbs).Error
	inbServer := make(map[uint64]uint64, len(inbs))
	for _, inb := range inbs {
		inbServer[inb.ID] = inb.ServerID
	}
	for i := range aps {
		ap := &aps[i]
		if !groupHit(apGroupMap[ap.ID], groupID) {
			continue
		}
		switch ap.TargetType {
		case "inbound":
			if ap.TargetInboundID != nil {
				if sid, ok := inbServer[*ap.TargetInboundID]; ok {
					set[sid] = true
				}
			}
		case "l4_rule":
			if ap.TargetL4RuleID != nil {
				if rule, ok := l4Map[*ap.TargetL4RuleID]; ok {
					// 入口 = 中转机；目标入站可用才展示（管道完整）
					if _, ok := inbServer[rule.TargetInboundID]; ok {
						set[rule.ServerID] = true
					}
				}
			}
		}
	}
	return set
}
