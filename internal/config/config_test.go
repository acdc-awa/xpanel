package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultBackup(t *testing.T) {
	cfg := Default()
	if !cfg.Backup.Enabled {
		t.Error("默认 backup.enabled 应为 true")
	}
	if cfg.Backup.Schedule != "0 3 * * *" {
		t.Errorf("默认 schedule = %q, want %q", cfg.Backup.Schedule, "0 3 * * *")
	}
	if cfg.Backup.Keep != 14 {
		t.Errorf("默认 keep = %d, want 14", cfg.Backup.Keep)
	}
	if cfg.Backup.Dir != "./data/backups" {
		t.Errorf("默认 dir = %q", cfg.Backup.Dir)
	}
}

func TestLoadBackupSection(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	content := `
app:
  port: 18080
jwt:
  secret: test-secret-test-secret-test-secret-test
backup:
  enabled: false
  schedule: "0 */6 * * *"
  keep: 7
  dir: /tmp/backups
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Backup.Enabled {
		t.Error("enabled 应为 false")
	}
	if cfg.Backup.Schedule != "0 */6 * * *" {
		t.Errorf("schedule = %q", cfg.Backup.Schedule)
	}
	if cfg.Backup.Keep != 7 {
		t.Errorf("keep = %d", cfg.Backup.Keep)
	}
	if cfg.Backup.Dir != "/tmp/backups" {
		t.Errorf("dir = %q", cfg.Backup.Dir)
	}
}
