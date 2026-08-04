package services

import (
	"context"
	"errors"
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
func (s *TrafficService) Save(tr protocol.TrafficReportPayload) error {
	periodStart, err := time.Parse(time.RFC3339, tr.Period)
	if err != nil {
		return err
	}
	periodEnd := time.Now()

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
			var user models.User
			if err := s.DB.Where("email = ?", e.Email).First(&user).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue // 未知用户（未注册/email 不匹配），跳过
				}
				return err
			}
			userID = user.ID
		}

		var log models.TrafficLog
		err := s.DB.Where(
			"user_id = ? AND inbound_id = ? AND period_start = ?",
			userID, e.Inbound, periodStart,
		).First(&log).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := s.DB.Create(&models.TrafficLog{
				UserID:      userID,
				InboundID:   0,
				UpBytes:     e.UpBytes,
				DownBytes:   e.DownBytes,
				PeriodStart: periodStart,
				PeriodEnd:   periodEnd,
			}).Error; err != nil {
				return err
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

// UserUsed 用户已用总流量（字节）。
func (s *TrafficService) UserUsed(userID uint64) (up, down int64, err error) {
	var row struct {
		Up   int64
		Down int64
	}
	err = s.DB.Model(&models.TrafficLog{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(up_bytes),0) AS up, COALESCE(SUM(down_bytes),0) AS down").
		Scan(&row).Error
	if err != nil {
		return 0, 0, err
	}
	return row.Up, row.Down, nil
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