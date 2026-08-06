package models

import "time"

// PermissionGroup 权限组：一个用户属于一个权限组，一个权限组包含多个入站。
// 套餐绑定权限组后，购买套餐自动获得该组所有入站的访问权限。
type PermissionGroup struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;uniqueIndex;not null" json:"name"`
	Remark    string    `gorm:"size:255" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PermissionGroupInbound 权限组-入站关联（多对多）。
type PermissionGroupInbound struct {
	PermissionGroupID uint64    `gorm:"primaryKey;index:idx_pgi_group" json:"permission_group_id"`
	InboundID         uint64    `gorm:"primaryKey;index:idx_pgi_inbound" json:"inbound_id"`
	CreatedAt         time.Time `json:"created_at"`
}
