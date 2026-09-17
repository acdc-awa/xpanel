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

	"github.com/acdc-awa/xpanel/internal/models"
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
	// 测试进程 time.Local=UTC（见 TestMain），按天口径默认取 business_timezone（Asia/Shanghai）；
	// 显式对齐为 UTC，避免 UTC 16:00–24:00 窗口内「今日」日期错位一天。
	if err := db.Create(&models.Setting{Key: "business_timezone", Value: "UTC"}).Error; err != nil {
		t.Fatalf("seed timezone: %v", err)
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
	// 月合计期望按日历月计算：月初 1 号时"昨日"属上月，不计入本月（500 只在同时月时计入）
	wantMonth := int64(1000)
	if yesterday[:7] == today[:7] {
		wantMonth += 500
	}
	if s.MonthTrafficTotal != wantMonth {
		t.Fatalf("month total = %d, want %d（昨日同月时 500 + 1000，不含重复今日 log）", s.MonthTrafficTotal, wantMonth)
	}
	for _, p := range resp.Data.TrafficTrend {
		if p.Date == today && p.UpBytes != 1000 {
			t.Fatalf("trend today up = %d, want 1000", p.UpBytes)
		}
	}
}

// dashReq 以指定查询串调用一次仪表盘，返回解析后的响应。
func dashReq(t *testing.T, db *gorm.DB, query string) DashboardData {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/dashboard"+query, nil)
	d := &Deps{DB: db}
	d.AdminDashboard(c)
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard%s = %d, want 200, body=%s", query, w.Code, w.Body.String())
	}
	var resp struct {
		Data DashboardData `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return resp.Data
}

// TestAdminDashboardRankWindow 时间口径排行榜与节点流量分布（2026-09-17）：
// rank_period 三档（today/7d/month）驱动 user_rank 与 server_rank/server_breakdown 同源切换；
// 节点流量按 traffic_logs.inbound_id → inbounds.server_id 归集，而非入站累计计数器。
func TestAdminDashboardRankWindow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&models.Setting{Key: "business_timezone", Value: "UTC"}).Error; err != nil {
		t.Fatalf("seed timezone: %v", err)
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	older := now.AddDate(0, 0, -2)
	olderDate := older.Format("2006-01-02")

	// 两台服务器、各一个入站；用户 1/2 分别落在 s1/s2 的入站上
	s1 := models.Server{Name: "节点一", Host: "1.1.1.1", NodeID: "n1", Secret: "s", Status: 1}
	s2 := models.Server{Name: "节点二", Host: "2.2.2.2", NodeID: "n2", Secret: "s", Status: 1}
	for _, s := range []*models.Server{&s1, &s2} {
		if err := db.Create(s).Error; err != nil {
			t.Fatalf("create server: %v", err)
		}
	}
	inb1 := models.Inbound{ServerID: s1.ID, Tag: "in-1", Protocol: "vless", Port: 443}
	inb2 := models.Inbound{ServerID: s2.ID, Tag: "in-2", Protocol: "vless", Port: 443}
	for _, i := range []*models.Inbound{&inb1, &inb2} {
		if err := db.Create(i).Error; err != nil {
			t.Fatalf("create inbound: %v", err)
		}
	}

	u1 := models.User{Username: "u1", Email: "u1@x.com", UUID: "uuid-1", PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "t1"}
	u2 := models.User{Username: "u2", Email: "u2@x.com", UUID: "uuid-2", PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "t2"}
	for _, u := range []*models.User{&u1, &u2} {
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}

	logAt := func(uid, iid uint64, up int64, at time.Time) {
		if err := db.Create(&models.TrafficLog{
			UserID: uid, InboundID: iid, UpBytes: up,
			PeriodStart: at, PeriodEnd: at.Add(time.Hour),
		}).Error; err != nil {
			t.Fatalf("create log: %v", err)
		}
	}
	dayStart := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 8, 0, 0, 0, t.Location())
	}
	// 今日：u1 走 s1 的入站 100；u2 走 s2 的入站 300
	logAt(u1.ID, inb1.ID, 100, dayStart(now))
	logAt(u2.ID, inb2.ID, 300, dayStart(now))
	// 2 天前：u1 走 s1 的入站 1000（7d 覆盖、today 不覆盖）
	logAt(u1.ID, inb1.ID, 1000, dayStart(older))

	// 每日汇总与明细同值（避免 max 修正干扰窗口断言）
	for _, d := range []struct {
		uid  uint64
		date string
		up   int64
	}{{u1.ID, today, 100}, {u2.ID, today, 300}, {u1.ID, olderDate, 1000}} {
		if err := db.Create(&models.TrafficDaily{UserID: d.uid, Date: d.date, UpBytes: d.up}).Error; err != nil {
			t.Fatalf("create daily: %v", err)
		}
	}

	// ---- 今日：只看今日流量，u2(300) 领先 u1(100) ----
	todayData := dashReq(t, db, "?rank_period=today")
	if todayData.RankPeriod != "today" || todayData.RankLabel != "今日" {
		t.Fatalf("today 口径回显错误: period=%q label=%q", todayData.RankPeriod, todayData.RankLabel)
	}
	if len(todayData.UserRank) != 2 || todayData.UserRank[0].Username != "u2" || todayData.UserRank[0].TotalBytes != 300 {
		t.Fatalf("today 用户排行错误: %+v", todayData.UserRank)
	}
	if todayData.UserRank[1].Username != "u1" || todayData.UserRank[1].TotalBytes != 100 {
		t.Fatalf("today 用户排行第二位错误: %+v", todayData.UserRank[1])
	}
	// 节点榜：s2=300 > s1=100
	if len(todayData.ServerRank) != 2 || todayData.ServerRank[0].Name != "节点二" || todayData.ServerRank[0].TotalBytes != 300 {
		t.Fatalf("today 节点排行错误: %+v", todayData.ServerRank)
	}
	if todayData.ServerRank[1].Name != "节点一" || todayData.ServerRank[1].TotalBytes != 100 {
		t.Fatalf("today 节点排行第二位错误: %+v", todayData.ServerRank[1])
	}
	// 分布与排行同源：s1=100/(100+300)=25%
	var s1Pct float64
	for _, b := range todayData.ServerBreakdown {
		if b.ServerID == s1.ID {
			s1Pct = b.Percent
		}
	}
	if s1Pct < 24.9 || s1Pct > 25.1 {
		t.Fatalf("today 分布占比错误: s1=%.2f%% want 25%%", s1Pct)
	}

	// ---- 近 7 天：纳入 2 天前的 1000，u1(1100) 反超 u2(300) ----
	weekData := dashReq(t, db, "?rank_period=7d")
	if weekData.RankPeriod != "7d" || weekData.RankLabel != "近 7 天" {
		t.Fatalf("7d 口径回显错误: period=%q label=%q", weekData.RankPeriod, weekData.RankLabel)
	}
	if len(weekData.UserRank) != 2 || weekData.UserRank[0].Username != "u1" || weekData.UserRank[0].TotalBytes != 1100 {
		t.Fatalf("7d 用户排行错误: %+v", weekData.UserRank)
	}
	if len(weekData.ServerRank) == 0 || weekData.ServerRank[0].Name != "节点一" || weekData.ServerRank[0].TotalBytes != 1100 {
		t.Fatalf("7d 节点排行错误: %+v", weekData.ServerRank)
	}

	// ---- 本月：2 天前是否同月决定 u1 是否为 1100 ----
	monthData := dashReq(t, db, "?rank_period=month")
	if monthData.RankPeriod != "month" || monthData.RankLabel != "本月" {
		t.Fatalf("month 口径回显错误: period=%q label=%q", monthData.RankPeriod, monthData.RankLabel)
	}
	wantU1 := int64(100)
	if olderDate[:7] == today[:7] {
		wantU1 = 1100
	}
	var gotU1 int64
	for _, r := range monthData.UserRank {
		if r.Username == "u1" {
			gotU1 = r.TotalBytes
		}
	}
	if gotU1 != wantU1 {
		t.Fatalf("month 用户 u1 用量 = %d, want %d（跨月时不含 2 天前）", gotU1, wantU1)
	}

	// 缺省/非法参数回退今日，且不得 500
	defData := dashReq(t, db, "")
	if defData.RankPeriod != "today" {
		t.Fatalf("缺省口径应为 today，实际 %q", defData.RankPeriod)
	}
	if bad := dashReq(t, db, "?rank_period=bogus"); bad.RankPeriod != "today" {
		t.Fatalf("非法口径应回退 today，实际 %q", bad.RankPeriod)
	}
}

// TestAdminDashboardRankTodayUsesMaxNotSum 今日修正取 max(daily, logs) 而非相加：
// traffic_dailies 每 5 分钟聚合，最近窗口尚未落盘时 logs 领先——
// 榜单须取两者较大值，否则今日用量被双计。
func TestAdminDashboardRankTodayUsesMaxNotSum(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&models.Setting{Key: "business_timezone", Value: "UTC"}).Error; err != nil {
		t.Fatalf("seed timezone: %v", err)
	}
	now := time.Now()
	today := now.Format("2006-01-02")
	u := models.User{Username: "solo", Email: "solo@x.com", UUID: "uuid-s", PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive, SubscribeToken: "ts"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	// 汇总落后于明细：daily=100（上一聚合周期），logs=250（已累计到当前）
	if err := db.Create(&models.TrafficDaily{UserID: u.ID, Date: today, UpBytes: 100}).Error; err != nil {
		t.Fatalf("create daily: %v", err)
	}
	if err := db.Create(&models.TrafficLog{
		UserID: u.ID, InboundID: 0, UpBytes: 250,
		PeriodStart: time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location()),
	}).Error; err != nil {
		t.Fatalf("create log: %v", err)
	}

	data := dashReq(t, db, "?rank_period=today")
	if len(data.UserRank) != 1 {
		t.Fatalf("应恰好 1 条排行，实际 %+v", data.UserRank)
	}
	if got := data.UserRank[0].TotalBytes; got != 250 {
		t.Fatalf("今日用量应取 max(daily=100, logs=250)=250，实际 %d（350=双计）", got)
	}
}
