package contracts

import (
	"context"
	"time"

	"github.com/acdc-awa/xpanel/internal/models"
)

// GiftCardQuery 礼品卡列表查询（Page/Size 由调用方归一化，1-based）。
type GiftCardQuery struct {
	Status string // 空 = 全部
	Search string // code/name 模糊匹配
	Page   int
	Size   int
}

// BillingStore 资金域仓储接口（Stage 8 数据库适配层首个落地域）。
// 方法语义化而非 SQL 化；事务经 Transaction 回调绑定（回调入参为事务内仓储）。
// 方言差异（行锁/错误文本）由实现层经 pkg/db 接缝消化，业务代码零感知。
// 返回错误约定：记录不存在返回底层 ErrRecordNotFound（GORM 实现即 gorm.ErrRecordNotFound），
// 由服务层翻译为业务文案。
type BillingStore interface {
	// Transaction 在单事务内执行 fn；fn 收到的仓储已绑定该事务。
	Transaction(ctx context.Context, fn func(tx BillingStore) error) error

	// 套餐
	GetPlan(ctx context.Context, id uint64) (*models.Plan, error)

	// 用户（资金视角）
	LockUser(ctx context.Context, id uint64) (*models.User, error) // 行锁读取（方言接缝内消化）
	UpdateBalance(ctx context.Context, userID uint64, newBalanceCents int64) error
	// UpdateSubscription 顺延套餐：permGroupID 为 0 时不改动权限组字段；
	// plan 提供本次购买/续费的套餐实体，快照三列（plan_traffic_bytes/plan_device_limit/plan_group_id）
	// 在同一事务内按其当前值写入（2026-09-01 Xboard 式隔离：分配即快照）。
	UpdateSubscription(ctx context.Context, userID uint64, plan *models.Plan, expireAt, cycleStart time.Time, permGroupID uint64) error

	// 订单
	FindRecentPaidOrder(ctx context.Context, userID, planID uint64, since time.Time) (*models.Order, error) // 无记录返回 nil,nil
	CreateOrder(ctx context.Context, o *models.Order) error
	ListOrdersByUser(ctx context.Context, userID uint64, limit int) ([]models.Order, error)

	// 礼品卡
	CreateGiftCard(ctx context.Context, card *models.GiftCard) error
	LockGiftCardByCode(ctx context.Context, code string) (*models.GiftCard, error) // 行锁读取
	MarkGiftCardUsed(ctx context.Context, id, userID uint64, at time.Time) error
	GetGiftCard(ctx context.Context, id uint64) (*models.GiftCard, error)
	DeleteGiftCard(ctx context.Context, id uint64) error
	ListGiftCards(ctx context.Context, q GiftCardQuery) ([]models.GiftCard, int64, error)

	// 余额流水
	CreateBalanceLog(ctx context.Context, l *models.BalanceLog) error
	ListBalanceLogs(ctx context.Context, userID uint64, page, size int) ([]models.BalanceLog, int64, error)
}
