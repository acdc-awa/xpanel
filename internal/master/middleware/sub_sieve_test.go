package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/master/services"
	"github.com/acdc/xray-panel/internal/models"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("failed to migrate db: %v", err)
	}
	return db
}

func TestSubSieveMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)

	// 开启智能清洗和自定义封禁
	_ = services.SetSetting(db, services.SettingSubCleanUA, "1")
	_ = services.SetSetting(db, services.SettingSubBlockedUA, "badbot,exploit")

	r := gin.New()
	r.Use(SubSieveMiddleware(db))
	r.GET("/sub/test", func(c *gin.Context) {
		c.String(http.StatusOK, "subscription content")
	})

	tests := []struct {
		name       string
		ua         string
		wantStatus int
	}{
		{name: "Empty UA blocked", ua: "", wantStatus: http.StatusForbidden},
		{name: "curl blocked", ua: "curl/7.88.1", wantStatus: http.StatusForbidden},
		{name: "python-requests blocked", ua: "python-requests/2.31.0", wantStatus: http.StatusForbidden},
		{name: "Go-http-client blocked", ua: "Go-http-client/1.1", wantStatus: http.StatusForbidden},
		{name: "Custom badbot blocked", ua: "Mozilla/5.0 (compatible; BadBot/1.0)", wantStatus: http.StatusForbidden},
		{name: "Clash client allowed", ua: "ClashforWindows/0.20.39", wantStatus: http.StatusOK},
		{name: "Mihomo client allowed", ua: "Mihomo/1.18.0", wantStatus: http.StatusOK},
		{name: "Shadowrocket allowed", ua: "Shadowrocket/2.2.35 (iOS 17.5.1)", wantStatus: http.StatusOK},
		{name: "Chrome browser allowed", ua: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/sub/test", nil)
			if tt.ua != "" {
				req.Header.Set("User-Agent", tt.ua)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Errorf("UA %q got status %d, want %d", tt.ua, w.Code, tt.wantStatus)
			}
		})
	}
}

func TestSubSieveStrictUAMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)

	// 开启严格客户端白名单模式
	_ = services.SetSetting(db, services.SettingSubCleanUA, "1")
	_ = services.SetSetting(db, services.SettingSubStrictUA, "1")

	r := gin.New()
	r.Use(SubSieveMiddleware(db))
	r.GET("/sub/test", func(c *gin.Context) {
		c.String(http.StatusOK, "subscription content")
	})

	tests := []struct {
		name       string
		ua         string
		wantStatus int
	}{
		{name: "Random unknown UA blocked in strict mode", ua: "SomeRandomScanner/1.0", wantStatus: http.StatusForbidden},
		{name: "sing-box allowed", ua: "sing-box/1.9.0-rc.1", wantStatus: http.StatusOK},
		{name: "v2rayN allowed", ua: "v2rayN/6.39", wantStatus: http.StatusOK},
		{name: "Stash allowed", ua: "Stash/2.5.0", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/sub/test", nil)
			req.Header.Set("User-Agent", tt.ua)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Errorf("Strict UA %q got status %d, want %d", tt.ua, w.Code, tt.wantStatus)
			}
		})
	}
}
