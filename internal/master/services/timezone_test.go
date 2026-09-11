package services

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func newSettingsTestDB(t *testing.T, modelsToMigrate ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(append([]any{&models.Setting{}}, modelsToMigrate...)...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// 时区分组：默认值回显、合法值保存、非法值拒绝、browser 特例放行。
func TestTimezoneSettingsGroup(t *testing.T) {
	db := newSettingsTestDB(t)

	got := TimezoneSettingsGroup(db)
	if got[SettingBusinessTimezone] != DefaultBusinessTimezone {
		t.Errorf("默认业务时区 = %q, want %q", got[SettingBusinessTimezone], DefaultBusinessTimezone)
	}
	if got[SettingDisplayTimezone] != DisplayTimezoneBrowser {
		t.Errorf("默认展示时区 = %q, want browser", got[SettingDisplayTimezone])
	}
	if loc := BusinessLocation(db); loc.String() != DefaultBusinessTimezone {
		t.Errorf("BusinessLocation = %q, want %q", loc, DefaultBusinessTimezone)
	}

	if err := SaveTimezoneSettingsGroup(db, map[string]string{
		SettingBusinessTimezone: "UTC",
		SettingDisplayTimezone:  DisplayTimezoneBrowser,
	}); err != nil {
		t.Fatalf("save valid: %v", err)
	}
	if loc := BusinessLocation(db); loc != time.UTC {
		t.Errorf("BusinessLocation = %q, want UTC", loc)
	}
	got = TimezoneSettingsGroup(db)
	if got[SettingBusinessTimezone] != "UTC" || got[SettingDisplayTimezone] != "browser" {
		t.Errorf("保存后回读 = %v", got)
	}

	if err := SaveTimezoneSettingsGroup(db, map[string]string{SettingBusinessTimezone: "Mars/Olympus"}); err == nil {
		t.Error("非法时区应被拒绝")
	}
	if err := SaveTimezoneSettingsGroup(db, map[string]string{"unknown_key": "UTC"}); err == nil {
		t.Error("未知设置键应被拒绝")
	}
}

// AggDaily 按业务时区切天：UTC 深夜的流量应归入次日的业务日（默认 Asia/Shanghai）。
func TestAggDailyUsesBusinessTimezone(t *testing.T) {
	db := newSettingsTestDB(t, &models.TrafficLog{}, &models.TrafficDaily{})
	now := time.Date(2026, 8, 16, 5, 0, 0, 0, time.UTC) // = 13:00 +08
	s := &TrafficService{DB: db, now: func() time.Time { return now }}

	// 2026-08-15T18:00Z = 2026-08-16 02:00 +08 → 业务日 2026-08-16（UTC 日则是 08-15）
	if err := db.Create(&models.TrafficLog{
		UserID: 1, InboundID: 1, UpBytes: 11,
		PeriodStart: time.Date(2026, 8, 15, 18, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatal(err)
	}

	s.AggDaily()

	var d models.TrafficDaily
	if err := db.Where("user_id = ?", 1).First(&d).Error; err != nil {
		t.Fatalf("traffic_daily 未生成: %v", err)
	}
	if d.Date != "2026-08-16" {
		t.Fatalf("业务日 = %q, want 2026-08-16（UTC 日应为 08-15，说明未按业务时区切天）", d.Date)
	}
	if d.UpBytes != 11 {
		t.Fatalf("up = %d, want 11", d.UpBytes)
	}
}

// DayStart 在业务时区下返回当天 00:00。
func TestDayStart(t *testing.T) {
	sh, _ := time.LoadLocation("Asia/Shanghai")
	at := time.Date(2026, 8, 15, 20, 30, 0, 0, time.UTC) // 2026-08-16 04:30 +08
	ds := DayStart(at, sh)
	if ds.Format("2006-01-02") != "2026-08-16" {
		t.Errorf("DayStart date = %s, want 2026-08-16", ds.Format("2006-01-02"))
	}
	if ds.Hour() != 0 || ds.Minute() != 0 {
		t.Errorf("DayStart 应为 00:00, got %s", ds)
	}
	if ds.Location() != sh {
		t.Errorf("DayStart location = %v, want %v", ds.Location(), sh)
	}
}

// 一次性回填：把历史上按 UTC 天写入的 traffic_daily 按业务时区重算，且幂等。
func TestBackfillDailyBusinessDateCorrectsUTCLabels(t *testing.T) {
	db := newSettingsTestDB(t, &models.TrafficLog{}, &models.TrafficDaily{})
	now := time.Date(2026, 8, 16, 5, 0, 0, 0, time.UTC) // = 13:00 +08

	// 明细 2026-08-15T18:00Z = 08-16 02:00 +08 → 业务日 08-16（UTC 日 08-15）
	if err := db.Create(&models.TrafficLog{
		UserID: 1, InboundID: 1, UpBytes: 11,
		PeriodStart: time.Date(2026, 8, 15, 18, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatal(err)
	}
	// 旧 UTC 标签行
	if err := db.Create(&models.TrafficDaily{UserID: 1, Date: "2026-08-15", UpBytes: 11}).Error; err != nil {
		t.Fatal(err)
	}

	done, err := BackfillDailyBusinessDate(db, now, 31)
	if err != nil || !done {
		t.Fatalf("backfill = (%v,%v), want (true,nil)", done, err)
	}
	var rows []models.TrafficDaily
	if err := db.Where("user_id = ?", 1).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Date != "2026-08-16" || rows[0].UpBytes != 11 {
		t.Fatalf("回填后行 = %+v, want 单行 date=2026-08-16 up=11", rows)
	}

	done2, err := BackfillDailyBusinessDate(db, now, 31)
	if err != nil {
		t.Fatal(err)
	}
	if done2 {
		t.Error("第二次调用应为 no-op（已落标记）")
	}
}
