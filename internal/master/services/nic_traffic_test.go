package services

import (
	"testing"
	"time"

	"github.com/acdc-awa/xpanel/internal/models"
)

func TestBillingCycleMonths(t *testing.T) {
	cases := []struct {
		label string
		want  int
		ok    bool
	}{
		{"月付", 1, true},
		{"月", 1, true},
		{"每月", 1, true},
		{"monthly", 1, true},
		{"季付", 3, true},
		{"季度", 3, true},
		{"quarterly", 3, true},
		{"半年付", 6, true},
		{"6个月", 6, true},
		{"yearly", 12, true},
		{"年付", 12, true},
		{"两年付", 24, true},
		{"2 年", 24, true},
		{"三年付", 36, true},
		{"一次性", 0, false},
		{"永久", 0, false},
		{"lifetime", 0, false},
		{"", 0, false},
		{"随便写", 0, false},
	}
	for _, c := range cases {
		got, ok := BillingCycleMonths(c.label)
		if got != c.want || ok != c.ok {
			t.Errorf("BillingCycleMonths(%q) = (%d,%v), want (%d,%v)", c.label, got, ok, c.want, c.ok)
		}
	}
}

func TestResolveNICCycle(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, loc)
	date := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	check := func(name string, srv models.Server, wantStart, wantEnd time.Time, wantMode string) {
		t.Helper()
		c := ResolveNICCycle(&srv, now, loc)
		if !c.Start.Equal(wantStart) || !c.End.Equal(wantEnd) {
			t.Errorf("%s: cycle = [%s, %s), want [%s, %s)", name,
				c.Start.Format(time.RFC3339), c.End.Format(time.RFC3339),
				wantStart.Format(time.RFC3339), wantEnd.Format(time.RFC3339))
		}
		if c.Mode != wantMode {
			t.Errorf("%s: mode = %q, want %q", name, c.Mode, wantMode)
		}
	}

	// 自动：月付 + 到期日，当前在到期前一个周期内
	check("auto monthly", models.Server{BillingCycle: "月付", ExpireAt: "2026-11-25"},
		date(2026, 10, 25), date(2026, 11, 25), "billing")
	// 自动：季付但到期日已过 → 按同一节奏向前推到包含今天的窗口
	check("auto expired quarter", models.Server{BillingCycle: "季付", ExpireAt: "2026-10-18"},
		date(2026, 10, 18), date(2027, 1, 18), "billing")
	// 自动：预付了多期（到期日在半年后）→ 找到包含今天的那个月窗口
	check("auto prepaid", models.Server{BillingCycle: "月付", ExpireAt: "2027-05-20"},
		date(2026, 10, 20), date(2026, 11, 20), "billing")
	// 自动：年付（大于月付）→ 以到期日为锚点，按 12 个月一格
	check("auto yearly", models.Server{BillingCycle: "年付", ExpireAt: "2027-05-20"},
		date(2026, 5, 20), date(2027, 5, 20), "billing")
	// 续费语义 = 旧到期日 + 整周期：网格平移一格，当前窗口不应变化
	renewBefore := ResolveNICCycle(&models.Server{BillingCycle: "季付", ExpireAt: "2027-02-15"}, now, loc)
	renewAfter := ResolveNICCycle(&models.Server{BillingCycle: "季付", ExpireAt: "2027-05-15"}, now, loc)
	if !renewBefore.Start.Equal(renewAfter.Start) || !renewBefore.End.Equal(renewAfter.End) {
		t.Errorf("续费整周期后当前窗口应不变: before [%s, %s), after [%s, %s)",
			renewBefore.Start.Format("2006-01-02"), renewBefore.End.Format("2006-01-02"),
			renewAfter.Start.Format("2006-01-02"), renewAfter.End.Format("2006-01-02"))
	}
	// 自定义起点 + 计费周期长度（月付）
	check("custom anchored monthly", models.Server{BillingCycle: "月付", NICCycleStart: "2026-05-05"},
		date(2026, 11, 5), date(2026, 12, 5), "custom_months")
	// 自定义起点 + 季付
	check("custom anchored quarter", models.Server{BillingCycle: "季付", NICCycleStart: "2026-01-15"},
		date(2026, 10, 15), date(2027, 1, 15), "custom_months")
	// 自定义起点 + 固定天数
	check("custom days", models.Server{NICCycleStart: "2026-10-01", NICCycleDays: 30},
		date(2026, 10, 31), date(2026, 11, 30), "custom_days")
	// 月末夹取：1/31 + 1 月 = 2/28，3/1 落在 [2/28, 3/31)
	marNow := time.Date(2026, 3, 1, 12, 0, 0, 0, loc)
	clamped := ResolveNICCycle(&models.Server{BillingCycle: "月付", NICCycleStart: "2026-01-31"}, marNow, loc)
	if !clamped.Start.Equal(date(2026, 2, 28)) || !clamped.End.Equal(date(2026, 3, 31)) {
		t.Errorf("month end clamp: cycle = [%s, %s), want [2026-02-28, 2026-03-31)",
			clamped.Start.Format("2006-01-02"), clamped.End.Format("2006-01-02"))
	}
	// 自动兜底：无计费周期
	check("natural month", models.Server{},
		date(2026, 11, 1), date(2026, 12, 1), "natural_month")
	// 自动兜底：一次性
	check("one time", models.Server{BillingCycle: "一次性", ExpireAt: "2026-11-20"},
		date(2026, 11, 1), date(2026, 12, 1), "natural_month")
	// 自动兜底：计费周期为空但填了到期日
	check("no billing cycle", models.Server{ExpireAt: "2026-11-20"},
		date(2026, 11, 1), date(2026, 12, 1), "natural_month")
	// 非法的固定天数组合（起点为空）回退自动，不 panic
	check("days without anchor", models.Server{BillingCycle: "月付", ExpireAt: "2026-11-25", NICCycleDays: 30},
		date(2026, 10, 25), date(2026, 11, 25), "billing")
}

