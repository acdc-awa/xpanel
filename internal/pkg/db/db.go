// Package db 负责按配置打开数据库（SQLite 为开发/生产默认，MySQL 为保留驱动）。
package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/acdc/xray-panel/internal/config"
)

// sqliteDSN 追加生产化 pragma：
//   - busy_timeout（ISSUE-15）：并发写冲突时等待而不是立刻返回 database is locked；
//   - journal_mode(WAL)：读写不互斥，崩溃恢复粒度更优，备份（VACUUM INTO）自动包含 WAL 数据；
//   - synchronous(NORMAL)：WAL 下安全（不损原子性），仅断电可能丢最后一批已提交事务。
func sqliteDSN(dsn string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
}

// Open 按配置打开数据库连接，返回 *gorm.DB。
func Open(cfg *config.DB) (*gorm.DB, error) {
	gcfg := &gorm.Config{}
	if cfg.Driver == "sqlite" {
		gcfg.Logger = logger.Default.LogMode(logger.Warn)
	}

	var dialector gorm.Dialector
	switch cfg.Driver {
	case "sqlite":
		// glebarez/sqlite 为纯 Go 实现，无需 cgo；生产换 mysql 仅需改配置。
		dialector = sqlite.Open(sqliteDSN(cfg.DSN))
	case "mysql":
		dialector = mysql.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %q（可选 sqlite / mysql）", cfg.Driver)
	}

	db, err := gorm.Open(dialector, gcfg)
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err == nil {
		if cfg.Driver == "sqlite" {
			// SQLite 单写者：连接池收敛为 1 是**承载正确性**的决策，勿调大——
			// glebarez 驱动静默丢弃 FOR UPDATE，billing/traffic 等事务的
			// 「检查后写入」序列化全靠单连接串行兜底（WAL 只解决读写并发，
			// 多连接会重新打开写-写 TOCTOU 窗口）。
			sqlDB.SetMaxIdleConns(1)
			sqlDB.SetMaxOpenConns(1)
		} else {
			sqlDB.SetMaxIdleConns(10)
			sqlDB.SetMaxOpenConns(100)
		}
		sqlDB.SetConnMaxLifetime(time.Hour)
	}
	return db, nil
}
