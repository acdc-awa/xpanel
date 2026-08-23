package models

import "time"

// PermissionGroup 权限组：一个用户属于一个权限组。
// 访问控制单点化：权限组通过「用户接入点（UserAccessPoint）」白名单决定用户可见的订阅入口；
// 套餐绑定权限组后，购买套餐自动获得该组所有接入点的访问权限。
type PermissionGroup struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"size:64;uniqueIndex;not null" json:"name"`
	Remark        string    `gorm:"size:255" json:"remark"`
	ClashTemplate string    `gorm:"type:text" json:"clash_template,omitempty"` // 自定义 Clash/Mihomo 订阅模板（留空则走系统内置默认模板）
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
