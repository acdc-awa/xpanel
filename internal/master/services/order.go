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

// OrderService 订单服务（人工转账确认 + 余额直接支付）。
type OrderService struct {
	DB *gorm.DB
}

// Create 用户创建待支付订单（线下转账模式）。
func (s *OrderService) Create(userID, planID uint64) (*models.Order, error) {
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
		orderNo := time.Now().Format("20060102150405") + util.RandomID(4)
		order = &models.Order{
			OrderNo:       orderNo,
			UserID:        userID,
			PlanID:        planID,
			AmountCents:   plan.PriceCents,
			PaymentMethod: models.PaymentMethodManual,
			Status:        models.OrderPending,
		}
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}

// PayWithBalance 用户使用账户余额直接支付套餐（原子扣款、生成已支付订单、顺延套餐、记流水）。
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

// Confirm 管理员确认收款：订单 → paid，用户套餐生效（plan_id + expire_at 顺延）。
func (s *OrderService) Confirm(orderID, adminID uint64) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var order models.Order
		if err := tx.First(&order, orderID).Error; err != nil {
			return errors.New("订单不存在")
		}
		if order.Status != models.OrderPending {
			return errors.New("订单不是待确认状态")
		}
		var plan models.Plan
		if err := tx.First(&plan, order.PlanID).Error; err != nil {
			return errors.New("套餐不存在")
		}
		var user models.User
		if err := tx.First(&user, order.UserID).Error; err != nil {
			return errors.New("用户不存在")
		}

		// 套餐生效：到期时间顺延
		now := time.Now()
		base := now
		if user.ExpireAt != nil && user.ExpireAt.After(now) {
			base = *user.ExpireAt
		}
		newExpire := base.AddDate(0, 0, plan.DurationDays)

		now2 := time.Now()
		if err := tx.Model(&order).Updates(map[string]any{
			"status":           models.OrderPaid,
			"paid_at":          now2,
			"confirm_admin_id": adminID,
		}).Error; err != nil {
			return err
		}
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
}

// Cancel 管理员取消订单。
func (s *OrderService) Cancel(orderID, adminID uint64) error {
	var order models.Order
	if err := s.DB.First(&order, orderID).Error; err != nil {
		return errors.New("订单不存在")
	}
	if order.Status != models.OrderPending {
		return errors.New("订单不是待确认状态")
	}
	return s.DB.Model(&order).Updates(map[string]any{
		"status":           models.OrderCancelled,
		"confirm_admin_id": adminID,
	}).Error
}

// ListByUser 用户订单列表。
func (s *OrderService) ListByUser(userID uint64) ([]models.Order, error) {
	var list []models.Order
	if err := s.DB.Where("user_id = ?", userID).Order("id DESC").Limit(50).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}
