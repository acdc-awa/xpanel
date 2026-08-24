package services

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 数据保留与聚合窗口（ISSUE-09）。
const (
	aggWindowDays           = 7   // AggDaily 只扫描最近 N 天，覆盖节点补报窗口
	trafficLogRetentionDays = 90  // traffic_logs 保留天数
	nodeReportRetentionDays = 30  // node_reports 保留天数
	auditLogRetentionDays   = 180 // audit_logs 保留天数
)

// TrafficService 处理节点流量上报与聚合。
type TrafficService struct {
	DB  *gorm.DB
	now func() time.Time // 可注入时钟（测试用）
}

// nowOrReal 返回当前时间（未注入时钟时用真实时间）。
func (s *TrafficService) nowOrReal() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// Save 处理 traffic_report（P1-1：单事务 + upsert 合并——并发/重复投递在唯一索引
// (user_id, inbound_id, period_start) 上自动累加，不再撞索引报错；每次投递统一补计入站计数）。
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

	return s.DB.Transaction(func(tx *gorm.DB) error {
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
					if err := tx.Where("email = ?", e.Email).First(&user).Error; err != nil {
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

			// P1-1：upsert 合并——唯一索引 (user_id, inbound_id, period_start) 兜底，
			// 并发/重复投递自动累加而非撞索引报错丢弃（替代原 select-then-create 竞态路径）
			log := models.TrafficLog{
				UserID:      userID,
				InboundID:   inboundID, // 2026-08-14 J10：原恒为 0，入站级统计整条链空转
				UpBytes:     e.UpBytes,
				DownBytes:   e.DownBytes,
				PeriodStart: periodStart,
				PeriodEnd:   periodEnd,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "user_id"}, {Name: "inbound_id"}, {Name: "period_start"},
				},
				DoUpdates: clause.Assignments(map[string]any{
					"up_bytes":   gorm.Expr("up_bytes + excluded.up_bytes"),
					"down_bytes": gorm.Expr("down_bytes + excluded.down_bytes"),
					"period_end": gorm.Expr("excluded.period_end"),
				}),
			}).Create(&log).Error; err != nil {
				return err
			}
			// 入站冗余计数累计（P1-1：每次投递统一补计一次，与 TrafficLog 合并口径一致）
			if inboundID > 0 {
				if err := tx.Model(&models.Inbound{}).Where("id = ?", inboundID).Updates(map[string]any{
					"up":   gorm.Expr("up + ?", e.UpBytes),
					"down": gorm.Expr("down + ?", e.DownBytes),
				}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
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

// resetPeriodKey 计算某个重置策略当前的周期键（同一天/周/月内保持不变）。
func resetPeriodKey(now time.Time, policy string) string {
	switch policy {
	case "daily":
		return now.Format("2006-01-02")
	case "weekly":
		daysFromMonday := (int(now.Weekday()) + 6) % 7 // Monday=0 ... Sunday=6
		return now.AddDate(0, 0, -daysFromMonday).Format("2006-01-02")
	case "monthly":
		return now.Format("2006-01") + "-01"
	default:
		return ""
	}
}

func (s *TrafficService) resetInboundTraffic() {
	var inbounds []models.Inbound
	if err := s.DB.Where("traffic_reset != ? AND traffic_reset != ''", "never").Find(&inbounds).Error; err != nil {
		return
	}
	now := s.nowOrReal()
	for _, inb := range inbounds {
		key := resetPeriodKey(now, inb.TrafficReset)
		if key == "" {
			continue
		}
		if inb.LastResetDate == key {
			continue
		}
		// 条件更新：只有仍处于旧周期/首次运行时才清零；并发 tick 只有一个能命中。
		res := s.DB.Model(&models.Inbound{}).
			Where("id = ? AND (last_reset_date IS NULL OR last_reset_date != ?)", inb.ID, key).
			Updates(map[string]any{
				"up":              0,
				"down":            0,
				"last_reset_date": key,
			})
		if res.Error != nil {
			continue
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
// ISSUE-09：只扫描最近 aggWindowDays 天的 logs，避免全表读入内存重算全部历史。
func (s *TrafficService) AggDaily() {
	var logs []models.TrafficLog
	windowStart := s.nowOrReal().AddDate(0, 0, -aggWindowDays)
	if err := s.DB.Where("period_start >= ?", windowStart).Find(&logs).Error; err != nil {
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

// StartRetentionCron 每天 04:00 清理过期明细数据（ISSUE-09 保留策略）。
func (s *TrafficService) StartRetentionCron(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if s.nowOrReal().Hour() == 4 {
					s.runRetention()
				}
			}
		}
	}()
}

func (s *TrafficService) runRetention() {
	now := s.nowOrReal()
	cutLogs := now.AddDate(0, 0, -trafficLogRetentionDays)
	cutReports := now.AddDate(0, 0, -nodeReportRetentionDays)
	cutAudit := now.AddDate(0, 0, -auditLogRetentionDays)

	if res := s.DB.Where("period_start < ?", cutLogs).Delete(&models.TrafficLog{}); res.Error != nil {
		log.Printf("traffic: 清理 traffic_logs 失败: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("traffic: 清理 %d 条过期 traffic_logs（保留 %d 天）", res.RowsAffected, trafficLogRetentionDays)
	}
	if res := s.DB.Where("reported_at < ?", cutReports).Delete(&models.NodeReport{}); res.Error != nil {
		log.Printf("traffic: 清理 node_reports 失败: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("traffic: 清理 %d 条过期 node_reports（保留 %d 天）", res.RowsAffected, nodeReportRetentionDays)
	}
	if res := s.DB.Where("created_at < ?", cutAudit).Delete(&models.AuditLog{}); res.Error != nil {
		log.Printf("traffic: 清理 audit_logs 失败: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("traffic: 清理 %d 条过期 audit_logs（保留 %d 天）", res.RowsAffected, auditLogRetentionDays)
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
