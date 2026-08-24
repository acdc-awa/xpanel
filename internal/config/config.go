// Package config 负责主控配置加载（YAML + 环境变量覆盖）。
package config

import (
	"fmt"
	"os"
	"strconv"
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
	Port        int    `yaml:"port"`        // SPA 前端端口
	APIPort     int    `yaml:"api_port"`     // 后端 HTTP API 端口（含 /healthz /readyz 探针）
	WSPort      int    `yaml:"ws_port"`      // 节点 WebSocket 网关端口
	SubPort     int    `yaml:"sub_port"`     // 订阅独立端口（0=禁用订阅服务）
	PublicURL   string `yaml:"public_url"`   // 面板公网地址（如 https://panel.example.com），用于生成节点一键安装命令
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
		App:    App{Name: "xray-panel", Env: "dev", Port: 18080, APIPort: 18081, WSPort: 18082, SubPort: 6000},
		DB:     DB{Driver: "sqlite", DSN: "./data/panel.db"},
		JWT:    JWT{Secret: "", AccessTTL: 2 * time.Hour, RefreshTTL: 7 * 24 * time.Hour},
		Backup: Backup{Enabled: true, Schedule: "0 3 * * *", Keep: 14, Dir: "./data/backups"},
	}
}

// Load 读取 YAML 文件并应用环境变量覆盖。
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

	cfg.applyEnv()

	// JWT secret 允许为空：首次启动由 ensureJWTSecret 自动生成强随机密钥并持久化到 DB
	// （模板不再提供任何默认值，杜绝默认密钥；显式指定请用 env JWT_SECRET）。

	return cfg, nil
}

// applyEnv 用环境变量覆盖关键项（便于容器/部署场景）。
func (c *Config) applyEnv() {
	if v := os.Getenv("APP_ENV"); v != "" {
		c.App.Env = v
	}
	if v := os.Getenv("APP_PUBLIC_URL"); v != "" {
		c.App.PublicURL = v
	}
	if v := os.Getenv("APP_WS_PUBLIC_URL"); v != "" {
		c.App.WSPublicURL = v
	}
	for _, e := range []struct {
		env  string
		dst  *int
	}{
		{"APP_PORT", &c.App.Port},
		{"APP_API_PORT", &c.App.APIPort},
		{"APP_WS_PORT", &c.App.WSPort},
		{"APP_SUB_PORT", &c.App.SubPort},
	} {
		if v := os.Getenv(e.env); v != "" {
			if p, err := strconv.Atoi(v); err == nil {
				*e.dst = p
			}
		}
	}
	if v := os.Getenv("DB_DRIVER"); v != "" {
		c.DB.Driver = v
	}
	if v := os.Getenv("DB_DSN"); v != "" {
		c.DB.DSN = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		c.JWT.Secret = v
	}
	if v := os.Getenv("ADMIN_USERNAME"); v != "" {
		c.Admin.Username = v
	}
	if v := os.Getenv("ADMIN_PASSWORD"); v != "" {
		c.Admin.Password = v
	}
	if v := os.Getenv("BACKUP_ENABLED"); v != "" {
		c.Backup.Enabled = v == "1" || v == "true"
	}
	if v := os.Getenv("BACKUP_SCHEDULE"); v != "" {
		c.Backup.Schedule = v
	}
	if v := os.Getenv("BACKUP_KEEP"); v != "" {
		var k int
		if _, err := fmt.Sscanf(v, "%d", &k); err == nil {
			c.Backup.Keep = k
		}
	}
	if v := os.Getenv("BACKUP_DIR"); v != "" {
		c.Backup.Dir = v
	}
}
