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
// 2026-09-01 入站维度接通：agent 额外上报 inbound>>> 计数器派生条目（Email 恒空、Inbound=tag），
// 仅累计 inbounds.up/down 冗余计数器；用户维度条目照旧落 traffic_logs。
// 返回本次投递实际计入的用户 ID 去重集合（供节点网关做事件驱动超额处置，见 FindViolators）。
func (s *TrafficService) Save(tr protocol.TrafficReportPayload, serverID uint64) ([]uint64, error) {
	periodStart, err := time.Parse(time.RFC3339, tr.Period)
	if err != nil {
		return nil, err
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

	reportedUsers := make(map[uint64]struct{})
	err = s.DB.Transaction(func(tx *gorm.DB) error {
		for i := range tr.Entries {
			e := &tr.Entries[i]
			if e.UpBytes <= 0 && e.DownBytes <= 0 {
				continue
			}
			// 入站 tag → ID（agent 按 tag 上报；未知 tag 按 0 处理，用户级统计不受影响）
			inboundID := inboundIDByTag[e.Inbound]

			// 入站维度条目（agent 从 inbound>>> 计数器派生，Email 恒空）：仅累计
			// inbounds.up/down——dashboard 节点流量占比/入站限额/lifecycle 消费；
			// 不落 traffic_logs（流水严格用户维度，防今日流量 KPI 双计）。
			// relay 入站与未知用户的节点流量由此入账（此前入站维度整条链空转）。
			if e.UserID == 0 && e.Email == "" {
				if inboundID > 0 {
					if err := tx.Model(&models.Inbound{}).Where("id = ?", inboundID).Updates(map[string]any{
						"up":   gorm.Expr("up + ?", e.UpBytes),
						"down": gorm.Expr("down + ?", e.DownBytes),
					}).Error; err != nil {
						return err
					}
				}
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
			reportedUsers[userID] = struct{}{}

			// P1-1：upsert 合并——唯一索引 (user_id, inbound_id, period_start) 兜底，
			// 并发/重复投递自动累加而非撞索引报错丢弃（替代原 select-then-create 竞态路径）
			log := models.TrafficLog{
				UserID:      userID,
				InboundID:   inboundID, // 用户条目携带 tag 时记录归属（当前 agent 用户维度不填，恒 0）
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
			// 用户条目携带入站 tag 时同步补计入站计数（入站维度条目已在上方单独处理，不会双计；
			// 与 TrafficLog 合并口径一致，每次投递补计一次）
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
	if err != nil {
		return nil, err
	}
	if len(reportedUsers) == 0 {
		return nil, nil
	}
	ids := make([]uint64, 0, len(reportedUsers))
	for id := range reportedUsers {
		ids = append(ids, id)
	}
	return ids, nil
}

// FindViolators 判定给定用户中已「违规」的（已过期或流量超额），供流量落库后事件驱动处置：
// 命中即热更节点用户列表将其移除，无需等 1h 校准。口径与 filterValidUsers 快照语义严格一致：
// 额度读用户行快照列 plan_traffic_bytes，周期同口径（period_start >= traffic_cycle_start，
// 零值起点 = 全量）。status 非活跃用户不在返回中（其移除由状态变更路径触发）。
func (s *TrafficService) FindViolators(userIDs []uint64) ([]uint64, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	type violatorRow struct {
		UserID    uint64
		UsedBytes int64
		Quota     int64
		ExpireAt  *time.Time
	}
	var rows []violatorRow
	err := s.DB.Raw(`
		SELECT u.id AS user_id,
		       COALESCE(SUM(CASE WHEN l.period_start >= u.traffic_cycle_start THEN l.up_bytes + l.down_bytes ELSE 0 END), 0) AS used_bytes,
		       u.plan_traffic_bytes AS quota,
		       u.expire_at AS expire_at
		FROM users u
		LEFT JOIN traffic_logs l ON l.user_id = u.id
		WHERE u.id IN ? AND u.status = ?
		GROUP BY u.id, u.traffic_cycle_start, u.plan_traffic_bytes, u.expire_at`,
		userIDs, models.StatusActive).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var out []uint64
	for _, r := range rows {
		if r.ExpireAt != nil && now.After(*r.ExpireAt) {
			out = append(out, r.UserID)
			continue
		}
		if r.Quota > 0 && r.UsedBytes >= r.Quota {
			out = append(out, r.UserID)
		}
	}
	return out, nil
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
