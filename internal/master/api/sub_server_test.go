package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/models"
)

func TestSubscribeServer_Lifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("failed to migrate db: %v", err)
	}

	cfg := config.Default()
	deps := &Deps{DB: db, Cfg: cfg}
	subSrv := NewSubscribeServer(deps)

	// 测试忽略 0 或负数端口
	if err := subSrv.Start(0); err != nil {
		t.Errorf("Start(0) should succeed with no-op: %v", err)
	}
	if subSrv.Port() != 0 {
		t.Errorf("Port() = %d, want 0", subSrv.Port())
	}

	// 测试启动独立端口（如 15001）
	port := 15001
	if err := subSrv.Start(port); err != nil {
		t.Fatalf("Start(%d) failed: %v", port, err)
	}
	if subSrv.Port() != port {
		t.Errorf("Port() = %d, want %d", subSrv.Port(), port)
	}

	// 等待服务监听就绪
	time.Sleep(100 * time.Millisecond)

	// 请求 healthz 检查
	resp, err := http.Get("http://127.0.0.1:15001/healthz")
	if err != nil {
		t.Fatalf("failed to GET /healthz on sub server: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// 测试热重载至新端口 15002
	if err := subSrv.Reload(15002); err != nil {
		t.Fatalf("Reload(15002) failed: %v", err)
	}
	if subSrv.Port() != 15002 {
		t.Errorf("Port() = %d, want 15002", subSrv.Port())
	}

	time.Sleep(100 * time.Millisecond)
	resp2, err := http.Get("http://127.0.0.1:15002/healthz")
	if err != nil {
		t.Fatalf("failed to GET /healthz on reloaded sub server: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz status = %d, want %d", resp2.StatusCode, http.StatusOK)
	}

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := subSrv.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
	if subSrv.Port() != 0 {
		t.Errorf("Port() after shutdown = %d, want 0", subSrv.Port())
	}
}
