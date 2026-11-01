package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"
)

// v5（2026-11-01）：users.username 并入 users.email。
//
// 历史背景：username 与 email 自建库起就是同值双写（注册/管理员创建/改邮箱三处都同时写），
// 但两列各自建了 UNIQUE 索引、且多个模块在两者之间摇摆（登录查 username、展示读 email），
// 于是「某处按 email 更新、另一处按 username 查询」这类不一致随时会变成 404。
// v5 只保留 email：它既是登录账号也是唯一身份显示（审计/订单/订阅/在线用户统一读它）。
//
// 这是一次**向后不兼容**变更——旧面板对 users 的 INSERT 会带 username 列直接失败，
// 因此 DBMinCompatibleVersion 与 DBSchemaVersion 同步提升到 5（fail-closed）。
//
// SQLite 无法 ALTER COLUMN 改 NOT NULL，也无法可靠地删列（见 dropLegacyAccessPointHostPort
// 对 glebarez DropColumn 静默失效的说明），故整表重建：建 users_v5 → 逐列 COPY → 校验 → 换名。
// 迁移在 HTTP 服务启动之前执行（cmd/master/main.go），单连接、无并发写，重建安全。

// userV5Columns users 表迁移后的列集合（顺序 = 生产库现有物理顺序，去掉 username）。
// 重建时按此列表逐列 COPY，多余列一律丢失——新增列必须同步登记到这里。
var userV5Columns = []string{
	"id", "uuid", "email", "password_hash", "role", "status", "subscribe_token", "plan_id",
	"expire_at", "balance_cents", "device_limit", "permission_group_id", "traffic_cycle_start",
	"must_change_pwd", "token_version", "totp_secret", "totp_enabled", "totp_failed_count",
	"totp_locked_until", "backup_codes", "created_at", "updated_at", "deleted_at",
	"plan_traffic_bytes", "plan_device_limit", "plan_group_id", "remark",
	"auto_renew_expire", "auto_renew_exhaust", "traffic_cycle_id",
}

// userV5NotNullColumns 无默认值的 NOT NULL 列：源表缺任何一列就无法逐列 COPY，只能报错退出。
var userV5NotNullColumns = []string{"id", "email", "password_hash", "balance_cents", "traffic_cycle_id"}

// userV5DDL 与 AutoMigrate 建出的形态保持一致（类型/默认值逐项对齐生产库 v4 的 users 表），
// 只做两处改动：去掉 username、email 加 NOT NULL。
const userV5DDL = "CREATE TABLE `users_v5` (" +
	"`id` integer PRIMARY KEY AUTOINCREMENT," +
	"`uuid` text," +
	"`email` text NOT NULL," +
	"`password_hash` text NOT NULL," +
	"`role` text DEFAULT \"user\"," +
	"`status` integer DEFAULT 1," +
	"`subscribe_token` text," +
	"`plan_id` integer," +
	"`expire_at` datetime," +
	"`balance_cents` integer NOT NULL DEFAULT 0," +
	"`device_limit` integer DEFAULT 0," +
	"`permission_group_id` integer DEFAULT 0," +
	"`traffic_cycle_start` datetime," +
	"`must_change_pwd` numeric DEFAULT false," +
	"`token_version` integer DEFAULT 0," +
	"`totp_secret` text," +
	"`totp_enabled` numeric DEFAULT false," +
	"`totp_failed_count` integer DEFAULT 0," +
	"`totp_locked_until` datetime," +
	"`backup_codes` text," +
	"`created_at` datetime," +
	"`updated_at` datetime," +
	"`deleted_at` datetime," +
	"`plan_traffic_bytes` integer DEFAULT 0," +
	"`plan_device_limit` integer DEFAULT 0," +
	"`plan_group_id` integer DEFAULT 0," +
	"`remark` text," +
	"`auto_renew_expire` numeric DEFAULT false," +
	"`auto_renew_exhaust` numeric DEFAULT false," +
	"`traffic_cycle_id` integer NOT NULL DEFAULT 1" +
	")"

// migrateUsersDropUsername v5 迁移入口（幂等：username 列不存在即跳过）。
//
// 全程在一个事务内：任何一步（回填/COPY/逐行校验/索引/自增序列）失败都整体回滚并中止启动，
// 宁可启动失败，也不让对方带着半张表运行。
func migrateUsersDropUsername(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable("users") {
		return nil
	}
	cols, err := tableColumns(db, "users")
	if err != nil {
		return fmt.Errorf("读取 users 表结构失败: %w", err)
	}
	if !containsColumn(cols, "username") {
		return nil
	}
	if err := backfillUserEmails(db); err != nil {
		return err
	}
	if db.Dialector.Name() != "sqlite" {
		return dropUsernameMySQL(db)
	}
	return rebuildUsersSQLite(db)
}

