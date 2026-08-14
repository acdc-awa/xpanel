package services

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/protocol"
)

// TrafficService 处理节点流量上报与聚合。
type TrafficService struct {
	DB *gorm.DB
}

// Save 处理 traffic_report（幂等：同一 (user, inbound, period) 覆盖合并）。
// serverID 用于把上报的入站 tag 解析为入站 ID（2026-08-14 J10：入站级统计激活）。
func (s *TrafficService) Save(tr protocol.TrafficReportPayload, serverID uint64) error {
	periodStart, err := time.Parse(time.RFC3339, tr.Period)
	if err != nil {
		return err
	}
	periodEnd := time.Now()

	// 该节点入站 tag → ID 映射（一次查询，循环复用）
	inboundIDByTag := map[string]uint64{}
	var inbs []models.Inbound
	if err := s.DB.Where("server_id = ?", serverID).Find(&inbs).Error; err == nil {
		for _, inb := range inbs {
			inboundIDByTag[inb.Tag] = inb.ID
		}
	}

	for i := range tr.Entries {
		e := &tr.Entries[i]
		if e.UpBytes <= 0 && e.DownBytes <= 0 {
			continue
		}
		// 主控解析 user_id（Agent 按 email 上报）
		userID := e.UserID
		if userID == 0 {
			if e.Email == "" {
				continue
			}
			// 优先解析固定格式 user-<id>@panel.local（主控生成配置时使用）
			if id, ok := parsePanelEmail(e.Email); ok {
				userID = id
			} else {
				var user models.User
				if err := s.DB.Where("email = ?", e.Email).First(&user).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						continue // 未知用户（未注册/email 不匹配），跳过
					}
					return err
				}
				userID = user.ID
			}
		}

		// 入站 tag → ID（agent 按 tag 上报；未知 tag 按 0 处理，用户级统计不受影响）
		inboundID := inboundIDByTag[e.Inbound]

		var log models.TrafficLog
		err := s.DB.Where(
			"user_id = ? AND inbound_id = ? AND period_start = ?",
			userID, inboundID, periodStart,
		).First(&log).Error

		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := s.DB.Create(&models.TrafficLog{
				UserID:      userID,
				InboundID:   inboundID, // 2026-08-14 J10：原恒为 0，入站级统计整条链空转
				UpBytes:     e.UpBytes,
				DownBytes:   e.DownBytes,
				PeriodStart: periodStart,
				PeriodEnd:   periodEnd,
			}).Error; err != nil {
				return err
			}
			// 入站冗余计数累计（仅首次创建时；重复投递由 TrafficLog 合并口径覆盖，避免重复累计）
			if inboundID > 0 {
				s.DB.Model(&models.Inbound{}).Where("id = ?", inboundID).Updates(map[string]any{
					"up":   gorm.Expr("up + ?", e.UpBytes),
					"down": gorm.Expr("down + ?", e.DownBytes),
				})
			}
		} else if err == nil {
			// 重复投递：合并累加
			if err := s.DB.Model(&log).Updates(map[string]any{
				"up_bytes":   gorm.Expr("up_bytes + ?", e.UpBytes),
				"down_bytes": gorm.Expr("down_bytes + ?", e.DownBytes),
				"period_end": periodEnd,
			}).Error; err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}

// UserUsed 用户当前计费周期内已用总流量（字节）。
// 从 user.traffic_cycle_start 开始计算；若为零值（旧数据）则回溯全部。
func (s *TrafficService) UserUsed(userID uint64) (up, down int64, err error) {
	var user models.User
	if err := s.DB.First(&user, userID).Error; err != nil {
		return 0, 0, err
	}
	q := s.DB.Model(&models.TrafficLog{}).Where("user_id = ?", userID)
	if !user.TrafficCycleStart.IsZero() {
		q = q.Where("period_start >= ?", user.TrafficCycleStart)
	}
	var row struct {
		Up   int64
		Down int64
	}
	err = q.Select("COALESCE(SUM(up_bytes),0) AS up, COALESCE(SUM(down_bytes),0) AS down").Scan(&row).Error
	if err != nil {
		return 0, 0, err
	}
	return row.Up, row.Down, nil
}

