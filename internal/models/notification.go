package models

import "time"

// NotificationDismissal 管理员对某条派生提醒的忽略记录。
//
// 提醒本身不落库：它由当前状态派生（服务器到期、Agent/面板版本落后），条件解除时自然消失。
// 因此「已读」只能是「忽略」，并且必须带 Fingerprint —— 条件没变化时不再出现，
// 条件升级（VPS 从 7 天变成 3 天、版本差拉大）时指纹变化，提醒重新出现。
type NotificationDismissal struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	AdminID     uint64    `gorm:"uniqueIndex:idx_notification_dismissal_admin_key,priority:1" json:"admin_id"`
	Key         string    `gorm:"size:128;uniqueIndex:idx_notification_dismissal_admin_key,priority:2" json:"key"`
	Fingerprint string    `gorm:"size:64" json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}

// TableName 显式命名，避免 GORM 复数化规则变化影响存量表。
func (NotificationDismissal) TableName() string { return "notification_dismissals" }
