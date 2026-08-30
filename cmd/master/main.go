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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/api"
	"github.com/acdc-awa/xpanel/internal/master/backup"
	"github.com/acdc-awa/xpanel/internal/master/billing"
	"github.com/acdc-awa/xpanel/internal/master/nodegate"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/master/store/gormstore"
	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/db"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// Version 面板版本（构建期 ldflags 注入：-X main.Version=<release tag>；未注入为 dev）。
var Version = "dev"

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 冒烟自检（面板内更新用）：仅验证配置可加载，退出码 0=通过。
	// 刻意不连库不带任何副作用——新二进制执行本检查时旧进程仍在运行，
	// 避免 SQLite 并发写与误锁；运行时风险由 entrypoint 失败回滚兜底。
	if len(flag.Args()) > 0 && flag.Args()[0] == "self-test" {
		os.Exit(runSelfTest(*cfgPath))
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

	// Stage 8：仓储适配层——GORM 实现为默认适配器；更换数据库适配只需替换此处构造。
	billingStore := gormstore.NewBillingStore(database)

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
	orderSvc := billing.NewOrderService(billingStore)
	auditSvc := &services.AuditService{DB: database}

	// Stage 5：进程内同步事件总线（订阅者在 Publish 调用栈内执行，失败仅记日志）。
	eventBus := contracts.NewEventBus()
	orderSvc.Events = eventBus

	// 核心驱动注册表（Stage 4）：默认 xray；CORE_DRIVER 环境变量可选注入已注册驱动。
	driverReg := contracts.NewDriverRegistry()
	driverReg.Register(xray.NewDriver())
	coreDriver := driverReg.Default()
	if name := os.Getenv("CORE_DRIVER"); name != "" {
		if d := driverReg.Find(name); d != nil {
			coreDriver = d
			log.Printf("核心驱动：%s（CORE_DRIVER 注入）", name)
		} else {
			log.Printf("CORE_DRIVER=%q 未注册，回退默认驱动 %s", name, coreDriver.Name())
		}
	}
	configSvc := &services.ConfigService{DB: database, Traffic: trafficSvc, Driver: coreDriver}
	siteSvc := services.NewSiteService(database)
	giftCardSvc := billing.NewGiftCardService(billingStore)
	hub := nodegate.NewHub(database, trafficSvc, configSvc)

	// Stage 5 事件订阅：订单支付成功 → 热更新用户到所有在线节点（原 api 层直调 Hub 的收口）。
	eventBus.Subscribe(contracts.EventOrderPaid, func(ctx context.Context, ev contracts.DomainEvent) error {
		hub.SyncUsersToAll()
		return nil
	})

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

	otpSvc := services.NewOTPService(database, cfg, jwtSecret)
	deps := &api.Deps{DB: database, Cfg: cfg, JWT: jwtMgr, Auth: authSvc, OTP: otpSvc, Hub: hub, Traffic: trafficSvc, Order: orderSvc, Audit: auditSvc, Config: configSvc, Site: siteSvc, GiftCard: giftCardSvc, Backup: backupSvc}
	api.PanelVersion = Version
	subServer := api.NewSubscribeServer(deps)
	deps.SubServer = subServer
	hub.CertPusher = deps.PushPendingCerts

	// 三端口模型（2026-08-25 拍板）：面板（SPA+API 合并）/ 节点 WS 网关 / 订阅 各自独立监听，
	// 域名与路径分流由反代（Caddy）承担——每个端口只做自己的职责，程序内全部根路径语义。
	type listener struct {
		srv  *http.Server
		role string
	}
	listeners := []listener{
		{srv: &http.Server{Addr: fmt.Sprintf(":%d", cfg.App.Port), Handler: api.NewWebRouter(deps), ReadHeaderTimeout: 10 * time.Second}, role: "面板(SPA+API)"},
		{srv: &http.Server{Addr: fmt.Sprintf(":%d", cfg.App.WSPort), Handler: api.NewWSRouter(deps), ReadHeaderTimeout: 10 * time.Second}, role: "节点 WS 网关"},
	}
	if cfg.App.SubPort > 0 {
		if err := subServer.Start(cfg.App.SubPort); err != nil {
			log.Printf("启动订阅服务失败: %v", err)
		}
	} else {
		log.Printf("订阅服务已禁用（sub_port=0）")
	}

	// 容器形态：本版本已完成全部初始化并即将监听，视为「更新已确认」——
	// 清理更新待确认标记与回滚备份（master.prev / web.prev），此后 crash 由 entrypoint 直接拉起不再回滚。
	// 仅在自更新流程写入这些文件时执行清理；非容器形态下文件不存在，无副作用。
	cleanupUpdateMarker()

	var wg sync.WaitGroup
	for _, l := range listeners {
		l := l
		wg.Add(1)
		go func(l listener) {
			defer wg.Done()
			log.Printf("%s v%s 启动 - %s 监听 %s（env=%s, db=%s）", cfg.App.Name, Version, l.role, l.srv.Addr, cfg.App.Env, cfg.DB.Driver)
			if err := l.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("%s（%s）启动失败: %v", l.role, l.srv.Addr, err)
			}
		}(l)
	}

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("收到退出信号，正在关闭…")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, l := range listeners {
		if err := l.srv.Shutdown(ctx); err != nil {
			log.Printf("%s（%s）关闭异常: %v", l.role, l.srv.Addr, err)
		}
	}
	_ = subServer.Shutdown(ctx)
	hub.Shutdown()
	wg.Wait()
	log.Println("已退出")
}