// ResetUserTraffic 重置用户流量周期起点（套餐续费或手动重置时调用）。
func (s *TrafficService) ResetUserTraffic(userID uint64) error {
	return s.DB.Model(&models.User{}).Where("id = ?", userID).
		Update("traffic_cycle_start", time.Now()).Error
}

// StartTrafficResetCron 启动 Inbound 级流量重置定时任务（每 5 分钟检查）。
func (s *TrafficService) StartTrafficResetCron(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				s.resetInboundTraffic()
				s.checkInboundLifecycle()
			}
		}
	}()
}

func (s *TrafficService) resetInboundTraffic() {
	var inbounds []models.Inbound
	if err := s.DB.Where("traffic_reset != ? AND traffic_reset != ''", "never").Find(&inbounds).Error; err != nil {
		return
	}
	now := time.Now()
	for _, inb := range inbounds {
		shouldReset := false
		switch inb.TrafficReset {
		case "daily":
			shouldReset = true // 每 5 分钟 tick 时简单清零，daily 粒度通过日期去重保证
		case "weekly":
			shouldReset = now.Weekday() == time.Monday
		case "monthly":
			shouldReset = now.Day() == 1
		}
		if shouldReset {
			_ = s.DB.Model(&inb).Updates(map[string]any{
				"up":   0,
				"down": 0,
			})
		}
	}
}

// StartDailyAgg 启动每日汇总定时任务（每 5 分钟把 traffic_logs 累加到 traffic_daily）。
func (s *TrafficService) StartDailyAgg(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		s.AggDaily()
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				s.AggDaily()
			}
		}
	}()
}

// AggDaily 按 用户×日期 汇总流量到 traffic_daily（upsert）。
func (s *TrafficService) AggDaily() {
	var logs []models.TrafficLog
	if err := s.DB.Find(&logs).Error; err != nil {
		return
	}
	type key struct {
		userID uint64
		date   string
	}
	sum := make(map[key]*models.TrafficDaily)
	for _, l := range logs {
		k := key{l.UserID, l.PeriodStart.Format("2006-01-02")}
		if sum[k] == nil {
			sum[k] = &models.TrafficDaily{UserID: l.UserID, Date: k.date}
		}
		sum[k].UpBytes += l.UpBytes
		sum[k].DownBytes += l.DownBytes
	}
	for _, d := range sum {
		var existing models.TrafficDaily
		err := s.DB.Where("user_id = ? AND date = ?", d.UserID, d.Date).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = s.DB.Create(d)
		} else if err == nil {
			_ = s.DB.Model(&existing).Updates(map[string]any{
				"up_bytes":   d.UpBytes,
				"down_bytes": d.DownBytes,
			})
		}
	}
}

// parsePanelEmail 解析 user-<id>@panel.local → user id。
func parsePanelEmail(email string) (uint64, bool) {
	const prefix = "user-"
	const suffix = "@panel.local"
	if !strings.HasPrefix(email, prefix) || !strings.HasSuffix(email, suffix) {
		return 0, false
	}
	idStr := strings.TrimSuffix(strings.TrimPrefix(email, prefix), suffix)
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// checkInboundLifecycle 每 5 分钟检查入站生命周期（J9 激活）：
// Total 跑满（up+down >= total）或 ExpiryTime 到期 → 自动停用。
// 停用后订阅端实时生效（查询同源过滤）；节点配置由 1h 校准/下次推送收敛。
func (s *TrafficService) checkInboundLifecycle() {
	var inbounds []models.Inbound
	if err := s.DB.Find(&inbounds).Error; err != nil {
		return
	}
	now := time.Now()
	for _, inb := range inbounds {
		if !inb.Enabled {
			continue
		}
		expired := (inb.Total > 0 && inb.Up+inb.Down >= inb.Total) ||
			(inb.ExpiryTime != nil && now.After(*inb.ExpiryTime))
		if expired {
			_ = s.DB.Model(&inb).Update("enabled", false)
		}
	}
}
