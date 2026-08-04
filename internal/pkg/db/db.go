// Package db 负责按配置打开数据库（sqlite 开发 / mysql 生产）。
package db

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/zhx/xray-panel/internal/config"
)

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
		dialector = sqlite.Open(cfg.DSN)
	case "mysql":
		dialector = mysql.Open(cfg.DSN)
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %q（可选 sqlite / mysql）", cfg.Driver)
	}

	return gorm.Open(dialector, gcfg)
}