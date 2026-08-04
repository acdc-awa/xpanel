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

	OrderPending   = "pending"
	OrderPaid      = "paid"
	OrderCancelled = "cancelled"
)

// All 返回全部模型，供 AutoMigrate 使用。
func All() []any {
	return []any{
		&User{}, &InvitationCode{},
		&Server{}, &Inbound{}, &UserInbound{}, &PendingConfig{},
		&ServerOutbound{}, &ServerRoutingRule{},
		&Plan{}, &Order{},
		&TrafficLog{}, &TrafficDaily{}, &NodeReport{},
		&AuditLog{}, &Setting{},
	}
}

// AutoMigrate 建表/补列（生产环境由启动时执行，后续可切换为显式迁移）。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(All()...)
}

// 时间辅助
func Now() time.Time { return time.Now() }