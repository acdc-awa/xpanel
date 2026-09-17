package db

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/acdc-awa/xpanel/internal/config"
)

func TestSQLiteDSNBusyTimeout(t *testing.T) {
	plain := SqliteDSN("./data/panel.db")
	for _, pragma := range []string{
		"_pragma=busy_timeout(5000)",
		"_pragma=journal_mode(WAL)",
		"_pragma=synchronous(NORMAL)",
		"_pragma=auto_vacuum(INCREMENTAL)",
		// WAL 截断上限：缺了它 WAL 会常年停在 1000 页水位（面板显示 3.9 MB）
		"_pragma=journal_size_limit(1048576)",
	} {
		if !strings.Contains(plain, pragma) {
			t.Fatalf("plain dsn 应追加 %s, got %q", pragma, plain)
		}
	}
	if strings.Count(plain, "?") != 1 {
		t.Fatalf("plain dsn 只应有一个问号, got %q", plain)
	}

	file := SqliteDSN("file:panel.db?mode=memory&cache=shared")
	if !strings.Contains(file, "&_pragma=busy_timeout(5000)") {
		t.Fatalf("file dsn 应用 & 追加参数, got %q", file)
	}
}

// TestSQLiteJournalSizeLimitEffective pragma 必须真正生效（DSN 里写了不等于驱动认了）：
// 打开库后读回 journal_size_limit 应为 1 MiB，否则日志治理形同虚设。
func TestSQLiteJournalSizeLimitEffective(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "pragma.db")
	database, err := Open(&config.DB{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var limit int64
	if err := database.Raw("PRAGMA journal_size_limit").Scan(&limit).Error; err != nil {
		t.Fatalf("read journal_size_limit: %v", err)
	}
	if limit != 1048576 {
		t.Fatalf("journal_size_limit 应为 1048576，实际 %d（DSN pragma 未生效）", limit)
	}

	var mode string
	if err := database.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if !strings.EqualFold(mode, "wal") {
		t.Fatalf("journal_mode 应为 wal，实际 %q", mode)
	}
}

type dbTestRow struct {
	ID uint `gorm:"primaryKey"`
	V  int
}

// walTestRow 带较大负载，便于把 WAL 撑到可观尺寸。
type walTestRow struct {
	ID      uint `gorm:"primaryKey"`
	Payload string
}

// TestSQLiteWALTruncatedAfterCheckpoint 锁定 2026-09-15 的 WAL 水位问题：
// WAL 只被复用、文件不缩，必须靠 TRUNCATE checkpoint 才能真正归还磁盘
// （面板「数据管理」页显示的 WAL 尺寸即该文件大小）。
func TestSQLiteWALTruncatedAfterCheckpoint(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "wal.db")
	database, err := Open(&config.DB{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := database.AutoMigrate(&walTestRow{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	payload := strings.Repeat("x", 512)
	for i := 0; i < 400; i++ {
		if err := database.Create(&walTestRow{Payload: payload}).Error; err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	walPath := dsn + "-wal"
	if got := fileSize(t, walPath); got == 0 {
		t.Fatal("写入后 WAL 应已存在且非空")
	}
	before := fileSize(t, walPath)

	if err := database.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		t.Fatalf("truncate checkpoint: %v", err)
	}
	after := fileSize(t, walPath)
	if after != 0 {
		t.Fatalf("TRUNCATE checkpoint 后 WAL 应被截断为 0 字节，实际 %d（截断前 %d）", after, before)
	}

	// 数据必须仍然可见（截断只归还磁盘，不改变可见性）。
	var cnt int64
	if err := database.Model(&walTestRow{}).Count(&cnt).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if cnt != 400 {
		t.Fatalf("截断后行数应为 400，实际 %d", cnt)
	}
}

// fileSize 读文件字节数；文件不存在返回 0。
func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("stat %s: %v", path, err)
	}
	return st.Size()
}

// TestSQLiteConcurrentWrites 连接池收敛为 1 + busy_timeout 后，
// 并发写不应再暴露原始 database is locked。
func TestSQLiteConcurrentWrites(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "panel.db")
	db, err := Open(&config.DB{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&dbTestRow{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				if err := db.Create(&dbTestRow{V: i}).Error; err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent insert error: %v", err)
	}
	var cnt int64
	db.Model(&dbTestRow{}).Count(&cnt)
	if cnt != 20 {
		t.Fatalf("rows = %d, want 20", cnt)
	}
}
