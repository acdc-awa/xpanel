package services

import (
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// OrderService 订单服务（余额直付：购买/续费套餐即时生效）。
// 2026-08-14 方向④：人工确认收款（manual 订单）已整体去除，充值=兑换码、购买=余额。
type OrderService struct {
	DB *gorm.DB
}

// payIdempotentWindow 余额直付幂等窗口：同用户+同套餐在此窗口内已支付则直接复用订单，防重放双扣款。
const payIdempotentWindow = 30 * time.Second

// PayWithBalance 用户使用账户余额直接支付套餐（原子扣款、生成已支付订单、顺延套餐、记流水）。
// 幂等：同用户+同套餐在 payIdempotentWindow 内已有 paid 订单 → 直接返回该订单（不重复扣款）。
func (s *OrderService) PayWithBalance(userID, planID uint64) (*models.Order, error) {
	var order *models.Order
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var plan models.Plan
		if err := tx.First(&plan, planID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("套餐不存在")
			}
			return err
		}
		if !plan.Enabled {
			return errors.New("套餐未上架")
		}

		// 幂等：窗口内已支付同套餐 → 直接复用
		var recent models.Order
		if err := tx.Where("user_id = ? AND plan_id = ? AND status = ? AND created_at >= ?",
			userID, planID, models.OrderPaid, time.Now().Add(-payIdempotentWindow)).
			Order("id DESC").First(&recent).Error; err == nil {
			order = &recent
			return nil
		}

		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return errors.New("用户不存在")
		}

		if user.BalanceCents < plan.PriceCents {
			return errors.New("账户余额不足，请先充值")
		}

		now := time.Now()
		newBalance := user.BalanceCents - plan.PriceCents
		if err := tx.Model(&user).Update("balance_cents", newBalance).Error; err != nil {
			return err
		}

		orderNo := now.Format("20060102150405") + util.RandomID(4)
		order = &models.Order{
			OrderNo:       orderNo,
			UserID:        userID,
			PlanID:        planID,
			AmountCents:   plan.PriceCents,
			PaymentMethod: models.PaymentMethodBalance,
			Status:        models.OrderPaid,
			PaidAt:        &now,
		}
		if err := tx.Create(order).Error; err != nil {
			return err
		}

		// 记录扣款流水
		log := models.BalanceLog{
			UserID:       userID,
			AmountCents:  -plan.PriceCents,
			BalanceAfter: newBalance,
			Type:         models.BalanceLogOrderPayment,
			RelatedID:    order.ID,
			Remark:       "余额购买套餐 #" + strconv.FormatUint(plan.ID, 10) + " (" + plan.Name + ")",
			CreatedAt:    now,
		}
		if err := tx.Create(&log).Error; err != nil {
			return err
		}

		// 顺延套餐有效期与重置周期
		base := now
		if user.ExpireAt != nil && user.ExpireAt.After(now) {
			base = *user.ExpireAt
		}
		newExpire := base.AddDate(0, 0, plan.DurationDays)

		updates := map[string]any{
			"plan_id":             plan.ID,
			"expire_at":           newExpire,
			"traffic_cycle_start": now,
		}
		if plan.PermissionGroupID > 0 {
			updates["permission_group_id"] = plan.PermissionGroupID
		}
		if err := tx.Model(&user).Updates(updates).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}

// ListByUser 用户订单列表。
func (s *OrderService) ListByUser(userID uint64) ([]models.Order, error) {
	var list []models.Order
	if err := s.DB.Where("user_id = ?", userID).Order("id DESC").Limit(50).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
