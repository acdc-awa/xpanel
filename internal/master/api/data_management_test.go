package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/models"
)

func newDataTestEnv(t *testing.T) (*gin.Engine, *Deps, time.Time) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(dir, "panel.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	// 测试进程 time.Local=UTC（见 TestMain），而按天口径默认取 business_timezone（Asia/Shanghai）：
	// 显式对齐为 UTC，避免 UTC 16:00–24:00 窗口内测试期望与 handler 结果错位一天。
	if err := db.Create(&models.Setting{Key: "business_timezone", Value: "UTC"}).Error; err != nil {
		t.Fatal(err)
	}
	deps := &Deps{
		DB:  db,
		Cfg: &config.Config{DB: config.DB{Driver: "sqlite", DSN: filepath.Join(dir, "panel.db")}},
	}
	r := gin.New()
	r.GET("/api/v1/admin/data/log-stats", deps.AdminLogStats)
	r.POST("/api/v1/admin/data/logs/cleanup", deps.AdminCleanupLogs)
	r.POST("/api/v1/admin/data/vacuum", deps.AdminVacuum)
	return r, deps, time.Now()
}

func seedLogRows(t *testing.T, db *gorm.DB, now time.Time) {
	t.Helper()
	// 用户：u1 计费周期 3 天前；u2 周期为零值（应被安全线推导忽略）
	if err := db.Create(&models.User{
		Username: "u1", Email: "u1@t.local", PasswordHash: "x", UUID: "u1-uuid", SubscribeToken: "tok-u1",
		TrafficCycleStart: now.AddDate(0, 0, -3),
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Create(&models.User{
		Username: "u2", Email: "u2@t.local", PasswordHash: "x", UUID: "u2-uuid", SubscribeToken: "tok-u2",
	}).Error; err != nil {
		t.Fatalf("seed user2: %v", err)
	}
	old := now.AddDate(0, 0, -10)
	mk := func(ps time.Time) {
		if err := db.Create(&models.TrafficLog{
			UserID: 1, InboundID: 1, UpBytes: 1, DownBytes: 1,
			PeriodStart: ps, PeriodEnd: ps.Add(time.Minute),
		}).Error; err != nil {
			t.Fatalf("seed traffic_log: %v", err)
		}
	}
	mk(old)
	mk(old.Add(time.Minute))
	mk(now.AddDate(0, 0, -2))
	if err := db.Create(&models.NodeReport{ServerID: 1, ReportedAt: old}).Error; err != nil {
		t.Fatalf("seed node_report: %v", err)
	}
	if err := db.Create(&models.NodeReport{ServerID: 1, ReportedAt: now.AddDate(0, 0, -1)}).Error; err != nil {
		t.Fatalf("seed node_report2: %v", err)
	}
	if err := db.Create(&models.AuditLog{OperatorType: "admin", Action: "t.old", CreatedAt: now.AddDate(0, 0, -40)}).Error; err != nil {
		t.Fatalf("seed audit: %v", err)
	}
	if err := db.Create(&models.AuditLog{OperatorType: "admin", Action: "t.new", CreatedAt: now.AddDate(0, 0, -1)}).Error; err != nil {
		t.Fatalf("seed audit2: %v", err)
	}
}

func postJSON(r *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b)))
	return w
}

type cleanupResp struct {
	Code int `json:"code"`
	Data struct {
		Deleted int64 `json:"deleted"`
	} `json:"data"`
}

