package backup

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/glebarez/sqlite"
)

// writePanelDB 写一个"像面板库"的 sqlite 文件（含 users/settings/servers 与 schema 版本）。
func writePanelDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)`,
		`CREATE TABLE servers (id INTEGER PRIMARY KEY)`,
		`INSERT INTO users (id, name) VALUES (7, 'restored')`,
		`INSERT INTO settings (key, value) VALUES ('schema_min_compatible', '1')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}
}

func TestUploadAcceptsPanelDBAndRejectsOthers(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir) // 当前库（仅 kv 表，够 Snapshot 用）
	svc, err := New(dsn, "sqlite", configForTest(dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return time.Date(2026, 9, 11, 3, 0, 0, 0, time.UTC) }

	// 合法面板库：接受并落入备份目录
	goodPath := filepath.Join(dir, "good.db")
	writePanelDB(t, goodPath)
	f, err := os.Open(goodPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := svc.Upload(f, fileSize(t, goodPath))
	_ = f.Close()
	if err != nil {
		t.Fatalf("Upload(good): %v", err)
	}
	if !tsRe.MatchString(info.File) {
		t.Fatalf("上传后文件名不符合备份命名: %q", info.File)
	}
	if _, err := os.Stat(filepath.Join(svc.dir, info.File)); err != nil {
		t.Fatalf("备份文件未落盘: %v", err)
	}

	// 非法库（缺面板表）：拒绝
	badPath := filepath.Join(dir, "kv.db")
	os.WriteFile(badPath, []byte("not a database"), 0o644)
	f2, _ := os.Open(badPath)
	if _, err := svc.Upload(f2, fileSize(t, badPath)); err == nil {
		t.Fatal("非面板数据库应被拒绝")
	}
	_ = f2.Close()
}

// TestUploadRejectsNewerSchema 备份记录的最低兼容版本高于当前面板时应拒绝。
func TestUploadRejectsNewerSchema(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir)
	svc, err := New(dsn, "sqlite", configForTest(dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "newer.db")
	writePanelDB(t, p)
	db, _ := sql.Open("sqlite", p)
	if _, err := db.Exec(`UPDATE settings SET value='999' WHERE key='schema_min_compatible'`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	f, _ := os.Open(p)
	defer f.Close()
	if _, err := svc.Upload(f, fileSize(t, p)); err == nil || !strings.Contains(err.Error(), "更新版本") {
		t.Fatalf("应因 schema 版本过新被拒绝，got err=%v", err)
	}
}

func TestApplyPendingRestoreReplacesDatabase(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir) // 当前库：kv 表 + 两行
	svc, err := New(dsn, "sqlite", configForTest(dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return time.Date(2026, 9, 11, 3, 0, 0, 0, time.UTC) }

	backName := "panel-20260805-030000.db"
	writePanelDB(t, filepath.Join(svc.dir, backName))

	if _, err := svc.ScheduleRestore(backName); err != nil {
		t.Fatalf("ScheduleRestore: %v", err)
	}
	if _, err := os.Stat(restoreMarkerPath(dsn)); err != nil {
		t.Fatalf("恢复标记未写入: %v", err)
	}

	restored, err := ApplyPendingRestore(dsn)
	if err != nil || !restored {
		t.Fatalf("ApplyPendingRestore = (%v,%v), want (true,nil)", restored, err)
	}

	// 当前库应已被替换为面板库
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err := db.QueryRow(`SELECT name FROM users WHERE id = 7`).Scan(&name); err != nil {
		t.Fatalf("恢复后应为面板库（users 表）: %v", err)
	}
	if name != "restored" {
		t.Fatalf("users.name = %q, want restored", name)
	}
	// 标记与暂存文件应清理
	if _, err := os.Stat(restoreMarkerPath(dsn)); !os.IsNotExist(err) {
		t.Error("恢复标记未清理")
	}
	if _, err := os.Stat(restoreStagingPath(dsn)); !os.IsNotExist(err) {
		t.Error("暂存文件未清理")
	}
}

// TestApplyPendingRestoreKeepsCurrentOnCorruptStaging 暂存文件损坏时保留当前库并清理标记（失败安全）。
func TestApplyPendingRestoreKeepsCurrentOnCorruptStaging(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir)
	svc, err := New(dsn, "sqlite", configForTest(dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	backName := "panel-20260805-030000.db"
	writePanelDB(t, filepath.Join(svc.dir, backName))
	if _, err := svc.ScheduleRestore(backName); err != nil {
		t.Fatal(err)
	}
	// 破坏暂存文件
	if err := os.WriteFile(restoreStagingPath(dsn), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	restored, err := ApplyPendingRestore(dsn)
	if err == nil || restored {
		t.Fatalf("损坏暂存应失败且不替换, got (%v,%v)", restored, err)
	}
	// 当前库应仍是原 kv 库
	db, _ := sql.Open("sqlite", dsn)
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM kv`).Scan(&n); err != nil {
		t.Fatalf("当前库应保持不变（kv 表存在）: %v", err)
	}
	if n != 2 {
		t.Fatalf("kv 行数 = %d, want 2", n)
	}
	if _, err := os.Stat(restoreMarkerPath(dsn)); !os.IsNotExist(err) {
		t.Error("失败后恢复标记应被清理")
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.Size()
}
