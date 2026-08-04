// Package config 定义节点 Agent 配置。
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config Agent 配置（对应 agent.yaml）。
type Config struct {
	Master  Master  `yaml:"master"`
	Xray    Xray    `yaml:"xray"`
	Heartbeat time.Duration `yaml:"heartbeat_interval"` // 心跳间隔
	ReconnectMax time.Duration `yaml:"reconnect_max"` // 重连退避上限
}

type Master struct {
	URL    string `yaml:"url"`    // 主控 ws 地址（不含 query），如 ws://127.0.0.1:18080/api/v1/node/ws
	NodeID string `yaml:"node_id"`
	Secret string `yaml:"secret"`
}

type Xray struct {
	Bin        string `yaml:"bin"`         // xray 可执行文件路径
	ConfigPath string `yaml:"config_path"` // 配置落盘路径
	LogPath    string `yaml:"log_path"`    // xray stdout/stderr 日志
	PidFile    string `yaml:"pid_file"`    // pid 文件
}

// Default 返回内置默认值。
func Default() *Config {
	return &Config{
		Heartbeat:   30 * time.Second,
		ReconnectMax: 60 * time.Second,
		Xray: Xray{
			Bin:        "/usr/local/bin/xray",
			ConfigPath: "/etc/xray-agent/config.json",
			LogPath:    "/var/log/xray-agent/xray.log",
			PidFile:    "/run/xray-agent/xray.pid",
		},
	}
}

// Load 读取并校验配置。
func Load(path string) (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if cfg.Master.URL == "" || cfg.Master.NodeID == "" || cfg.Master.Secret == "" {
		return nil, fmt.Errorf("配置缺失 master.url / master.node_id / master.secret")
	}
	if cfg.Xray.Bin == "" {
		return nil, fmt.Errorf("配置缺失 xray.bin")
	}
	return cfg, nil
}