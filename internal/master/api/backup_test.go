package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/zhx/xray-panel/internal/config"
	"github.com/zhx/xray-panel/internal/master/backup"
	"github.com/zhx/xray-panel/internal/master/services"
	"gorm.io/gorm"
)

func newBackupTestEnv(t *testing.T) (*gin.Engine, *backup.Service) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "panel.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	// Windows 下连接池未关闭会锁住 panel.db，导致 t.TempDir 清理失败
	t.Cleanup(func() { _ = sqlDB.Close() })
	audit := &services.AuditService{DB: db}
	svc, err := backup.New(filepath.Join(dir, "panel.db"), "sqlite", config.Backup{Enabled: false, Schedule: "0 3 * * *", Keep: 14, Dir: filepath.Join(dir, "backups")}, audit)
	if err != nil {
		t.Fatal(err)
	}
	deps := &Deps{DB: db, Backup: svc, Audit: audit}
	r := gin.New()
	// 默认按解码后路径路由：..%2f.. 会被解码为 /，穿越请求提前 404 而到不了 handler；
	// 按原始路径路由才能让穿越请求抵达 OpenFile 校验并返回 400。
	r.UseRawPath = true
	r.POST("/api/v1/admin/backup", deps.AdminCreateBackup)
	r.GET("/api/v1/admin/backup", deps.AdminListBackups)
	r.GET("/api/v1/admin/backup/:file", deps.AdminDownloadBackup)
	return r, svc
}

func TestAdminCreateBackup(t *testing.T) {
	r, svc := newBackupTestEnv(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backup", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			File string `json:"file"`
			Size int64  `json:"size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != 0 || resp.Data.File == "" {
		t.Fatalf("resp = %+v", resp)
	}
	items, _ := svc.List()
	if len(items) != 1 {
		t.Fatalf("备份数 = %d", len(items))
	}
}

func TestAdminListBackups(t *testing.T) {
	r, svc := newBackupTestEnv(t)
	if _, err := svc.Snapshot(); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backup", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Data struct {
			Items []struct {
				File string `json:"file"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data.Items) != 1 {
		t.Fatalf("items = %d", len(resp.Data.Items))
	}
}

func TestAdminDownloadBackup(t *testing.T) {
	r, svc := newBackupTestEnv(t)
	info, err := svc.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backup/"+info.File, nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Disposition"); !bytes.Contains([]byte(ct), []byte("attachment")) {
		t.Errorf("缺少 Content-Disposition: %q", ct)
	}
	if w.Body.Len() == 0 {
		t.Error("下载内容为空")
	}
}

func TestAdminDownloadBackupTraversal(t *testing.T) {
	r, _ := newBackupTestEnv(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/backup/..%2f..%2fpanel.db", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
