package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

const notificationsTestAdminID = 7

func setupNotificationsTest(t *testing.T) (*gorm.DB, *Deps) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite memory db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	// 固定业务时区，让「今天」在测试里是确定的 UTC 日历日。
	if err := db.Create(&models.Setting{Key: services.SettingBusinessTimezone, Value: "UTC"}).Error; err != nil {
		t.Fatalf("seed timezone: %v", err)
	}
	d := &Deps{DB: db, Cfg: &config.Config{Update: config.Update{Enabled: true, Repo: "acdc-awa/xpanel"}}}
	return db, d
}

// setProbes 直接写入探测缓存（不触网），并在用例结束后清空。
func setProbes(t *testing.T, panel, agent releaseProbe) {
	t.Helper()
	reset := func() {
		releaseProbeMu.Lock()
		panelReleaseProbe = releaseProbe{}
		agentReleaseProbe = releaseProbe{}
		xrayReleaseProbe = releaseProbe{}
		releaseProbeMu.Unlock()
	}
	reset()
	t.Cleanup(reset)

	releaseProbeMu.Lock()
	panelReleaseProbe = panel
	agentReleaseProbe = agent
	releaseProbeMu.Unlock()
}

func setXrayProbe(t *testing.T, probe releaseProbe) {
	t.Helper()
	t.Cleanup(func() {
		releaseProbeMu.Lock()
		xrayReleaseProbe = releaseProbe{}
		releaseProbeMu.Unlock()
	})
	releaseProbeMu.Lock()
	xrayReleaseProbe = probe
	releaseProbeMu.Unlock()
}

func createServer(t *testing.T, db *gorm.DB, name, nodeID, expireAt, agentVersion string, status int) models.Server {
	t.Helper()
	srv := models.Server{
		Name:         name,
		Host:         name + ".example.com",
		NodeID:       nodeID,
		ExpireAt:     expireAt,
		AgentVersion: agentVersion,
		Status:       status,
	}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatalf("create server %s: %v", name, err)
	}
	return srv
}

