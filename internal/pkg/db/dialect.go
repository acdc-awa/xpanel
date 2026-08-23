package db

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// IsUniqueViolation 按数据库方言归一化唯一约束冲突判断，是方言差异的收口点
//（后续 Stage 数据库适配层在此扩展 Postgres 等方言）。
// column 非空时要求错误指向该列（如 "users.username"），避免误捕同表其他约束。
//
// 方言文本：SQLite(modernc/glebarez) → "UNIQUE constraint failed: <table>.<col>"；
// MySQL → "Error 1062 (23000): Duplicate entry '...' for key '...'"。
func IsUniqueViolation(err error, column string) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if !strings.Contains(msg, "UNIQUE constraint failed") && !strings.Contains(msg, "Duplicate entry") {
		return false
	}
	return column == "" || strings.Contains(msg, column)
}

// LockForUpdate 按方言注入行锁（Stage 8 方言接缝）。
// MySQL/Postgres → SELECT ... FOR UPDATE（真实行锁）；
// SQLite → 不注入（glebarez 驱动会静默丢弃该子句；事务串行由单连接池兜底，见 Open 注释）。
// 业务代码一律经本函数取锁语义，禁止直接散落 clause.Locking。
func LockForUpdate(tx *gorm.DB) *gorm.DB {
	if tx.Dialector.Name() == "sqlite" {
		return tx
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"})
}

// SupportsOnlineSnapshot 报告驱动是否支持在线一致性快照备份（SQLite VACUUM INTO）。
// MySQL 等需外部工具（mysqldump），不在进程内支持。
func SupportsOnlineSnapshot(driver string) bool {
	return driver == "sqlite"
}

