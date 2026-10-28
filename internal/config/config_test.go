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

// app.trusted_proxies：缺省键保留默认（仅回环），显式空列表覆盖为「不信任任何转发头」。
func TestLoadTrustedProxies(t *testing.T) {
	write := func(t *testing.T, content string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// 1) 未写该键：默认回环（反代与面板同机开箱可用）
	cfg, err := Load(write(t, "app:\n  port: 18080\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.App.TrustedProxies) != 2 ||
		cfg.App.TrustedProxies[0] != "127.0.0.1/32" || cfg.App.TrustedProxies[1] != "::1/128" {
		t.Fatalf("默认 trusted_proxies = %v, want [127.0.0.1/32 ::1/128]", cfg.App.TrustedProxies)
	}

	// 2) 显式空列表：完全不信转发头
	cfg, err = Load(write(t, "app:\n  trusted_proxies: []\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.App.TrustedProxies) != 0 {
		t.Fatalf("显式空列表应覆盖默认，实际 %v", cfg.App.TrustedProxies)
	}

	// 3) 自定义网段原样读入
	cfg, err = Load(write(t, "app:\n  trusted_proxies:\n    - 172.18.0.0/16\n    - 10.0.0.5\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.App.TrustedProxies) != 2 || cfg.App.TrustedProxies[0] != "172.18.0.0/16" || cfg.App.TrustedProxies[1] != "10.0.0.5" {
		t.Fatalf("自定义 trusted_proxies = %v", cfg.App.TrustedProxies)
	}
}
