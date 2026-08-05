package models

import (
	"time"
)

// User 用户。plan_id / expire_at 是对《系统设计方案》§5 简表的补充，
// 对应 §4.1「用户详情（当前套餐、剩余流量、到期时间）」。
type User struct {
	ID             uint64     `gorm:"primaryKey" json:"id"`
	Username       string     `gorm:"size:32;uniqueIndex;not null" json:"username"`
	UUID           string     `gorm:"size:36;uniqueIndex" json:"-"` // VLESS 用户账号（UUID v4）
	Email          string     `gorm:"size:128;uniqueIndex" json:"email"`
	PasswordHash   string     `gorm:"size:255;not null" json:"-"`
	Role           string     `gorm:"size:16;default:user;index" json:"role"`
	Status         int        `gorm:"default:1;index" json:"status"`
	SubscribeToken string     `gorm:"size:64;uniqueIndex" json:"-"`
	PlanID         uint64     `gorm:"index" json:"plan_id"`                 // 当前套餐（冗余，P4 前可为 0）
	ExpireAt       *time.Time `json:"expire_at"`                            // 当前套餐到期时间
	MustChangePwd  bool       `gorm:"default:false" json:"must_change_pwd"` // 必须修改密码
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// InvitationCode 邀请码（一次性，可设过期）。
type InvitationCode struct {
	ID        uint64     `gorm:"primaryKey" json:"id"`
	Code      string     `gorm:"size:32;uniqueIndex;not null" json:"code"`
	CreatedBy uint64     `gorm:"index" json:"created_by"`
	UsedBy    uint64     `gorm:"index" json:"used_by"`
	UsedAt    *time.Time `json:"used_at"`
	ExpiresAt *time.Time `json:"expires_at"`
	Status    int        `gorm:"default:0;index" json:"status"`
	CreatedAt time.Time  `json:"created_at"`
}
