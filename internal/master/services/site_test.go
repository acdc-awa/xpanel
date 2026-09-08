package services

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// newSiteTestDB 站点设置测试库（内存 sqlite，仅 Setting 表）。
func newSiteTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	return db
}

// TestSubProfileTitleFallback 订阅标题回退链：sub_profile_title → app_name → 空由调用方兜底。
func TestSubProfileTitleFallback(t *testing.T) {
	db := newSiteTestDB(t)

	// 全空 → 空（subscribe.go 兜底 xray）
	if got := SubProfileTitle(db); got != "" {
		t.Errorf("全空应为空串，得到 %q", got)
	}
	// 仅 app_name → 回退 app_name
	if err := SetSetting(db, SettingAppName, "站点名"); err != nil {
		t.Fatalf("写 app_name 失败: %v", err)
	}
	if got := SubProfileTitle(db); got != "站点名" {
		t.Errorf("应回退 app_name，得到 %q", got)
	}
	// sub_profile_title 优先于 app_name
	if err := SetSetting(db, SettingSubProfileTitle, "自定义订阅"); err != nil {
		t.Fatalf("写 sub_profile_title 失败: %v", err)
	}
	if got := SubProfileTitle(db); got != "自定义订阅" {
		t.Errorf("自定义标题应优先，得到 %q", got)
	}
	// 纯空白等价于未设置
	if err := SetSetting(db, SettingSubProfileTitle, "   "); err != nil {
		t.Fatalf("覆写 sub_profile_title 失败: %v", err)
	}
	if got := SubProfileTitle(db); got != "站点名" {
		t.Errorf("空白标题应回退 app_name，得到 %q", got)
	}
}

// TestSubUpdateIntervalHours 更新间隔：缺省 24，clamp 1–168，非法串回退默认。
func TestSubUpdateIntervalHours(t *testing.T) {
	db := newSiteTestDB(t)

	cases := []struct {
		raw  string
		want int
	}{
		{"", 24},      // 未设置 → 默认
		{"48", 48},    // 正常
		{"0", 24},     // 非正 → 默认（写入端已拦，读取端防御）
		{"-5", 24},    // 负数 → 默认
		{"abc", 24},   // 非法 → 默认
		{"1", 1},      // 下界
		{"168", 168},  // 上界
		{"999", 168},  // 超上界 → clamp
	}
	for _, c := range cases {
		if err := SetSetting(db, SettingSubUpdateInterval, c.raw); err != nil {
			t.Fatalf("写 %q 失败: %v", c.raw, err)
		}
		if got := SubUpdateIntervalHours(db); got != c.want {
			t.Errorf("SubUpdateIntervalHours(%q) = %d, want %d", c.raw, got, c.want)
		}
	}
}

// TestSetSiteGroupSubFields 设置页新键校验：标题长度上限、间隔值域。
func TestSetSiteGroupSubFields(t *testing.T) {
	db := newSiteTestDB(t)
	svc := NewSiteService(db)

	if err := svc.SetSiteGroup(map[string]string{
		SettingSubProfileTitle:   "我的订阅",
		SettingSubUpdateInterval: "12",
	}); err != nil {
		t.Fatalf("合法值被拒: %v", err)
	}
	if got := GetSetting(db, SettingSubUpdateInterval); got != "12" {
		t.Errorf("间隔应保存原值，得到 %q", got)
	}

	err := svc.SetSiteGroup(map[string]string{SettingSubUpdateInterval: "0"})
	if err == nil {
		t.Error("间隔 0 应被拒绝")
	}
	err = svc.SetSiteGroup(map[string]string{SettingSubUpdateInterval: "200"})
	if err == nil {
		t.Error("间隔 200 超上界应被拒绝")
	}
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	err = svc.SetSiteGroup(map[string]string{SettingSubProfileTitle: string(long)})
	if err == nil {
		t.Error("标题超 64 字符应被拒绝")
	}
	// 未知键仍拒绝（白名单收口不被新键破坏）
	if err := svc.SetSiteGroup(map[string]string{"sub_unknown": "x"}); err == nil {
		t.Error("未知键应被拒绝")
	}
}
