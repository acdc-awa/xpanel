// 数据管理：日志量可视化、按日期清理、SQLite 空间回收（VACUUM）。
// 安全约束见 services.TrafficSafeDeleteBefore 与各 handler 注释。
package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// shiftedDayExpr 把存储的 UTC 时间按业务时区偏移后取日期（YYYY-MM-DD），用于日志量按天分桶。
// 与「按业务时区切天」的标签口径保持一致，避免 UTC 天与业务天错位。
// field 取自 logTables 白名单常量，driver 为 Dialector 名。
func shiftedDayExpr(driver, field string, offMin int) string {
	if driver == "mysql" {
		return fmt.Sprintf("DATE(DATE_ADD(%s, INTERVAL %d MINUTE))", field, offMin)
	}
	// SQLite：datetime() 可解析 'YYYY-MM-DD HH:MM:SS+00:00' 并按修饰符换算
	return fmt.Sprintf("substr(datetime(%s, '%+d minutes'), 1, 10)", field, offMin)
}

// logTables 可清理的日志表 → 时间字段（白名单，表名/字段名不进任何用户输入）。
// traffic_logs 按 period_start（与计费周期、每日聚合同源）；本地日期分桶与删除边界一致。
var logTables = map[string]string{
	"traffic_logs": "period_start",
	"node_reports": "reported_at",
	"audit_logs":   "created_at",
}

// vacuumMu 序列化 VACUUM（回收期间独占写连接，防连点叠加）。
var vacuumMu sync.Mutex

type logDayStat struct {
	Date        string `json:"date"`
	TrafficLogs int64  `json:"traffic_logs"`
	NodeReports int64  `json:"node_reports"`
	AuditLogs   int64  `json:"audit_logs"`
}

// AdminLogStats GET /api/v1/admin/data/log-stats?days=30
// 各日志表每日行数（本地日期分桶，缺天补零）+ 总行数 + traffic_logs 安全清理上界 +
// 保留天数生效值 + 数据库文件大小。
func (d *Deps) AdminLogStats(c *gin.Context) {
	days := 30
	if v := c.Query("days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			days = n
		}
	}
	if days < 7 {
		days = 7
	}
	if days > 90 {
		days = 90
	}

	loc := services.BusinessLocation(d.DB)
	now := time.Now()
	today := services.DayStart(now, loc)
	start := today.AddDate(0, 0, -(days - 1))
	// 业务时区相对 UTC 的分钟偏移（用于把存储的 UTC 时间换算成业务日期）
	_, offSec := now.In(loc).Zone()
	offMin := offSec / 60

	type dayCount struct {
		D string `gorm:"column:d"`
		N int64  `gorm:"column:n"`
	}
	byDate := make(map[string]*logDayStat, days)
	dayList := make([]logDayStat, 0, days)
	for i := 0; i < days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		byDate[date] = &logDayStat{Date: date}
	}
	for table, field := range logTables {
		var rows []dayCount
		q := d.DB.Table(table).
			Select(shiftedDayExpr(d.DB.Dialector.Name(), field, offMin)+" AS d, COUNT(*) AS n").
			Where(field+" >= ?", start.UTC()).
			Group("d")
		if err := q.Scan(&rows).Error; err != nil {
			util.ServerError(c, "统计失败")
			return
		}
		for _, r := range rows {
			if s, ok := byDate[r.D]; ok {
				switch table {
				case "traffic_logs":
					s.TrafficLogs = r.N
				case "node_reports":
					s.NodeReports = r.N
				default:
					s.AuditLogs = r.N
				}
			}
		}
	}
	for i := 0; i < days; i++ {
		dayList = append(dayList, *byDate[start.AddDate(0, 0, i).Format("2006-01-02")])
	}

	totals := gin.H{}
	for table := range logTables {
		var total int64
		if err := d.DB.Table(table).Count(&total).Error; err != nil {
			util.ServerError(c, "统计失败")
			return
		}
		totals[table] = total
	}

	resp := gin.H{
		"days":               dayList,
		"totals":             totals,
		"traffic_min_delete": services.TrafficSafeDeleteBefore(d.DB, now),
		"retention":          services.RetentionSettingsGroup(d.DB),
		"sqlite_avail":       d.Cfg != nil && d.Cfg.DB.Driver == "sqlite",
	}
	if resp["sqlite_avail"] == true {
		path := strings.SplitN(d.Cfg.DB.DSN, "?", 2)[0]
		dbSize, walSize := sqliteFileSizes(path)
		resp["db_size"] = dbSize
		resp["wal_size"] = walSize
	}
	util.OK(c, resp)
}

