package models

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTimeNormDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Setting{}, &AuditLog{}))
	return db
}

func insertAuditRaw(t *testing.T, db *gorm.DB, createdAt string) {
	t.Helper()
	require.NoError(t, db.Exec(
		"INSERT INTO audit_logs (operator_type, action, detail, ip, created_at) VALUES (?,?,?,?,?)",
		"admin", "test.action", "d", "127.0.0.1", createdAt,
	).Error)
}

// rawAuditTimes 用 CAST 读取列的原始文本（直接 Scan 到 string 会被 GORM 按 datetime 重新格式化）。
func rawAuditTimes(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var vals []string
	require.NoError(t, db.Raw("SELECT CAST(created_at AS TEXT) FROM audit_logs ORDER BY id").Scan(&vals).Error)
	return vals
}

// TestCanonicalMatchesDriverFormat 守住规范格式与驱动实际写格式的一致性：
// 若驱动默认写格式变了，本用例会失败，提示同步 DBTimeLayout。
func TestCanonicalMatchesDriverFormat(t *testing.T) {
	type probe struct {
		ID        uint64 `gorm:"primaryKey"`
		CreatedAt time.Time
		Note      string
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&probe{}))

	at := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&probe{CreatedAt: at, Note: "utc"}).Error)
	var got []string
	require.NoError(t, db.Raw("SELECT CAST(created_at AS TEXT) FROM probes ORDER BY id").Scan(&got).Error)
	require.Len(t, got, 1)
	assert.Equal(t, FormatDBTime(at), got[0])
	assert.Equal(t, "2026-09-11 12:00:00+00:00", got[0], "驱动默认应为空格分隔 + 数字偏移")
}

func TestNormalizeTimesToUTC(t *testing.T) {
	db := setupTimeNormDB(t)

	insertAuditRaw(t, db, "2026-09-11T20:00:00+08:00") // 偏移 + 分隔符都不规范
	insertAuditRaw(t, db, "2026-09-11 12:00:00+00:00") // 已是规范格式：保持不变
	insertAuditRaw(t, db, "2026-09-11T12:00:00Z")      // 仅格式不规范（T/Z）→ 归一为库内格式

	require.NoError(t, NormalizeTimesToUTC(db))

	assert.Equal(t, []string{
		"2026-09-11 12:00:00+00:00", // 偏移归一
		"2026-09-11 12:00:00+00:00", // 原样
		"2026-09-11 12:00:00+00:00", // 格式归一
	}, rawAuditTimes(t, db))

	// 归一后按 UTC 绑定的范围查询应命中全部三条。
	from := time.Date(2026, 9, 11, 11, 0, 0, 0, time.UTC)
	var cnt int64
	require.NoError(t, db.Model(&AuditLog{}).Where("created_at >= ?", from).Count(&cnt).Error)
	assert.Equal(t, int64(3), cnt)
}

func TestNormalizeTimesToUTC_Idempotent(t *testing.T) {
	db := setupTimeNormDB(t)
	insertAuditRaw(t, db, "2026-09-11T20:00:00+08:00")
	require.NoError(t, NormalizeTimesToUTC(db))
	require.Equal(t, []string{"2026-09-11 12:00:00+00:00"}, rawAuditTimes(t, db))

	// 标记已落库：再次调用直接返回，新插入的非规范行不会被处理（一次性迁移语义）。
	var mark Setting
	require.NoError(t, db.Where("key = ?", dbTimesNormalizedMark).First(&mark).Error)
	insertAuditRaw(t, db, "2026-09-11T20:00:00+08:00")
	require.NoError(t, NormalizeTimesToUTC(db))
	assert.Equal(t, "2026-09-11T20:00:00+08:00", rawAuditTimes(t, db)[1])
}

func TestNormalizeTimesToUTC_DirtyDataSkipped(t *testing.T) {
	db := setupTimeNormDB(t)
	insertAuditRaw(t, db, "not-a-time")
	require.NoError(t, NormalizeTimesToUTC(db))
	assert.Equal(t, []string{"not-a-time"}, rawAuditTimes(t, db))
}

func TestParseDBTimeLayouts(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"2026-09-11 20:00:00+08:00":           "2026-09-11 12:00:00+00:00",
		"2026-09-11T20:00:00+08:00":           "2026-09-11 12:00:00+00:00",
		"2026-09-11T12:00:00Z":                "2026-09-11 12:00:00+00:00",
		"2026-09-11 12:00:00.123456789+00:00": "2026-09-11 12:00:00.123456789+00:00",
		"2026-09-11 12:00:00":                 "2026-09-11 12:00:00+00:00",
	}
	for in, want := range cases {
		got, ok := parseDBTime(in)
		require.True(t, ok, "应能解析 %q", in)
		assert.Equal(t, want, FormatDBTime(got), "输入 %q", in)
	}
	_, ok := parseDBTime("2026-13-99")
	assert.False(t, ok)
}
