package services

import (
	"context"
	"errors"
	"log"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/master/xray"
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
// 2026-09-06 倍率计费：用户维度条目同时落计费口径 billed 两列（原始字节 × 生效倍率）；
// 统计键 u<uid>.i<iid>@panel.local 反解出入站后按该入站精确倍率计、流水挂真实 inbound_id，
// 旧格式条目（过渡期）回退组内 max 兜底；入站维度与 inbounds.up/down 冗余计数恒为
// 原始字节（展示/容量口径，不乘倍率）。
// 返回本次投递实际计入的用户 ID 去重集合（供节点网关做事件驱动超额处置，见 FindViolators）。
func (s *TrafficService) Save(tr protocol.TrafficReportPayload, serverID uint64) ([]uint64, error) {
	periodStart, err := time.Parse(time.RFC3339, tr.Period)
	if err != nil {
		return nil, err
	}
	periodEnd := time.Now()

	// 该节点入站 tag → ID 与 ID → 行映射（一次查询，循环复用；含停用入站——
	// 节点残留旧配置仍在计流量，按现行倍率归账）
	inboundIDByTag := map[string]uint64{}
	inboundByID := map[uint64]models.Inbound{}
	var inbs []models.Inbound
	if err := s.DB.Where("server_id = ?", serverID).Find(&inbs).Error; err == nil {
		for _, inb := range inbs {
			inboundIDByTag[inb.Tag] = inb.ID
			inboundByID[inb.ID] = inb
		}
	}

	// 计费倍率规则（旧格式条目兜底口径，见 billingRatioFor）。
	billingRules := s.buildBillingRules(serverID)

	// 预解析本帧用户维度涉及的生效组（每帧两次批量查询替代旧版逐条查库；
	// 解析语义与旧版一致：email 查无此人的条目跳过，仅凭 ID/email 规则解析出的
	// 用户不做存在性校验——查无行则组为 0，倍率回退 1，照旧落库）。
	idSet := map[uint64]struct{}{}
	emailSet := map[string]struct{}{}
	for i := range tr.Entries {
		e := &tr.Entries[i]
		if (e.UpBytes <= 0 && e.DownBytes <= 0) || (e.UserID == 0 && e.Email == "") {
			continue
		}
		if e.UserID > 0 {
			idSet[e.UserID] = struct{}{}
		} else if uid, _, ok := xray.ParseUserEmailAny(e.Email); ok {
			idSet[uid] = struct{}{}
		} else {
			emailSet[e.Email] = struct{}{}
		}
	}
	userGroups := map[uint64]uint64{}
	if len(idSet) > 0 {
		ids := make([]uint64, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		var rows []models.User
		if err := s.DB.Select("id, permission_group_id, plan_group_id").Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, u := range rows {
			userGroups[u.ID] = u.EffectiveGroupID()
		}
	}
	emailUsers := map[string]uint64{}
	if len(emailSet) > 0 {
		emails := make([]string, 0, len(emailSet))
		for em := range emailSet {
			emails = append(emails, em)
		}
		var rows []models.User
		if err := s.DB.Select("id, email, permission_group_id, plan_group_id").Where("email IN ?", emails).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, u := range rows {
			userGroups[u.ID] = u.EffectiveGroupID()
			emailUsers[u.Email] = u.ID
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
			tagInboundID := inboundIDByTag[e.Inbound]
			// 用户条目归账入站：优先 agent 携带的 tag；否则反解统计键注入的入站维度
			// u<uid>.i<iid>@panel.local。iid 必须命中本服务器入站才采信（入站已删/
			// 跨服异常按 0 处理，倍率回退组内 max 口径），避免把流水挂到别服入站上。
			inboundID := tagInboundID
			inboundRatio := 0.0
			exactRatio := false
			if tagInboundID == 0 && e.Email != "" {
				if uid, iid, ok := xray.ParseUserEmailFor(e.Email); ok && uid > 0 {
					if inb, found := inboundByID[iid]; found {
						inboundID = iid
						inboundRatio = inb.Ratio
						exactRatio = true
					}
				}
			}

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
				// 优先解析面板生成的统计键（新格式 u<uid>.i<iid> / 旧格式 user-<uid>@panel.local）
				if uid, _, ok := xray.ParseUserEmailAny(e.Email); ok {
					userID = uid
				} else {
					var ok bool
					if userID, ok = emailUsers[e.Email]; !ok {
						continue // 未知用户（未注册/email 不匹配），跳过
					}
				}
			}
			reportedUsers[userID] = struct{}{}

			// 计费倍率：统计键注入了入站维度 → 按该入站精确计（ratio=0 免费入站成立）；
			// 未注入（旧格式过渡期/agent 携带 tag/未知入站）→ (组, 服务器) 生效入站 max 兜底。
			ratio := billingRatioFor(billingRules, userGroups[userID])
			if exactRatio {
				ratio = inboundRatio
			}

			// P1-1：upsert 合并——唯一索引 (user_id, inbound_id, period_start) 兜底，
			// 并发/重复投递自动累加而非撞索引报错丢弃（替代原 select-then-create 竞态路径）
			log := models.TrafficLog{
				UserID:      userID,
				InboundID:   inboundID, // tag 或统计键反解出的归账入站（未知按 0）
				UpBytes:     e.UpBytes,
				DownBytes:   e.DownBytes,
				BilledUp:    roundBilled(e.UpBytes, ratio),
				BilledDown:  roundBilled(e.DownBytes, ratio),
				PeriodStart: periodStart,
				PeriodEnd:   periodEnd,
			}
			var doUpdates clause.Set
			if tx.Dialector != nil && tx.Dialector.Name() == "mysql" {
				doUpdates = clause.Assignments(map[string]any{
					"up_bytes":    gorm.Expr("traffic_logs.up_bytes + VALUES(up_bytes)"),
					"down_bytes":  gorm.Expr("traffic_logs.down_bytes + VALUES(down_bytes)"),
					"billed_up":   gorm.Expr("traffic_logs.billed_up + VALUES(billed_up)"),
					"billed_down": gorm.Expr("traffic_logs.billed_down + VALUES(billed_down)"),
					"period_end":  gorm.Expr("VALUES(period_end)"),
				})
			} else {
				doUpdates = clause.Assignments(map[string]any{
					"up_bytes":    gorm.Expr("up_bytes + excluded.up_bytes"),
					"down_bytes":  gorm.Expr("down_bytes + excluded.down_bytes"),
					"billed_up":   gorm.Expr("billed_up + excluded.billed_up"),
					"billed_down": gorm.Expr("billed_down + excluded.billed_down"),
					"period_end":  gorm.Expr("excluded.period_end"),
				})
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "user_id"}, {Name: "inbound_id"}, {Name: "period_start"},
				},
				DoUpdates: doUpdates,
			}).Create(&log).Error; err != nil {
				return err
			}
			// 入站冗余计数只从 agent 携带 tag 的条目补计（入站维度条目已在上方单独处理，
			// 不会双计）。统计键反解出的入站仅用于流水归账/计费，不补计——inbound>>>
			// 计数器已按原始字节记账，用户条目再补计会双计。
			if tagInboundID > 0 {
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

// billingRule 单台服务器上一条「权限组 → 计费倍率」规则，来自该服务器启用用户入站的
// 授权组集合（AP 白名单派生）与入站倍率。
type billingRule struct {
	groups map[uint64]bool
	ratio  float64
}

// buildBillingRules 取该服务器启用用户入站的计费倍率规则（relay 入站不参与用户计费）。
func (s *TrafficService) buildBillingRules(serverID uint64) []billingRule {
	var inbs []models.Inbound
	if err := s.DB.Where("server_id = ? AND enabled = ? AND type = ?", serverID, true, models.InboundTypeUser).Find(&inbs).Error; err != nil {
		return nil
	}
	if len(inbs) == 0 {
		return nil
	}
	ids := make([]uint64, 0, len(inbs))
	for i := range inbs {
		ids = append(ids, inbs[i].ID)
	}
	groupMap := BatchInboundAuthorizedGroupIDs(s.DB, ids)
	rules := make([]billingRule, 0, len(inbs))
	for i := range inbs {
		groups := groupMap[inbs[i].ID]
		if len(groups) == 0 {
			continue // 未接入任何启用 AP 的入站不对任何人开放，不产生计费规则
		}
		gs := make(map[uint64]bool, len(groups))
		for _, g := range groups {
			gs[g] = true
		}
		rules = append(rules, billingRule{groups: gs, ratio: inbs[i].Ratio})
	}
	return rules
}

// billingRatioFor 用户计费倍率的兜底口径：生效组命中的该服务器入站倍率取最高。
// 仅用于统计键未注入入站维度的条目（旧格式过渡期/agent 携带 tag/入站已删等竞态）；
// 新格式 u<uid>.i<iid> 条目按该入站精确倍率计（Save 主路径），不再走这里。
// 无任何命中（未分组/入站未授权该组/用户行已删等竞态）回退 1；ratio=0 = 免费入站。
func billingRatioFor(rules []billingRule, groupID uint64) float64 {
	if groupID == 0 {
		return 1
	}
	found := false
	ratio := 0.0
	for _, r := range rules {
		if r.groups[groupID] && (!found || r.ratio > ratio) {
			ratio = r.ratio
			found = true
		}
	}
	if !found {
		return 1
	}
	return ratio
}

// roundBilled 按倍率折算并四舍五入到整字节（ratio=1 恒等还原，无浮点漂移）。
func roundBilled(b int64, ratio float64) int64 {
	if ratio == 1 {
		return b
	}
	return int64(math.Round(float64(b) * ratio))
}

// FindViolators 判定给定用户中已「违规」的（已过期或流量超额），供流量落库后事件驱动处置：
// 命中即热更节点用户列表将其移除，无需等 1h 校准。口径与 filterValidUsers 快照语义严格一致：
// 额度读用户行快照列 plan_traffic_bytes，用量读计费口径 billed 两列（原始字节 × 落库时倍率），
// 周期同口径（period_start >= traffic_cycle_start，零值起点 = 全量）。
// status 非活跃用户不在返回中（其移除由状态变更路径触发）。
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
		       COALESCE(SUM(CASE WHEN l.period_start >= u.traffic_cycle_start THEN l.billed_up + l.billed_down ELSE 0 END), 0) AS used_bytes,
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

// UserBilled 用户当前计费周期内已按倍率折算的用量（字节，计费口径）。
// 从 user.traffic_cycle_start 开始计算；若为零值（旧数据）则回溯全部。
// 读 billed 两列（落库时按生效入站倍率折算）：全部「已用/剩余额度」展示
// （管理端用户列表 / 用户主页 / Subscription-Userinfo）与配额判定
// （订阅 403 门 / 节点摘除 / 自动续费触发）唯一口径；
// 真实流量统计走 dashboard/入站计数的 SQL 聚合（原始口径），不经过本方法。
func (s *TrafficService) UserBilled(userID uint64) (up, down int64, err error) {
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
	err = q.Select("COALESCE(SUM(billed_up),0) AS up, COALESCE(SUM(billed_down),0) AS down").Scan(&row).Error
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
// 优化：采用 SQL 聚合 GROUP BY user_id, date(period_start) 直接输出天汇总，
// 避免将 7 天上千万条 logs 逐条实例化到 Go 内存中导致 OOM。
func (s *TrafficService) AggDaily() {
	windowStart := s.nowOrReal().AddDate(0, 0, -aggWindowDays)

	type aggRow struct {
		UserID    uint64 `gorm:"column:user_id"`
		Date      string `gorm:"column:date"`
		UpBytes   int64  `gorm:"column:up_bytes"`
		DownBytes int64  `gorm:"column:down_bytes"`
	}
	var rows []aggRow

	// SQLite 与 MySQL 均原生支持 date(period_start)
	if err := s.DB.Model(&models.TrafficLog{}).
		Select("user_id, date(period_start) AS date, SUM(up_bytes) AS up_bytes, SUM(down_bytes) AS down_bytes").
		Where("period_start >= ?", windowStart).
		Group("user_id, date(period_start)").
		Scan(&rows).Error; err != nil {
		return
	}

	for _, r := range rows {
		if r.Date == "" {
			continue
		}
		var existing models.TrafficDaily
		err := s.DB.Where("user_id = ? AND date = ?", r.UserID, r.Date).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = s.DB.Create(&models.TrafficDaily{
				UserID:    r.UserID,
				Date:      r.Date,
				UpBytes:   r.UpBytes,
				DownBytes: r.DownBytes,
			})
		} else if err == nil {
			_ = s.DB.Model(&existing).Updates(map[string]any{
				"up_bytes":   r.UpBytes,
				"down_bytes": r.DownBytes,
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
	// 保留天数可在数据管理页调整（settings 表），硬编码常量仅作缺省值。
	daysLogs := RetentionTrafficDays(s.DB)
	daysReports := RetentionNodeReportDays(s.DB)
	daysAudit := RetentionAuditDays(s.DB)
	cutLogs := now.AddDate(0, 0, -daysLogs)
	cutReports := now.AddDate(0, 0, -daysReports)
	cutAudit := now.AddDate(0, 0, -daysAudit)

	if res := s.DB.Where("period_start < ?", cutLogs).Delete(&models.TrafficLog{}); res.Error != nil {
		log.Printf("traffic: 清理 traffic_logs 失败: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("traffic: 清理 %d 条过期 traffic_logs（保留 %d 天）", res.RowsAffected, daysLogs)
	}
	if res := s.DB.Where("reported_at < ?", cutReports).Delete(&models.NodeReport{}); res.Error != nil {
		log.Printf("traffic: 清理 node_reports 失败: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("traffic: 清理 %d 条过期 node_reports（保留 %d 天）", res.RowsAffected, daysReports)
	}
	if res := s.DB.Where("created_at < ?", cutAudit).Delete(&models.AuditLog{}); res.Error != nil {
		log.Printf("traffic: 清理 audit_logs 失败: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("traffic: 清理 %d 条过期 audit_logs（保留 %d 天）", res.RowsAffected, daysAudit)
	}
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
