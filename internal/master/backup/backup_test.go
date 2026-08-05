package backup

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/glebarez/sqlite"
	"github.com/zhx/xray-panel/internal/config"
)

// setupDB 在 tmpdir 建一个带数据的 sqlite 文件，返回其 DSN 与路径。
func setupDB(t *testing.T, dir string) (dsn, path string) {
	t.Helper()
	path = filepath.Join(dir, "panel.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO kv VALUES ('a','1'),('b','2')`); err != nil {
		t.Fatal(err)
	}
	return path, path
}

func TestSnapshotCreatesValidBackup(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir)
	cfg := configForTest(dir)
	svc, err := New(dsn, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	info, err := svc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(info.File, "panel-") || !strings.HasSuffix(info.File, ".db") {
		t.Fatalf("备份文件名异常: %q", info.File)
	}
	if info.Size == 0 {
		t.Error("备份文件大小应为非 0")
	}
	// 重新打开备份，校验完整性与数据
	backPath := filepath.Join(dir, "backups", info.File)
	db, err := sql.Open("sqlite", backPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check = %q", integrity)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM kv`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("备份中行数 = %d, want 2", n)
	}
}

// TestSnapshotQuotedPath 备份目录路径含单引号时快照仍应成功（SQL 字面量转义）。
func TestSnapshotQuotedPath(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir)
	cfg := configForTest(filepath.Join(dir, "back'ups"))
	svc, err := New(dsn, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	info, err := svc.Snapshot()
	if err != nil {
		t.Fatalf("含单引号路径快照失败: %v", err)
	}
	backPath := filepath.Join(cfg.Dir, info.File)
	if _, err := os.Stat(backPath); err != nil {
		t.Fatalf("备份文件未生成: %v", err)
	}
	db, err := sql.Open("sqlite", backPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "ok" {
		t.Fatalf("integrity_check = %q", integrity)
	}
}

func TestSnapshotUsesProvidedTime(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir)
	svc, err := New(dsn, configForTest(dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return time.Date(2026, 8, 5, 3, 0, 0, 0, time.UTC) }
	info, err := svc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if info.File != "panel-20260805-030000.db" {
		t.Fatalf("文件名 = %q", info.File)
	}
	if !info.CreatedAt.Equal(time.Date(2026, 8, 5, 3, 0, 0, 0, time.UTC)) {
		t.Errorf("CreatedAt = %v", info.CreatedAt)
	}
}

func TestRotateKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir)
	cfg := configForTest(dir)
	cfg.Keep = 2
	svc, err := New(dsn, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		svc.now = func() time.Time { return base.Add(time.Duration(i) * 24 * time.Hour) }
		if _, err := svc.Snapshot(); err != nil {
			t.Fatal(err)
		}
	}
	items, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("保留份数 = %d, want 2", len(items))
	}
	if items[0].File != "panel-20260805-030000.db" { // 最新在前
		t.Errorf("最新备份 = %q", items[0].File)
	}
	if items[1].File != "panel-20260804-030000.db" {
		t.Errorf("次新备份 = %q", items[1].File)
	}
}

func TestListSortedDesc(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir)
	svc, err := New(dsn, configForTest(dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 1, 3, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		svc.now = func() time.Time { return base.Add(time.Duration(i) * time.Hour) }
		if _, err := svc.Snapshot(); err != nil {
			t.Fatal(err)
		}
	}
	items, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("数量 = %d", len(items))
	}
	for i := 1; i < len(items); i++ {
		if items[i].CreatedAt.After(items[i-1].CreatedAt) {
			t.Error("List 应按时间倒序")
		}
	}
}

func TestOpenFileRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir)
	svc, err := New(dsn, configForTest(dir), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.OpenFile("../../etc/passwd"); err == nil {
		t.Error("路径穿越应被拒绝")
	}
	if _, err := svc.OpenFile("panel-20260805.db"); err == nil {
		t.Error("格式不符应被拒绝")
	}
	if _, err := svc.OpenFile("panel-20260805-030000.db"); err == nil {
		t.Error("不存在文件应报错")
	}
}

// configForTest 测试用配置（backup 目录在 tmpdir/backups）。
func configForTest(dir string) config.Backup {
	return config.Backup{Enabled: true, Schedule: "0 3 * * *", Keep: 14, Dir: filepath.Join(dir, "backups")}
}