// cleanupUpdateMarker 清理容器自更新遗留的待确认标记与回滚备份
// （面板内更新 API 写入，见 internal/master/api/update.go；entrypoint 据此判定失败回滚）。
func cleanupUpdateMarker() {
	for _, p := range []string{"/app/.update-pending", "/app/master.prev", "/app/web.prev"} {
		if _, err := os.Stat(p); err == nil {
			log.Printf("清理更新确认标记/回滚备份: %s", p)
			_ = os.RemoveAll(p)
		}
	}
}

// runSelfTest 冒烟自检：加载配置并打印版本概要，返回退出码（0=通过）。
// 供面板内更新下载新二进制后先试跑（挡架构错误/损坏/配置兼容性），不连库无副作用。
func runSelfTest(cfgPath string) int {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "self-test 失败: %v\n", err)
		return 1
	}
	fmt.Printf("self-test OK: app=%s version=%s env=%s db=%s\n", cfg.App.Name, Version, cfg.App.Env, cfg.DB.Driver)
	return 0
}

// invalidJWTSecret 模板/示例中的占位值一律视为未配置（自动生成），杜绝弱密钥被采信。
// 2026-08-24：密钥不再写进 configs 模板（留空=首次自动生成落库），此处防御旧配置残留。
func invalidJWTSecret(v string) bool {
	switch strings.TrimSpace(v) {
	case "", "change-me-in-production-must-be-32-bytes", "dev-secret-change-in-production",
		"change-me-in-production-at-least-32-chars", "replace-with-openssl-rand-hex-32":
		return true
	}
	return false
}

// ensureJWTSecret 保证 JWT Secret 安全存在：
// 1. 若 settings 表已有 jwt_secret，优先使用；
// 2. 若无但 cfg.JWT.Secret 提供了显式非占位值，存入 DB 并使用；
// 3. 否则通过 crypto/rand 自动生成 64 字符安全随机 Hex 存入 DB。
func ensureJWTSecret(database *gorm.DB, cfg *config.Config) string {
	var s models.Setting
	if err := database.Where("`key` = ?", "jwt_secret").First(&s).Error; err == nil && strings.TrimSpace(s.Value) != "" {
		return strings.TrimSpace(s.Value)
	}

	secret := strings.TrimSpace(cfg.JWT.Secret)
	if invalidJWTSecret(secret) {
		secret = ""
	}
	if secret == "" {
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

// ensureAdmin 首次启动时创建初始管理员。账密不写进 configs 模板：
// 未指定（或占位弱值）时用户名回退 admin@panel.local、密码自动生成 16 位强随机串并在控制台高亮输出；
// 显式指定请写入 config.yaml 的 admin.username / admin.password（唯一配置入口，2026-08-30 起环境变量退役）。
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
	// 2026-08-24：模板与 .env.example 不再提供默认账密（留空=随机生成）；
	// 以下弱值/占位值防御旧配置与照抄示例的部署者。
	if rawPassword == "" || rawPassword == "admin123" || rawPassword == "admin" || rawPassword == "replace-with-strong-password" {
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
