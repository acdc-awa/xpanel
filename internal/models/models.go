// Package models 定义全部 GORM 模型，对应《系统设计方案》§5 数据库设计。
package models

import (
	"time"

	"gorm.io/gorm"
)

// 角色 / 状态常量
const (
	RoleAdmin = "admin"
	RoleUser  = "user"

	StatusActive   = 1 // 正常
	StatusDisabled = 0 // 禁用

	InviteUnused   = 0 // 邀请码未使用
	InviteUsed     = 1 // 邀请码已使用
	InviteDisabled = 2 // 邀请码已禁用/过期

	OrderPaid = "paid" // 订单状态（余额直付即时生效，无 pending/人工确认）

	// Phase T：入站三态
	InboundTypeUser  = "user"  // 进订阅、参与用户授权与 SyncUsers（默认）
	InboundTypeRelay = "relay" // 内部转发入站，被出站 InboundRef 引用，clients 固定为 InternalUUID
	InboundTypeIdle  = "idle"  // 闲置：未接线也未启用用户
)

// All 返回全部模型，供 AutoMigrate 使用。
func All() []any {
	return []any{
		&User{}, &InvitationCode{}, &GiftCard{}, &BalanceLog{},
		&Server{}, &Inbound{}, &PendingConfig{}, &PendingCert{},
		&ServerOutbound{}, &ServerRoutingRule{},
		&Plan{}, &Order{}, &Cert{},
		&TrafficLog{}, &TrafficDaily{}, &NodeReport{},
		&AuditLog{}, &Setting{},
		&PermissionGroup{}, &PermissionGroupInbound{},
	}
}

// AutoMigrate 建表/补列（生产环境由启动时执行，后续可切换为显式迁移）。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(All()...)
}

// 时间辅助
func Now() time.Time { return time.Now() }
