// Package api 承载 HTTP 路由与 handler。
package api

import (
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/config"
	"github.com/zhx/xray-panel/internal/master/nodegate"
	"github.com/zhx/xray-panel/internal/master/services"
)

// Deps 集中注入 handler 依赖。
type Deps struct {
	DB      *gorm.DB
	Cfg     *config.Config
	JWT     *services.JWTManager
	Auth    *services.AuthService
	Hub     *nodegate.Hub
	Traffic *services.TrafficService
}