package api

import (
	"context"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

var processStartedAt = time.Now()

// PanelVersion 面板版本（由 cmd/master 启动时注入；未注入为 dev）。
var PanelVersion = "dev"

// AdminSystemStatus GET /api/v1/admin/system/status —— 系统状态页数据（ISSUE-17）。
// 只暴露运维信息，不返回任何密钥/令牌。
func (d *Deps) AdminSystemStatus(c *gin.Context) {
	data := gin.H{
		"go_version":     runtime.Version(),
		"panel_version":  PanelVersion,
		"goroutines":     runtime.NumGoroutine(),
		"uptime_seconds": int64(time.Since(processStartedAt).Seconds()),
		"server_time":    time.Now().Format(time.RFC3339),
	}
	if d.Cfg != nil {
		data["app_name"] = d.Cfg.App.Name
		data["app_env"] = d.Cfg.App.Env
		data["db_driver"] = d.Cfg.DB.Driver
		data["backup_enabled"] = d.Cfg.Backup.Enabled
	} else {
		data["app_name"] = "xray-panel"
		data["app_env"] = "unknown"
		data["db_driver"] = ""
		data["backup_enabled"] = false
	}

	// 内存使用概况
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	data["mem_alloc_mb"] = int64(mem.Alloc / 1024 / 1024)
	data["mem_sys_mb"] = int64(mem.Sys / 1024 / 1024)

	// 数据库连通性
	if d.DB != nil {
		sqlDB, err := d.DB.DB()
		if err == nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			start := time.Now()
			err = sqlDB.PingContext(ctx)
			cancel()
			latency := time.Since(start).Milliseconds()
			data["db_ok"] = err == nil
			data["db_latency_ms"] = latency
			if err != nil {
				data["db_error"] = err.Error()
			}
		} else {
			data["db_ok"] = false
			data["db_error"] = err.Error()
		}
	} else {
		data["db_ok"] = false
		data["db_error"] = "DB 未初始化"
	}

	// 关键表计数（大表只 count，不做全表扫描）
	counts := map[string]*int64{
		"users":      new(int64),
		"servers":    new(int64),
		"inbounds":   new(int64),
		"orders":     new(int64),
		"gift_cards": new(int64),
		"audit_logs": new(int64),
	}
	for name, ptr := range counts {
		switch name {
		case "users":
			d.DB.Model(&models.User{}).Count(ptr)
		case "servers":
			d.DB.Model(&models.Server{}).Count(ptr)
		case "inbounds":
			d.DB.Model(&models.Inbound{}).Count(ptr)
		case "orders":
			d.DB.Model(&models.Order{}).Count(ptr)
		case "gift_cards":
			d.DB.Model(&models.GiftCard{}).Count(ptr)
		case "audit_logs":
			d.DB.Model(&models.AuditLog{}).Count(ptr)
		}
	}
	data["counts"] = gin.H{
		"users":      *counts["users"],
		"servers":    *counts["servers"],
		"inbounds":   *counts["inbounds"],
		"orders":     *counts["orders"],
		"gift_cards": *counts["gift_cards"],
		"audit_logs": *counts["audit_logs"],
	}

	util.OK(c, data)
}
