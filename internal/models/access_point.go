package models

import "time"

// UserAccessPoint 用户接入点（消费者模型：命名 + 权限组白名单 + 可选对外端点覆写，数据沿管道自适应继承）
type UserAccessPoint struct {
	ID              uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name            string    `gorm:"type:varchar(128);not null" json:"name"`                    // 接入点显示名称，如 "🇭🇰 香港直连", "🇨🇳 广州移动 10G BGP"
	CustomHost      string    `gorm:"type:varchar(255);default:''" json:"custom_host,omitempty"` // 可选自定义域名覆写（默认空，直连=入站分享地址；中转端点同样用此字段）
	CustomPort      int       `gorm:"default:0" json:"custom_port,omitempty"`                    // 可选自定义端口覆写（默认 0，自动继承目标入站端口）
	TargetType      string    `gorm:"type:varchar(32);not null;default:''" json:"target_type"`   // "inbound" | ""（2026-08-24 起 L4 中转建模退役，端点由 CustomHost/Port 覆写表达）
	TargetInboundID *uint64   `gorm:"index" json:"target_inbound_id"`                            // 连线目标入站 ID
	Enabled         bool      `gorm:"not null;default:true" json:"enabled"`
	Remark          string    `gorm:"type:varchar(255)" json:"remark"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (UserAccessPoint) TableName() string {
	return "user_access_points"
}

// PermissionGroupAccessPoint 权限组与用户接入点的多对多关联（显式白名单权限模型）
type PermissionGroupAccessPoint struct {
	PermissionGroupID uint64 `gorm:"primaryKey;autoIncrement:false" json:"permission_group_id"`
	AccessPointID     uint64 `gorm:"primaryKey;autoIncrement:false;index" json:"access_point_id"`
}

func (PermissionGroupAccessPoint) TableName() string {
	return "permission_group_access_points"
}
