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
	// 套餐快照三列（2026-09-01 Xboard 式隔离）：购买/续费/管理员分配套餐时从 Plan 复制，
	// 判定链（filterValidUsers/订阅/展示/超额处置）只读快照、不实时 join plans——
	// 套餐编辑默认零影响存量用户（仅新购/续费生效），勾选「同步存量用户」才批量重快照。
	PlanTrafficBytes int64  `gorm:"default:0" json:"plan_traffic_bytes"` // 流量额度快照（字节，0=不限）
	PlanDeviceLimit  int    `gorm:"default:0" json:"plan_device_limit"`  // 设备限制快照（0=不限）
	PlanGroupID      uint64 `gorm:"default:0" json:"plan_group_id"`      // 权限组快照（0=不绑定）
	MustChangePwd    bool   `gorm:"default:false" json:"must_change_pwd"`
	// 会话吊销版本号（改密/重置密码/封禁后 bump；JWT claims 携带，refresh 时校验）
	TokenVersion uint32 `gorm:"default:0" json:"-"`
	// TOTP 2FA（2026-08-14 方向③）：secret AES 加密存储；BackupCodes 为 bcrypt 哈希 JSON 数组
	TotpSecret      string     `gorm:"size:512" json:"-"`
	TotpEnabled     bool       `gorm:"default:false" json:"totp_enabled"`
	TotpFailedCount int        `gorm:"default:0" json:"-"`
	TotpLockedUntil *time.Time `json:"-"`
	BackupCodes     string     `gorm:"type:text" json:"-"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate 自动设置流量周期起点（首次创建时）。
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.TrafficCycleStart.IsZero() {
		u.TrafficCycleStart = time.Now()
	}
	return nil
}

// EffectiveTrafficBytes 套餐流量额度快照（字节，0=不限）。
func (u *User) EffectiveTrafficBytes() int64 { return u.PlanTrafficBytes }

// EffectiveDeviceLimit 生效设备限制（用户自定义优先，fallback 快照；0=不限）。
func (u *User) EffectiveDeviceLimit() int {
	if u.DeviceLimit > 0 {
		return u.DeviceLimit
	}
	return u.PlanDeviceLimit
}

// EffectiveGroupID 生效权限组（用户显式分组优先，fallback 快照；0=未分组）。
func (u *User) EffectiveGroupID() uint64 {
	if u.PermissionGroupID > 0 {
		return u.PermissionGroupID
	}
	return u.PlanGroupID
}

// ApplyPlanSnapshot 从套餐复制快照（购买/续费/管理员分配套餐共用；Xboard 语义：
// 分配即按当前套餐值快照，此后套餐编辑不影响该用户直至下次分配/同步）。
func (u *User) ApplyPlanSnapshot(p *Plan) {
	u.PlanID = p.ID
	u.PlanTrafficBytes = p.TrafficGB * 1024 * 1024 * 1024
	u.PlanDeviceLimit = p.DeviceLimit
	u.PlanGroupID = p.PermissionGroupID
}

// PlanSnapshotColumns 快照三列的 UPDATE map（订单事务/批量同步共用）。
func PlanSnapshotColumns(p *Plan) map[string]any {
	return map[string]any{
		"plan_traffic_bytes": p.TrafficGB * 1024 * 1024 * 1024,
		"plan_device_limit":  p.DeviceLimit,
		"plan_group_id":      p.PermissionGroupID,
	}
}

// ClearPlanSnapshotColumns 解绑套餐（plan_id=0）时清空快照。
func ClearPlanSnapshotColumns() map[string]any {
	return map[string]any{
		"plan_traffic_bytes": 0,
		"plan_device_limit":  0,
		"plan_group_id":      0,
	}
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
