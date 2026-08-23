// Package cli 提供 xray-agent 的管理子命令：status / restart / logs / uninstall / help。
// 无参数或 run 走主循环（见 cmd/agent/main.go）。
package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/acdc/xray-panel/internal/agent/config"
	"github.com/acdc/xray-panel/internal/agent/upgrade"
	"github.com/acdc/xray-panel/internal/agent/xrayproc"
)

// commands 全部子命令与一句话说明（help 输出用）。
var commands = map[string]string{
	"run":       "启动 agent 主循环（默认行为）",
	"status":    "查看 agent/xray/systemd 状态与配置摘要",
	"restart":   "重启 agent 服务（systemd）或 xray 进程（手动）",
	"logs":      "查看日志（systemd: journalctl；手动: xray 日志文件）",
	"uninstall": "卸载 agent（含 xray-core 与 geo 数据）",
	"upgrade":   "升级 agent 到主控最新版本（--check 仅检查）",
	"help":      "显示帮助或命令详情",
}

var helpAliases = map[string]bool{"help": true, "-h": true, "-help": true, "--help": true}

// IsSubcommand 判断第一个参数是否为已知子命令或帮助别名。
func IsSubcommand(arg string) bool {
	if _, ok := commands[arg]; ok {
		return true
	}
	return helpAliases[arg]
}

// configCandidates 子命令配置文件探测顺序（可被测试覆盖）。
var configCandidates = []string{"/etc/xray-agent/config.yml", "agent.yaml"}

// systemdUnitPaths systemd 单元文件候选路径（可被测试覆盖）。
var systemdUnitPaths = []string{"/etc/systemd/system/xray-agent.service", "/lib/systemd/system/xray-agent.service"}

// Run 执行子命令，返回进程退出码（0 成功 / 1 执行失败 / 2 用法错误）。
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "缺少子命令")
		printHelp(stderr)
		return 2
	}
	cmd := args[0]
	rest := args[1:]

	if helpAliases[cmd] {
		if len(rest) > 0 {
			if _, ok := commands[rest[0]]; ok {
				printCmdHelp(rest[0], stdout)
				return 0
			}
			fmt.Fprintf(stderr, "未知命令: %s\n\n", rest[0])
		}
		printHelp(stdout)
		return 0
	}

	switch cmd {
	case "status":
		return runStatus(rest, stdout, stderr)
	case "restart":
		return runRestart(rest, stdout, stderr)
	case "logs":
		return runLogs(rest, stdout, stderr)
	case "uninstall":
		return runUninstall(rest, stdin, stdout, stderr)
	case "upgrade":
		return runUpgrade(rest, stdout, stderr)
	case "run":
		fmt.Fprintln(stderr, "run 子命令请直接使用 xray-agent 启动（或省略 run）")
		return 2
	default:
		fmt.Fprintf(stderr, "未知命令: %s\n\n", cmd)
		printHelp(stderr)
		return 2
	}
}

// resolveConfigPath 解析配置文件路径：-config 优先，否则按候选顺序探测。
func resolveConfigPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	for _, cand := range configCandidates {
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return ""
}

// loadTolerant 加载配置，允许缺失（卸载场景按默认路径继续）。
func loadTolerant(path string) (*config.Config, error) {
	if path == "" {
		return nil, errors.New("未找到配置文件（可加 -config 指定）")
	}
	return config.Load(path)
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "xray-agent - Xray 节点 Agent 管理工具")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "用法:")
	fmt.Fprintln(w, "  xray-agent [run] [-config <path>]    启动 agent 主循环（默认行为）")
	fmt.Fprintln(w, "  xray-agent <命令> [选项]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "命令:")
	for _, c := range []string{"status", "restart", "logs", "uninstall", "upgrade", "help"} {
		fmt.Fprintf(w, "  %-10s %s\n", c, commands[c])
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "选项:")
	fmt.Fprintln(w, "  -config <path>   配置文件路径（默认探测 /etc/xray-agent/config.yml 或 ./agent.yaml）")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "更多: xray-agent help <命令>")
}

