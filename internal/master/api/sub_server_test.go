package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

// TestSubscribeServer_Lifecycle 订阅服务生命周期 + 入口统一行为：
// 唯一入口 = 设置页 subscribe_path（缺省 /sub）；非入口路径与无效 token 统一按
// sub_deny_code（缺省 404）拒绝；subscribe_path/sub_deny_code 变更后 Reload 重建。
func TestSubscribeServer_Lifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("failed to migrate db: %v", err)
	}

	// 有效 token 用户（无接入点时订阅返回 404「暂无可用的节点」，可据此区分「token 校验通过」）
	user := models.User{Username: "u@x.com", Email: "u@x.com", UUID: "uuid-1", PasswordHash: "h", SubscribeToken: "tok-valid", Status: models.StatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	deps := &Deps{DB: db, Cfg: config.Default()}
	subSrv := NewSubscribeServer(deps)

	// 测试忽略 0 或负数端口
	if err := subSrv.Start(0); err != nil {
		t.Errorf("Start(0) should succeed with no-op: %v", err)
	}
	if subSrv.Port() != 0 {
		t.Errorf("Port() = %d, want 0", subSrv.Port())
	}

	// 启动独立端口（缺省订阅入口 /sub）
	port := 15001
	if err := subSrv.Start(port); err != nil {
		t.Fatalf("Start(%d) failed: %v", port, err)
	}
	if subSrv.Port() != port {
		t.Errorf("Port() = %d, want %d", subSrv.Port(), port)
	}
	time.Sleep(100 * time.Millisecond)

	// 缺省入口 /sub：有效 token 走 Subscribe（无 AP → 404 暂无可用的节点）
	status, body := getBody(t, "http://127.0.0.1:15001/sub/tok-valid")
	if status != http.StatusNotFound || !strings.Contains(body, "暂无可用的节点") {
		t.Errorf("GET /sub/tok-valid = %d %q, want 404 暂无可用的节点", status, body)
	}
	status, body = getBody(t, "http://127.0.0.1:15001/sub?token=tok-valid")
	if status != http.StatusNotFound || !strings.Contains(body, "暂无可用的节点") {
		t.Errorf("GET /sub?token= = %d %q, want 404 暂无可用的节点", status, body)
	}
	// 无效 token：统一拒绝码 404（缺省），文案与 NoRoute 不同
	status, body = getBody(t, "http://127.0.0.1:15001/sub/tok-bad")
	if status != http.StatusNotFound || !strings.Contains(body, "订阅链接无效") {
		t.Errorf("GET /sub/tok-bad = %d %q, want 404 订阅链接无效", status, body)
	}
	// 旧入口 /link / 根路径兜底已退役：一律 NoRoute 拒绝
	for _, p := range []string{"/link/tok-valid", "/tok-valid", "/api/v1/sub/tok-valid", "/healthz"} {
		status, body = getBody(t, "http://127.0.0.1:15001"+p)
		if status != http.StatusNotFound || !strings.Contains(body, "接口不存在") {
			t.Errorf("GET %s = %d %q, want 404 接口不存在（退役入口）", p, status, body)
		}
	}

	// 变更 subscribe_path（/ehisnodn）与 sub_deny_code（401）→ Reload 重建
	if err := services.SetSetting(db, services.SettingSubscribePath, "/ehisnodn"); err != nil {
		t.Fatalf("set subscribe_path: %v", err)
	}
	if err := services.SetSetting(db, services.SettingSubDenyCode, "401"); err != nil {
		t.Fatalf("set sub_deny_code: %v", err)
	}
	if err := subSrv.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// 新入口生效：有效 token 仍走 Subscribe（404 暂无可用的节点）
	status, body = getBody(t, "http://127.0.0.1:15001/ehisnodn/tok-valid")
	if status != http.StatusNotFound || !strings.Contains(body, "暂无可用的节点") {
		t.Errorf("GET /ehisnodn/tok-valid = %d %q, want 404 暂无可用的节点", status, body)
	}
	// 无效 token：按 sub_deny_code=401 拒绝
	status, body = getBody(t, "http://127.0.0.1:15001/ehisnodn/tok-bad")
	if status != http.StatusUnauthorized || !strings.Contains(body, "订阅链接无效") {
		t.Errorf("GET /ehisnodn/tok-bad = %d %q, want 401 订阅链接无效", status, body)
	}
	// 旧入口 /sub 已随重建退役：NoRoute → 401 未授权
	status, body = getBody(t, "http://127.0.0.1:15001/sub/tok-valid")
	if status != http.StatusUnauthorized || !strings.Contains(body, "未授权") {
		t.Errorf("GET /sub/tok-valid（退役后）= %d %q, want 401 未授权", status, body)
	}

	// 端口热重载仍可用
	if err := subSrv.ReloadPort(15002); err != nil {
		t.Fatalf("ReloadPort(15002) failed: %v", err)
	}
	if subSrv.Port() != 15002 {
		t.Errorf("Port() = %d, want 15002", subSrv.Port())
	}
	time.Sleep(100 * time.Millisecond)
	status, body = getBody(t, "http://127.0.0.1:15002/ehisnodn/tok-valid")
	if status != http.StatusNotFound || !strings.Contains(body, "暂无可用的节点") {
		t.Errorf("GET reloaded /ehisnodn/tok-valid = %d %q", status, body)
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

func getBody(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}