func listNotifications(t *testing.T, d *Deps) []notificationItem {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/notifications", nil)
	c.Set(middleware.CtxClaimsKey, &contracts.JWTClaims{UserID: notificationsTestAdminID, Role: models.RoleAdmin})
	d.AdminListNotifications(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Items []notificationItem `json:"items"`
			Total int                `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Total != len(resp.Data.Items) {
		t.Fatalf("total = %d, items = %d", resp.Data.Total, len(resp.Data.Items))
	}
	return resp.Data.Items
}

func dismissNotification(t *testing.T, d *Deps, key string) bool {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/admin/notifications/"+key, nil)
	c.Params = gin.Params{{Key: "key", Value: key}}
	c.Set(middleware.CtxClaimsKey, &contracts.JWTClaims{UserID: notificationsTestAdminID, Role: models.RoleAdmin})
	d.AdminDismissNotification(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("dismiss status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Dismissed bool `json:"dismissed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return resp.Data.Dismissed
}

func keysOf(items []notificationItem) map[string]notificationItem {
	out := make(map[string]notificationItem, len(items))
	for _, it := range items {
		out[it.Key] = it
	}
	return out
}

// 四类提醒的派生 + 严重度排序。
func TestAdminListNotificationsDerivesItems(t *testing.T) {
	db, d := setupNotificationsTest(t)
	prevPanel := PanelVersion
	PanelVersion = "v0.1.0"
	t.Cleanup(func() { PanelVersion = prevPanel })

	today := time.Now().UTC()
	expired := today.AddDate(0, 0, -2).Format("2006-01-02")
	soon := today.AddDate(0, 0, 2).Format("2006-01-02")
	far := today.AddDate(0, 0, 20).Format("2006-01-02")

	createServer(t, db, "expired", "node-expired", expired, "", 1)
	createServer(t, db, "soon", "node-soon", soon, "", 1)
	createServer(t, db, "far", "node-far", far, "", 1)
	createServer(t, db, "outdated", "node-outdated", "", "v0.1.0", 1)
	createServer(t, db, "legacy", "node-legacy", "", "", 1)

	setProbes(t,
		releaseProbe{Version: "v0.1.1", Verified: true},
		releaseProbe{Version: "v0.2.0", Verified: true},
	)

	items := listNotifications(t, d)
	byKey := keysOf(items)

	if len(items) != 4 {
		t.Fatalf("got %d items, want 4: %+v", len(items), byKey)
	}
	if _, ok := byKey["server-expired:1"]; !ok {
		t.Fatalf("missing expired item: %+v", byKey)
	}
	if byKey["server-expired:1"].Severity != notifSeverityCritical {
		t.Fatalf("expired severity = %q", byKey["server-expired:1"].Severity)
	}
	if byKey["server-expiring:2"].Severity != notifSeverityHigh {
		t.Fatalf("soon severity = %q", byKey["server-expiring:2"].Severity)
	}
	if _, ok := byKey["agent-outdated:4"]; !ok {
		t.Fatalf("missing agent item: %+v", byKey)
	}
	if _, ok := byKey["panel-outdated"]; !ok {
		t.Fatalf("missing panel item: %+v", byKey)
	}
	// 排序：critical → high → medium → low
	wantOrder := []string{"server-expired:1", "server-expiring:2", "agent-outdated:4", "panel-outdated"}
	for i, want := range wantOrder {
		if items[i].Key != want {
			t.Fatalf("order[%d] = %q, want %q (all=%v)", i, items[i].Key, want, keysOf(items))
		}
	}
}

// 探测未通过资产校验时不能提示版本落后（release 资产可能还没上传完）。
func TestVersionItemsRequireVerifiedProbe(t *testing.T) {
	db, d := setupNotificationsTest(t)
	prevPanel := PanelVersion
	PanelVersion = "v0.1.0"
	t.Cleanup(func() { PanelVersion = prevPanel })

	createServer(t, db, "outdated", "node-outdated", "", "v0.1.0", 1)
	setProbes(t,
		releaseProbe{Version: "v0.1.1", Verified: false},
		releaseProbe{Version: "v0.2.0", Verified: false},
	)

	if items := listNotifications(t, d); len(items) != 0 {
		t.Fatalf("got %d items, want 0: %+v", len(items), keysOf(items))
	}
}

// 忽略语义：条件不变不再出现；条件升级重新出现；条件解除时忽略记录被清理。
func TestDismissReappearsWhenConditionChanges(t *testing.T) {
	db, d := setupNotificationsTest(t)
	setProbes(t, releaseProbe{}, releaseProbe{})

	today := time.Now().UTC()
	soon := today.AddDate(0, 0, 5).Format("2006-01-02")
	srv := createServer(t, db, "soon", "node-soon", soon, "", 1)
	key := "server-expiring:" + strconv.FormatUint(srv.ID, 10)

	if !dismissNotification(t, d, key) {
		t.Fatalf("dismiss should report dismissed=true for an active item")
	}
	if items := listNotifications(t, d); len(items) != 0 {
		t.Fatalf("dismissed item should be hidden, got %+v", keysOf(items))
	}

	// 条件升级：5 天 → 1 天，指纹变化，提醒重新出现。
	tighter := today.AddDate(0, 0, 1).Format("2006-01-02")
	if err := db.Model(&models.Server{}).Where("id = ?", srv.ID).Update("expire_at", tighter).Error; err != nil {
		t.Fatalf("update expire_at: %v", err)
	}
	items := listNotifications(t, d)
	if len(items) != 1 || items[0].Key != key {
		t.Fatalf("escalated condition should reappear, got %+v", keysOf(items))
	}

	// 条件解除：到期日清空后条目消失，忽略接口返回 dismissed=false 并清理记录。
	if err := db.Model(&models.Server{}).Where("id = ?", srv.ID).Update("expire_at", "").Error; err != nil {
		t.Fatalf("clear expire_at: %v", err)
	}
	if dismissNotification(t, d, key) {
		t.Fatalf("dismiss on a cleared condition should report dismissed=false")
	}
	var count int64
	if err := db.Model(&models.NotificationDismissal{}).Where("key = ?", key).Count(&count).Error; err != nil {
		t.Fatalf("count dismissals: %v", err)
	}
	if count != 0 {
		t.Fatalf("cleared condition should drop its dismissal, got %d rows", count)
	}
}

// Xray 内核落后聚合成一条；空版本不参与，全部最新时不产出。
func TestXrayVersionItemAggregates(t *testing.T) {
	db, d := setupNotificationsTest(t)
	setProbes(t, releaseProbe{}, releaseProbe{})
	setXrayProbe(t, releaseProbe{Version: "v26.11.5", Verified: true})

	a := createServer(t, db, "a", "node-a", "", "", 1)
	b := createServer(t, db, "b", "node-b", "", "", 1)
	c := createServer(t, db, "c", "node-c", "", "", 1)
	createServer(t, db, "unknown", "node-unknown", "", "", 1)
	for id, version := range map[uint64]string{a.ID: "26.10.29", b.ID: "26.10.29", c.ID: "26.11.5"} {
		if err := db.Model(&models.Server{}).Where("id = ?", id).Update("xray_version", version).Error; err != nil {
			t.Fatalf("update xray_version: %v", err)
		}
	}

	items := listNotifications(t, d)
	if len(items) != 1 || items[0].Key != "xray-outdated" {
		t.Fatalf("got %+v, want only xray-outdated", keysOf(items))
	}
	if items[0].Detail != "2 台服务器当前 26.10.29，最新 v26.11.5" {
		t.Fatalf("detail = %q", items[0].Detail)
	}

	// 剩下两台也升到最新后条目消失。
	if err := db.Model(&models.Server{}).Where("id IN ?", []uint64{a.ID, b.ID}).Update("xray_version", "26.11.5").Error; err != nil {
		t.Fatalf("update xray_version: %v", err)
	}
	if items := listNotifications(t, d); len(items) != 0 {
		t.Fatalf("got %+v, want none", keysOf(items))
	}
}

// 资产齐备判定：只有成对存在才算「这个版本可以下载」。
func TestReleaseAssetChecks(t *testing.T) {
	panelTarball := "xpanel-master-v1.0.0-linux-" + runtime.GOARCH + ".tar.gz"

	panelOK := map[string]bool{panelTarball: true, panelTarball + ".sha256": true}
	if !panelAssetsComplete("v1.0.0", panelOK) {
		t.Fatalf("panel assets should be complete: %v", panelOK)
	}
	if panelAssetsComplete("v1.0.0", map[string]bool{panelTarball: true}) {
		t.Fatalf("panel assets missing .sha256 should not be complete")
	}

	agentOK := map[string]bool{"checksums.txt": true, "xray-agent-linux-arm64": true}
	if !agentAssetsComplete(agentOK) {
		t.Fatalf("agent assets should be complete: %v", agentOK)
	}
	if agentAssetsComplete(map[string]bool{"xray-agent-linux-amd64": true}) {
		t.Fatalf("agent assets missing checksums.txt should not be complete")
	}

	xrayOK := map[string]bool{"Xray-linux-64.zip": true, "Xray-linux-64.zip.dgst": true}
	if !xrayAssetsComplete(xrayOK) {
		t.Fatalf("xray assets should be complete: %v", xrayOK)
	}
	if xrayAssetsComplete(map[string]bool{"Xray-linux-64.zip": true}) {
		t.Fatalf("xray assets missing .dgst should not be complete")
	}
	if xrayAssetsComplete(map[string]bool{"Xray-windows-64.zip": true, "Xray-windows-64.zip.dgst": true}) {
		t.Fatalf("xray requires a linux artifact")
	}
}

// 日历日差：不受时区/夏令时影响，过期日为负。
func TestDaysUntilCalendarDay(t *testing.T) {
	loc, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("load UTC: %v", err)
	}
	now := time.Date(2026, 11, 1, 23, 30, 0, 0, loc)

	cases := []struct {
		expire string
		want   int
		ok     bool
	}{
		{"2026-11-01", 0, true},
		{"2026-11-02", 1, true},
		{"2026-10-30", -2, true},
		{"", 0, false},
		{"2026-13-01", 0, false},
		{"garbage", 0, false},
	}
	for _, tc := range cases {
		got, ok := daysUntilCalendarDay(tc.expire, now)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Fatalf("daysUntilCalendarDay(%q) = (%d,%v), want (%d,%v)", tc.expire, got, ok, tc.want, tc.ok)
		}
	}
}
