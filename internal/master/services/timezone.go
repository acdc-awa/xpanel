// 时区配置（设置页「时区」分组）。
//
// 两类语义分开：
//   - business_timezone：按天口径（仪表盘「今日/本月」、每日汇总 traffic_daily、清理/保留边界、
//     入站日/周/月重置）使用的时区。面板可能部署在任何地域，这些口径必须由业务时区决定，
//     而不是进程本地时区，否则不同机器上的「今日流量」会对不上。
//   - display_timezone：前端展示时区，browser 表示跟随浏览器；绝对时刻仍以带时区的 RFC3339 传输。
//
// 时间写入侧统一为 UTC（main 里 time.Local = time.UTC），本文件负责把按天口径换算到业务时区。
package services

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 设置键（settings 表 key）。
const (
	SettingBusinessTimezone = "business_timezone" // 按天口径时区（IANA 名，如 Asia/Shanghai）
	SettingDisplayTimezone  = "display_timezone"  // 前端展示时区：browser 或 IANA 名
)

const (
	// DefaultBusinessTimezone 未配置时的按天口径时区。
	DefaultBusinessTimezone = "Asia/Shanghai"
	// DisplayTimezoneBrowser 前端展示时区「跟随浏览器」的特殊值。
	DisplayTimezoneBrowser = "browser"
)

// BusinessLocation 返回按天口径所用时区；设置缺失/非法时回退默认（Asia/Shanghai，再退 UTC）。
func BusinessLocation(db *gorm.DB) *time.Location {
	name := strings.TrimSpace(GetSetting(db, SettingBusinessTimezone))
	if name == "" {
		name = DefaultBusinessTimezone
	}
	if loc, err := time.LoadLocation(name); err == nil {
		return loc
	}
	if loc, err := time.LoadLocation(DefaultBusinessTimezone); err == nil {
		return loc
	}
	return time.UTC
}

// DayStart 返回 t 在 loc 时区下当天的 00:00:00。
func DayStart(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// TimezoneSettingsGroup 读取「时区」分组（DB 直读；空值回显默认）。
func TimezoneSettingsGroup(db *gorm.DB) map[string]string {
	biz := strings.TrimSpace(GetSetting(db, SettingBusinessTimezone))
	if biz == "" {
		biz = DefaultBusinessTimezone
	}
	dis := strings.TrimSpace(GetSetting(db, SettingDisplayTimezone))
	if dis == "" {
		dis = DisplayTimezoneBrowser
	}
	return map[string]string{
		SettingBusinessTimezone: biz,
		SettingDisplayTimezone:  dis,
	}
}

// SaveTimezoneSettingsGroup 保存「时区」分组（仅接受白名单键，值须为合法 IANA 时区或 browser）。
func SaveTimezoneSettingsGroup(db *gorm.DB, vals map[string]string) error {
	allowed := map[string]bool{SettingBusinessTimezone: true, SettingDisplayTimezone: true}
	for k, v := range vals {
		if !allowed[k] {
			return errors.New("未知设置键: " + k)
		}
		v = strings.TrimSpace(v)
		if v == "" {
			return errors.New("时区不能为空")
		}
		if !(k == SettingDisplayTimezone && v == DisplayTimezoneBrowser) {
			if _, err := time.LoadLocation(v); err != nil {
				return errors.New("非法时区: " + v + "（需 IANA 名称，如 Asia/Shanghai）")
			}
		}
		if err := SetSetting(db, k, v); err != nil {
			return err
		}
	}
	return nil
}
