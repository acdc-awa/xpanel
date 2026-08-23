package billing

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/contracts"
	"github.com/acdc/xray-panel/internal/models"
	"github.com/acdc/xray-panel/internal/pkg/util"
)

// GiftCardService 礼品卡与余额账务服务。
// 2026-08-23 Stage 8：存储访问收口 contracts.BillingStore（GORM 实现见 store/gormstore）。
type GiftCardService struct {
	store contracts.BillingStore
}

// NewGiftCardService 构造礼品卡/余额服务。
func NewGiftCardService(store contracts.BillingStore) *GiftCardService {
	return &GiftCardService{store: store}
}

// BatchGenerate 批量生成礼品卡。
func (s *GiftCardService) BatchGenerate(adminID uint64, count int, name string, faceValueCents int64, expiresAt *time.Time) ([]models.GiftCard, error) {
	if count <= 0 || count > 500 {
		return nil, errors.New("单次生成数量需在 1~500 之间")
	}
	if faceValueCents <= 0 {
		return nil, errors.New("面值必须大于 0")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "通用礼品卡"
	}

	cards := make([]models.GiftCard, 0, count)
	now := time.Now()
	ctx := context.Background()

	err := s.store.Transaction(ctx, func(tx contracts.BillingStore) error {
		for i := 0; i < count; i++ {
			code, err := util.NewGiftCardCode()
			if err != nil {
				return err
			}
			card := models.GiftCard{
				Code:           code,
				Name:           name,
				FaceValueCents: faceValueCents,
				Status:         models.GiftCardUnused,
				ExpiresAt:      expiresAt,
				CreatedBy:      adminID,
				CreatedAt:      now,
			}
			if err := tx.CreateGiftCard(ctx, &card); err != nil {
				return err
			}
			cards = append(cards, card)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return cards, nil
}

// Redeem 兑换礼品卡充值余额（原子事务 + 行锁；行锁方言差异在 pkg/db.LockForUpdate 收口）。
func (s *GiftCardService) Redeem(userID uint64, code string) (*models.GiftCard, int64, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, 0, errors.New("卡密不能为空")
	}

	var card *models.GiftCard
	var newBalance int64
	ctx := context.Background()

	err := s.store.Transaction(ctx, func(tx contracts.BillingStore) error {
		// 1. 查找并锁定卡密
		c, err := tx.LockGiftCardByCode(ctx, code)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("卡密不存在或无效")
			}
			return err
		}
		card = c

		// 2. 校验状态与有效期
		if card.Status != models.GiftCardUnused {
			if card.Status == models.GiftCardUsed {
				return errors.New("该卡密已被使用")
			}
			return errors.New("该卡密已作废")
		}
		now := time.Now()
		if card.ExpiresAt != nil && card.ExpiresAt.Before(now) {
			return errors.New("该卡密已过期")
		}

		// 3. 查找并锁定用户
		user, err := tx.LockUser(ctx, userID)
		if err != nil {
			return errors.New("用户不存在")
		}

		// 4. 更新卡密为已使用（同步本地实体状态：仓储更新不走 GORM 模型回写）
		if err := tx.MarkGiftCardUsed(ctx, card.ID, userID, now); err != nil {
			return err
		}
		card.Status = models.GiftCardUsed
		card.UsedBy = userID
		card.UsedAt = &now

		// 5. 更新用户余额
		newBalance = user.BalanceCents + card.FaceValueCents
		if err := tx.UpdateBalance(ctx, userID, newBalance); err != nil {
			return err
		}

		// 6. 记流水
		entry := models.BalanceLog{
			UserID:       userID,
			AmountCents:  card.FaceValueCents,
			BalanceAfter: newBalance,
			Type:         models.BalanceLogRechargeGiftCard,
			RelatedID:    card.ID,
			Remark:       "兑换礼品卡: " + card.Name + " (" + card.Code + ")",
			CreatedAt:    now,
		}
		if err := tx.CreateBalanceLog(ctx, &entry); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, 0, err
	}
	return card, newBalance, nil
}

// ListCards 管理端查询礼品卡列表。
func (s *GiftCardService) ListCards(page, size int, status, search string) ([]models.GiftCard, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return s.store.ListGiftCards(context.Background(), contracts.GiftCardQuery{
		Status: status,
		Search: search,
		Page:   page,
		Size:   size,
	})
}

// DisableOrDelete 作废或删除礼品卡（未使用的删除或标记作废）。
func (s *GiftCardService) DisableOrDelete(cardID uint64) error {
	ctx := context.Background()
	card, err := s.store.GetGiftCard(ctx, cardID)
	if err != nil {
		return errors.New("卡密不存在")
	}
	if card.Status == models.GiftCardUsed {
		return errors.New("已使用的卡密不可删除或作废")
	}
	return s.store.DeleteGiftCard(ctx, cardID)
}

// ListBalanceLogs 查询用户余额流水。
func (s *GiftCardService) ListBalanceLogs(userID uint64, page, size int) ([]models.BalanceLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return s.store.ListBalanceLogs(context.Background(), userID, page, size)
}

// AdminAdjustBalance 管理员手动调整用户余额。
func (s *GiftCardService) AdminAdjustBalance(adminID, targetUserID uint64, deltaCents int64, remark string) (int64, error) {
	if deltaCents == 0 {
		return 0, errors.New("变动金额不能为 0")
	}
	remark = strings.TrimSpace(remark)
	if remark == "" {
		remark = "管理员手动调账"
	}

	var newBalance int64
	ctx := context.Background()
	err := s.store.Transaction(ctx, func(tx contracts.BillingStore) error {
		user, err := tx.LockUser(ctx, targetUserID)
		if err != nil {
			return errors.New("目标用户不存在")
		}

		newBalance = user.BalanceCents + deltaCents
		if newBalance < 0 {
			return errors.New("调账后余额不能小于 0")
		}

		if err := tx.UpdateBalance(ctx, targetUserID, newBalance); err != nil {
			return err
		}

		entry := models.BalanceLog{
			UserID:       targetUserID,
			AmountCents:  deltaCents,
			BalanceAfter: newBalance,
			Type:         models.BalanceLogAdminAdjust,
			RelatedID:    adminID,
			Remark:       remark,
			CreatedAt:    time.Now(),
		}
		if err := tx.CreateBalanceLog(ctx, &entry); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return 0, err
	}
	return newBalance, nil
}
