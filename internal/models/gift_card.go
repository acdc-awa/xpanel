package models

import (
	"time"
)

// 礼品卡与账务状态常量
const (
	GiftCardUnused   = "unused"   // 未使用
	GiftCardUsed     = "used"     // 已使用
	GiftCardDisabled = "disabled" // 已作废

	BalanceLogRechargeGiftCard = "recharge_gift_card" // 礼品卡充值
	BalanceLogOrderPayment     = "order_payment"     // 订单余额支付
	BalanceLogAdminAdjust      = "admin_adjust"      // 管理员调账
	BalanceLogRefund           = "refund"            // 退款

	PaymentMethodBalance = "balance" // 余额支付
	PaymentMethodManual  = "manual"  // 人工/线下转账
)

// GiftCard 礼品卡（卡密充值）。
type GiftCard struct {
	ID             uint64     `gorm:"primaryKey" json:"id"`
	Code           string     `gorm:"size:64;uniqueIndex;not null" json:"code"` // 唯一卡密
	Name           string     `gorm:"size:64" json:"name"`                      // 批次名称/备注
	FaceValueCents int64      `gorm:"not null" json:"face_value_cents"`         // 面值（分）
	Status         string     `gorm:"size:16;default:unused;index" json:"status"`// unused/used/disabled
	UsedBy         uint64     `gorm:"index" json:"used_by"`                     // 兑换用户 ID
	UsedAt         *time.Time `json:"used_at"`                                  // 兑换时间
	ExpiresAt      *time.Time `json:"expires_at"`                               // 过期时间（nil 为永久）
	CreatedBy      uint64     `gorm:"index" json:"created_by"`                  // 创建管理员 ID
	CreatedAt      time.Time  `json:"created_at"`
}

// BalanceLog 余额变动明细账本。
type BalanceLog struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	UserID       uint64    `gorm:"index;not null" json:"user_id"`
	AmountCents  int64     `gorm:"not null" json:"amount_cents"`              // 变动金额（+充值 / -扣款）
	BalanceAfter int64     `gorm:"not null" json:"balance_after"`             // 变动后余额（分）
	Type         string    `gorm:"size:32;not null;index" json:"type"`        // recharge_gift_card / order_payment / admin_adjust
	RelatedID    uint64    `gorm:"index" json:"related_id"`                   // 关联 ID（GiftCard ID 或 Order ID）
	Remark       string    `gorm:"size:255" json:"remark"`                    // 备注描述
	CreatedAt    time.Time `json:"created_at"`
}