// backfillUserEmails 在删列前把 email 补满（email 即将变成 NOT NULL）。
//
// 两段：先用 username 填（历史上两者同值，长邮箱曾被塞进 size:32 的列，SQLite 不校验长度）；
// 仍为空的用 user<id>@panel.local 兜底，并在启动日志里列出受影响 id——空邮箱的账号无法登录，
// 但也不能因此拒绝启动（运维可从日志定位并手工改）。
//
// 注意 Unscoped：软删行的 email 同样会变成 NOT NULL，漏掉它们会让重建 INSERT 直接失败。
func backfillUserEmails(db *gorm.DB) error {
	if err := db.Exec("UPDATE `users` SET `email` = `username` WHERE `email` IS NULL OR TRIM(`email`) = ''").Error; err != nil {
		return fmt.Errorf("回填 users.email 失败: %w", err)
	}
	var ids []uint64
	if err := db.Unscoped().Model(&User{}).
		Where("email IS NULL OR TRIM(email) = ''").Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("查询空邮箱用户失败: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		email := fmt.Sprintf("user%d@panel.local", id)
		var n int64
		if err := db.Unscoped().Model(&User{}).Where("email = ?", email).Count(&n).Error; err != nil {
			return fmt.Errorf("检查兜底邮箱冲突失败: %w", err)
		}
		if n > 0 {
			email = fmt.Sprintf("user%d.%d@panel.local", id, id)
		}
		if err := db.Exec("UPDATE `users` SET `email` = ? WHERE `id` = ?", email, id).Error; err != nil {
			return fmt.Errorf("兜底用户 %d 邮箱失败: %w", id, err)
		}
	}
	log.Printf("schema v5: %d 个用户邮箱为空，已兜底为 user<id>@panel.local，id=%v", len(ids), ids)
	return nil
}

// rebuildUsersSQLite 整表重建（SQLite 无 ALTER COLUMN / 删列能力）。
func rebuildUsersSQLite(db *gorm.DB) error {
	cols, err := tableColumns(db, "users")
	if err != nil {
		return err
	}
	for _, c := range userV5NotNullColumns {
		if !containsColumn(cols, c) {
			return fmt.Errorf("users 表缺少 NOT NULL 列 %q，无法安全重建，请从升级前备份恢复后重试", c)
		}
	}

	oldSeq, seqErr := readSQLiteSequence(db, "users")
	srcRows, srcHash, err := hashUserRows(db, "users", userV5Columns)
	if err != nil {
		return fmt.Errorf("读取迁移前 users 快照失败: %w", err)
	}

	statements := []string{
		"DROP TABLE IF EXISTS `users_v5`",
		userV5DDL,
		"INSERT INTO `users_v5` (" + backtickJoin(userV5Columns) + ") SELECT " + backtickJoin(userV5Columns) + " FROM `users`",
		"DROP TABLE `users`",
		"ALTER TABLE `users_v5` RENAME TO `users`",
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		for i, stmt := range statements {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("重建 users 表第 %d 步失败: %w", i+1, err)
			}
		}
		// 逐行校验：行数与内容指纹必须与迁移前完全一致（除 username 外全部列）。
		dstRows, dstHash, err := hashUserRows(tx, "users", userV5Columns)
		if err != nil {
			return fmt.Errorf("读取迁移后 users 快照失败: %w", err)
		}
		if dstRows != srcRows || dstHash != srcHash {
			return fmt.Errorf("users 表重建校验失败：行数 %d→%d，内容指纹不一致，已回滚", srcRows, dstRows)
		}
		if err := createUserV5Indexes(tx); err != nil {
			return err
		}
		return restoreSQLiteSequence(tx, "users", oldSeq, seqErr)
	}); err != nil {
		return err
	}
	log.Printf("schema v5: users 表已重建（%d 行，删除 username 列，email 改为 NOT NULL）", srcRows)
	return nil
}

// createUserV5Indexes 重建 users 索引（DROP TABLE 会连带删掉旧索引）。
// 索引集合 = v4 存量 9 个去掉 idx_users_username。
func createUserV5Indexes(tx *gorm.DB) error {
	stmts := []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS `idx_users_email` ON `users`(`email`)",
		"CREATE UNIQUE INDEX IF NOT EXISTS `idx_users_uuid` ON `users`(`uuid`)",
		"CREATE UNIQUE INDEX IF NOT EXISTS `idx_users_subscribe_token` ON `users`(`subscribe_token`)",
		"CREATE INDEX IF NOT EXISTS `idx_users_role` ON `users`(`role`)",
		"CREATE INDEX IF NOT EXISTS `idx_users_status` ON `users`(`status`)",
		"CREATE INDEX IF NOT EXISTS `idx_users_plan_id` ON `users`(`plan_id`)",
		"CREATE INDEX IF NOT EXISTS `idx_users_permission_group_id` ON `users`(`permission_group_id`)",
		"CREATE INDEX IF NOT EXISTS `idx_users_deleted_at` ON `users`(`deleted_at`)",
	}
	for _, stmt := range stmts {
		if err := tx.Exec(stmt).Error; err != nil {
			return fmt.Errorf("重建 users 索引失败: %w", err)
		}
	}
	return nil
}

