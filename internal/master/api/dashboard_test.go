package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/models"
)

// TestAdminDashboardNoDoubleCountToday ISSUE-06：traffic_dailies 已含今日，traffic_logs 不得再叠加。
func TestAdminDashboardNoDoubleCountToday(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	periodStart := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location())

	if err := db.Create(&models.TrafficDaily{UserID: 1, Date: today, UpBytes: 1000, DownBytes: 0}).Error; err != nil {
		t.Fatalf("create today daily: %v", err)
	}
	if err := db.Create(&models.TrafficLog{UserID: 1, InboundID: 1, UpBytes: 100, DownBytes: 0, PeriodStart: periodStart, PeriodEnd: now}).Error; err != nil {
		t.Fatalf("create today log: %v", err)
	}
	// 同月昨日流量：月合计应 = 昨日 + 今日（不含今日 log 重复叠加）
	if err := db.Create(&models.TrafficDaily{UserID: 1, Date: yesterday, UpBytes: 500, DownBytes: 0}).Error; err != nil {
		t.Fatalf("create yesterday daily: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard", nil)
	d := &Deps{DB: db}
	d.AdminDashboard(c)

	if w.Code != http.StatusOK {
		t.Fatalf("dashboard = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data DashboardData `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	s := resp.Data.Summary
	if s.TodayTrafficUp != 1000 {
		t.Fatalf("today up = %d, want 1000（daily 与 log 不得叠加）", s.TodayTrafficUp)
	}
	if s.MonthTrafficTotal != 1500 {
		t.Fatalf("month total = %d, want 1500（500 + 1000，不含重复今日 log）", s.MonthTrafficTotal)
	}
	for _, p := range resp.Data.TrafficTrend {
		if p.Date == today && p.UpBytes != 1000 {
			t.Fatalf("trend today up = %d, want 1000", p.UpBytes)
		}
	}
}