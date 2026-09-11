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
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// 存量日志一次性压缩迁移（启动期执行）的 settings 键与版本。
const (
	// SettingLogsCompacted 存量日志压缩迁移完成标记，值 = logsCompactionVersion。
	SettingLogsCompacted = "logs_compacted_version"
	// SettingLogsCompactedAt 迁移完成时刻（RFC3339，排障用）。
	SettingLogsCompactedAt = "logs_compacted_at"
	// SettingLogsCompactedResult 迁移前后行数摘要（排障用）。
	SettingLogsCompactedResult = "logs_compacted_result"
	// settingTrafficCompactCursor 流量压缩断点（已完成到的小时起点，RFC3339）。
	settingTrafficCompactCursor = "traffic_compact_cursor"

	// logsCompactionVersion 压缩迁移自身版本，与 DBSchemaVersion 解耦：
	// 本迁移 sum 守恒、旧面板仍可安全读写，故不提升 schema 版本、不阻断回滚。
	logsCompactionVersion = 1
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

	protect, perr := loadCycleStartProtection(db)
	if perr != nil {
		return before, before, perr
	}
	for h := startHour; !h.After(lastHour); h = h.Add(trafficPeriodBucket) {
		if err = compactTrafficHour(db, h, protect[hourBucketKey(h)]); err != nil {
			return before, before, err
		}
	}

	if err = db.Model(&models.TrafficLog{}).Count(&after).Error; err != nil {
		return before, before, err
	}
	return before, after, nil
}

// hourBucketKey 小时桶的整数键（Unix 秒），用作保护表的 map 键（time.Time 含时区/单调时钟不宜直接比较）。
func hourBucketKey(h time.Time) int64 {
	return h.UTC().Truncate(trafficPeriodBucket).Unix()
}

// loadCycleStartProtection 构建「周期起点落在某小时桶内」的用户集合。
//
// 背景：计费/配额按 `period_start >= user.traffic_cycle_start` 统计，而压缩会把同一小时的多行
// 合并到整点。若某用户的 cycle_start 落在这个小时中间，合并行的 period_start（整点）
// 会早于 cycle_start，导致该小时整段被排除，最多漏计 1 小时流量。
// 因此这些用户在该小时桶内的行不参与合并（原样保留），合计与计费口径逐字节不变。
//
// users 表不存在（部分测试仅迁移子集）时返回空集合，不阻断压缩。
func loadCycleStartProtection(db *gorm.DB) (map[int64]map[uint64]bool, error) {
	out := map[int64]map[uint64]bool{}
	if !db.Migrator().HasTable(&models.User{}) {
		return out, nil
	}
	var users []models.User
	if err := db.Select("id", "traffic_cycle_start").Find(&users).Error; err != nil {
		return nil, err
	}
	for i := range users {
		if users[i].TrafficCycleStart.IsZero() {
			continue // 零值起点 = 全量统计，无边界问题
		}
		k := hourBucketKey(users[i].TrafficCycleStart)
		if out[k] == nil {
			out[k] = map[uint64]bool{}
		}
		out[k][users[i].ID] = true
	}
	return out, nil
}

// compactTrafficHour 合并单个小时桶 [h, h+1) 内的 traffic_logs 明细：
// 同一 (user_id, inbound_id) 归并为一行（sum 各字节列、保留最早 created_at / 最晚 period_end）；
// protect 中的用户在该小时的行不参与合并，原样保留（见 loadCycleStartProtection）。
// 已是「一桶一行且全在整点」时跳过，幂等可反复执行。
func compactTrafficHour(db *gorm.DB, h time.Time, protect map[uint64]bool) error {
	var rows []models.TrafficLog
	if err := db.Where("period_start >= ? AND period_start < ?", h, h.Add(trafficPeriodBucket)).
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

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

	groups := make(map[groupKey]*agg, len(rows))
	mergeCount := 0
	for i := range rows {
		if protect[rows[i].UserID] {
			continue
		}
		mergeCount++
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
	if mergeCount == 0 {
		return nil // 该小时全为受保护行，无可合并
	}

	// 已是「一桶一行且全在整点」（且无受保护行）则跳过，避免无谓重写。
	if mergeCount == len(rows) && len(rows) == len(groups) {
		compacted := true
		for i := range rows {
			if !rows[i].PeriodStart.Equal(h) {
				compacted = false
				break
			}
		}
		if compacted {
			return nil
		}
	}

	merged := make([]models.TrafficLog, 0, len(groups)+len(rows)-mergeCount)
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
	// 受保护用户的行原样回抄（ID 归零由库重分配，无外部引用；created_at/period_end 保留）。
	for i := range rows {
		if protect[rows[i].UserID] {
			r := rows[i]
			r.ID = 0
			merged = append(merged, r)
		}
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Where("period_start >= ? AND period_start < ?", h, h.Add(trafficPeriodBucket)).
			Delete(&models.TrafficLog{}).Error; e != nil {
			return e
		}
		return tx.CreateInBatches(&merged, 200).Error
	})
}

