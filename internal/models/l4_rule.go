package models

import "time"

// ServerType 常量
const (
	ServerTypeXray    = "xray"     // 托管 Xray-core 计算节点（默认）
	ServerTypeL4Relay = "l4_relay" // 纯四层端口转发节点（无需安装 Xray Agent，提供端口中转）
)

// L4PortRule L4 端口转发规则（纯四层 TCP/UDP 转发管道，将中转服务器端口映射到目标节点的用户入站）。
// 仅为传输管道定义，不含权限语义——访问控制由用户接入点（UserAccessPoint）白名单单点收口。
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
