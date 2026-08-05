package upgrade

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyUpgradesBinary(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "xray-agent")
	if err := os.WriteFile(exePath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	newData := []byte("new-binary")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Agent-Version", "v9.9.9")
		w.Header().Set("X-Agent-Sha256", Sha256Hex(newData))
		w.Write(newData)
	}))
	defer srv.Close()

	restarted := false
	var out bytes.Buffer
	err := Apply(&Fetcher{BaseURL: srv.URL}, exePath, func() error { restarted = true; return nil }, &out)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exePath)
	if string(got) != "new-binary" {
		t.Errorf("替换后内容 = %q", got)
	}
	if !restarted {
		t.Error("未调用 restart")
	}
	if !strings.Contains(out.String(), "v9.9.9") {
		t.Errorf("输出应含新版本号: %q", out.String())
	}
}

func TestApplySameVersionNoop(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "xray-agent")
	if err := os.WriteFile(exePath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Agent-Version", CurrentVersion()) // 与本地相同
		w.Header().Set("X-Agent-Sha256", Sha256Hex([]byte("x")))
		w.Write([]byte("x"))
	}))
	defer srv.Close()

	err := Apply(&Fetcher{BaseURL: srv.URL}, exePath, func() error { t.Error("不应重启"); return nil }, &bytes.Buffer{})
	if !errors.Is(err, ErrUpToDate) {
		t.Fatalf("err = %v, want ErrUpToDate", err)
	}
	got, _ := os.ReadFile(exePath)
	if string(got) != "old" {
		t.Error("版本相同不应替换")
	}
}

func TestApplyShaMismatchRejects(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "xray-agent")
	if err := os.WriteFile(exePath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Agent-Version", "v2.0.0")
		w.Header().Set("X-Agent-Sha256", "deadbeef") // 错误摘要
		w.Write([]byte("corrupt"))
	}))
	defer srv.Close()

	err := Apply(&Fetcher{BaseURL: srv.URL}, exePath, func() error { t.Error("不应重启"); return nil }, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("err = %v, want sha256 错误", err)
	}
	got, _ := os.ReadFile(exePath)
	if string(got) != "old" {
		t.Error("校验失败不应触碰现有二进制")
	}
	// 临时文件应被清理
	matches, _ := filepath.Glob(filepath.Join(dir, "xray-agent.*.tmp"))
	if len(matches) != 0 {
		t.Errorf("临时文件未清理: %v", matches)
	}
}
