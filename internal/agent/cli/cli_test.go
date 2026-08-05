package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (code int, out, errOut string) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	code = Run(args, nil, &outBuf, &errBuf)
	return code, outBuf.String(), errBuf.String()
}

func TestHelpListsAllCommands(t *testing.T) {
	code, out, _ := run(t, "help")
	if code != 0 {
		t.Fatalf("help exit code = %d, want 0", code)
	}
	for _, c := range []string{"run", "status", "restart", "logs", "uninstall", "help"} {
		if !strings.Contains(out, c) {
			t.Errorf("help 输出未包含子命令 %q", c)
		}
	}
	if !strings.Contains(out, "用法") {
		t.Error("help 输出缺少用法说明")
	}
}

func TestHelpAliases(t *testing.T) {
	for _, a := range []string{"-h", "-help", "--help"} {
		code, out, _ := run(t, a)
		if code != 0 {
			t.Errorf("%s exit code = %d, want 0", a, code)
		}
		if !strings.Contains(out, "用法") {
			t.Errorf("%s 输出缺少用法说明", a)
		}
	}
}

func TestHelpSubcommand(t *testing.T) {
	code, out, _ := run(t, "help", "logs")
	if code != 0 {
		t.Fatalf("help logs exit code = %d, want 0", code)
	}
	if !strings.Contains(out, "用法") {
		t.Error("help logs 输出缺少用法说明")
	}
}

func TestHelpUnknownSubcommand(t *testing.T) {
	code, out, errOut := run(t, "help", "frobnicate")
	if code != 0 {
		t.Fatalf("help frobnicate exit code = %d, want 0", code)
	}
	if !strings.Contains(errOut, "未知命令") {
		t.Errorf("stderr 应提示未知命令，实际: %q", errOut)
	}
	if !strings.Contains(out, "用法") {
		t.Error("应回退到总帮助")
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, errOut := run(t, "frobnicate")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut, "未知命令") || !strings.Contains(errOut, "用法") {
		t.Errorf("stderr 应提示未知命令与用法，实际: %q", errOut)
	}
}

func TestResolveConfigPath(t *testing.T) {
	old := configCandidates
	defer func() { configCandidates = old }()

	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.yml")
	f2 := filepath.Join(dir, "b.yml")
	os.WriteFile(f1, []byte("x"), 0o644)
	os.WriteFile(f2, []byte("x"), 0o644)

	configCandidates = []string{f1, f2}
	if got := resolveConfigPath(""); got != f1 {
		t.Errorf("探测顺序错误: got %q want %q", got, f1)
	}
	if err := os.Remove(f1); err != nil {
		t.Fatal(err)
	}
	if got := resolveConfigPath(""); got != f2 {
		t.Errorf("第一候选缺失时应回退到第二候选: got %q want %q", got, f2)
	}
	if got := resolveConfigPath("explicit.yml"); got != "explicit.yml" {
		t.Errorf("显式路径应优先: got %q", got)
	}
	configCandidates = []string{filepath.Join(dir, "nope.yml")}
	if got := resolveConfigPath(""); got != "" {
		t.Errorf("全部缺失应返回空: got %q", got)
	}
}

func TestIsSubcommand(t *testing.T) {
	for _, c := range []string{"status", "restart", "logs", "uninstall", "help", "run", "-h", "-help", "--help"} {
		if !IsSubcommand(c) {
			t.Errorf("IsSubcommand(%q) = false, want true", c)
		}
	}
	for _, c := range []string{"frobnicate", "", "-config"} {
		if IsSubcommand(c) {
			t.Errorf("IsSubcommand(%q) = true, want false", c)
		}
	}
}

func TestLogsWithoutConfig(t *testing.T) {
	old := configCandidates
	defer func() { configCandidates = old }()
	configCandidates = []string{filepath.Join(t.TempDir(), "nope.yml")}

	code, _, errOut := run(t, "logs")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut, "未找到配置文件") {
		t.Errorf("应提示未找到配置文件，实际: %q", errOut)
	}
}

func TestStatusWithoutConfig(t *testing.T) {
	old := configCandidates
	defer func() { configCandidates = old }()
	configCandidates = []string{filepath.Join(t.TempDir(), "nope.yml")}

	code, _, errOut := run(t, "status")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut, "未找到配置文件") {
		t.Errorf("应提示未找到配置文件，实际: %q", errOut)
	}
}

func TestConfirm(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"n\n", false},
		{"", false},
		{"\n", false},
	}
	for _, c := range cases {
		if got := confirm(strings.NewReader(c.input), io.Discard, "p"); got != c.want {
			t.Errorf("confirm(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestRemoveTargets(t *testing.T) {
	dir := t.TempDir()

	binDir := filepath.Join(dir, "bin")
	xrayBin := filepath.Join(binDir, "xray")
	os.MkdirAll(binDir, 0o755)
	os.WriteFile(xrayBin, []byte("x"), 0o755)

	etcDir := filepath.Join(dir, "etc")
	os.MkdirAll(filepath.Join(etcDir, "sub"), 0o755)
	os.WriteFile(filepath.Join(etcDir, "sub", "config.json"), []byte("{}"), 0o644)

	emptyDir := filepath.Join(dir, "empty")
	os.MkdirAll(emptyDir, 0o755)

	pidFile := filepath.Join(dir, "xray.pid")
	os.WriteFile(pidFile, []byte("1"), 0o644)

	targets := uninstallTargets{
		files:   []string{xrayBin},
		dirs:    []string{etcDir, emptyDir},
		pidFile: pidFile,
	}
	warns := removeTargets(targets, io.Discard)
	if len(warns) != 0 {
		t.Fatalf("不应有警告: %v", warns)
	}
	for _, p := range []string{xrayBin, etcDir, emptyDir, pidFile} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("应已删除 %s", p)
		}
	}
	if _, err := os.Stat(binDir); err != nil {
		t.Errorf("父目录不应被删除: %v", err)
	}
}

func TestRemoveTargetsMissingIsNotWarning(t *testing.T) {
	targets := uninstallTargets{
		files: []string{filepath.Join(t.TempDir(), "nope")},
		dirs:  []string{filepath.Join(t.TempDir(), "nodir")},
	}
	if warns := removeTargets(targets, io.Discard); len(warns) != 0 {
		t.Fatalf("删除不存在的路径不应告警: %v", warns)
	}
}

func TestHelpListsUpgrade(t *testing.T) {
	code, out, _ := run(t, "help")
	if code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if !strings.Contains(out, "upgrade") {
		t.Error("help 输出应包含 upgrade")
	}
}

func TestUpgradeUnknownFlag(t *testing.T) {
	code, _, errOut := run(t, "upgrade", "--bogus")
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut, "参数错误") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestFormatDur(t *testing.T) {
	cases := []struct {
		sec  int64
		want string
	}{
		{5, "5s"},
		{65, "1m5s"},
		{3600, "1h0m"},
		{3661, "1h1m"},
	}
	for _, c := range cases {
		if got := formatDur(c.sec); got != c.want {
			t.Errorf("formatDur(%d) = %q, want %q", c.sec, got, c.want)
		}
	}
}
