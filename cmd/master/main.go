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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/config"
	"github.com/acdc/xray-panel/internal/master/api"
	"github.com/acdc/xray-panel/internal/master/backup"
	"github.com/acdc/xray-panel/internal/master/billing"
	"github.com/acdc/xray-panel/internal/master/nodegate"
	"github.com/acdc/xray-panel/internal/master/services"
	"github.com/acdc/xray-panel/internal/models"
	"github.com/acdc/xray-panel/internal/pkg/db"
	"github.com/acdc/xray-panel/internal/pkg/util"
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

	// 支持 CLI 子命令（如 reset-admin）
	args := flag.Args()
	if len(args) > 0 && args[0] == "reset-admin" {
		handleResetAdmin(database, args[1:])
		return
	}

	// JWT Secret 安全自闭环：优先从 DB 获取，无则自动生成强随机密钥落库
	jwtSecret := ensureJWTSecret(database, cfg)
	jwtMgr := services.NewJWTManager(jwtSecret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	authSvc := &services.AuthService{
		DB:  database,
		JWT: jwtMgr,
	}

	ensureAdmin(database, cfg)
	ensureUserUUIDs(database)
	ensureServerDefaultOutbounds(database)

	trafficSvc := &services.TrafficService{DB: database}
	trafficSvc.StartDailyAgg(context.Background())
	trafficSvc.StartTrafficResetCron(context.Background())
	trafficSvc.StartRetentionCron(context.Background())
	orderSvc := billing.NewOrderService(database)
	auditSvc := &services.AuditService{DB: database}
	configSvc := &services.ConfigService{DB: database, Traffic: trafficSvc}
	siteSvc := services.NewSiteService(database, cfg)
	giftCardSvc := billing.NewGiftCardService(database)
	hub := nodegate.NewHub(database, trafficSvc, configSvc)

	backupSvc, err := backup.New(cfg.DB.DSN, cfg.DB.Driver, cfg.Backup, auditSvc)
	if err != nil {
		log.Fatalf("初始化备份服务失败: %v", err)
	}
	backupCtx, backupCancel := context.WithCancel(context.Background())
	defer backupCancel()
	backupSvc.Start(backupCtx)
	if cfg.Backup.Enabled {
		log.Printf("备份服务已启用（schedule=%s, keep=%d, dir=%s）", cfg.Backup.Schedule, cfg.Backup.Keep, cfg.Backup.Dir)
	} else {
		log.Printf("备份服务已禁用（仅手动触发可用）")
	}

	if cfg.App.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	otpSvc := services.NewOTPService(database, cfg)
	deps := &api.Deps{DB: database, Cfg: cfg, JWT: jwtMgr, Auth: authSvc, OTP: otpSvc, Hub: hub, Traffic: trafficSvc, Order: orderSvc, Audit: auditSvc, Config: configSvc, Site: siteSvc, GiftCard: giftCardSvc, Backup: backupSvc}
	subServer := api.NewSubscribeServer(deps)
	deps.SubServer = subServer
	hub.CertPusher = deps.PushPendingCerts
	router := deps.NewRouter()

	// 启动独立订阅服务（若 settings.subscribe_port 已配置）
	subPortStr := services.GetSetting(database, services.SettingSubscribePort)
	if subPortStr != "" {
		if subPort, err := strconv.Atoi(strings.TrimSpace(subPortStr)); err == nil && subPort > 0 {
			if err := subServer.Start(subPort); err != nil {
				log.Printf("启动独立订阅服务失败: %v", err)
			}
		}
	}

	// Web Base：在 gin 路由前剥离自定义前缀（如 /panel/api/v1/... → /api/v1/...）。
	// 放在 Handler 层而不是 gin 中间件，因为 gin 的路由树在中间件执行前已按原始路径匹配。
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := siteSvc.WebBase()
		if base != "" {
			p := r.URL.Path
			if p == "/" {
				http.Redirect(w, r, base+"/", http.StatusFound)
				return
			}
			if p == base || strings.HasPrefix(p, base+"/") {
				rest := strings.TrimPrefix(p, base)
				if rest == "" {
					rest = "/"
				}
				r.URL.Path = rest
			}
		}
		router.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.App.Port),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
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
		log.Printf("主服务关闭异常: %v", err)
	}
	_ = subServer.Shutdown(ctx)
	hub.Shutdown()
	wg.Wait()
	log.Println("已退出")
}

