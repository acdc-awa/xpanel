package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

func setupResetTrafficTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, models.AutoMigrate(db))
	return db
}

// usedBytes 读生产口径的已用量（与用户列表/用户主页/订阅同源）。
func usedBytes(t *testing.T, db *gorm.DB, userID uint64) int64 {
	t.Helper()
	up, down, err := (&services.TrafficService{DB: db}).UserBilled(userID)
	require.NoError(t, err)
	return up + down
}

// 重置流量周期（J12）：起点对齐整点并清零该整点桶的计费字节。
//
// 只改起点不清零 → 重置后已用量不为 0（前半小时的字节仍落在新起点之后的桶里）。
// 只清零不对齐 → 该桶起点早于新的非整点起点，整桶继续被计费口径排除，重置后怎么用都是 0。
func TestAdminResetUserTrafficAlignsCycleAndClearsUsage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupResetTrafficTestDB(t)

	bucket := models.TrafficCycleAlign(time.Now())
	if !models.TrafficCycleAlign(time.Now()).Equal(bucket) {
		t.Skip("刚好跨小时边界，跳过")
	}

	// 存量形态：周期起点不在整点（旧版写入），当前小时桶内已累计计费字节
	user := models.User{
		Username: "reset@panel.local", Email: "reset@panel.local",
		UUID: "uuid-reset", PasswordHash: "x", Status: models.StatusActive,
		TrafficCycleStart: bucket.Add(-30 * time.Minute),
	}
	require.NoError(t, db.Create(&user).Error)

	boundary := models.TrafficLog{
		UserID: user.ID, InboundID: 12,
		UpBytes: 5_000_000, DownBytes: 9_000_000,
		BilledUp: 50_000, BilledDown: 90_000,
		PeriodStart: bucket,
	}
	previous := models.TrafficLog{
		UserID: user.ID, InboundID: 12,
		UpBytes: 1_000, DownBytes: 2_000,
		BilledUp: 1_000, BilledDown: 2_000,
		PeriodStart: bucket.Add(-time.Hour),
	}
	require.NoError(t, db.Create(&boundary).Error)
	require.NoError(t, db.Create(&previous).Error)

	if got := usedBytes(t, db, user.ID); got != 140_000 {
		t.Fatalf("重置前已用量 = %d, want 140000（边界桶计入）", got)
	}

	deps := &Deps{DB: db}
	r := gin.New()
	r.POST("/api/v1/admin/users/:id/reset-traffic", deps.AdminResetUserTraffic)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/"+strconv.FormatUint(user.ID, 10)+"/reset-traffic", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var got models.User
	require.NoError(t, db.First(&got, user.ID).Error)
	if !got.TrafficCycleStart.Equal(bucket) {
		t.Fatalf("周期起点 = %v, want 对齐后的 %v", got.TrafficCycleStart, bucket)
	}
	if used := usedBytes(t, db, user.ID); used != 0 {
		t.Fatalf("重置后已用量 = %d, want 0", used)
	}

	// 清零范围只限边界桶：更早小时的行不动（其计费字节不属于本周期，但保留原值供统计/回溯）
	var b, p models.TrafficLog
	require.NoError(t, db.First(&b, boundary.ID).Error)
	require.NoError(t, db.First(&p, previous.ID).Error)
	if b.BilledUp != 0 || b.BilledDown != 0 {
		t.Fatalf("边界桶计费字节未清零: %d/%d", b.BilledUp, b.BilledDown)
	}
	if p.BilledUp != previous.BilledUp || p.BilledDown != previous.BilledDown {
		t.Fatalf("边界桶之外的行被误改: %d/%d, want %d/%d", p.BilledUp, p.BilledDown, previous.BilledUp, previous.BilledDown)
	}
	// 原始字节列一律不动：仪表盘/每日汇总口径不受周期切换影响
	if b.UpBytes != boundary.UpBytes || b.DownBytes != boundary.DownBytes {
		t.Fatalf("原始字节被改动: %d/%d, want %d/%d", b.UpBytes, b.DownBytes, boundary.UpBytes, boundary.DownBytes)
	}

	// 重置后同一小时内的上报继续累加进同一行 → 计入新周期（对齐不会导致这一小时白干）
	require.NoError(t, db.Model(&models.TrafficLog{}).Where("id = ?", boundary.ID).
		Updates(map[string]any{"billed_up": 1_000, "billed_down": 2_000}).Error)
	if used := usedBytes(t, db, user.ID); used != 3_000 {
		t.Fatalf("重置后新增用量 = %d, want 3000", used)
	}
}
