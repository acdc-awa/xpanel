// Package gormstore 提供 contracts 仓储接口的 GORM 实现（Stage 8 默认适配器）。
// 方言差异经 pkg/db 接缝消化；事务通过 Transaction 回调内的仓储重绑定实现。
package gormstore

import (
	"context"
	"errors"
	"fmt"
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

// UpdateSubscription 顺延套餐并切换流量账期。
//
// 账期归属（审计 F3）：递增 traffic_cycle_id 是「新周期从 0 起算」的**唯一依据**——节点按
// 该 ID 给采集到的增量打标，计费/配额按 `traffic_logs.cycle_id = 用户当前账期` 归属，因此
// 切换前产生的迟到增量（含断线补报、跨小时/跨天）留在旧账期，不会被算进新套餐。
//
// 周期起点仍对齐整点（models.TrafficCycleAlign）并清零该整点桶内 cycle_id = 0 的计费字节：
// 那批行没有账期标记（存量行 / 未升级的旧 agent），只能回退按时间轴归属，不清零会把
// 「整点到本次切换时刻」的旧消费算进新周期。带账期标记的行一律不动，旧账期账本完整留痕。
//
// 三条写（套餐/到期/周期起点/账期 ID、清零）在同一事务内（调用方经 Transaction 绑定），
// 不存在「已切周期但未清零」的中间态。
//
// 调用方（购买/续费/自动续费）事务提交后须触发一次用户列表推送（OrderPaidEvent →
// SyncUsersToAll），节点才会拿到新账期 ID 并从此刻起按新账期打标。
func (s *BillingStore) UpdateSubscription(ctx context.Context, userID uint64, plan *models.Plan, expireAt, cycleStart time.Time) error {
	aligned := models.TrafficCycleAlign(cycleStart)
	updates := map[string]any{
		"plan_id":             plan.ID,
		"expire_at":           expireAt,
		"traffic_cycle_start": aligned,
		// 账期 ID 递增：SQL 表达式在库内自增，避免「读-改-写」竞态丢更新
		//（同一用户的并发续费被单连接/行锁串行化，但表达式自增更省一次读）。
		"traffic_cycle_id": gorm.Expr("traffic_cycle_id + 1"),
		// 购买即跟随套餐权限组（2026-09-17 拍板）：清空用户自定义分组，生效组由下面的
		// 快照列 plan_group_id 回落提供。此处写 plan.PermissionGroupID 会固化「假自定义」，
		// 使该用户此后不再跟随套餐权限组变更（面板显示「(自定义)」而非「(套餐继承)」）。
		"permission_group_id": 0,
	}
	// 套餐快照（2026-09-01 Xboard 式隔离：购买/续费即按当前套餐值重新快照，
	// 此后套餐编辑不影响该用户直至下次分配/续费/勾选同步）
	for k, v := range models.PlanSnapshotColumns(plan) {
		updates[k] = v
	}
	if err := s.with(ctx).Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
		return err
	}
	return models.ZeroBilledInCycleBucket(s.with(ctx), userID, aligned)
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
	// 未使用且未过期的排最前（过期不算未使用，服务端分页下必须 SQL 层排序），
	// 组内按 id 倒序。时间字面量由 Go 生成（无用户输入）；GORM Order 不支持绑定参数，
	// SQLite 侧驱动按统一格式存文本，同部署同时区下字典序即时序；MySQL 侧原生解析该字面量。
	orderExpr := fmt.Sprintf(
		"CASE WHEN status = '%s' AND (expires_at IS NULL OR expires_at > '%s') THEN 0 ELSE 1 END, id DESC",
		models.GiftCardUnused, time.Now().Format("2006-01-02 15:04:05"))
	var list []models.GiftCard
	if err := q.Order(orderExpr).Offset((query.Page - 1) * query.Size).Limit(query.Size).Find(&list).Error; err != nil {
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
