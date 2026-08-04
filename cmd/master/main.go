// Xray 面板主控入口。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/config"
	"github.com/zhx/xray-panel/internal/master/api"
	"github.com/zhx/xray-panel/internal/master/nodegate"
	"github.com/zhx/xray-panel/internal/master/services"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/db"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// sqlite 需要保证数据目录存在
	if cfg.DB.Driver == "sqlite" {
		if dir := filepath.Dir(cfg.DB.DSN); dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				log.Fatalf("创建数据目录失败: %v", err)
			}
		}
	}

	database, err := db.Open(&cfg.DB)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}
	if err := models.AutoMigrate(database); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}

	jwtMgr := services.NewJWTManager(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	authSvc := &services.AuthService{
		DB:             database,
		JWT:            jwtMgr,
		InviteRequired: cfg.Auth.InviteRequired,
	}

	ensureAdmin(database, cfg)

	trafficSvc := &services.TrafficService{DB: database}
	trafficSvc.StartDailyAgg(context.Background())
	hub := nodegate.NewHub(database, trafficSvc)

	if cfg.App.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	deps := &api.Deps{DB: database, Cfg: cfg, JWT: jwtMgr, Auth: authSvc, Hub: hub, Traffic: trafficSvc}
	router := deps.NewRouter()

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.App.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("%s 启动，监听 :%d（env=%s, db=%s）", cfg.App.Name, cfg.App.Port, cfg.App.Env, cfg.DB.Driver)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("服务启动失败: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到退出信号，正在关闭…")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("关闭异常: %v", err)
	}
	log.Println("已退出")
}

// ensureAdmin 首次启动时创建初始管理员。
func ensureAdmin(database *gorm.DB, cfg *config.Config) {
	var cnt int64
	if err := database.Model(&models.User{}).Where("role = ?", models.RoleAdmin).Count(&cnt).Error; err != nil {
		log.Fatalf("查询管理员失败: %v", err)
	}
	if cnt > 0 {
		return
	}
	hash, err := argon2id.CreateHash(cfg.Admin.Password, argon2id.DefaultParams)
	if err != nil {
		log.Fatalf("生成管理员密码失败: %v", err)
	}
	token, err := util.NewSubscribeToken()
	if err != nil {
		log.Fatalf("生成订阅 token 失败: %v", err)
	}
	admin := &models.User{
		Username:       cfg.Admin.Username,
		PasswordHash:   hash,
		Role:           models.RoleAdmin,
		Status:         models.StatusActive,
		SubscribeToken: token,
	}
	if err := database.Create(admin).Error; err != nil {
		log.Fatalf("创建初始管理员失败: %v", err)
	}
	log.Printf("已创建初始管理员 %q（密码来自配置，请尽快登录后修改）", cfg.Admin.Username)
}