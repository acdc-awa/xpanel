// Package db 负责按配置打开数据库（sqlite 开发 / mysql 生产）。
package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/zhx/xray-panel/internal/config"
)

// sqliteDSN 追加 busy_timeout（ISSUE-15），并发写冲突时等待而不是立刻返回 database is locked。
func sqliteDSN(dsn string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + "_pragma=busy_timeout(5000)"
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
			// SQLite 单写者：连接池收敛为 1，配合 busy_timeout 避免并发写直接报 database is locked。
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