// NeedsTrafficCompaction 廉价探测是否存在「真正待合并」的明细（旧版本面板写入/回滚/迁移中断）。
// 已按小时分桶的库恒为 false，供每日维护跳过全量压缩扫描。
//
// 周期起点落在桶内的受保护行（见 loadCycleStartProtection）本就不参与合并，须排除在外，
// 否则这些历史行会让探测永远为真、每日触发全量扫描。
func NeedsTrafficCompaction(db *gorm.DB) (bool, error) {
	isMySQL := db.Dialector != nil && db.Dialector.Name() == "mysql"
	q := db.Model(&models.TrafficLog{}).Select("1")
	if isMySQL {
		q = q.Where("MINUTE(period_start) <> 0 OR SECOND(period_start) <> 0")
	} else {
		// SQLite 时间文本格式 "2006-01-02 15:04:05.999999999-07:00"，第 15–19 位即 "MM:SS"。
		q = q.Where("substr(period_start, 15, 5) <> '00:00'")
	}
	if db.Migrator().HasTable(&models.User{}) {
		if isMySQL {
			q = q.Joins("LEFT JOIN users u ON u.id = traffic_logs.user_id").
				Where("u.traffic_cycle_start IS NULL OR DATE_FORMAT(traffic_logs.period_start, '%Y-%m-%d %H') <> DATE_FORMAT(u.traffic_cycle_start, '%Y-%m-%d %H')")
		} else {
			q = q.Joins("LEFT JOIN users u ON u.id = traffic_logs.user_id").
				Where("u.traffic_cycle_start IS NULL OR substr(traffic_logs.period_start, 1, 13) <> substr(u.traffic_cycle_start, 1, 13)")
		}
	}
	var one int
	if err := q.Limit(1).Scan(&one).Error; err != nil {
		return false, err
	}
	return one == 1, nil
}

// compactTrafficLogsResumable 存量流量明细的可续跑压缩：逐小时合并并持久化断点，
// 进程中断后下次启动从断点续跑（而非从头）。当前小时不处理（未结束）。
func compactTrafficLogsResumable(db *gorm.DB, now time.Time) error {
	var oldest models.TrafficLog
	if err := db.Order("period_start ASC").Limit(1).Find(&oldest).Error; err != nil {
		return err
	}
	if oldest.ID == 0 {
		return nil // 无明细
	}
	startHour := oldest.PeriodStart.UTC().Truncate(trafficPeriodBucket)
	lastHour := now.UTC().Truncate(trafficPeriodBucket).Add(-trafficPeriodBucket)
	if cur := strings.TrimSpace(GetSetting(db, settingTrafficCompactCursor)); cur != "" {
		if t, perr := time.Parse(time.RFC3339, cur); perr == nil {
			if next := t.UTC().Truncate(trafficPeriodBucket).Add(trafficPeriodBucket); next.After(startHour) {
				startHour = next
			}
		}
	}
	if startHour.After(lastHour) {
		return nil
	}
	protect, err := loadCycleStartProtection(db)
	if err != nil {
		return err
	}
	for h := startHour; !h.After(lastHour); h = h.Add(trafficPeriodBucket) {
		if err := compactTrafficHour(db, h, protect[hourBucketKey(h)]); err != nil {
			return fmt.Errorf("压缩流量小时桶 %s 失败: %w", h.Format(time.RFC3339), err)
		}
		if err := SetSetting(db, settingTrafficCompactCursor, h.Format(time.RFC3339)); err != nil {
			return err
		}
	}
	return nil
}

