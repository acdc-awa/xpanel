// Package config 负责主控配置加载。configs/config.yaml 是唯一配置入口
// （2026-08-30 拍板：退役环境变量覆盖，杜绝双源漂移；编排参数见 deploy 侧 .env）。
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 为全量配置，对应 configs/config.yaml。
type Config struct {
	App    App    `yaml:"app"`
	DB     DB     `yaml:"db"`
	JWT    JWT    `yaml:"jwt"`
	Totp   Totp   `yaml:"totp"`
	Admin  Admin  `yaml:"admin"`
	Backup Backup `yaml:"backup"`
	Update Update `yaml:"update"`
}

// Update 面板内更新配置（容器内自更新：下载/校验/替换后自杀退出，由 restart: unless-stopped 拉起新版本）。
type Update struct {
	Enabled bool   `yaml:"enabled"` // 面板内更新开关（false 时仅展示版本，不提供更新入口）
	Repo    string `yaml:"repo"`    // GitHub owner/repo，空 = acdc-awa/xpanel
	Mirror  string `yaml:"mirror"`  // github.com 的替代基址/代理前缀（可选，如 https://ghproxy.net/https://github.com）
}

// Backup 备份配置。
type Backup struct {
	Enabled  bool   `yaml:"enabled"`
	Schedule string `yaml:"schedule"` // cron 表达式，如 "0 3 * * *"（每天 03:00）
	Keep     int    `yaml:"keep"`     // 保留备份份数
	Dir      string `yaml:"dir"`      // 备份目录
}

type App struct {
	Name        string `yaml:"name"`
	Env         string `yaml:"env"`
	Port        int    `yaml:"port"`          // 面板端口（SPA 前端 + 后端 HTTP API，含 /healthz /readyz 探针）
	WSPort      int    `yaml:"ws_port"`       // 节点 WebSocket 网关端口
	SubPort     int    `yaml:"sub_port"`      // 订阅独立端口（0=禁用订阅服务）
	PublicURL   string `yaml:"public_url"`    // 面板公网地址（如 https://panel.example.com），用于生成节点一键安装命令
	WSPublicURL string `yaml:"ws_public_url"` // 节点 WebSocket 对外地址（如 wss://ws.example.com/node/ws；空=默认面板域名 + /node/ws）
}

type DB struct {
	Driver string `yaml:"driver"` // sqlite | mysql
	DSN    string `yaml:"dsn"`
}

type JWT struct {
	Secret     string        `yaml:"secret"`
	AccessTTL  time.Duration `yaml:"access_ttl"`
	RefreshTTL time.Duration `yaml:"refresh_ttl"`
}

// Totp 2FA 加密密钥（2026-08-14 方向③）：totp_secret 入库前 AES-GCM 加密。
// 为空时回退用 JWT.Secret 派生（开发可用；生产建议单独配置）。
type Totp struct {
	EncryptKey string `yaml:"encrypt_key"`
}

type Admin struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Default 返回内置默认值（config.yaml 缺省时兜底）。
func Default() *Config {
	return &Config{
		App:    App{Name: "xray-panel", Env: "dev", Port: 18080, WSPort: 18082, SubPort: 6000},
		DB:     DB{Driver: "sqlite", DSN: "./data/panel.db"},
		JWT:    JWT{Secret: "", AccessTTL: 2 * time.Hour, RefreshTTL: 7 * 24 * time.Hour},
		Backup: Backup{Enabled: true, Schedule: "0 3 * * *", Keep: 14, Dir: "./data/backups"},
		Update: Update{Enabled: true, Repo: "acdc-awa/xpanel"},
	}
}

// Load 读取 YAML 配置文件（唯一入口，无环境变量覆盖）。
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("配置文件不存在: %s（可复制 configs/config.example.yaml）", path)
		}
		return nil, fmt.Errorf("读取配置失败: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}

	// JWT secret 允许为空：首次启动由 ensureJWTSecret 自动生成强随机密钥并持久化到 DB
	// （模板不再提供任何默认值，杜绝默认密钥；显式指定请写进本文件 jwt.secret 段）。

	return cfg, nil
}

// applyEnv 已退役（2026-08-30 拍板）：config.yaml 唯一入口，杜绝 env/config 双源漂移。
// 历史版本用环境变量覆盖的字段（APP_* / DB_* / JWT_SECRET / ADMIN_* / BACKUP_*）一律改在该文件配置。