// AdminCleanupLogs POST /api/v1/admin/data/logs/cleanup
// {table, before}：删除时间字段早于 before（YYYY-MM-DD 00:00）的日志行。
// traffic_logs 受安全上界约束（计费周期 + 每日聚合窗口）；审计由中间件自动落库。
func (d *Deps) AdminCleanupLogs(c *gin.Context) {
	var req struct {
		Table  string `json:"table" binding:"required"`
		Before string `json:"before" binding:"required"`
	}
	if !util.BindJSON(c, &req) {
		return
	}
	field, ok := logTables[req.Table]
	if !ok {
		util.BadRequest(c, "无效的日志类型")
		return
	}
	before, err := time.ParseInLocation("2006-01-02", req.Before, services.BusinessLocation(d.DB))
	if err != nil {
		util.BadRequest(c, "无效的日期格式，需为 YYYY-MM-DD")
		return
	}
	now := time.Now()
	if before.After(now) {
		util.BadRequest(c, "清理日期不能晚于今天")
		return
	}
	if req.Table == "traffic_logs" {
		safe := services.TrafficSafeDeleteBefore(d.DB, now)
		if safe == "" {
			util.ServerError(c, "暂时无法校验安全清理日期，请稍后再试")
			return
		}
		if req.Before > safe {
			util.BadRequest(c, "超出安全清理范围：流量明细受计费周期与每日聚合保护，仅可清理 "+safe+" 及更早的日期")
			return
		}
	}

	var model any
	switch req.Table {
	case "traffic_logs":
		model = &models.TrafficLog{}
	case "node_reports":
		model = &models.NodeReport{}
	default:
		model = &models.AuditLog{}
	}
	res := d.DB.Where(field+" < ?", before.UTC()).Delete(model)
	if res.Error != nil {
		util.ServerError(c, "清理失败")
		return
	}
	util.OK(c, gin.H{"table": req.Table, "before": req.Before, "deleted": res.RowsAffected})
}

// AdminVacuum POST /api/v1/admin/data/vacuum —— SQLite 在线回收空间。
// 先 TRUNCATE checkpoint 落盘 WAL，VACUUM 后再 checkpoint，返回前后文件尺寸。
// 回收期间独占数据库写连接（短暂阻塞其他写请求），由前端提示低峰执行。
func (d *Deps) AdminVacuum(c *gin.Context) {
	if d.Cfg == nil || d.Cfg.DB.Driver != "sqlite" {
		util.BadRequest(c, "当前数据库不支持在线回收空间")
		return
	}
	if !vacuumMu.TryLock() {
		util.BadRequest(c, "空间回收正在进行中，请稍候")
		return
	}
	defer vacuumMu.Unlock()
	path := strings.SplitN(d.Cfg.DB.DSN, "?", 2)[0]
	if path == "" {
		util.ServerError(c, "数据库路径未配置")
		return
	}
	d.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	dbBefore, walBefore := sqliteFileSizes(path)
	if err := d.DB.Exec("VACUUM").Error; err != nil {
		util.Fail(c, http.StatusInternalServerError, "空间回收失败")
		return
	}
	d.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	dbAfter, walAfter := sqliteFileSizes(path)
	util.OK(c, gin.H{
		"db_size_before":  dbBefore,
		"wal_size_before": walBefore,
		"db_size_after":   dbAfter,
		"wal_size_after":  walAfter,
		"reclaimed":       (dbBefore + walBefore) - (dbAfter + walAfter),
	})
}

func sqliteFileSizes(path string) (dbSize, walSize int64) {
	if st, err := os.Stat(path); err == nil {
		dbSize = st.Size()
	}
	if st, err := os.Stat(path + "-wal"); err == nil {
		walSize = st.Size()
	}
	return
}

// AdminCompact POST /api/v1/admin/data/compact —— 压缩历史明细（在线迁移）。
// 把存量 traffic_logs 归并到小时桶、node_reports 抽稀到分钟并清理超期行，口径不变、幂等；
// SQLite 下随后 VACUUM 回收磁盘（既有库 auto_vacuum 多为 NONE，仅 incremental_vacuum 不生效，
// 全量 VACUUM 才会真正缩小文件并转换为增量回收模式）。与「回收空间」共用 vacuumMu 串行化。
func (d *Deps) AdminCompact(c *gin.Context) {
	if !vacuumMu.TryLock() {
		util.BadRequest(c, "数据维护正在进行中，请稍候")
		return
	}
	defer vacuumMu.Unlock()

	isSQLite := d.Cfg != nil && d.Cfg.DB.Driver == "sqlite"
	var path string
	var dbBefore, walBefore int64
	if isSQLite {
		path = strings.SplitN(d.Cfg.DB.DSN, "?", 2)[0]
		dbBefore, walBefore = sqliteFileSizes(path)
	}

	st, err := services.CompactAll(d.DB, time.Now())
	if err != nil {
		log.Printf("data: 历史数据压缩失败: %v", err)
		util.ServerError(c, "压缩失败，请查看服务端日志")
		return
	}

	resp := gin.H{"stats": st}
	if !isSQLite {
		util.OK(c, resp)
		return
	}

	d.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	if err := d.DB.Exec("VACUUM").Error; err != nil {
		// 明细已压缩完成，仅空间回收失败：如实返回压缩统计并提示重试回收。
		log.Printf("data: 压缩后空间回收失败: %v", err)
		resp["vacuum_error"] = "空间回收失败，请稍后重试"
	} else {
		d.DB.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	}
	dbAfter, walAfter := sqliteFileSizes(path)
	resp["db_size_before"] = dbBefore
	resp["wal_size_before"] = walBefore
	resp["db_size_after"] = dbAfter
	resp["wal_size_after"] = walAfter
	resp["reclaimed"] = (dbBefore + walBefore) - (dbAfter + walAfter)
	util.OK(c, resp)
}
