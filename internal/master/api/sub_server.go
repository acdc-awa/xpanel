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

	"github.com/acdc/xray-panel/internal/master/middleware"
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

// Port 返回当前监听端口（0 表示未启动）。
func (s *SubscribeServer) Port() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.port
}

// buildEngine 构建独立订阅服务的极简 Gin Engine（纯订阅路由与清洗网关，零后台 API）。
func (s *SubscribeServer) buildEngine() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	// 1. 原生订阅清洗防探测网关（智能 UA 过滤 + 严格客户端白名单 + 自定义黑名单）
	r.Use(middleware.SubSieveMiddleware(s.deps.DB))

	// 2. 订阅频次限流（单真实 IP 60 次/分钟）
	r.Use(middleware.RateLimit(60, time.Minute))

	// 3. 极简订阅路由（全格式兼容）
	// /sub/:token 及 /sub?token=xxx
	r.GET("/sub/:token", s.deps.Subscribe)
	r.GET("/sub", s.deps.Subscribe)

	// /link/:token 及 /link?token=xxx
	r.GET("/link/:token", s.deps.Subscribe)
	r.GET("/link", s.deps.Subscribe)

	// 兼容 Xboard / 传统客户端路径 /api/v1/client/subscribe?token=xxx 及 /api/v1/sub/:token
	r.GET("/api/v1/client/subscribe", s.deps.Subscribe)
	r.GET("/api/v1/sub/:token", s.deps.Subscribe)
	r.GET("/api/v1/sub", s.deps.Subscribe)

	// 健康检查
	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	// 根路径兜底匹配 /:token
	r.GET("/:token", s.deps.Subscribe)

	return r
}

// Start 启动独立订阅服务（port <= 0 则忽略）。
func (s *SubscribeServer) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

// Reload 热重载监听端口（newPort <= 0 时优雅关闭服务）。
func (s *SubscribeServer) Reload(newPort int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if newPort == s.port {
		return nil
	}

	// 若旧服务正在运行，先优雅关闭
	if s.server != nil {
		log.Printf("正在切换独立订阅服务端口（原端口 :%d → 新端口 :%d）…", s.port, newPort)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = s.server.Shutdown(ctx)
		cancel()
		s.server = nil
		s.port = 0
	}

	if newPort <= 0 {
		return nil
	}

	engine := s.buildEngine()
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", newPort),
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.server = srv
	s.port = newPort

	go func() {
		log.Printf("独立订阅服务热重载启动，监听 :%d", newPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("独立订阅服务异常退出: %v", err)
		}
	}()

	return nil
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
