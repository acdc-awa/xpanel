package backup

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zhx/xray-panel/internal/config"
)

func TestStartRunsScheduledSnapshot(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir)
	cfg := config.Backup{Enabled: true, Schedule: "* * * * * *", Keep: 14, Dir: filepath.Join(dir, "backups")} // 每秒
	svc, err := New(dsn, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx)
	defer cancel()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		items, _ := svc.List()
		if len(items) > 0 {
			cancel()
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("5s 内未触发定时备份")
}

func TestStartDisabledNoSchedule(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir)
	cfg := config.Backup{Enabled: false, Schedule: "* * * * * *", Keep: 14, Dir: filepath.Join(dir, "backups")}
	svc, err := New(dsn, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx)
	defer cancel()
	time.Sleep(1500 * time.Millisecond)
	items, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("disabled 不应定时备份, 发现 %d 份", len(items))
	}
}

func TestStartInvalidScheduleLogsAndContinues(t *testing.T) {
	dir := t.TempDir()
	dsn, _ := setupDB(t, dir)
	cfg := config.Backup{Enabled: true, Schedule: "not-a-cron", Keep: 14, Dir: filepath.Join(dir, "backups")}
	svc, err := New(dsn, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx) // 不应 panic
	defer cancel()
	time.Sleep(300 * time.Millisecond)
	items, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatal("非法 schedule 不应触发备份")
	}
}
