package db

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/acdc-awa/xpanel/internal/config"
)

func TestSQLiteDSNBusyTimeout(t *testing.T) {
	plain := SqliteDSN("./data/panel.db")
	for _, pragma := range []string{"_pragma=busy_timeout(5000)", "_pragma=journal_mode(WAL)", "_pragma=synchronous(NORMAL)"} {
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

type dbTestRow struct {
	ID uint `gorm:"primaryKey"`
	V  int
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
