package accounts

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestStoreSetLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "internal_accounts.json")
	s := New(path)

	if err := s.Set("relay-a", "uuid-1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set("relay-b", "uuid-2"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	m, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m["relay-a"] != "uuid-1" || m["relay-b"] != "uuid-2" {
		t.Errorf("Load 结果不符: %v", m)
	}

	// 覆盖写
	if err := s.Set("relay-a", "uuid-1-new"); err != nil {
		t.Fatal(err)
	}
	m, _ = s.Load()
	if m["relay-a"] != "uuid-1-new" {
		t.Errorf("覆盖写失败: %v", m)
	}

	// 新实例重读（持久化验证）
	s2 := New(path)
	m2, err := s2.Load()
	if err != nil || m2["relay-b"] != "uuid-2" {
		t.Errorf("新实例重读失败: %v %v", m2, err)
	}
}

func TestStoreFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "internal_accounts.json") // 嵌套目录
	s := New(path)
	if err := s.Set("a", "uuid-a"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("文件未创建: %v", err)
	}
	// POSIX 权限位仅 Linux 有效（Windows 恒 0666）
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("文件权限 = %o, want 600", perm)
		}
	}
	// 无 .tmp 残留
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("残留临时文件: %s", e.Name())
		}
	}
}

func TestStoreMissingFile(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "nope.json"))
	m, err := s.Load()
	if err != nil || len(m) != 0 {
		t.Errorf("文件不存在应返回空 map: %v %v", m, err)
	}
}

func TestStoreCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "internal_accounts.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	if _, err := s.Load(); err == nil {
		t.Error("损坏文件应报错（不静默覆盖）")
	}
}

func TestStoreRemove(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "accounts.json"))
	if err := s.Set("a", "u1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("b", "u2"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove("a"); err != nil { // 幂等
		t.Fatal(err)
	}
	m, _ := s.Load()
	if _, ok := m["a"]; ok {
		t.Error("a 应被删除")
	}
	if m["b"] != "u2" {
		t.Error("b 应保留")
	}
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUUID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		u, err := NewUUID()
		if err != nil {
			t.Fatalf("NewUUID: %v", err)
		}
		if !uuidRe.MatchString(u) {
			t.Fatalf("UUID v4 格式不符: %q", u)
		}
		if seen[u] {
			t.Fatalf("UUID 重复: %q", u)
		}
		seen[u] = true
	}
}
