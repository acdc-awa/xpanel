package db

import "strings"

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