func printCmdHelp(cmd string, w io.Writer) {
	switch cmd {
	case "status":
		fmt.Fprintln(w, "xray-agent status - 查看 agent/xray/systemd 状态与配置摘要")
		fmt.Fprintln(w, "用法: xray-agent status [-config <path>]")
	case "restart":
		fmt.Fprintln(w, "xray-agent restart - 重启 agent 服务（systemd 模式）或 xray 进程（手动模式）")
		fmt.Fprintln(w, "用法: xray-agent restart [-config <path>]")
	case "logs":
		fmt.Fprintln(w, "xray-agent logs - 查看日志（systemd: journalctl -u xray-agent；手动: xray 日志文件）")
		fmt.Fprintln(w, "用法: xray-agent logs [-n <行数>] [--follow] [-config <path>]")
	case "uninstall":
		fmt.Fprintln(w, "xray-agent uninstall - 卸载 agent（含 xray-core 与 geo 数据，自动适配 systemd/手动模式）")
		fmt.Fprintln(w, "用法: xray-agent uninstall [--force] [-config <path>]")
	case "upgrade":
		fmt.Fprintln(w, "xray-agent upgrade - 升级 agent 到主控最新版本（下载 → sha256 校验 → 原子替换 → 重启）")
		fmt.Fprintln(w, "用法: xray-agent upgrade [--check] [-config <path>]")
	case "help":
		fmt.Fprintln(w, "xray-agent help - 显示帮助或命令详情")
		fmt.Fprintln(w, "用法: xray-agent help [命令]")
	}
}

// newFlagSet 构造带空输出的 FlagSet（错误信息由调用方输出）。
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

// ---------- status ----------

func runStatus(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("status")
	cfgPath := fs.String("config", "", "配置文件路径")
	if err := fs.Parse(rest); err != nil {
		fmt.Fprintf(stderr, "参数错误: %v\n", err)
		return 2
	}
	path := resolveConfigPath(*cfgPath)
	cfg, err := loadTolerant(path)
	if err != nil {
		fmt.Fprintln(stderr, "status 失败:", err)
		return 1
	}

	fmt.Fprintf(stdout, "配置文件    : %s\n", path)
	fmt.Fprintf(stdout, "主控地址    : %s\n", cfg.Master.URL)
	fmt.Fprintf(stdout, "节点 ID     : %s\n", cfg.Master.NodeID)

	if isSystemdManaged() {
		fmt.Fprintf(stdout, "运行方式    : systemd\n")
		active, _ := runCmd("systemctl", "is-active", "xray-agent")
		enabled, _ := runCmd("systemctl", "is-enabled", "xray-agent")
		if active == "" {
			active = "未知"
		}
		if enabled == "" {
			enabled = "未知"
		}
		fmt.Fprintf(stdout, "systemd 服务: %s, %s\n", active, enabled)
	} else {
		fmt.Fprintf(stdout, "运行方式    : 手动\n")
	}

	exe, _ := os.Executable()
	pids := findPids(exe, true)
	if len(pids) == 0 {
		pids = findPids("xray-agent", true)
	}
	if len(pids) > 0 {
		pidStrs := make([]string, 0, len(pids))
		for _, p := range pids {
			pidStrs = append(pidStrs, strconv.Itoa(p))
		}
		fmt.Fprintf(stdout, "agent 进程  : 运行中 (PID %s)\n", strings.Join(pidStrs, ", "))
	} else {
		fmt.Fprintf(stdout, "agent 进程  : 未运行\n")
	}

	proc := xrayproc.New(cfg.Xray.Bin, cfg.Xray.ConfigPath, cfg.Xray.LogPath, cfg.Xray.PidFile)
	if proc.IsRunning() {
		pid := readPidFile(cfg.Xray.PidFile)
		fmt.Fprintf(stdout, "xray 进程   : 运行中 (PID %d%s)\n", pid, procUptime(pid))
	} else {
		fmt.Fprintf(stdout, "xray 进程   : 未运行\n")
	}
	fmt.Fprintf(stdout, "xray 二进制 : %s\n", cfg.Xray.Bin)
	fmt.Fprintf(stdout, "xray 配置   : %s\n", cfg.Xray.ConfigPath)
	return 0
}

// ---------- restart ----------

func runRestart(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("restart")
	cfgPath := fs.String("config", "", "配置文件路径")
	if err := fs.Parse(rest); err != nil {
		fmt.Fprintf(stderr, "参数错误: %v\n", err)
		return 2
	}
	path := resolveConfigPath(*cfgPath)
	cfg, err := loadTolerant(path)
	if err != nil {
		fmt.Fprintln(stderr, "restart 失败:", err)
		return 1
	}

	if isSystemdManaged() {
		if err := runCmdErr("systemctl", "restart", "xray-agent"); err != nil {
			fmt.Fprintln(stderr, "restart 失败:", err)
			return 1
		}
		fmt.Fprintln(stdout, "已重启 systemd 服务 xray-agent")
		return 0
	}

	proc := xrayproc.New(cfg.Xray.Bin, cfg.Xray.ConfigPath, cfg.Xray.LogPath, cfg.Xray.PidFile)
	if err := proc.Stop(); err != nil {
		fmt.Fprintln(stderr, "停止 xray 失败:", err)
		return 1
	}
	if err := proc.Start(); err != nil {
		fmt.Fprintln(stderr, "启动 xray 失败:", err, "（请先在主控生成并下发配置）")
		return 1
	}
	fmt.Fprintln(stdout, "xray 已重启（手动模式）")
	return 0
}

