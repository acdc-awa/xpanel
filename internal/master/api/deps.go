// Package api 承载 HTTP 路由与 handler。
package api

import (
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/nodegate"
)

// Deps 集中注入 handler 依赖。
type Deps struct {
	DB        *gorm.DB
	Cfg       *config.Config
	JWT       contracts.JWTManager
	Auth      contracts.AuthService
	OTP       contracts.OTPService
	Hub       *nodegate.Hub
	Traffic   contracts.TrafficService
	Order     contracts.OrderService
	Audit     contracts.AuditService
	Config    contracts.ConfigService
	Site      contracts.SiteService
	GiftCard  contracts.GiftCardService
	Backup    contracts.BackupService
	SubServer *SubscribeServer
}
