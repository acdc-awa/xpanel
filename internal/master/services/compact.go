// 历史数据压缩迁移（数据管理页「压缩历史数据」）。
//
// 背景：traffic_logs 早期按「每次上报一行」落库（period_start 为发送时刻），
// node_reports 按每次心跳一行（默认 30s）落库，几天即可堆积上百 MB。
// 写入侧已改为按小时分桶 / 按分钟采样，但存量数据不会自动收敛，故提供一次
// 幂等的在线压缩：把存量明细归并到与新写入一致的桶粒度，再回收磁盘。
//
// 设计与安全约束：
//   - 纯 Go 聚合，不依赖 SQLite/MySQL 各自的日期函数文本格式，跨驱动一致；
//   - 按小时（traffic）/按服务器天（node_reports）分块，每块一个短事务，
//     避免长事务长时间独占写连接（SQLite 单写者）；
//   - 只归并「已完整结束」的桶（跳过当前及尚未结束的小时/分钟），不与实时上报竞争；
//   - 归并保持 up/down/billed 求和、period 最小 created_at、最大 period_end，
//     计费（周期内 SUM）、排名、每日汇总口径完全不变，可重复执行（幂等）。
package services

import (
	"time"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// nodeReportBucket 最细心跳压缩粒度，与 nodegate.nodeReportSampleInterval 保持一致。
const nodeReportBucket = time.Minute

// 节点曲线分级降采样档位：与读取端（api/servers.go AdminServerMetrics）二次分桶的最细粒度对齐，
// 使降采样对任何可回看区间都不降分辨率：
//   - 近 6h  → 1 分钟（1h 视图@1m、6h 视图@3m）
//   - 6–24h → 10 分钟（24h 视图@10m）
//   - 24h 以上 → 1 小时（7d 视图@1h；7d 之外已不展示，仅留粗历史）
const (
	nodeTier1Window = 6 * time.Hour
	nodeTier2Window = 24 * time.Hour
	nodeTier2Bucket = 10 * time.Minute
	nodeTier3Bucket = time.Hour
)

// nodeBucketFor 返回某行按「行龄」应归入的降采样桶起点（桶边界按绝对时间对齐）。
func nodeBucketFor(reportedAt, now time.Time) time.Time {
	age := now.Sub(reportedAt)
	switch {
	case age <= nodeTier1Window:
		return reportedAt.Truncate(nodeReportBucket)
	case age <= nodeTier2Window:
		return reportedAt.Truncate(nodeTier2Bucket)
	default:
		return reportedAt.Truncate(nodeTier3Bucket)
	}
}

// deleteIDBatch 单条 DELETE ... IN 的 id 数量上限，规避 SQLite 变量数限制。
const deleteIDBatch = 500

// CompactStats 压缩前后行数统计。
type CompactStats struct {
	TrafficRowsBefore  int64 `json:"traffic_rows_before"`
	TrafficRowsAfter   int64 `json:"traffic_rows_after"`
	TrafficRowsRemoved int64 `json:"traffic_rows_removed"`
	NodeRowsBefore     int64 `json:"node_rows_before"`
	NodeRowsAfter      int64 `json:"node_rows_after"`
	NodeRowsRemoved    int64 `json:"node_rows_removed"`
}

// CompactTrafficLogs 把存量 traffic_logs 明细归并到小时桶（与 trafficPeriodBucket 一致）：
// 同一 (user_id, inbound_id, 整点小时) 的多行合并为一行。返回压缩前/后行数。
func CompactTrafficLogs(db *gorm.DB, now time.Time) (before, after int64, err error) {
	if err = db.Model(&models.TrafficLog{}).Count(&before).Error; err != nil {
		return 0, 0, err
	}
	if before == 0 {
		return 0, 0, nil
	}

	// 存量 period_start 由 agent 以 UTC 上报，统一按 UTC 计算桶边界，保证文本可比较。
	var oldest models.TrafficLog
	if err = db.Order("period_start ASC").Limit(1).Find(&oldest).Error; err != nil {
		return before, before, err
	}
	startHour := oldest.PeriodStart.UTC().Truncate(trafficPeriodBucket)
	// 当前小时尚未结束，跳过；只处理 lastHour 及更早的完整小时。
	lastHour := now.UTC().Truncate(trafficPeriodBucket).Add(-trafficPeriodBucket)

	type groupKey struct {
		userID    uint64
		inboundID uint64
	}
	type agg struct {
		up, down        int64
		billedUp, down2 int64
		createdAt       time.Time
		periodEnd       time.Time
	}

	for h := startHour; !h.After(lastHour); h = h.Add(trafficPeriodBucket) {
		var rows []models.TrafficLog
		if err = db.Where("period_start >= ? AND period_start < ?", h, h.Add(trafficPeriodBucket)).
			Find(&rows).Error; err != nil {
			return before, before, err
		}
		if len(rows) == 0 {
			continue
		}

		groups := make(map[groupKey]*agg, len(rows))
		for i := range rows {
			k := groupKey{rows[i].UserID, rows[i].InboundID}
			g := groups[k]
			if g == nil {
				g = &agg{}
				groups[k] = g
			}
			g.up += rows[i].UpBytes
			g.down += rows[i].DownBytes
			g.billedUp += rows[i].BilledUp
			g.down2 += rows[i].BilledDown
			if g.createdAt.IsZero() || rows[i].CreatedAt.Before(g.createdAt) {
				g.createdAt = rows[i].CreatedAt
			}
			if rows[i].PeriodEnd.After(g.periodEnd) {
				g.periodEnd = rows[i].PeriodEnd
			}
		}

		// 已是「一桶一行」则跳过，避免无谓重写（幂等、可反复执行）。
		if len(rows) == len(groups) {
			compacted := true
			for i := range rows {
				if !rows[i].PeriodStart.Equal(h) {
					compacted = false
					break
				}
			}
			if compacted {
				continue
			}
		}

		merged := make([]models.TrafficLog, 0, len(groups))
		for k, g := range groups {
			merged = append(merged, models.TrafficLog{
				UserID:      k.userID,
				InboundID:   k.inboundID,
				UpBytes:     g.up,
				DownBytes:   g.down,
				BilledUp:    g.billedUp,
				BilledDown:  g.down2,
				PeriodStart: h,
				PeriodEnd:   g.periodEnd,
				CreatedAt:   g.createdAt,
			})
		}

		err = db.Transaction(func(tx *gorm.DB) error {
			if e := tx.Where("period_start >= ? AND period_start < ?", h, h.Add(trafficPeriodBucket)).
				Delete(&models.TrafficLog{}).Error; e != nil {
				return e
			}
			if len(merged) > 0 {
				return tx.CreateInBatches(&merged, 200).Error
			}
			return nil
		})
		if err != nil {
			return before, before, err
		}
	}

	if err = db.Model(&models.TrafficLog{}).Count(&after).Error; err != nil {
		return before, before, err
	}
	return before, after, nil
}

// CompactNodeReports 分级压缩存量 node_reports：
//  1. 删除超出 RetentionNodeReportDays 的行（与保留策略一致，回收最长历史）；
//  2. 保留期内按行龄分级降采样（见 nodeTier* 常量）：桶内每服务器保留最早一行，删除其余。
//     近 6h 保 1 分钟、6–24h 保 10 分钟、24h 以上保 1 小时。
//
// 心跳 reported_at 以 UTC 写入（main 固定 time.Local=UTC），桶边界按绝对时间对齐。
func CompactNodeReports(db *gorm.DB, now time.Time) (before, after int64, err error) {
	if err = db.Model(&models.NodeReport{}).Count(&before).Error; err != nil {
		return 0, 0, err
	}
	if before == 0 {
		return 0, 0, nil
	}

	cutoff := now.AddDate(0, 0, -RetentionNodeReportDays(db))
	if res := db.Where("reported_at < ?", cutoff).Delete(&models.NodeReport{}); res.Error != nil {
		return before, before, res.Error
	}

	var serverIDs []uint64
	if err = db.Model(&models.NodeReport{}).Distinct().Pluck("server_id", &serverIDs).Error; err != nil {
		return before, before, err
	}

	loc := now.Location()
	dayStart := func(t time.Time) time.Time {
		y, m, d := t.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	currentMinute := now.Truncate(nodeReportBucket) // 当前分钟可能仍在写入，跳过
	startDay := dayStart(cutoff)
	endDay := dayStart(now)

	for _, sid := range serverIDs {
		for d := startDay; !d.After(endDay); d = d.AddDate(0, 0, 1) {
			var rows []models.NodeReport
			if err = db.Where("server_id = ? AND reported_at >= ? AND reported_at < ?", sid, d, d.AddDate(0, 0, 1)).
				Order("reported_at ASC").Find(&rows).Error; err != nil {
				return before, before, err
			}
			if len(rows) < 2 {
				continue
			}
			seen := make(map[int64]struct{}, len(rows))
			var delIDs []uint64
			for i := range rows {
				ra := rows[i].ReportedAt
				if !ra.Before(currentMinute) {
					continue
				}
				b := nodeBucketFor(ra, now).Unix()
				if _, ok := seen[b]; ok {
					delIDs = append(delIDs, rows[i].ID)
					continue
				}
				seen[b] = struct{}{}
			}
			for i := 0; i < len(delIDs); i += deleteIDBatch {
				end := i + deleteIDBatch
				if end > len(delIDs) {
					end = len(delIDs)
				}
				if err = db.Where("id IN ?", delIDs[i:end]).Delete(&models.NodeReport{}).Error; err != nil {
					return before, before, err
				}
			}
		}
	}

	if err = db.Model(&models.NodeReport{}).Count(&after).Error; err != nil {
		return before, before, err
	}
	return before, after, nil
}

// CompactAll 依次执行两类压缩并汇总统计（traffic 先于 node，互不依赖）。
func CompactAll(db *gorm.DB, now time.Time) (*CompactStats, error) {
	st := &CompactStats{}
	b, a, err := CompactTrafficLogs(db, now)
	if err != nil {
		return nil, err
	}
	st.TrafficRowsBefore, st.TrafficRowsAfter = b, a
	st.TrafficRowsRemoved = b - a

	b, a, err = CompactNodeReports(db, now)
	if err != nil {
		return nil, err
	}
	st.NodeRowsBefore, st.NodeRowsAfter = b, a
	st.NodeRowsRemoved = b - a
	return st, nil
}