// ---------- logs ----------

func runLogs(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("logs")
	cfgPath := fs.String("config", "", "配置文件路径")
	lines := fs.Int("n", 100, "行数")
	follow := fs.Bool("follow", false, "持续跟踪")
	fs.BoolVar(follow, "f", false, "持续跟踪")
	if err := fs.Parse(rest); err != nil {
		fmt.Fprintf(stderr, "参数错误: %v\n", err)
		return 2
	}
	path := resolveConfigPath(*cfgPath)
	cfg, err := loadTolerant(path)
	if err != nil {
		fmt.Fprintln(stderr, "logs 失败:", err)
		return 1
	}

	if isSystemdManaged() {
		jArgs := []string{"-u", "xray-agent", "--no-pager", "-n", strconv.Itoa(*lines)}
		if *follow {
			jArgs = append(jArgs, "-f")
		}
		cmd := exec.Command("journalctl", jArgs...)
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(stderr, "journalctl 失败:", err)
			return 1
		}
		return 0
	}

	proc := xrayproc.New(cfg.Xray.Bin, cfg.Xray.ConfigPath, cfg.Xray.LogPath, cfg.Xray.PidFile)
	if *follow {
		cmd := exec.Command("tail", "-f", "-n", strconv.Itoa(*lines), cfg.Xray.LogPath)
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(stderr, "tail 失败:", err)
			return 1
		}
		return 0
	}
	logs, err := proc.Logs(*lines)
	if err != nil {
		fmt.Fprintln(stderr, "读取日志失败:", err)
		return 1
	}
	fmt.Fprintf(stdout, "# xray 日志 (%s)\n", cfg.Xray.LogPath)
	if logs == "" {
		fmt.Fprintln(stdout, "(无日志)")
	} else {
		fmt.Fprintln(stdout, logs)
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "# 提示: 手动模式下 agent 自身日志走 stdout（无文件）；systemd 模式请用 journalctl -u xray-agent")
	return 0
}

// ---------- uninstall ----------

// uninstallTargets 卸载目标清单。
type uninstallTargets struct {
	unitPaths []string
	files     []string
	dirs      []string
	pidFile   string
}

func collectTargets(cfg *config.Config) uninstallTargets {
	exe, _ := os.Executable()
	return uninstallTargets{
		unitPaths: systemdUnitPaths,
		files:     []string{cfg.Xray.Bin, exe},
		dirs:      []string{"/etc/xray-agent", "/var/log/xray-agent", "/run/xray-agent", "/usr/local/share/xray"},
		pidFile:   cfg.Xray.PidFile,
	}
}

// removeTargets 执行删除，返回警告列表。
func removeTargets(t uninstallTargets, w io.Writer) []string {
	var warns []string
	rm := func(p string, desc string) {
		if err := os.RemoveAll(p); err != nil {
			warns = append(warns, fmt.Sprintf("删除%s失败 %s: %v", desc, p, err))
		}
	}
	for _, f := range t.files {
		rm(f, "文件")
	}
	for _, d := range t.dirs {
		rm(d, "目录")
	}
	if t.pidFile != "" {
		rm(t.pidFile, "pid 文件")
	}
	return warns
}

func runUninstall(rest []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := newFlagSet("uninstall")
	cfgPath := fs.String("config", "", "配置文件路径")
	force := fs.Bool("force", false, "跳过确认")
	if err := fs.Parse(rest); err != nil {
		fmt.Fprintf(stderr, "参数错误: %v\n", err)
		return 2
	}
	path := resolveConfigPath(*cfgPath)
	cfg, err := loadTolerant(path)
	if err != nil {
		fmt.Fprintf(stderr, "警告: %v（按默认路径卸载）\n", err)
		cfg = config.Default()
	}

	if !*force {
		if !confirm(stdin, stdout, "将卸载 xray-agent（含 xray-core 与 geo 数据），确认? [y/N] ") {
			fmt.Fprintln(stdout, "已取消")
			return 1
		}
	}

	exe, _ := os.Executable()
	mode := "手动"
	if isSystemdManaged() {
		mode = "systemd"
	}
	fmt.Fprintf(stdout, "==> 停止服务（%s 模式）\n", mode)

	if isSystemdManaged() {
		_ = runCmdErr("systemctl", "stop", "xray-agent")
		_ = runCmdErr("systemctl", "disable", "xray-agent")
		for _, p := range systemdUnitPaths {
			_ = os.RemoveAll(p)
		}
		_ = runCmdErr("systemctl", "daemon-reload")
	} else {
		pids := findPids(exe, true)
		if len(pids) == 0 {
			pids = findPids("xray-agent", true)
		}
		if len(pids) > 0 {
			killPids(pids)
		}
	}

	proc := xrayproc.New(cfg.Xray.Bin, cfg.Xray.ConfigPath, cfg.Xray.LogPath, cfg.Xray.PidFile)
	_ = proc.Stop()

	fmt.Fprintln(stdout, "==> 删除文件")
	warns := removeTargets(collectTargets(cfg), stdout)
	for _, w := range warns {
		fmt.Fprintln(stderr, "警告:", w)
	}

	fmt.Fprintln(stdout, "==> 卸载完成")
	fmt.Fprintln(stdout, "提示: 可在主控管理端删除对应服务器节点以清理主控侧数据")
	return 0
}

