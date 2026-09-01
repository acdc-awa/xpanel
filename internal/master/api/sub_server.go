package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// SubscribeServer 独立订阅 HTTP 服务（物理端口隔离，无特权管理 API）。
type SubscribeServer struct {
	deps   *Deps
	server *http.Server
	port   int
	mu     sync.Mutex
}

// NewSubscribeServer 构造独立订阅服务。
func NewSubscribeServer(deps *Deps) *SubscribeServer {
	return &SubscribeServer{deps: deps}
}

// buildEngine 构建独立订阅服务的极简 Gin Engine（纯订阅路由与清洗网关，零后台 API）。
// 2026-08-24 入口统一：唯一订阅入口 = 设置页 subscribe_path（如 /ehisnodn），
// 路径参数与 ?token= 查询参数两种形式；其余路径一律按 sub_deny_code 返回 404/401。
func (s *SubscribeServer) buildEngine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	// 1. 订阅清洗防探测网关（智能 UA 过滤 + 严格客户端白名单 + 自定义黑名单）
	// 2. 订阅频次限流（单真实 IP 60 次/分钟）
	r.Use(middleware.SubSieveMiddleware(s.deps.DB), middleware.RateLimit(60, time.Minute))

	path := services.SubscribePath(s.deps.DB)
	// {path}（?token=）与 {path}/:token（路径参数）
	r.GET(path, s.deps.Subscribe)
	r.GET(path+"/:token", s.deps.Subscribe)

	// 其余路径：统一拒绝码（404 防探测 / 401 要求鉴权，设置页 sub_deny_code）
	r.NoRoute(func(c *gin.Context) {
		code := services.SubDenyCode(s.deps.DB)
		msg := "接口不存在"
		if code == http.StatusUnauthorized {
			msg = "未授权"
		}
		util.Fail(c, code, msg)
	})
	return r
}

// Start 启动独立订阅服务（port <= 0 则忽略）。
func (s *SubscribeServer) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startLocked(port)
}

func (s *SubscribeServer) startLocked(port int) error {
	if port <= 0 {
		return nil
	}

	engine := s.buildEngine()
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}

	s.server = srv
	s.port = port

	go func() {
		log.Printf("独立订阅服务启动，监听 :%d", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("独立订阅服务异常退出: %v", err)
		}
	}()

	return nil
}

// Reload 热重载：设置页 subscribe_path / sub_deny_code 变更后重建引擎并重启（端口不变）。
func (s *SubscribeServer) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server == nil {
		return nil
	}
	port := s.port
	log.Printf("订阅设置变更，重建订阅服务（端口 :%d）…", port)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = s.server.Shutdown(ctx)
	cancel()
	s.server = nil
	s.port = 0
	return s.startLocked(port)
}

// Shutdown 优雅关闭独立订阅服务。
func (s *SubscribeServer) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		log.Printf("正在关闭独立订阅服务（:%d）…", s.port)
		err := s.server.Shutdown(ctx)
		s.server = nil
		s.port = 0
		return err
	}
	return nil
}
