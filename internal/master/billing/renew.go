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
)

// AutoRenewService 订阅周期处置（自动续费 + 到期退订）。
//
// 两路触发，扣费均按当前持有套餐调用 PayWithBalance——与手动购买完全同语义
// （作废重算 + 流量周期重置 + 支付事件热更全节点），没有独立续费路径。
//
//   - 到期处置（每分钟）：expire_at 已过 → 开启到期自动续费且账号正常的用户先尝试扣款，
//     成功即续期；业务性拒绝（余额不足 / 已停止续费 / 套餐不存在）或未开启开关 → 退订
//     （清空套餐、快照与权限组，并把当前账期用量归零）。瞬时错误保留套餐，下一轮重试，
//     避免数据库抖动误删权益。
//   - 流量耗尽（每 5 分钟）：当前周期用量 >= 额度且开启耗尽自动续费 → 扣款重购；
//     失败只记日志等下轮，不清空套餐（耗尽状态仍展示套餐并提示用户重购）。
//
// 处置流程不改两个自动续费开关：退订后用户重新购买时，开关仍按原意愿在下一个周期生效。
type AutoRenewService struct {
	DB     *gorm.DB
	Orders *OrderService
}

// NewAutoRenewService 构造自动续费服务。
func NewAutoRenewService(db *gorm.DB, orders *OrderService) *AutoRenewService {
	return &AutoRenewService{DB: db, Orders: orders}
}

// 两路节奏不同：到期处置查询很轻且要尽量贴近「到期那一刻」，按分钟；
// 流量耗尽要按账期聚合 traffic_logs（较重），保持 5 分钟。
const (
	expirySweepInterval  = time.Minute
	exhaustSweepInterval = 5 * time.Minute
)

// StartCron 启动周期处置。
func (s *AutoRenewService) StartCron(ctx context.Context) {
	expiryTick := time.NewTicker(expirySweepInterval)
	exhaustTick := time.NewTicker(exhaustSweepInterval)
	go func() {
		defer expiryTick.Stop()
		defer exhaustTick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-expiryTick.C:
				s.RunExpiryOnce(ctx)
			case <-exhaustTick.C:
				s.RunExhaustOnce(ctx)
			}
		}
	}()
}

// RunOnce 执行一整轮处置（到期 + 耗尽），供测试与手工触发。
func (s *AutoRenewService) RunOnce(ctx context.Context) {
	s.RunExpiryOnce(ctx)
	s.RunExhaustOnce(ctx)
}

// expiredUser 到期处置候选。
type expiredUser struct {
	UserID          uint64
	PlanID          uint64
	PlanName        string
	Status          int
	AutoRenewExpire bool
}

// RunExpiryOnce 到期处置：先尝试自动续费，失败或未开启则退订。
//
// 查询失败必须留痕而不是当作「没有候选」：静默当空集会让到期退订无声停摆，
// 用户过期后一直挂着套餐且面板上没有任何异常可查（审计 F5 同类）。
func (s *AutoRenewService) RunExpiryOnce(ctx context.Context) {
	now := time.Now()
	var rows []expiredUser
	// 套餐名只为审计可读；plans 是软删除表，归档后仍可取到名字。
	if err := s.DB.WithContext(ctx).Raw(`
		SELECT u.id AS user_id, u.plan_id, COALESCE(p.name, '') AS plan_name,
		       u.status, u.auto_renew_expire
		FROM users u
		LEFT JOIN plans p ON p.id = u.plan_id
		WHERE u.deleted_at IS NULL AND u.plan_id > 0
		  AND u.expire_at IS NOT NULL AND u.expire_at <= ?
		ORDER BY u.id ASC`, now).Scan(&rows).Error; err != nil {
		log.Printf("billing: 读取到期处置候选失败，本轮跳过: %v", err)
		return
	}

	var cleared []uint64
	for _, r := range rows {
		// 被禁用的账号不应为其扣费：按「未开启」处理，直接退订。
		if r.Status == models.StatusActive && r.AutoRenewExpire {
			// RenewWithBalance 在行锁内复核持有关系：扫描后被改套餐会返回 ErrPlanChanged，
			// 本轮跳过，下一轮按新套餐处理（不能把旧套餐当新购卖出覆盖新套餐）。
			order, err := s.Orders.RenewWithBalance(r.UserID, r.PlanID)
			if err == nil {
				s.auditRenew(r.UserID, r.PlanID, order.OrderNo, "expire")
				continue
			}
			if errors.Is(err, ErrPlanChanged) {
				log.Printf("billing: 到期续费前套餐已变更，本轮跳过 user=%d plan=%d", r.UserID, r.PlanID)
				continue
			}
			if !IsRenewalRejection(err) {
				log.Printf("billing: 到期自动续费遇到瞬时错误，保留套餐待下轮重试 user=%d plan=%d: %v", r.UserID, r.PlanID, err)
				continue
			}
			log.Printf("billing: 到期自动续费被拒绝，改为退订 user=%d plan=%d reason=%v", r.UserID, r.PlanID, err)
			s.clearExpired(r, err.Error(), &cleared)
			continue
		}

		reason := "未开启到期自动续费"
		if r.Status != models.StatusActive {
			reason = "账号已禁用"
		}
		s.clearExpired(r, reason, &cleared)
	}

	if len(cleared) > 0 {
		s.publishCleared(ctx, cleared)
	}
}

