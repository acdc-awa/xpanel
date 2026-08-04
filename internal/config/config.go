// Package config 负责主控配置加载（YAML + 环境变量覆盖）。
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 为全量配置，对应 configs/config.yaml。
type Config struct {
	App   App   `yaml:"app"`
	DB    DB    `yaml:"db"`
	JWT   JWT   `yaml:"jwt"`
	Admin Admin `yaml:"admin"`
	Auth  Auth  `yaml:"auth"`
}

type App struct {
	Name      string `yaml:"name"`
	Env       string `yaml:"env"`
	Port      int    `yaml:"port"`
	PublicURL string `yaml:"public_url"` // 面板公网地址（如 https://panel.example.com），用于生成节点一键安装命令
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

type Admin struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type Auth struct {
	InviteRequired bool `yaml:"invite_required"`
}

// Default 返回内置默认值（config.yaml 缺省时兜底）。
func Default() *Config {
	return &Config{
		App: App{Name: "xray-panel", Env: "dev", Port: 8080},
		DB:  DB{Driver: "sqlite", DSN: "./data/panel.db"},
		JWT: JWT{Secret: "dev-secret-change-me", AccessTTL: 2 * time.Hour, RefreshTTL: 7 * 24 * time.Hour},
		Auth: Auth{InviteRequired: true},
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
	return cfg, nil
}

// applyEnv 用环境变量覆盖关键项（便于容器/部署场景）。
func (c *Config) applyEnv() {
	if v := os.Getenv("APP_PUBLIC_URL"); v != "" {
		c.App.PublicURL = v
	}
	if v := os.Getenv("APP_PORT"); v != "" {
		var p int
		if _, err := fmt.Sscanf(v, "%d", &p); err == nil {
			c.App.Port = p
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
}