func TestAdminLogStats(t *testing.T) {
	r, deps, now := newDataTestEnv(t)
	seedLogRows(t, deps.DB, now)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/data/log-stats?days=7", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Days []struct {
				Date        string `json:"date"`
				TrafficLogs int64  `json:"traffic_logs"`
				NodeReports int64  `json:"node_reports"`
				AuditLogs   int64  `json:"audit_logs"`
			} `json:"days"`
			Totals struct {
				TrafficLogs int64 `json:"traffic_logs"`
				NodeReports int64 `json:"node_reports"`
				AuditLogs   int64 `json:"audit_logs"`
			} `json:"totals"`
			TrafficMinDelete string            `json:"traffic_min_delete"`
			Retention        map[string]string `json:"retention"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data.Days) != 7 {
		t.Fatalf("days = %d, want 7", len(resp.Data.Days))
	}
	if resp.Data.Totals.TrafficLogs != 3 || resp.Data.Totals.NodeReports != 2 || resp.Data.Totals.AuditLogs != 2 {
		t.Fatalf("totals = %+v", resp.Data.Totals)
	}
	// 安全线：u1 周期 3 天前、u2 零值被忽略 → 取聚合窗口上界 date(now-7d)
	wantSafe := now.AddDate(0, 0, -7).Format("2006-01-02")
	if resp.Data.TrafficMinDelete != wantSafe {
		t.Fatalf("traffic_min_delete = %s, want %s", resp.Data.TrafficMinDelete, wantSafe)
	}
	if resp.Data.Retention["retention_traffic_days"] == "" {
		t.Fatalf("retention group 缺失: %+v", resp.Data.Retention)
	}
}

func TestAdminCleanupLogsAudit(t *testing.T) {
	r, deps, now := newDataTestEnv(t)
	seedLogRows(t, deps.DB, now)

	w := postJSON(r, "/api/v1/admin/data/logs/cleanup", map[string]string{
		"table": "audit_logs", "before": now.AddDate(0, 0, -30).Format("2006-01-02"),
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var resp cleanupResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Deleted != 1 {
		t.Fatalf("deleted = %d, want 1", resp.Data.Deleted)
	}
	var remain int64
	deps.DB.Model(&models.AuditLog{}).Count(&remain)
	if remain != 1 {
		t.Fatalf("剩余 audit_logs = %d, want 1", remain)
	}
}

func TestAdminCleanupLogsTrafficGuard(t *testing.T) {
	r, deps, now := newDataTestEnv(t)
	seedLogRows(t, deps.DB, now)
	safe := now.AddDate(0, 0, -7).Format("2006-01-02")

	// 窗口内（1 天前）→ 拒绝
	w := postJSON(r, "/api/v1/admin/data/logs/cleanup", map[string]string{
		"table": "traffic_logs", "before": now.AddDate(0, 0, -1).Format("2006-01-02"),
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("窗口内应拒绝: status = %d", w.Code)
	}
	// 安全线当天 → 允许，只删 10 天前的 2 条
	w = postJSON(r, "/api/v1/admin/data/logs/cleanup", map[string]string{
		"table": "traffic_logs", "before": safe,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("安全线上界应允许: status = %d body = %s", w.Code, w.Body.String())
	}
	var resp cleanupResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Deleted != 2 {
		t.Fatalf("deleted = %d, want 2", resp.Data.Deleted)
	}
	var remain int64
	deps.DB.Model(&models.TrafficLog{}).Count(&remain)
	if remain != 1 {
		t.Fatalf("剩余 traffic_logs = %d, want 1", remain)
	}
}

func TestAdminCleanupLogsValidation(t *testing.T) {
	r, deps, now := newDataTestEnv(t)
	seedLogRows(t, deps.DB, now)

	// 非法表名 / 未来日期 / 非法日期格式 → 均 400 且不动数据
	for _, body := range []map[string]string{
		{"table": "users", "before": now.Format("2006-01-02")},
		{"table": "audit_logs", "before": now.AddDate(0, 0, 1).Format("2006-01-02")},
		{"table": "audit_logs", "before": "not-a-date"},
	} {
		w := postJSON(r, "/api/v1/admin/data/logs/cleanup", body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%v: status = %d, want 400", body, w.Code)
		}
	}
	var total int64
	deps.DB.Model(&models.AuditLog{}).Count(&total)
	if total != 2 {
		t.Fatalf("校验失败不应删数据, audit_logs = %d", total)
	}
}

func TestAdminVacuum(t *testing.T) {
	r, deps, now := newDataTestEnv(t)
	seedLogRows(t, deps.DB, now)
	w := postJSON(r, "/api/v1/admin/data/vacuum", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Reclaimed    int64 `json:"reclaimed"`
			DbSizeAfter  int64 `json:"db_size_after"`
			WalSizeAfter int64 `json:"wal_size_after"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != 0 || resp.Data.DbSizeAfter <= 0 {
		t.Fatalf("resp = %+v", resp)
	}
}