// ensureJWTSecret 保证 JWT Secret 安全存在：
// 1. 若 settings 表已有 jwt_secret，优先使用；
// 2. 若无但 cfg.JWT.Secret 提供了显式非默认值，存入 DB 并使用；
// 3. 否则通过 crypto/rand 自动生成 64 字符安全随机 Hex 存入 DB。
func ensureJWTSecret(database *gorm.DB, cfg *config.Config) string {
	var s models.Setting
	if err := database.Where("`key` = ?", "jwt_secret").First(&s).Error; err == nil && strings.TrimSpace(s.Value) != "" {
		return strings.TrimSpace(s.Value)
	}

	secret := strings.TrimSpace(cfg.JWT.Secret)
	if secret == "" || secret == "change-me-in-production-must-be-32-bytes" || secret == "dev-secret-change-in-production" {
		generated, err := util.RandomHex(32)
		if err == nil && generated != "" {
			secret = generated
		} else {
			secret = util.GenerateSecurePassword(32)
		}
	}

	setting := models.Setting{
		Key:   "jwt_secret",
		Value: secret,
	}
	_ = database.Save(&setting).Error
	return secret
}

// ensureAdmin 首次启动时创建初始管理员。若未在配置指定密码或使用了默认弱密码，自动生成 16 位随机强密码并在控制台高亮输出。
func ensureAdmin(database *gorm.DB, cfg *config.Config) {
	var cnt int64
	if err := database.Model(&models.User{}).Where("role = ?", models.RoleAdmin).Count(&cnt).Error; err != nil {
		log.Fatalf("查询管理员失败: %v", err)
	}
	if cnt > 0 {
		return
	}

	username := strings.TrimSpace(cfg.Admin.Username)
	if username == "" {
		username = "admin@panel.local"
	}

	rawPassword := strings.TrimSpace(cfg.Admin.Password)
	isRandom := false
	if rawPassword == "" || rawPassword == "admin123" || rawPassword == "admin" {
		rawPassword = util.GenerateSecurePassword(16)
		isRandom = true
	}

	hash, err := argon2id.CreateHash(rawPassword, argon2id.DefaultParams)
	if err != nil {
		log.Fatalf("生成管理员密码哈希失败: %v", err)
	}
	token, err := util.NewSubscribeToken()
	if err != nil {
		log.Fatalf("生成订阅 token 失败: %v", err)
	}
	uuid, err := util.NewUUID()
	if err != nil {
		log.Fatalf("生成管理员 UUID 失败: %v", err)
	}
	admin := &models.User{
		Username:          username,
		Email:             username,
		PasswordHash:      hash,
		Role:              models.RoleAdmin,
		Status:            models.StatusActive,
		SubscribeToken:    token,
		UUID:              uuid,
		TrafficCycleStart: time.Now(),
		MustChangePwd:     true, // 首次必须改密
	}
	if err := database.Create(admin).Error; err != nil {
		log.Fatalf("创建初始管理员失败: %v", err)
	}

	printAdminInitCard(username, rawPassword, isRandom)
}

