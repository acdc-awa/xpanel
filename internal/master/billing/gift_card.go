package billing

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/acdc/xray-panel/internal/models"
	"github.com/acdc/xray-panel/internal/pkg/util"
)

// GiftCardService 礼品卡与余额账务服务。
type GiftCardService struct {
	DB *gorm.DB
}

// NewGiftCardService 构造礼品卡/余额服务。
func NewGiftCardService(db *gorm.DB) *GiftCardService {
	return &GiftCardService{DB: db}
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

	err := s.DB.Transaction(func(tx *gorm.DB) error {
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
			if err := tx.Create(&card).Error; err != nil {
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

// Redeem 兑换礼品卡充值余额（原子事务 + 行锁；SQLite 下 FOR UPDATE 被驱动丢弃，由 pkg/db 单连接池串行兜底）。
func (s *GiftCardService) Redeem(userID uint64, code string) (*models.GiftCard, int64, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, 0, errors.New("卡密不能为空")
	}

	var card models.GiftCard
	var newBalance int64

	err := s.DB.Transaction(func(tx *gorm.DB) error {
		// 1. 查找并锁定卡密
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("code = ?", code).First(&card).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("卡密不存在或无效")
			}
			return err
		}

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
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, userID).Error; err != nil {
			return errors.New("用户不存在")
		}

		// 4. 更新卡密为已使用
		if err := tx.Model(&card).Updates(map[string]any{
			"status":  models.GiftCardUsed,
			"used_by": userID,
			"used_at": now,
		}).Error; err != nil {
			return err
		}

		// 5. 更新用户余额
		newBalance = user.BalanceCents + card.FaceValueCents
		if err := tx.Model(&user).Update("balance_cents", newBalance).Error; err != nil {
			return err
		}

		// 6. 记录变动流水
		log := models.BalanceLog{
			UserID:       userID,
			AmountCents:  card.FaceValueCents,
			BalanceAfter: newBalance,
			Type:         models.BalanceLogRechargeGiftCard,
			RelatedID:    card.ID,
			Remark:       "兑换礼品卡: " + card.Name + " (" + card.Code + ")",
			CreatedAt:    now,
		}
		if err := tx.Create(&log).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, 0, err
	}
	return &card, newBalance, nil
}

// ListCards 管理端查询礼品卡列表。
func (s *GiftCardService) ListCards(page, size int, status, search string) ([]models.GiftCard, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	q := s.DB.Model(&models.GiftCard{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if search = strings.TrimSpace(search); search != "" {
		kw := "%" + search + "%"
		q = q.Where("code LIKE ? OR name LIKE ?", kw, kw)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []models.GiftCard
	if err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// DisableOrDelete 作废或删除礼品卡（未使用的删除或标记作废）。
func (s *GiftCardService) DisableOrDelete(cardID uint64) error {
	var card models.GiftCard
	if err := s.DB.First(&card, cardID).Error; err != nil {
		return errors.New("卡密不存在")
	}
	if card.Status == models.GiftCardUsed {
		return errors.New("已使用的卡密不可删除或作废")
	}
	return s.DB.Delete(&card).Error
}

// ListBalanceLogs 查询用户余额流水。
func (s *GiftCardService) ListBalanceLogs(userID uint64, page, size int) ([]models.BalanceLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	q := s.DB.Model(&models.BalanceLog{}).Where("user_id = ?", userID)
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
	err := s.DB.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, targetUserID).Error; err != nil {
			return errors.New("目标用户不存在")
		}

		newBalance = user.BalanceCents + deltaCents
		if newBalance < 0 {
			return errors.New("调账后余额不能小于 0")
		}

		if err := tx.Model(&user).Update("balance_cents", newBalance).Error; err != nil {
			return err
		}

		now := time.Now()
		log := models.BalanceLog{
			UserID:       targetUserID,
			AmountCents:  deltaCents,
			BalanceAfter: newBalance,
			Type:         models.BalanceLogAdminAdjust,
			RelatedID:    adminID,
			Remark:       remark,
			CreatedAt:    now,
		}
		if err := tx.Create(&log).Error; err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return 0, err
	}
	return newBalance, nil
}