func TestBackfillNICDaily(t *testing.T) {
	db := newSettingsTestDB(t, &models.Server{}, &models.NodeReport{}, &models.ServerNICTrafficDaily{})
	if err := SetSetting(db, SettingBusinessTimezone, "UTC"); err != nil {
		t.Fatal(err)
	}
	s1 := models.Server{Name: "s1", Host: "h1", NodeID: "n1", Secret: "x"}
	s2 := models.Server{Name: "s2", Host: "h2", NodeID: "n2", Secret: "x"}
	if err := db.Create(&s1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&s2).Error; err != nil {
		t.Fatal(err)
	}

	rows := []models.NodeReport{
		{ServerID: s1.ID, RxBytes: 1000, TxBytes: 500, ReportedAt: time.Date(2026, 11, 1, 23, 59, 0, 0, time.UTC)},
		{ServerID: s1.ID, RxBytes: 1500, TxBytes: 700, ReportedAt: time.Date(2026, 11, 2, 0, 1, 0, 0, time.UTC)},
		// 计数器归零（节点重启）：新值按从 0 起算
		{ServerID: s1.ID, RxBytes: 200, TxBytes: 100, ReportedAt: time.Date(2026, 11, 2, 0, 2, 0, 0, time.UTC)},
		// 0/0 = 该帧没上报网卡数据：跳过，不污染基线
		{ServerID: s1.ID, RxBytes: 0, TxBytes: 0, ReportedAt: time.Date(2026, 11, 2, 0, 3, 0, 0, time.UTC)},
		{ServerID: s1.ID, RxBytes: 400, TxBytes: 200, ReportedAt: time.Date(2026, 11, 2, 0, 4, 0, 0, time.UTC)},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := BackfillNICDaily(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var got []models.ServerNICTrafficDaily
	if err := db.Order("server_id ASC, date ASC").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("rows = %d, want 2: %+v", len(got), got)
	}
	if got[0].ServerID != s1.ID || got[0].Date != "2026-11-01" || got[0].RxBytes != 0 || got[0].TxBytes != 0 ||
		got[0].LastRawRx != 1000 || got[0].LastRawTx != 500 {
		t.Errorf("day1 = %+v", got[0])
	}
	if got[1].Date != "2026-11-02" || got[1].RxBytes != 900 || got[1].TxBytes != 400 ||
		got[1].LastRawRx != 400 || got[1].LastRawTx != 200 {
		t.Errorf("day2 = %+v", got[1])
	}

	if v := GetSetting(db, SettingNICDailyBackfilled); v == "" {
		t.Fatal("回填完成标记未写入")
	}

	// 幂等：再次调用不新增、不重复累加
	if err := BackfillNICDaily(db); err != nil {
		t.Fatalf("backfill second: %v", err)
	}
	var count int64
	if err := db.Model(&models.ServerNICTrafficDaily{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("second backfill changed rows: %d", count)
	}
}

// 日表已有数据但标记缺失（备份恢复/手工删标记）时不能覆盖已有数据。
func TestBackfillNICDailyDoesNotClobberExisting(t *testing.T) {
	db := newSettingsTestDB(t, &models.Server{}, &models.NodeReport{}, &models.ServerNICTrafficDaily{})
	if err := SetSetting(db, SettingBusinessTimezone, "UTC"); err != nil {
		t.Fatal(err)
	}
	srv := models.Server{Name: "s", Host: "h", NodeID: "n", Secret: "x"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ServerNICTrafficDaily{
		ServerID: srv.ID, Date: "2026-11-10", RxBytes: 123, TxBytes: 456, LastRawRx: 1, LastRawTx: 2,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := BackfillNICDaily(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	var row models.ServerNICTrafficDaily
	if err := db.Where("server_id = ? AND date = ?", srv.ID, "2026-11-10").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.RxBytes != 123 || row.TxBytes != 456 {
		t.Fatalf("已有数据被覆盖: %+v", row)
	}
	if GetSetting(db, SettingNICDailyBackfilled) == "" {
		t.Fatal("标记未补写")
	}
}
