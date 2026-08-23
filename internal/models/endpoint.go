package models

import "time"

// InboundEndpoint 入站附加接入点（如 BGP 中转、IPv6 直连、备用 CDN 域名；与 Xray 物理监听解耦）。
// 一个入站可关联多个附加接入点，实现物理监听单开、订阅多线路派生与细粒度客制化分权。
type InboundEndpoint struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	InboundID uint64    `gorm:"index;not null" json:"inbound_id"` // 所属入站 ID
	Name      string    `gorm:"size:64;not null" json:"name"`     // 接入点名称后缀/备注，如 "广州 BGP 中转"、"IPv6"
	Host      string    `gorm:"size:255;not null" json:"host"`    // 覆写接入地址（域名或 IP）
	Port      int       `gorm:"not null" json:"port"`             // 覆写接入端口
	Enabled   bool      `gorm:"default:true" json:"enabled"`      // 是否启用
	Priority  int       `gorm:"default:0" json:"priority"`        // 排序优先级（数值越小越靠前）
	Remark    string    `gorm:"size:255" json:"remark"`           // 备注说明
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PermissionGroupEndpoint 权限组-附加接入点关联（多对多显式白名单授权）。
// 只有显式绑定了权限组的接入点才会对该权限组用户可见（未绑定默认全部不可见）。
type PermissionGroupEndpoint struct {
	PermissionGroupID uint64    `gorm:"primaryKey;index:idx_pge_group" json:"permission_group_id"`
	EndpointID        uint64    `gorm:"primaryKey;index:idx_pge_endpoint" json:"endpoint_id"`
	CreatedAt         time.Time `json:"created_at"`
}
