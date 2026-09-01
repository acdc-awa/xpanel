// Package billing 承载订单、余额、礼品卡等资金/账务领域逻辑。
package billing

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// OrderService 订单服务（余额直付：购买/续费套餐即时生效）。
// 2026-08-14 方向④：人工确认收款（manual 订单）已整体去除，充值=兑换码、购买=余额。
// 2026-08-23 Stage 8：存储访问收口 contracts.BillingStore（GORM 实现见 store/gormstore）。
// Events（Stage 5）：事务提交后发布 OrderPaidEvent；nil 时不发布（兼容测试与旧构造）。
type OrderService struct {
	store  contracts.BillingStore
	Events contracts.EventPublisher
}

// NewOrderService 构造订单服务。
func NewOrderService(store contracts.BillingStore) *OrderService {
	return &OrderService{store: store}
}

// payIdempotentWindow 余额直付幂等窗口：同用户+同套餐在此窗口内已支付则直接复用订单，防重放双扣款。
const payIdempotentWindow = 30 * time.Second

// PayWithBalance 用户使用账户余额直接支付套餐（原子扣款、生成已支付订单、顺延套餐、记流水）。
// 幂等：同用户+同套餐在 payIdempotentWindow 内已有 paid 订单 → 直接返回该订单（不重复扣款）。
func (s *OrderService) PayWithBalance(userID, planID uint64) (*models.Order, error) {
	var order *models.Order
	paid := false // 本次是否真实扣款（幂等复用旧订单不算新支付，不发布事件）
	ctx := context.Background()
	err := s.store.Transaction(ctx, func(tx contracts.BillingStore) error {
		plan, err := tx.GetPlan(ctx, planID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("套餐不存在")
			}
			return err
		}
		if !plan.Enabled {
			return errors.New("套餐未上架")
		}

		// 先锁用户行：序列化同一用户的并发支付。
		// 方言差异在 pkg/db.LockForUpdate 收口：SQLite 由单连接池串行兜底，MySQL 真实行锁。
		user, err := tx.LockUser(ctx, userID)
		if err != nil {
			return errors.New("用户不存在")
		}

		// 幂等：窗口内已支付同套餐 → 直接复用。
		// 必须在行锁之后检查：MySQL 并发事务各自持旧快照，锁前检查会双双通过（TOCTOU 双扣款）；
		// 锁后由 InnoDB 等待-提交顺序保证看到对方已提交的订单。
		recent, err := tx.FindRecentPaidOrder(ctx, userID, planID, time.Now().Add(-payIdempotentWindow))
		if err != nil {
			return err
		}
		if recent != nil {
			order = recent
			return nil
		}

		if user.BalanceCents < plan.PriceCents {
			return errors.New("账户余额不足，请先充值")
		}

		now := time.Now()
		newBalance := user.BalanceCents - plan.PriceCents
		if err := tx.UpdateBalance(ctx, userID, newBalance); err != nil {
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
		if err := tx.CreateOrder(ctx, order); err != nil {
			return err
		}

		// 记录扣款流水
		entry := models.BalanceLog{
			UserID:       userID,
			AmountCents:  -plan.PriceCents,
			BalanceAfter: newBalance,
			Type:         models.BalanceLogOrderPayment,
			RelatedID:    order.ID,
			Remark:       "余额购买套餐 #" + strconv.FormatUint(plan.ID, 10) + " (" + plan.Name + ")",
			CreatedAt:    now,
		}
		if err := tx.CreateBalanceLog(ctx, &entry); err != nil {
			return err
		}

		// 顺延套餐有效期与重置周期
		base := now
		if user.ExpireAt != nil && user.ExpireAt.After(now) {
			base = *user.ExpireAt
		}
		newExpire := base.AddDate(0, 0, plan.DurationDays)
		if err := tx.UpdateSubscription(ctx, userID, plan, newExpire, now, plan.PermissionGroupID); err != nil {
			return err
		}

		paid = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Stage 5：事务提交后发布支付事件（如热更新用户到在线节点）。
	// 事件失败不回滚已提交事务，仅记录日志（验收：事件发布不改变支付事务语义）。
	if paid && s.Events != nil {
		ev := contracts.OrderPaidEvent{
			OrderID: order.ID,
			OrderNo: order.OrderNo,
			UserID:  userID,
			PlanID:  planID,
			PaidAt:  *order.PaidAt,
		}
		if err := s.Events.Publish(context.Background(), ev); err != nil {
			log.Printf("billing: 订单 #%s 支付事件发布失败（订单已生效）: %v", order.OrderNo, err)
		}
	}
	return order, nil
}

// ListByUser 用户订单列表。
func (s *OrderService) ListByUser(userID uint64) ([]models.Order, error) {
	return s.store.ListOrdersByUser(context.Background(), userID, 50)
}
