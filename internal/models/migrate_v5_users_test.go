package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// legacyUsersDDL 生产库 v4 的 users 表形态（含 username 列与 9 个索引），
// 用于验证 v5 整表重建：删列、email NOT NULL、逐行内容不变、自增序列不回退。
const legacyUsersDDL = "CREATE TABLE `users` (" +
	"`id` integer PRIMARY KEY AUTOINCREMENT," +
	"`username` text NOT NULL," +
	"`uuid` text," +
	"`email` text," +
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

func newLegacyUsersDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(legacyUsersDDL).Error)
	for _, idx := range []string{
		"CREATE UNIQUE INDEX `idx_users_username` ON `users`(`username`)",
		"CREATE UNIQUE INDEX `idx_users_email` ON `users`(`email`)",
		"CREATE UNIQUE INDEX `idx_users_uuid` ON `users`(`uuid`)",
		"CREATE UNIQUE INDEX `idx_users_subscribe_token` ON `users`(`subscribe_token`)",
		"CREATE INDEX `idx_users_role` ON `users`(`role`)",
		"CREATE INDEX `idx_users_status` ON `users`(`status`)",
		"CREATE INDEX `idx_users_plan_id` ON `users`(`plan_id`)",
		"CREATE INDEX `idx_users_permission_group_id` ON `users`(`permission_group_id`)",
		"CREATE INDEX `idx_users_deleted_at` ON `users`(`deleted_at`)",
	} {
		require.NoError(t, db.Exec(idx).Error)
	}
	return db
}

// TestMigrateUsersDropUsername 覆盖 v5 的三个关键不变量：
// 逐行内容不变（除 username）、email 无空值、自增序列不回退（id 永不复用）。
func TestMigrateUsersDropUsername(t *testing.T) {
	db := newLegacyUsersDB(t)
	now := "2026-10-01 00:00:00"
	rows := []string{
		// id, username, uuid, email
		`(1, 'admin@panel.local', 'uuid-1', 'admin@panel.local', 'h', 'admin', 1, 'tok-1', 0, 500, NULL, '${now}')`,
		`(2, 'u2@x.com', 'uuid-2', '', 'h', 'user', 1, 'tok-2', 0, 0, NULL, '${now}')`,
		`(3, '', 'uuid-3', NULL, 'h', 'user', 0, 'tok-3', 0, 0, '2026-10-02 00:00:00', '${now}')`,
	}
	for _, r := range rows {
		r = replaceNow(r, now)
		require.NoError(t, db.Exec("INSERT INTO `users` (id, username, uuid, email, password_hash, role, status, subscribe_token, balance_cents, remark, deleted_at, created_at) VALUES "+r).Error)
	}
	// 造出「删过号」：seq 比现存 max(id) 更大（id=9 曾用过）
	require.NoError(t, db.Exec("INSERT INTO `users` (id, username, uuid, email, password_hash) VALUES (9, 'gone@x.com', 'uuid-9', 'gone@x.com', 'h')").Error)
	require.NoError(t, db.Exec("DELETE FROM `users` WHERE id = 9").Error)

	require.NoError(t, migrateUsersDropUsername(db))

	var cols []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('users')").Scan(&cols).Error)
	assert.NotContains(t, cols, "username")
	assert.Contains(t, cols, "email")

	// email 全部非空，且空邮箱按 username 回填 / 兜底 user<id>@panel.local
	var emails []string
	require.NoError(t, db.Raw("SELECT email FROM `users` ORDER BY id").Scan(&emails).Error)
	assert.Equal(t, []string{"admin@panel.local", "u2@x.com", "user3@panel.local"}, emails)

	// 自增序列不回退：下一个 id 必须 > 9（曾用过的 9 不能复用）
	require.NoError(t, db.Exec("INSERT INTO `users` (email, password_hash) VALUES ('x@x.com','h')").Error)
	var nextID uint64
	require.NoError(t, db.Raw("SELECT max(id) FROM `users`").Scan(&nextID).Error)
	assert.Greater(t, nextID, uint64(9), "删除过的 id 不得复用（审计可追溯的前提）")

	// 幂等：再跑一次是空操作
	require.NoError(t, migrateUsersDropUsername(db))
	var cnt int64
	require.NoError(t, db.Raw("SELECT count(*) FROM `users`").Scan(&cnt).Error)
	assert.EqualValues(t, 4, cnt)
}

// TestMigrateUsersDropUsernameColumnSet 迁移后列集合必须与模型一致（防止漏 COPY）。
func TestMigrateUsersDropUsernameColumnSet(t *testing.T) {
	db := newLegacyUsersDB(t)
	require.NoError(t, db.Exec("INSERT INTO `users` (id, username, email, password_hash) VALUES (1, 'a@x.com','a@x.com','h')").Error)
	require.NoError(t, migrateUsersDropUsername(db))

	var cols []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_table_info('users')").Scan(&cols).Error)
	assert.ElementsMatch(t, userV5Columns, cols)

	var idx []string
	require.NoError(t, db.Raw("SELECT name FROM pragma_index_list('users')").Scan(&idx).Error)
	assert.NotContains(t, idx, "idx_users_username")
	assert.Contains(t, idx, "idx_users_email")
}

// replaceNow 把 SQL 文本里的占位符替换成固定时间戳（测试可读性）。
func replaceNow(s, now string) string {
	out := ""
	for i := 0; i < len(s); i++ {
		if i+7 <= len(s) && s[i:i+7] == "${now}" {
			out += now
			i += 6
			continue
		}
		out += string(s[i])
	}
	return out
}
