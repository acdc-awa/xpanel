package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/config"
)

// TestSwapUpdateFilesSuccessAndWebPrevPath 验证自更新备份路径与 entrypoint.sh / cleanupUpdateMarker 一致（/app/web.prev）。
func TestSwapUpdateFilesSuccessAndWebPrevPath(t *testing.T) {
	appRoot := t.TempDir()
	masterPath := filepath.Join(appRoot, "master")
	webDist := filepath.Join(appRoot, "web", "dist")
	if err := os.MkdirAll(webDist, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(masterPath, []byte("old-bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDist, "index.html"), []byte("old-web"), 0o644); err != nil {
		t.Fatal(err)
	}

	stage := t.TempDir()
	newBin := filepath.Join(stage, "master")
	newDist := filepath.Join(stage, "dist")
	if err := os.MkdirAll(newDist, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newBin, []byte("new-bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDist, "index.html"), []byte("new-web"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := swapUpdateFiles(appRoot, masterPath, newBin, newDist, "v0.9.9"); err != nil {
		t.Fatalf("swapUpdateFiles 失败: %v", err)
	}

	gotBin, _ := os.ReadFile(masterPath)
	if string(gotBin) != "new-bin" {
		t.Fatalf("master 内容 = %q, want new-bin", gotBin)
	}
	gotWeb, _ := os.ReadFile(filepath.Join(webDist, "index.html"))
	if string(gotWeb) != "new-web" {
		t.Fatalf("web/dist 内容 = %q, want new-web", gotWeb)
	}
	prevBin, err := os.ReadFile(masterPath + ".prev")
	if err != nil || string(prevBin) != "old-bin" {
		t.Fatalf("master.prev = %q err=%v, want old-bin", prevBin, err)
	}
	prevWeb, err := os.ReadFile(filepath.Join(appRoot, "web.prev", "index.html"))
	if err != nil || string(prevWeb) != "old-web" {
		t.Fatalf("web.prev/index.html = %q err=%v, want old-web", prevWeb, err)
	}
	if _, err := os.Stat(webDist + ".prev"); !os.IsNotExist(err) {
		t.Fatalf("不应生成错误的 web/dist.prev 目录")
	}
	if _, err := os.Stat(webDist + ".old"); !os.IsNotExist(err) {
		t.Fatalf("成功替换后应清理临时 web/dist.old")
	}
	pending, err := os.ReadFile(filepath.Join(appRoot, ".update-pending"))
	if err != nil || string(pending) != "v0.9.9" {
		t.Fatalf(".update-pending = %q err=%v, want v0.9.9", pending, err)
	}
}

// TestSwapUpdateFilesRollbackOnMasterRenameFailure 替换新二进制失败时，已替换的前端与待确认标记必须完整回滚。
func TestSwapUpdateFilesRollbackOnMasterRenameFailure(t *testing.T) {
	appRoot := t.TempDir()
	masterPath := filepath.Join(appRoot, "master")
	webDist := filepath.Join(appRoot, "web", "dist")
	if err := os.MkdirAll(webDist, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(masterPath, []byte("old-bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDist, "index.html"), []byte("old-web"), 0o644); err != nil {
		t.Fatal(err)
	}

	stage := t.TempDir()
	newDist := filepath.Join(stage, "dist")
	if err := os.MkdirAll(newDist, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDist, "index.html"), []byte("new-web"), 0o644); err != nil {
		t.Fatal(err)
	}
	missingBin := filepath.Join(stage, "nonexistent-master")

	err := swapUpdateFiles(appRoot, masterPath, missingBin, newDist, "v0.9.9")
	if err == nil {
		t.Fatal("缺少 newBin 时 swapUpdateFiles 必须返回错误")
	}

	gotWeb, rerr := os.ReadFile(filepath.Join(webDist, "index.html"))
	if rerr != nil || string(gotWeb) != "old-web" {
		t.Fatalf("失败回滚后 web/dist 应还原为 old-web, got %q err=%v", gotWeb, rerr)
	}
	gotBin, _ := os.ReadFile(masterPath)
	if string(gotBin) != "old-bin" {
		t.Fatalf("失败后 master 应仍为 old-bin, got %q", gotBin)
	}
	if _, serr := os.Stat(filepath.Join(appRoot, ".update-pending")); !os.IsNotExist(serr) {
		t.Fatal("替换未执行时不应残留 .update-pending")
	}
}

// TestAdminUpdateApplyInvalidVersionStateReset 验证应用阶段早退校验失败时能释放 Running 状态。
func TestAdminUpdateApplyInvalidVersionStateReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	updateActive.Store(false)
	updateStateMu.Lock()
	updateState = updateProgress{}
	updateStateMu.Unlock()

	if _, err := os.Stat("/app/master"); err != nil {
		// 非容器环境下无法走完前段检查，只验证 setUpdateProgress(phaseFailed) 语义闭环；
		// 若未来测试环境在容器内则直接跑 HTTP 路径。
		updateActive.Store(true)
		updateStateMu.Lock()
		updateState = updateProgress{Running: true, Phase: phaseChecking}
		updateStateMu.Unlock()
		setUpdateProgress(phaseFailed, "版本号格式不正确", "")
		snap := snapshotUpdateProgress()
		if snap.Running || snap.Phase != phaseFailed || updateActive.Load() {
			t.Fatalf("failed 状态应清零 Running 与 updateActive，got snap=%+v active=%v", snap, updateActive.Load())
		}
		return
	}

	d := &Deps{Cfg: &config.Config{Update: config.Update{Enabled: true}}}
	r := gin.New()
	r.POST("/admin/update/apply", d.AdminUpdateApply)
	req := httptest.NewRequest(http.MethodPost, "/admin/update/apply", strings.NewReader(`{"version":"../bad"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法版本应返回 400, got %d", w.Code)
	}
	snap := snapshotUpdateProgress()
	if snap.Running || snap.Phase != phaseFailed || updateActive.Load() {
		t.Fatalf("非法版本拒绝后应清零 Running，got snap=%+v active=%v", snap, updateActive.Load())
	}
}
