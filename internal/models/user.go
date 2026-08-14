package models

import (
	"time"

	"gorm.io/gorm"
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
	PlanID              uint64     `gorm:"index" json:"plan_id"`
	ExpireAt            *time.Time `json:"expire_at"`
	BalanceCents        int64      `gorm:"default:0;not null" json:"balance_cents"`     // 账户余额（分）
	DeviceLimit         int        `gorm:"default:0" json:"device_limit"`              // 自定义设备数限制（0=继承套餐）
	PermissionGroupID   uint64     `gorm:"index;default:0" json:"permission_group_id"` // 所属权限组（0=未分组）
	TrafficCycleStart   time.Time  `json:"traffic_cycle_start"`                        // 当前计费周期起点（流量只算此后）
	MustChangePwd       bool       `gorm:"default:false" json:"must_change_pwd"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// BeforeCreate 自动设置流量周期起点（首次创建时）。
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.TrafficCycleStart.IsZero() {
		u.TrafficCycleStart = time.Now()
	}
	return nil
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