// ---------- upgrade ----------

func runUpgrade(rest []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("upgrade")
	cfgPath := fs.String("config", "", "配置文件路径")
	checkOnly := fs.Bool("check", false, "仅检查版本")
	if err := fs.Parse(rest); err != nil {
		fmt.Fprintf(stderr, "参数错误: %v\n", err)
		return 2
	}
	path := resolveConfigPath(*cfgPath)
	cfg, err := loadTolerant(path)
	if err != nil {
		fmt.Fprintln(stderr, "upgrade 失败:", err)
		return 1
	}

	f := &upgrade.Fetcher{BaseURL: upgrade.EnsureURL(cfg.Master.URL)}
	if *checkOnly {
		latest, err := f.Latest()
		if err != nil {
			fmt.Fprintln(stderr, "检查失败:", err)
			return 1
		}
		if upgrade.Compare(upgrade.CurrentVersion(), latest) >= 0 {
			fmt.Fprintf(stdout, "已是最新（当前 %s，主控 %s）\n", upgrade.CurrentVersion(), latest)
		} else {
			fmt.Fprintf(stdout, "有新版本 %s（当前 %s），执行 xray-agent upgrade 升级\n", latest, upgrade.CurrentVersion())
		}
		return 0
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "获取自身路径失败:", err)
		return 1
	}
	restart := func() error {
		if isSystemdManaged() {
			return runCmdErr("systemctl", "restart", "xray-agent")
		}
		fmt.Fprintln(stdout, "手动模式：请手动重启 agent（systemctl restart xray-agent 或重新运行）")
		return nil
	}
	if err := upgrade.Apply(f, exe, restart, stdout); err != nil {
		if errors.Is(err, upgrade.ErrUpToDate) {
			return 0
		}
		fmt.Fprintln(stderr, "升级失败:", err)
		return 1
	}
	return 0
}

// ---------- 工具 ----------

func isSystemdManaged() bool {
	for _, p := range systemdUnitPaths {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

func confirm(r io.Reader, w io.Writer, prompt string) bool {
	fmt.Fprint(w, prompt)
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	ans := strings.ToLower(strings.TrimSpace(line))
	return ans == "y" || ans == "yes"
}

// findPids 用 pgrep -f 查找匹配进程，可排除自身。
func findPids(pattern string, excludeSelf bool) []int {
	out, err := exec.Command("pgrep", "-f", pattern).Output()
	if err != nil {
		return nil // 无匹配或 pgrep 不存在
	}
	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid, perr := strconv.Atoi(strings.TrimSpace(line))
		if perr != nil || pid <= 0 {
			continue
		}
		if excludeSelf && pid == os.Getpid() {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}

func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func runCmdErr(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func readPidFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return pid
}

// procUptime 返回 "，已运行 XmYs"（读取 /proc/<pid>/stat 的 starttime）。
func procUptime(pid int) string {
	if pid <= 0 {
		return ""
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return ""
	}
	s := string(stat)
	i := strings.LastIndex(s, ") ")
	if i < 0 {
		return ""
	}
	fields := strings.Fields(s[i+2:])
	if len(fields) <= 19 {
		return ""
	}
	ticks, err := strconv.ParseFloat(fields[19], 64)
	if err != nil {
		return ""
	}
	up, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return ""
	}
	upSec, _ := strconv.ParseFloat(strings.Fields(string(up))[0], 64)
	sec := int64(upSec - ticks/100) // CLK_TCK=100
	if sec < 0 {
		return ""
	}
	return ", 已运行 " + formatDur(sec)
}

func formatDur(sec int64) string {
	if sec >= 3600 {
		return fmt.Sprintf("%dh%dm", sec/3600, sec%3600/60)
	}
	if sec >= 60 {
		return fmt.Sprintf("%dm%ds", sec/60, sec%60)
	}
	return fmt.Sprintf("%ds", sec)
}
