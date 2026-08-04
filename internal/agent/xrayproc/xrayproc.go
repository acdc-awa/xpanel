// Package xrayproc 托管 xray-core 子进程：启动/停止/配置校验重启/看门狗拉起。
// 结论依据《知识状态清单》A 类实测：SIGTERM 优雅退出(exit 0)、SIGKILL(137)、
// `-test` 配置错误 exit 23、启动失败 exit 255、watchdog 2s 拉起。
package xrayproc

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	watchdogInterval = 2 * time.Second
	stopGracePeriod  = 5 * time.Second
)

// Proc 管理单个 xray 实例。
type Proc struct {
	Bin        string
	ConfigPath string
	LogPath    string
	PidFile    string

	mu        sync.Mutex
	cmd       *exec.Cmd
	started   bool // 是否曾启动（用于看门狗判定"崩溃后拉起"）
	startedAt time.Time
}

// New 构造托管器。
func New(bin, configPath, logPath, pidFile string) *Proc {
	return &Proc{Bin: bin, ConfigPath: configPath, LogPath: logPath, PidFile: pidFile}
}

// TestConfig 用 `xray -test` 校验配置（exit 0 = 通过）。
func (p *Proc) TestConfig(path string) error {
	cmd := exec.Command(p.Bin, "-test", "-config", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// 退出码 23 = 配置错误（实测）；其他为启动异常
		return fmt.Errorf("xray -test 未通过: %v / %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CleanupStale 停止同配置路径的残留 xray 实例（agent 异常退出遗留的孤儿进程）。
// 用 pgrep -f 匹配完整 config_path（唯一），避免误杀；pgrep 自动排除自身。
func (p *Proc) CleanupStale() {
	cmd := exec.Command("pgrep", "-f", p.ConfigPath)
	out, err := cmd.Output()
	if err != nil {
		return // 无匹配（pgrep 无结果时 exit 1）
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, perr := strconv.Atoi(strings.TrimSpace(line))
		if perr != nil || pid <= 0 || pid == os.Getpid() {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	// 等待残留进程退出（最多 3s）
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		left := false
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			pid, perr := strconv.Atoi(strings.TrimSpace(line))
			if perr == nil && pid > 0 && pid != os.Getpid() && syscall.Kill(pid, 0) == nil {
				left = true
				break
			}
		}
		if !left {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Start 启动 xray 子进程（配置已存在时使用）。
func (p *Proc) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.IsRunning() {
		return nil // 已在运行
	}
	if _, err := os.Stat(p.ConfigPath); err != nil {
		return fmt.Errorf("xray 配置不存在: %s", p.ConfigPath)
	}

	if err := os.MkdirAll(filepath.Dir(p.LogPath), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(p.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	cmd := exec.Command(p.Bin, "run", "-c", p.ConfigPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	setSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("启动 xray 失败: %w", err)
	}
	// 异步 Wait 回收子进程，避免僵尸（僵尸会导致 kill(pid,0) 误判存活）
	go func() { _ = cmd.Wait() }()
	p.cmd = cmd
	p.started = true
	p.startedAt = time.Now()
	if err := os.MkdirAll(filepath.Dir(p.PidFile), 0o755); err == nil {
		_ = os.WriteFile(p.PidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
	}
	return nil
}

// Stop 停止 xray（SIGTERM 优雅退出，超时后 SIGKILL）。
func (p *Proc) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.started = false

	pid := p.pidFromFile()
	if pid <= 0 {
		_ = os.Remove(p.PidFile)
		return nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		_ = os.Remove(p.PidFile)
		return nil // 进程已不存在
	}
	deadline := time.Now().Add(stopGracePeriod)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			_ = os.Remove(p.PidFile)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	_ = os.Remove(p.PidFile)
	return nil
}

// RestartWithConfig 写配置 → 校验 → 重启。
func (p *Proc) RestartWithConfig(configJSON string) error {
	if err := os.MkdirAll(filepath.Dir(p.ConfigPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p.ConfigPath, []byte(configJSON), 0o644); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	if err := p.TestConfig(p.ConfigPath); err != nil {
		return err
	}
	if err := p.Stop(); err != nil {
		return err
	}
	return p.Start()
}

// IsRunning 通过 pid 文件 + kill(0) 判断进程存活；僵尸（Z）视为不在运行。
func (p *Proc) IsRunning() bool {
	pid := p.pidFromFile()
	if pid <= 0 {
		return false
	}
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	// 僵尸进程 kill(0) 仍成功，需检查 /proc/<pid>/stat 的 state
	if state, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		// 形如 "12345 (xray) S ..."，state 是最后一个 ') ' 后的第一个字符
		rest := string(state)
		if i := strings.LastIndex(rest, ") "); i >= 0 && i+2 < len(rest) {
			return rest[i+2] != 'Z'
		}
	}
	return true
}

func (p *Proc) pidFromFile() int {
	data, err := os.ReadFile(p.PidFile)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

// Watchdog 崩溃自动拉起（每 2s 检测）。
func (p *Proc) Watchdog(stop <-chan struct{}) {
	ticker := time.NewTicker(watchdogInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			p.mu.Lock()
			started := p.started
			p.mu.Unlock()
			if started && !p.IsRunning() {
				if err := p.Start(); err != nil {
					// 崩溃后拉起失败：等下一个周期重试
					continue
				}
			}
		}
	}
}

// Status 返回运行状态。
func (p *Proc) Status() (running bool, pid int, startedAt time.Time, uptimeSec int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started && p.IsRunning() {
		return true, p.pidFromFile(), p.startedAt, int64(time.Since(p.startedAt).Seconds())
	}
	return false, 0, time.Time{}, 0
}

// Logs 读取最近 n 行日志。
func (p *Proc) Logs(n int) (string, error) {
	if n <= 0 {
		n = 100
	}
	data, err := os.ReadFile(p.LogPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n"), nil
}