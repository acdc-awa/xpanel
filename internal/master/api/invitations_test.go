package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// TestAdminInvitationsOrder 邀请码列表默认排序：未使用且未过期在前
// （过期现场推导，不落库），其余随后，组内按 id 倒序。
func TestAdminInvitationsOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "panel.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}

	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(24 * time.Hour)
	// 依次创建（id 递增）：已使用 / 未使用已过期 / 已禁用 / 未使用未过期 / 未使用永久
	seeds := []struct {
		code    string
		status  int
		expires *time.Time
	}{
		{"INV-USED", models.InviteUsed, nil},
		{"INV-EXPIRED", models.InviteUnused, &past},
		{"INV-DISABLED", models.InviteDisabled, nil},
		{"INV-FUTURE", models.InviteUnused, &future},
		{"INV-PERMANENT", models.InviteUnused, nil},
	}
	for _, s := range seeds {
		if err := db.Create(&models.InvitationCode{Code: s.code, Status: s.status, ExpiresAt: s.expires}).Error; err != nil {
			t.Fatalf("seed %s: %v", s.code, err)
		}
	}

	deps := &Deps{DB: db}
	r := gin.New()
	r.GET("/api/v1/admin/invitations", deps.AdminInvitations)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/invitations", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp struct {
		Data struct {
			Items []struct {
				Code string `json:"code"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	want := []string{"INV-PERMANENT", "INV-FUTURE", "INV-DISABLED", "INV-EXPIRED", "INV-USED"}
	if len(resp.Data.Items) != len(want) {
		t.Fatalf("items = %d, want %d", len(resp.Data.Items), len(want))
	}
	for i, code := range want {
		if resp.Data.Items[i].Code != code {
			t.Fatalf("顺序不符: got[%d]=%s want[%d]=%s", i, resp.Data.Items[i].Code, i, code)
		}
	}
}
