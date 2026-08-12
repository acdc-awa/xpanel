// Xray 节点 Agent 入口：托管 xray-core 并与主控保持 WSS 长连接。
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/zhx/xray-panel/internal/agent/accounts"
	"github.com/zhx/xray-panel/internal/agent/cli"
	"github.com/zhx/xray-panel/internal/agent/client"
	"github.com/zhx/xray-panel/internal/agent/collector"
	"github.com/zhx/xray-panel/internal/agent/config"
	"github.com/zhx/xray-panel/internal/agent/stats"
	"github.com/zhx/xray-panel/internal/agent/xrayproc"
)

var version = "dev" // 构建期 -ldflags -X main.version=<git describe>（未注入时保持 dev）

func main() {
	// 管理子命令分派：xray-agent status|restart|logs|uninstall|help
	args := os.Args[1:]
	if len(args) > 0 && cli.IsSubcommand(args[0]) {
		if args[0] == "run" {
			os.Args = append([]string{os.Args[0]}, args[1:]...)
		} else {
			os.Exit(cli.Run(args, os.Stdin, os.Stdout, os.Stderr))
		}
	}

	cfgPath := flag.String("config", "", "配置文件路径（默认探测 /etc/xray-agent/config.yml、二进制同目录下的 agent.yaml、或 ./agent.yaml）")
	flag.Parse()

	path := *cfgPath
	if path == "" {
		// 按优先级探测：系统安装路径 → 二进制同目录 → 当前工作目录
		candidates := []string{"/etc/xray-agent/config.yml"}
		if exe, err := os.Executable(); err == nil {
			candidates = append(candidates, filepath.Join(filepath.Dir(exe), "agent.yaml"))
		}
		candidates = append(candidates, "agent.yaml")
		for _, cand := range candidates {
			if _, err := os.Stat(cand); err == nil {
				path = cand
				break
			}
		}
	}
	if path == "" {
		log.Fatal("未找到配置文件（可加 -config 指定路径，默认探测 /etc/xray-agent/config.yml、二进制同目录 agent.yaml、或当前目录 agent.yaml）")
	}

	cfg, err := config.Load(path)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	proc := xrayproc.New(cfg.Xray.Bin, cfg.Xray.ConfigPath, cfg.Xray.LogPath, cfg.Xray.PidFile)
	proc.CleanupStale()

	// 启动时若已有配置则拉起 xray（崩溃由 watchdog 保持）
	if _, err := os.Stat(cfg.Xray.ConfigPath); err == nil {
		if err := proc.Start(); err != nil {
			log.Printf("xray 启动失败（watchdog 将重试）: %v", err)
		} else {
			log.Printf("xray 已启动")
		}
	}

	wdStop := make(chan struct{})
	go proc.Watchdog(wdStop)

	statsCollector := stats.New(cfg.Stats.APIAddr)

	cli := &client.Client{
		BaseURL:         cfg.Master.URL,
		NodeID:          cfg.Master.NodeID,
		Secret:          cfg.Master.Secret,
		Heartbeat:       cfg.Heartbeat,
		ReconnectMax:    cfg.ReconnectMax,
		Xray:            proc,
		Collector:       collector.New(),
		Stats:           statsCollector,
		CollectInterval: cfg.Stats.CollectInterval,
		ReportInterval:  cfg.Stats.ReportInterval,
		Accounts:        accounts.New(cfg.AccountsPath),
		CertsDir:        cfg.CertsDir,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Printf("xray-agent 启动（node=%s, master=%s, heartbeat=%s）", cfg.Master.NodeID, cfg.Master.URL, cfg.Heartbeat)
	cli.Run(ctx)

	// 退出清理（同步执行，确保 xray 被优雅停止，不遗留孤儿进程）
	close(wdStop)
	statsCollector.Close()
	_ = proc.Stop()
	log.Println("xray-agent 已退出")
}
