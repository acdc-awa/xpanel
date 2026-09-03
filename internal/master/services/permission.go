package services

import (
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// 访问控制单点化（2026-08-23）：
// 用户可见性/可注入性的唯一权威来源 = 用户接入点（UserAccessPoint）的权限组白名单。
// - 订阅生成：只从「用户生效权限组命中的启用 AP」产出节点（直连目标入站，端点可被 CustomHost/Port 覆写）。
// - 配置注入：入站 clients = 所有「解析到该入站的启用 AP」的权限组并集命中的用户。
// 零信任：AP 未绑定权限组 = 对全员不可见；用户无生效权限组 = 无任何节点。
// （用户生效组/设备限制已收口为 models.User 的 Effective* 快照方法，2026-09-01 套餐快照化。）

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
// 2026-09-03 组内排序保留式同步：该 AP 既有绑定保留原 sort_order（不冲掉各权限组内已排好的
// 优先级）；新增绑定取目标组当前 max(sort_order)+1 追加到组内末尾。
func SyncAccessPointPermissionGroups(tx *gorm.DB, apID uint64, groupIDs []uint64) error {
	var existing []models.PermissionGroupAccessPoint
	if err := tx.Where("access_point_id = ?", apID).Find(&existing).Error; err != nil {
		return err
	}
	oldOrder := make(map[uint64]int, len(existing))
	for _, l := range existing {
		oldOrder[l.PermissionGroupID] = l.SortOrder
	}
	if err := tx.Where("access_point_id = ?", apID).Delete(&models.PermissionGroupAccessPoint{}).Error; err != nil {
		return err
	}
	if len(groupIDs) == 0 {
		return nil
	}
	rows := make([]models.PermissionGroupAccessPoint, 0, len(groupIDs))
	for _, gid := range groupIDs {
		if gid == 0 {
			continue
		}
		so, ok := oldOrder[gid]
		if !ok {
			// 新绑定：追加到该组已有排序末尾（无绑定时 -1+1=0）
			var maxSO int
			if err := tx.Model(&models.PermissionGroupAccessPoint{}).
				Where("permission_group_id = ?", gid).
				Select("COALESCE(MAX(sort_order), -1)").
				Scan(&maxSO).Error; err != nil {
				return err
			}
			so = maxSO + 1
		}
		rows = append(rows, models.PermissionGroupAccessPoint{PermissionGroupID: gid, AccessPointID: apID, SortOrder: so})
	}
	if len(rows) > 0 {
		return tx.Create(&rows).Error
	}
	return nil
}

// SetGroupAccessPointOrder 组视角原子重排（2026-09-03）：权限组编辑器提交**有序**接入点 ID 列表，
// 组内绑定全量替换、sort_order = 数组下标。校验由调用方完成（AP 必须存在）。
func SetGroupAccessPointOrder(tx *gorm.DB, groupID uint64, orderedAPIDs []uint64) error {
	if err := tx.Where("permission_group_id = ?", groupID).Delete(&models.PermissionGroupAccessPoint{}).Error; err != nil {
		return err
	}
	rows := make([]models.PermissionGroupAccessPoint, 0, len(orderedAPIDs))
	for i, apID := range orderedAPIDs {
		if apID == 0 {
			continue
		}
		rows = append(rows, models.PermissionGroupAccessPoint{
			PermissionGroupID: groupID,
			AccessPointID:     apID,
			SortOrder:         i,
		})
	}
	if len(rows) > 0 {
		return tx.Create(&rows).Error
	}
	return nil
}

// ResolveAccessPointInboundID 解析接入点最终落地（消费）的用户入站 ID。
// AP 只直连入站（L4 中转建模已退役，端点由 CustomHost/Port 覆写表达）。无法解析返回 0。
func ResolveAccessPointInboundID(ap *models.UserAccessPoint) uint64 {
	if ap == nil || ap.TargetType != "inbound" || ap.TargetInboundID == nil {
		return 0
	}
	return *ap.TargetInboundID
}

// loadEnabledAPs 加载全部启用接入点与其权限组映射（AP 派生计算共用取数段）。
func loadEnabledAPs(db *gorm.DB) ([]models.UserAccessPoint, map[uint64][]uint64) {
	var aps []models.UserAccessPoint
	_ = db.Where("enabled = ?", true).Find(&aps).Error
	apIDs := make([]uint64, 0, len(aps))
	for _, ap := range aps {
		apIDs = append(apIDs, ap.ID)
	}
	return aps, BatchAccessPointPermissionGroupIDs(db, apIDs)
}

// BatchInboundAuthorizedGroupIDs 批量计算入站的授权权限组映射（inboundID -> []permissionGroupID），
// 由启用 AP 白名单派生：AP 直连入站，AP 的开放组并入该入站的授权组集。
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
	aps, apGroupMap := loadEnabledAPs(db)
	sets := make(map[uint64]map[uint64]bool)
	for i := range aps {
		ap := &aps[i]
		inbID := ResolveAccessPointInboundID(ap)
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
// 入口 = 目标入站所在服务器（用户实际连接的对外端点由 AP 覆写/接入层决议，展示归服务器维度）。
func AuthorizedEntryServerIDs(db *gorm.DB, user *models.User) map[uint64]bool {
	set := make(map[uint64]bool)
	if user == nil {
		return set
	}
	groupID := user.EffectiveGroupID()
	if groupID == 0 {
		return set
	}
	aps, apGroupMap := loadEnabledAPs(db)
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
		if ap.TargetType == "inbound" && ap.TargetInboundID != nil {
			if sid, ok := inbServer[*ap.TargetInboundID]; ok {
				set[sid] = true
			}
		}
	}
	return set
}