func printAdminInitCard(username, password string, isRandom bool) {
	fmt.Println()
	fmt.Println("==========================================================================")
	fmt.Println("                   XrayPanel 主控系统首次初始化成功！                     ")
	fmt.Println("==========================================================================")
	fmt.Printf("   管理后台:       http://127.0.0.1:18080 (或您的反代域名)\n")
	fmt.Printf("   管理员账号:     %s\n", username)
	fmt.Printf("   初始管理员密码: %s\n", password)
	fmt.Println("--------------------------------------------------------------------------")
	if isRandom {
		fmt.Println("   [安全提示] 初始随机密码仅在控制台显示一次，请妥善保存！")
	}
	fmt.Println("   [安全提示] 首次登录后系统将强制要求修改密码。")
	fmt.Println("==========================================================================")
	fmt.Println()
}

// handleResetAdmin 执行 reset-admin CLI 子命令，重置管理员密码并递增 token_version 吊销旧会话。
func handleResetAdmin(database *gorm.DB, args []string) {
	resetFlags := flag.NewFlagSet("reset-admin", flag.ExitOnError)
	email := resetFlags.String("email", "", "指定重置的管理员邮箱/用户名（留空自动重置首个管理员）")
	newPassword := resetFlags.String("password", "", "指定新密码（留空自动生成 16 位强随机密码）")
	_ = resetFlags.Parse(args)

	var admin models.User
	query := database.Where("role = ?", models.RoleAdmin)
	if *email != "" {
		query = query.Where("username = ? OR email = ?", *email, *email)
	}
	if err := query.First(&admin).Error; err != nil {
		fmt.Printf("[错误] 未找到管理员账户 (err=%v)\n", err)
		os.Exit(1)
	}

	finalPassword := strings.TrimSpace(*newPassword)
	isRandom := false
	if finalPassword == "" {
		finalPassword = util.GenerateSecurePassword(16)
		isRandom = true
	}

	hash, err := argon2id.CreateHash(finalPassword, argon2id.DefaultParams)
	if err != nil {
		fmt.Printf("[错误] 生成密码哈希失败: %v\n", err)
		os.Exit(1)
	}

	newTokenVersion := admin.TokenVersion + 1
	updates := map[string]any{
		"password_hash":   hash,
		"token_version":   newTokenVersion,
		"must_change_pwd": false,
	}
	if err := database.Model(&admin).Updates(updates).Error; err != nil {
		fmt.Printf("[错误] 更新数据库失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("==========================================================================")
	fmt.Println("                        管理员密码重置成功！                              ")
	fmt.Println("==========================================================================")
	fmt.Printf("   管理员账号:   %s\n", admin.Username)
	fmt.Printf("   新密码:       %s\n", finalPassword)
	fmt.Printf("   会话安全:     已递增 token_version 至 %d，所有旧登录 Token 已全部失效\n", newTokenVersion)
	fmt.Println("--------------------------------------------------------------------------")
	if isRandom {
		fmt.Println("   [提示] 请复制新密码并在登录后妥善保管。")
	}
	fmt.Println("==========================================================================")
	fmt.Println()
}

// ensureUserUUIDs 为历史用户补全 UUID（升级迁移）。
func ensureUserUUIDs(database *gorm.DB) {
	var users []models.User
	if err := database.Where("uuid = ? OR uuid IS NULL", "").Find(&users).Error; err != nil {
		log.Printf("查询缺 UUID 用户失败: %v", err)
		return
	}
	for _, u := range users {
		uuid, err := util.NewUUID()
		if err != nil {
			continue
		}
		if err := database.Model(&u).Update("uuid", uuid).Error; err != nil {
			log.Printf("补全用户 %s UUID 失败: %v", u.Username, err)
		}
	}
	if len(users) > 0 {
		log.Printf("已为 %d 个用户补全 UUID", len(users))
	}
}

// ensureServerDefaultOutbounds 为所有服务器确保存在 direct 与 blocked 内置出站（升级迁移与去重）。
func ensureServerDefaultOutbounds(database *gorm.DB) {
	var servers []models.Server
	if err := database.Find(&servers).Error; err != nil {
		return
	}
	for _, s := range servers {
		api.EnsureDefaultServerOutbounds(database, s.ID)
	}
}