// MigrateLegacyLogs 老库存量日志一次性压缩迁移：启动期（节点未接入、cron 未启动）独占写窗口内调用。
//   - 幂等：settings 标记 logs_compacted_version ≥ logsCompactionVersion 时整体跳过；
//   - 可续跑：流量按小时持久化断点，中断后从断点继续；
//   - 口径不变：合并仅做同键 SUM，且周期起点落在桶内的用户行不合并（见 loadCycleStartProtection）；
//   - 空间回收：SQLite 下压缩后 VACUUM（老库 auto_vacuum=NONE，DELETE 不缩盘），失败仅告警不中断。
//
// 失败返回错误，调用方应中止启动（迁移 sum 守恒且幂等，重试安全）。
func MigrateLegacyLogs(db *gorm.DB, appVersion string) error {
	if !db.Migrator().HasTable(&models.TrafficLog{}) {
		return nil
	}
	if v, _ := strconv.Atoi(strings.TrimSpace(GetSetting(db, SettingLogsCompacted))); v >= logsCompactionVersion {
		return nil
	}

	now := time.Now().UTC()
	var trafficBefore, nodeBefore int64
	if err := db.Model(&models.TrafficLog{}).Count(&trafficBefore).Error; err != nil {
		return err
	}
	if err := db.Model(&models.NodeReport{}).Count(&nodeBefore).Error; err != nil {
		return err
	}
	log.Printf("compact: 开始存量日志一次性压缩迁移（traffic_logs %d 行 / node_reports %d 行）…", trafficBefore, nodeBefore)
	startedAt := time.Now()

	if err := compactTrafficLogsResumable(db, now); err != nil {
		return err
	}
	if _, _, err := CompactNodeReports(db, now); err != nil {
		return err
	}

	var trafficAfter, nodeAfter int64
	if err := db.Model(&models.TrafficLog{}).Count(&trafficAfter).Error; err != nil {
		return err
	}
	if err := db.Model(&models.NodeReport{}).Count(&nodeAfter).Error; err != nil {
		return err
	}
	removed := (trafficBefore - trafficAfter) + (nodeBefore - nodeAfter)
	if removed > 0 {
		reclaimSpaceAfterCompaction(db)
	}

	result := fmt.Sprintf("traffic %d->%d, node %d->%d, removed %d, took %s, by %s",
		trafficBefore, trafficAfter, nodeBefore, nodeAfter, removed, time.Since(startedAt).Round(time.Millisecond), appVersion)
	for k, v := range map[string]string{
		SettingLogsCompacted:       strconv.Itoa(logsCompactionVersion),
		SettingLogsCompactedAt:     now.Format(time.RFC3339),
		SettingLogsCompactedResult: result,
	} {
		if err := SetSetting(db, k, v); err != nil {
			return fmt.Errorf("写入迁移标记 %s 失败: %w", k, err)
		}
	}
	deleteSetting(db, settingTrafficCompactCursor)
	log.Printf("compact: 存量日志一次性压缩迁移完成（%s）", result)
	return nil
}

// reclaimSpaceAfterCompaction SQLite 在线回收：DELETE 只把页还给 freelist、文件不缩，
// 老库 auto_vacuum=NONE 时增量回收亦无效，需全量 VACUUM（同时把库转为增量回收模式）。
// 启动期无并发写者，且 VACUUM 有事务保护，失败仅告警（数据已压缩，可稍后手动重试）。
func reclaimSpaceAfterCompaction(db *gorm.DB) {
	if db.Dialector == nil || db.Dialector.Name() != "sqlite" {
		return
	}
	db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	if err := db.Exec("VACUUM").Error; err != nil {
		log.Printf("compact: 迁移后空间回收失败（明细已压缩，可稍后在数据管理页重试回收）: %v", err)
		return
	}
	db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
}

// deleteSetting 删除一个 settings 键（键不存在时静默）。
func deleteSetting(db *gorm.DB, key string) {
	db.Where("key = ?", key).Delete(&models.Setting{})
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