// clearExpired 退订单个用户并把成功的 ID 收进本轮批量（供一次全节点同步）。
func (s *AutoRenewService) clearExpired(r expiredUser, reason string, cleared *[]uint64) {
	ok, err := s.Orders.ClearExpiredSubscription(r.UserID)
	if err != nil {
		log.Printf("billing: 到期退订失败 user=%d: %v", r.UserID, err)
		return
	}
	if !ok {
		// 行锁复核发现已续费/已换套餐/不再到期：不处理。
		return
	}
	*cleared = append(*cleared, r.UserID)
	s.auditClear(r, reason)
}

// RunExhaustOnce 流量耗尽自动续费（不清空套餐）。
func (s *AutoRenewService) RunExhaustOnce(ctx context.Context) {
	exhaust, err := s.exhaustCandidates(ctx)
	if err != nil {
		log.Printf("billing: 读取超量续费候选失败，本轮跳过超量触发: %v", err)
		return
	}
	for userID, planID := range exhaust {
		order, err := s.Orders.RenewWithBalance(userID, planID)
		if err != nil {
			// 余额不足/停售停续/套餐变更等：本轮放弃，等下轮（开关不关闭）
			log.Printf("billing: 自动续费未执行 user=%d plan=%d reason=exhausted: %v", userID, planID, err)
			continue
		}
		s.auditRenew(userID, planID, order.OrderNo, "exhausted")
	}
}

// exhaustCandidates 当前周期流量耗尽的开关用户（额度 0=不限，永不触发）。
// 用量读计费口径 billed 两列（原始字节 × 落库时入站倍率），与节点摘除判定一致。
// 耗尽用户已被判定链摘除停止上报，used 冻结在阈值附近，判定稳定。
func (s *AutoRenewService) exhaustCandidates(ctx context.Context) (map[uint64]uint64, error) {
	type row struct {
		UserID uint64
		PlanID uint64
	}
	var rows []row
	// 口径取自 models.CycleUsageSQL（与 services 的配额判定/展示同源，禁止在此另写一份）
	err := s.DB.WithContext(ctx).Raw(`
		SELECT u.id AS user_id, u.plan_id
		FROM users u
		WHERE u.status = ? AND u.auto_renew_exhaust = ? AND u.plan_id > 0 AND u.plan_traffic_bytes > 0
		  AND (
		    SELECT COALESCE(SUM(`+models.CycleUsageSQL+`), 0)
		    FROM traffic_logs l
		    WHERE l.user_id = u.id
		  ) >= u.plan_traffic_bytes`,
		models.StatusActive, true).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uint64]uint64, len(rows))
	for _, r := range rows {
		out[r.UserID] = r.PlanID
	}
	return out, nil
}

// auditRenew 自动续费审计。
func (s *AutoRenewService) auditRenew(userID, planID uint64, orderNo, reason string) {
	log.Printf("billing: 自动续费完成 user=%d plan=%d order=%s", userID, planID, orderNo)
	detail := "自动续费触发（" + renewReasonText(reason) + "）：用户 #" + strconv.FormatUint(userID, 10) + " 套餐"
	if name := s.planName(planID); name != "" {
		detail += "「" + name + "」"
	}
	detail += " #" + strconv.FormatUint(planID, 10) + " 订单 " + orderNo
	s.DB.Create(&models.AuditLog{
		OperatorType: "system",
		Action:       "auto_renew",
		Category:     models.AuditCategory("auto_renew"),
		Detail:       detail,
	})
}

// auditClear 到期退订审计（原因：未开启/余额不足/已停止续费/套餐不存在/账号被禁用）。
func (s *AutoRenewService) auditClear(r expiredUser, reason string) {
	log.Printf("billing: 套餐到期退订 user=%d plan=%d reason=%s", r.UserID, r.PlanID, reason)
	detail := "套餐到期自动退订：用户 #" + strconv.FormatUint(r.UserID, 10) + " 套餐"
	if r.PlanName != "" {
		detail += "「" + r.PlanName + "」"
	}
	detail += " #" + strconv.FormatUint(r.PlanID, 10) + "（原因：" + reason + "）"
	s.DB.Create(&models.AuditLog{
		OperatorType: "system",
		Action:       "subscription_expired",
		Category:     models.AuditCategory("subscription_expired"),
		Detail:       detail,
	})
}

// planName 套餐展示名（查不到返回空，审计退化为只带编号）。
func (s *AutoRenewService) planName(planID uint64) string {
	var plan models.Plan
	if err := s.DB.Select("name").First(&plan, planID).Error; err != nil {
		return ""
	}
	return plan.Name
}

// publishCleared 本轮有退订时发一次批量事件，订阅方据此同步全节点用户列表。
func (s *AutoRenewService) publishCleared(ctx context.Context, userIDs []uint64) {
	if s.Orders == nil || s.Orders.Events == nil {
		return
	}
	ev := contracts.SubscriptionClearedEvent{UserIDs: userIDs, At: time.Now()}
	if err := s.Orders.Events.Publish(ctx, ev); err != nil {
		log.Printf("billing: 到期退订事件发布失败（清空已生效）: %v", err)
	}
}

func renewReasonText(reason string) string {
	if reason == "expire" {
		return "套餐到期"
	}
	return "流量耗尽"
}