// restoreSQLiteSequence 把自增序列恢复到重建前的水位。
//
// 关键：id 永不复用是审计可追溯的前提（删号后同名邮箱重建 = 新主体、新 id）。
// 重建只插入现存行，若删过号，新表的 seq 会低于旧表（例如旧表 max seq=42、现存 max id=40），
// 下次插入就会复用 41/42——必须显式抬回旧水位。
func restoreSQLiteSequence(tx *gorm.DB, table string, oldSeq int64, seqErr error) error {
	if seqErr != nil {
		return nil // 旧库没有 sqlite_sequence 记录：新表由插入行自动维护，无需额外动作
	}
	var cur int64
	if err := tx.Raw("SELECT seq FROM sqlite_sequence WHERE name = ?", table).Scan(&cur).Error; err != nil {
		return fmt.Errorf("读取 %s 自增序列失败: %w", table, err)
	}
	if oldSeq <= cur {
		return nil
	}
	res := tx.Exec("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", oldSeq, table)
	if res.Error != nil {
		return fmt.Errorf("恢复 %s 自增序列失败: %w", table, res.Error)
	}
	if res.RowsAffected == 0 {
		if err := tx.Exec("INSERT INTO sqlite_sequence(name, seq) VALUES(?, ?)", table, oldSeq).Error; err != nil {
			return fmt.Errorf("写入 %s 自增序列失败: %w", table, err)
		}
	}
	return nil
}

// dropUsernameMySQL MySQL 分支：原生 DDL 即可（不改表结构语义）。
func dropUsernameMySQL(db *gorm.DB) error {
	if db.Migrator().HasIndex("users", "idx_users_username") {
		if err := db.Exec("ALTER TABLE `users` DROP INDEX `idx_users_username`").Error; err != nil {
			return fmt.Errorf("删除 idx_users_username 失败: %w", err)
		}
	}
	if err := db.Exec("ALTER TABLE `users` DROP COLUMN `username`").Error; err != nil {
		return fmt.Errorf("删除 users.username 失败: %w", err)
	}
	if err := db.Exec("ALTER TABLE `users` MODIFY `email` VARCHAR(128) NOT NULL").Error; err != nil {
		return fmt.Errorf("修改 users.email 为 NOT NULL 失败: %w", err)
	}
	log.Printf("schema v5: users 表已删除 username 列，email 改为 NOT NULL（MySQL）")
	return nil
}

// tableColumns 读取真实列集合（按物理顺序）。走 catalog 查询而非 GORM Migrator：
// 迁移要判断的是「库里有什么」，而不是「模型里有什么」。
func tableColumns(db *gorm.DB, table string) ([]string, error) {
	var out []string
	switch db.Dialector.Name() {
	case "sqlite":
		rows, err := db.Raw("PRAGMA table_info(" + quoteIdent(table) + ")").Rows()
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var (
				cid       int
				name      string
				ctype     string
				notnull   int
				dfltValue any
				pk        int
			)
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
				return nil, err
			}
			out = append(out, name)
		}
		return out, rows.Err()
	case "mysql":
		err := db.Raw("SELECT column_name FROM information_schema.columns "+
			"WHERE table_schema = DATABASE() AND table_name = ? ORDER BY ordinal_position", table).
			Scan(&out).Error
		return out, err
	default:
		return nil, fmt.Errorf("不支持的数据库方言 %q", db.Dialector.Name())
	}
}

// hashUserRows 逐行指纹：对指定列集合做稳定序列化后整体 sha256。
// 用于「重建前后除 username 外逐行逐列一致」的断言——比逐列写 SQL 对比更不易漏。
func hashUserRows(db *gorm.DB, table string, cols []string) (int64, string, error) {
	rows, err := db.Raw("SELECT " + backtickJoin(cols) + " FROM " + backtick(table) + " ORDER BY `id`").Rows()
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()

	h := sha256.New()
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	var n int64
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return 0, "", err
		}
		for _, v := range vals {
			switch t := v.(type) {
			case nil:
				h.Write([]byte{0x00})
			case []byte:
				h.Write(t)
			default:
				fmt.Fprintf(h, "%v", t)
			}
			h.Write([]byte{'|'})
		}
		h.Write([]byte{'\n'})
		n++
	}
	if err := rows.Err(); err != nil {
		return 0, "", err
	}
	return n, hex.EncodeToString(h.Sum(nil)), nil
}

func readSQLiteSequence(db *gorm.DB, table string) (int64, error) {
	var seq int64
	err := db.Raw("SELECT seq FROM sqlite_sequence WHERE name = ?", table).Scan(&seq).Error
	return seq, err
}

func containsColumn(cols []string, want string) bool {
	for _, c := range cols {
		if strings.EqualFold(c, want) {
			return true
		}
	}
	return false
}

func backtickJoin(cols []string) string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = backtick(c)
	}
	return strings.Join(out, ", ")
}

func backtick(s string) string { return "`" + s + "`" }

// quoteIdent 只允许标识符字符，避免拼接注入（表名来源固定，这里是防御性约束）。
func quoteIdent(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "``"
	}
	return "`" + b.String() + "`"
}
