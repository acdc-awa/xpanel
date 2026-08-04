// Xray 节点 Agent 入口：托管 xray-core 并与主控保持 WSS 长连接。
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/zhx/xray-panel/internal/agent/client"
	"github.com/zhx/xray-panel/internal/agent/collector"
	"github.com/zhx/xray-panel/internal/agent/config"
	"github.com/zhx/xray-panel/internal/agent/stats"
	"github.com/zhx/xray-panel/internal/agent/xrayproc"
)

func main() {
	cfgPath := flag.String("config", "agent.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
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