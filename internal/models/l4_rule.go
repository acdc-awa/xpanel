package models

import "time"

// ServerType 常量
const (
	ServerTypeXray    = "xray"     // 托管 Xray-core 计算节点（默认）
	ServerTypeL4Relay = "l4_relay" // 纯四层端口转发节点（无需安装 Xray Agent，提供端口中转）
)

// L4PortRule L4 端口转发规则（纯四层 TCP/UDP 转发，将中转服务器端口映射到目标节点的用户入站）。
type L4PortRule struct {
	ID              uint64    `gorm:"primaryKey" json:"id"`
	ServerID        uint64    `gorm:"index;not null" json:"server_id"`         // 所属 L4 中转服务器 ID
	ListenPort      int       `gorm:"not null" json:"listen_port"`             // 中转机监听端口
	TargetServerID  uint64    `gorm:"index;not null" json:"target_server_id"`  // 目标服务器 ID
	TargetInboundID uint64    `gorm:"index;not null" json:"target_inbound_id"` // 目标入站 ID
	Remark          string    `gorm:"size:255" json:"remark"`                  // 备注说明
	Enabled         bool      `gorm:"default:true" json:"enabled"`             // 是否启用
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// PermissionGroupL4Rule 权限组-L4 端口转发关联表（显式白名单多对多）。
// 只有显式绑定了权限组的 L4 规则才会为该权限组用户生成中转订阅节点（未绑定默认全部不可见）。
type PermissionGroupL4Rule struct {
	PermissionGroupID uint64    `gorm:"primaryKey;index:idx_pgl4_group" json:"permission_group_id"`
	L4RuleID          uint64    `gorm:"primaryKey;index:idx_pgl4_rule" json:"l4_rule_id"`
	CreatedAt         time.Time `json:"created_at"`
}
