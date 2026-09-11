package models

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// dbTimesNormalizedMark 记录「存量时间已归一为 UTC」的一次性标记（存 settings 表）。
const dbTimesNormalizedMark = "db_times_normalized_utc"

// DBTimeLayout 是 SQLite 驱动实际落库的时间文本格式：空格分隔 + 数字偏移，
// UTC 落为 +00:00（不是 Z），小数秒按需保留（Go 的 .999… 会去掉尾零）。
// 必须与 glebarez/go-sqlite 的默认写格式（parseTimeFormats[0]）保持一致，
// 否则范围查询的字面量比较会错位。
const DBTimeLayout = "2006-01-02 15:04:05.999999999-07:00"

// FormatDBTime 把时间按库内实际存储格式（UTC）格式化为可比较文本。
func FormatDBTime(t time.Time) string { return t.UTC().Format(DBTimeLayout) }

// dbTimeLayouts 兼容驱动写入格式与历史版本/外部导入可能出现的多种写法。
// 无偏移的写法（如 "2006-01-02 15:04:05"）按 UTC 解释。
var dbTimeLayouts = []string{
	"2006-01-02 15:04:05.999999999-07:00", // 库内实际格式
	time.RFC3339Nano,                      // T 分隔（外部导入/JSON）
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// NormalizeTimesToUTC 把 SQLite 库中所有 datetime 列的存量值一次性归一为 UTC 文本。
//
// 背景：进程本地时区已在 cmd/master/main.go 固定为 UTC，新写入的时间恒为 UTC，
// 格式与 DBTimeLayout 一致。但历史库、曾以非 UTC 时区运行的实例、或外部导入的库
// 可能残留带本地偏移的文本（如 2026-09-11 20:00:00+08:00）。SQLite 以带偏移文本按字面量比较，
// 偏移不一致会让
// 范围查询整体偏移数小时——受影响的有审计时间筛选、仪表盘今日/本月统计、日志清理边界等。
//
// 本迁移只改写「偏移非 UTC」的行，已是 UTC（以 Z 或 +00:00 结尾）的行原样保留，
// 因此幂等可重跑；改写结果与驱动写入格式（DBTimeLayout，UTC）完全一致，保证新老行可比。
// MySQL 的 DATETIME 无偏移、驱动按会话时区读写，无需文本归一。
func NormalizeTimesToUTC(db *gorm.DB) error {
	if db.Dialector.Name() != "sqlite" {
		return nil
	}
	var markCount int64
	if err := db.Model(&Setting{}).Where("key = ?", dbTimesNormalizedMark).Count(&markCount).Error; err != nil {
		return err
	}
	if markCount > 0 {
		return nil
	}

	total := 0
	for _, model := range All() {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(model); err != nil {
			return err
		}
		table := stmt.Schema.Table
		// 复合主键/无 id 的表（如 permission_group_access_points）不含 datetime 列，直接跳过。
		if !db.Migrator().HasColumn(model, "id") {
			continue
		}
		cols, cerr := db.Migrator().ColumnTypes(model)
		if cerr != nil {
			continue // 表未建（部分测试只迁移子集）时静默跳过
		}
		for _, col := range cols {
			dt := strings.ToLower(col.DatabaseTypeName())
			if !strings.Contains(dt, "datetime") && !strings.Contains(dt, "timestamp") {
				continue
			}
			n, nerr := normalizeColumnToUTC(db, table, col.Name())
			if nerr != nil {
				return fmt.Errorf("归一表 %s.%s 时间失败: %w", table, col.Name(), nerr)
			}
			total += n
		}
	}
	if total > 0 {
		log.Printf("时区归一：已将 %d 个非 UTC 时间戳改写为 UTC", total)
	}
	return db.Create(&Setting{Key: dbTimesNormalizedMark, Value: "1"}).Error
}

// normalizeColumnToUTC 归一单列：读取全部非空值，凡文本与规范格式（FormatDBTime，UTC）
// 不一致的行都改写——既修偏移（+08:00→+00:00），也修分隔符/格式（T/Z→空格/+00:00）。
// 需与规范格式逐字比较，故不能只按后缀预筛（例如 "…T12:00:00Z" 与 "…12:00:00.500+00:00"
// 后缀看似 UTC，实际都不是库内格式）。一次性执行，代价可接受。
func normalizeColumnToUTC(db *gorm.DB, table, col string) (int, error) {
	// 用 CAST(... AS TEXT) 读原始文本：直接把 datetime 列 Scan 到 string 会被 GORM
	// 按时间类型重新格式化，拿不到库里的真实字面量。
	rows, err := db.Table(table).
		Select(fmt.Sprintf("id, CAST(`%s` AS TEXT) AS val", col)).
		Where(fmt.Sprintf("`%s` IS NOT NULL AND `%s` <> ''", col, col)).
		Rows()
	if err != nil {
		return 0, err
	}
	type pending struct {
		id    uint64
		canon string
	}
	var todo []pending
	for rows.Next() {
		var id uint64
		var val string
		if err := rows.Scan(&id, &val); err != nil {
			rows.Close()
			return 0, err
		}
		t, ok := parseDBTime(val)
		if !ok {
			continue // 非时间文本（脏数据）保持原样，避免误伤
		}
		if canon := FormatDBTime(t); canon != val {
			todo = append(todo, pending{id, canon})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	// 先读完再写：避免在同一连接上边开结果集边更新。
	for _, p := range todo {
		if err := db.Table(table).Where("id = ?", p.id).UpdateColumn(col, p.canon).Error; err != nil {
			return 0, err
		}
	}
	return len(todo), nil
}

func parseDBTime(s string) (time.Time, bool) {
	for _, layout := range dbTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
