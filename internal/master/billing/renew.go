package billing

import (
	"context"
	"log"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// AutoRenewService 自动续费（2026-09-04）：
// 用户勾选后由 cron 周期扫描，命中即按当前持有套餐调用 PayWithBalance——
// 与手动购买完全同语义（作废重算+流量周期重置+支付事件热更全节点），无独立续费路径。
//
// 双触发（两开关独立）：
//   - AutoRenewExpire：到期窗口（expire_at <= now+1h，含已过期）内扣费续期——
//     提前 1 小时扣费，续期后 expire_at 推进至未来自然退出窗口，幂等无需额外标记；
//   - AutoRenewExhaust：当前周期流量耗尽（used >= quota，quota=0 不限量永不触发）——
//     耗尽用户已被判定链摘除停止产生上报，used 冻结，扫描判定稳定；
//     续费后 traffic_cycle_start 重置退出耗尽条件。
//
// 余额不足 / 套餐停续（renewable=false）/ 其他支付失败：本轮跳过，开关保持、
// 下轮重试（用户充值后自动补续）；不自动关闭开关、本期不做用户侧通知。
type AutoRenewService struct {
	DB     *gorm.DB
	Orders *OrderService
}

// NewAutoRenewService 构造自动续费服务。
func NewAutoRenewService(db *gorm.DB, orders *OrderService) *AutoRenewService {
	return &AutoRenewService{DB: db, Orders: orders}
}

// autoRenewWindow 到期触发窗口：到期前 1 小时起可续（含已过期，过期后每轮扫描继续尝试直至续上）。
const autoRenewWindow = time.Hour

// StartCron 启动自动续费扫描（每 5 分钟，与流量重置 cron 同节奏）。
func (s *AutoRenewService) StartCron(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				s.RunOnce(ctx)
			}
		}
	}()
}

// RunOnce 执行一轮扫描。单轮内两个触发列表合并去重（同用户只扣一次）。
func (s *AutoRenewService) RunOnce(ctx context.Context) {
	expireIDs := s.expiryCandidates(ctx)
	exhaust := s.exhaustCandidates(ctx) // map[userID]struct{}

	type task struct {
		userID, planID uint64
		reason         string
	}
	seen := make(map[uint64]bool)
	var tasks []task
	add := func(ids []uint64, planOf func(uint64) uint64, reason string) {
		for _, id := range ids {
			if seen[id] {
				continue
			}
			if pid := planOf(id); pid > 0 {
				seen[id] = true
				tasks = append(tasks, task{userID: id, planID: pid, reason: reason})
			}
		}
	}
	add(expireIDs, s.planIDOf, "expire")
	add(exhaustIDs(exhaust), func(id uint64) uint64 { return exhaust[id] }, "exhausted")

	for _, t := range tasks {
		order, err := s.Orders.PayWithBalance(t.userID, t.planID)
		if err != nil {
			// 余额不足/停售停续/套餐删除等：本轮放弃，等下轮（开关不关闭）
			log.Printf("billing: 自动续费未执行 user=%d plan=%d reason=%s: %v", t.userID, t.planID, t.reason, err)
			continue
		}
		log.Printf("billing: 自动续费完成 user=%d plan=%d order=%s", t.userID, t.planID, order.OrderNo)
		planName := ""
		var plan models.Plan
		if err := s.DB.Select("name").First(&plan, t.planID).Error; err == nil {
			planName = plan.Name
		}
		detail := "自动续费触发（" + renewReasonText(t.reason) + "）：用户 #" + strconv.FormatUint(t.userID, 10) + " 套餐"
		if planName != "" {
			detail += "「" + planName + "」"
		}
		detail += " #" + strconv.FormatUint(t.planID, 10) + " 订单 " + order.OrderNo
		s.DB.Create(&models.AuditLog{
			OperatorType: "system",
			Action:       "auto_renew",
			Detail:       detail,
		})
	}
}

// expiryCandidates 到期窗口内的开关用户（含已过期：过期后持续尝试直至续上或开关关闭）。
func (s *AutoRenewService) expiryCandidates(ctx context.Context) []uint64 {
	var ids []uint64
	s.DB.WithContext(ctx).Model(&models.User{}).
		Where("status = ? AND auto_renew_expire = ? AND plan_id > 0 AND expire_at IS NOT NULL AND expire_at <= ?",
			models.StatusActive, true, time.Now().Add(autoRenewWindow)).
		Pluck("id", &ids)
	return ids
}

// exhaustCandidates 当前周期流量耗尽的开关用户（额度 0=不限，永不触发）。
// 耗尽用户已被节点摘除停止上报，used 冻结在阈值附近，判定稳定。
func (s *AutoRenewService) exhaustCandidates(ctx context.Context) map[uint64]uint64 {
	type row struct {
		UserID uint64
		PlanID uint64
	}
	var rows []row
	s.DB.WithContext(ctx).Raw(`
		SELECT u.id AS user_id, u.plan_id
		FROM users u
		WHERE u.status = ? AND u.auto_renew_exhaust = ? AND u.plan_id > 0 AND u.plan_traffic_bytes > 0
		  AND (
		    SELECT COALESCE(SUM(l.up_bytes + l.down_bytes), 0)
		    FROM traffic_logs l
		    WHERE l.user_id = u.id AND l.period_start >= u.traffic_cycle_start
		  ) >= u.plan_traffic_bytes`,
		models.StatusActive, true).Scan(&rows)
	out := make(map[uint64]uint64, len(rows))
	for _, r := range rows {
		out[r.UserID] = r.PlanID
	}
	return out
}

func exhaustIDs(m map[uint64]uint64) []uint64 {
	ids := make([]uint64, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	return ids
}

// planIDOf 复查用户当前持有套餐（扫描与扣费之间 plan 可能被管理员变更）。
func (s *AutoRenewService) planIDOf(userID uint64) uint64 {
	var planID uint64
	s.DB.Model(&models.User{}).Where("id = ?", userID).Pluck("plan_id", &planID)
	return planID
}

func renewReasonText(reason string) string {
	if reason == "expire" {
		return "套餐到期"
	}
	return "流量耗尽"
}
