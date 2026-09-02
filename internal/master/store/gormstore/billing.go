// Package gormstore 提供 contracts 仓储接口的 GORM 实现（Stage 8 默认适配器）。
// 方言差异经 pkg/db 接缝消化；事务通过 Transaction 回调内的仓储重绑定实现。
package gormstore

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/db"
)

// BillingStore 是 contracts.BillingStore 的 GORM 实现。
type BillingStore struct {
	db *gorm.DB
}

// NewBillingStore 以给定 GORM 连接构造资金域仓储。
func NewBillingStore(base *gorm.DB) *BillingStore {
	return &BillingStore{db: base}
}

var _ contracts.BillingStore = (*BillingStore)(nil)

func (s *BillingStore) with(ctx context.Context) *gorm.DB { return s.db.WithContext(ctx) }

// Transaction 单事务执行；fn 收到绑定该事务的仓储实例。
func (s *BillingStore) Transaction(ctx context.Context, fn func(contracts.BillingStore) error) error {
	return s.with(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&BillingStore{db: tx})
	})
}

func (s *BillingStore) GetPlan(ctx context.Context, id uint64) (*models.Plan, error) {
	var p models.Plan
	if err := s.with(ctx).First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *BillingStore) LockUser(ctx context.Context, id uint64) (*models.User, error) {
	var u models.User
	if err := db.LockForUpdate(s.with(ctx)).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *BillingStore) UpdateBalance(ctx context.Context, userID uint64, newBalanceCents int64) error {
	return s.with(ctx).Model(&models.User{}).Where("id = ?", userID).
		Update("balance_cents", newBalanceCents).Error
}

func (s *BillingStore) UpdateSubscription(ctx context.Context, userID uint64, plan *models.Plan, expireAt, cycleStart time.Time, permGroupID uint64) error {
	updates := map[string]any{
		"plan_id":             plan.ID,
		"expire_at":           expireAt,
		"traffic_cycle_start": cycleStart,
		"permission_group_id": permGroupID,
	}
	// 套餐快照（2026-09-01 Xboard 式隔离：购买/续费即按当前套餐值重新快照，
	// 此后套餐编辑不影响该用户直至下次分配/续费/勾选同步）
	for k, v := range models.PlanSnapshotColumns(plan) {
		updates[k] = v
	}
	return s.with(ctx).Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error
}

func (s *BillingStore) FindRecentPaidOrder(ctx context.Context, userID, planID uint64, since time.Time) (*models.Order, error) {
	var o models.Order
	err := s.with(ctx).
		Where("user_id = ? AND plan_id = ? AND status = ? AND created_at >= ?",
			userID, planID, models.OrderPaid, since).
		Order("id DESC").First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (s *BillingStore) CreateOrder(ctx context.Context, o *models.Order) error {
	return s.with(ctx).Create(o).Error
}

func (s *BillingStore) ListOrdersByUser(ctx context.Context, userID uint64, limit int) ([]models.Order, error) {
	var list []models.Order
	if err := s.with(ctx).Where("user_id = ?", userID).Order("id DESC").Limit(limit).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *BillingStore) CreateGiftCard(ctx context.Context, card *models.GiftCard) error {
	return s.with(ctx).Create(card).Error
}

func (s *BillingStore) LockGiftCardByCode(ctx context.Context, code string) (*models.GiftCard, error) {
	var card models.GiftCard
	if err := db.LockForUpdate(s.with(ctx)).Where("code = ?", code).First(&card).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

func (s *BillingStore) MarkGiftCardUsed(ctx context.Context, id, userID uint64, at time.Time) error {
	return s.with(ctx).Model(&models.GiftCard{}).Where("id = ?", id).
		Updates(map[string]any{
			"status":  models.GiftCardUsed,
			"used_by": userID,
			"used_at": at,
		}).Error
}

func (s *BillingStore) GetGiftCard(ctx context.Context, id uint64) (*models.GiftCard, error) {
	var card models.GiftCard
	if err := s.with(ctx).First(&card, id).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

func (s *BillingStore) DeleteGiftCard(ctx context.Context, id uint64) error {
	return s.with(ctx).Delete(&models.GiftCard{}, id).Error
}

func (s *BillingStore) ListGiftCards(ctx context.Context, query contracts.GiftCardQuery) ([]models.GiftCard, int64, error) {
	q := s.with(ctx).Model(&models.GiftCard{})
	if query.Status != "" {
		q = q.Where("status = ?", query.Status)
	}
	if search := strings.TrimSpace(query.Search); search != "" {
		kw := "%" + search + "%"
		q = q.Where("code LIKE ? OR name LIKE ?", kw, kw)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []models.GiftCard
	if err := q.Order("id DESC").Offset((query.Page - 1) * query.Size).Limit(query.Size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (s *BillingStore) CreateBalanceLog(ctx context.Context, l *models.BalanceLog) error {
	return s.with(ctx).Create(l).Error
}

func (s *BillingStore) ListBalanceLogs(ctx context.Context, userID uint64, page, size int) ([]models.BalanceLog, int64, error) {
	q := s.with(ctx).Model(&models.BalanceLog{}).Where("user_id = ?", userID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []models.BalanceLog
	if err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}
