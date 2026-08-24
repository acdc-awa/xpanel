package services

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// 站点设置键（2026-08-14 批7 设置域，17 号 P0 ①——分组式站点设置；
// 2026-08-24 四端口拆分：web_base 与 subscribe_port 退役——端口全部走 env/配置，web_base 由域名分流取代）。
const (
	SettingAppName         = "app_name"          // 系统标题（注入 <title> + 订阅文件名）
	SettingAppDesc         = "app_description"   // 站点描述
	SettingLogo            = "logo"              // LOGO URL（注入 window.__PANEL_SETTINGS__）
	SettingFavicon         = "favicon"           // favicon URL（注入 <link rel="icon">）
	SettingSubscribeDomain = "subscribe_domain"  // 订阅域名（预留：多域名分发 P2）
	SettingSubscribeURL    = "subscribe_url"     // 订阅对外根 URL（如 https://sub.example.com）
	SettingSubscribePath   = "subscribe_path"    // 订阅路径前缀（如 /sub，/ehisnodn）
	SettingSubDenyCode     = "sub_deny_code"     // 订阅端口非订阅路径/无效 token 的统一错误码（404|401，空=404）
	SettingSubCleanUA      = "sub_clean_ua"      // 订阅爬虫清洗（1=开启，阻断 curl/python/空UA）
	SettingSubStrictUA     = "sub_strict_ua"     // 严格客户端模式（1=仅放行知名代理客户端）
	SettingSubBlockedUA    = "sub_blocked_ua"    // 自定义封禁 UA 关键词（逗号分隔）
	SettingTOSURL          = "tos_url"           // 服务条款 URL
	SettingStopRegister    = "stop_register"     // 关闭注册（1=关闭，注册接口拒绝）
	SettingCurrency        = "currency"          // 货币代码（CNY/USD）
	SettingCurrencySymbol  = "currency_symbol"   // 货币符号（¥/$）
)

// SiteKeys 站点分组全部键（设置页「站点」tab；SetSiteGroup 白名单）。
var SiteKeys = []string{
	SettingAppName, SettingAppDesc, SettingLogo, SettingFavicon,
	SettingSubscribeDomain, SettingSubscribeURL, SettingSubscribePath, SettingSubDenyCode,
	SettingSubCleanUA, SettingSubStrictUA, SettingSubBlockedUA,
	SettingTOSURL, SettingStopRegister, SettingCurrency, SettingCurrencySymbol,
}

// SubDenyCode 订阅端口统一拒绝错误码：非订阅路径 / 无效 token 时按设置返回 404 或 401
// （防探测默认 404；401 用于希望客户端感知鉴权失败的部署）。
func SubDenyCode(db *gorm.DB) int {
	switch GetSetting(db, SettingSubDenyCode) {
	case "401":
		return 401
	default:
		return 404
	}
}

// SubscribePath 订阅端口入口路径（设置页 subscribe_path；空缺省 /sub）。
func SubscribePath(db *gorm.DB) string {
	p := strings.TrimSpace(GetSetting(db, SettingSubscribePath))
	if p == "" {
		return "/sub"
	}
	return p
}

// GetSetting 读取单个设置（DB 直读；不存在返回空串）。设置写入极少，无需缓存。
func GetSetting(db *gorm.DB, key string) string {
	var s models.Setting
	if err := db.Where("key = ?", key).First(&s).Error; err != nil {
		return ""
	}
	return s.Value
}

// SetSetting 保存单个设置（upsert）。
func SetSetting(db *gorm.DB, key, value string) error {
	var s models.Setting
	err := db.Where("key = ?", key).First(&s).Error
	if err == nil {
		return db.Model(&s).Update("value", value).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&models.Setting{Key: key, Value: value}).Error
	}
	return err
}

// StopRegister 注册是否关闭（stop_register=1）。
func StopRegister(db *gorm.DB) bool {
	return GetSetting(db, SettingStopRegister) == "1"
}

// SiteService 站点设置（管理端「设置」页）。四端口拆分后无 web_base——
// 面板/订阅域名分流由反代承担，程序内全部路径均为根路径语义。
type SiteService struct {
	DB *gorm.DB
}

// NewSiteService 构造站点设置服务。
func NewSiteService(db *gorm.DB) *SiteService {
	return &SiteService{DB: db}
}

// SiteGroup 读取站点分组全部键（DB 直读，改完立即生效）。
func (s *SiteService) SiteGroup() map[string]string {
	out := make(map[string]string, len(SiteKeys))
	for _, k := range SiteKeys {
		out[k] = GetSetting(s.DB, k)
	}
	return out
}

// SetSiteGroup 保存站点分组（整体覆盖；仅接受白名单键）。
func (s *SiteService) SetSiteGroup(vals map[string]string) error {
	for k := range vals {
		ok := false
		for _, known := range SiteKeys {
			if k == known {
				ok = true
				break
			}
		}
		if !ok {
			return errors.New("未知设置键: " + k)
		}
	}
	for k, v := range vals {
		switch k {
		case SettingStopRegister:
			if v != "" && v != "0" && v != "1" {
				return errors.New("关闭注册仅接受 1（关闭）/ 0（开放）")
			}
		case SettingCurrencySymbol:
			if len(v) > 8 {
				return errors.New("货币符号过长（最多 8 字符）")
			}
		case SettingAppName:
			if len(v) > 64 {
				return errors.New("站点名称过长（最多 64 字符）")
			}
		case SettingLogo, SettingFavicon:
			if strings.HasPrefix(v, "data:image/") {
				if len(v) > 2*1024*1024 {
					return errors.New("图片 Base64 数据过大（最大支持 2MB）")
				}
			} else if len(v) > 2048 {
				return errors.New("URL 过长（最多 2048 字符）")
			}
		case SettingSubscribeDomain, SettingSubscribeURL, SettingTOSURL:
			if len(v) > 1024 {
				return errors.New("URL 过长（最多 1024 字符）")
			}
		case SettingSubscribePath:
			v = strings.TrimSpace(v)
			if v != "" {
				if v == "/" || !strings.HasPrefix(v, "/") {
					return errors.New("订阅路径必须为以 / 开头的具体路径（例如 /sub 或 /ehisnodn，不能是根路径）")
				}
				if len(v) > 128 {
					return errors.New("订阅路径过长（最多 128 字符）")
				}
			}
		case SettingSubDenyCode:
			if v != "" && v != "404" && v != "401" {
				return errors.New("订阅拒绝错误码仅接受 404 或 401（留空=404）")
			}
		case SettingSubCleanUA, SettingSubStrictUA:
			if v != "" && v != "0" && v != "1" {
				return errors.New("开关仅接受 1（启用）/ 0（关闭）")
			}
		case SettingSubBlockedUA:
			if len(v) > 500 {
				return errors.New("封禁 UA 列表过长（最多 500 字符）")
			}
		}
	}
	for k, v := range vals {
		if err := SetSetting(s.DB, k, v); err != nil {
			return err
		}
	}
	return nil
}
