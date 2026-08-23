package services

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"gorm.io/gorm"

	"github.com/acdc/xray-panel/internal/config"
	"github.com/acdc/xray-panel/internal/models"
)

// 站点设置键（2026-08-14 批7 设置域，17 号 P0 ①——分组式站点设置）。
// web_base 为特殊键（有独立校验），其余归入 site 分组由设置页统一管理。
const (
	SettingWebBase         = "web_base"
	SettingAppName         = "app_name"          // 系统标题（注入 <title> + 订阅文件名）
	SettingAppDesc         = "app_description"   // 站点描述
	SettingLogo            = "logo"              // LOGO URL（注入 window.__PANEL_SETTINGS__）
	SettingFavicon         = "favicon"           // favicon URL（注入 <link rel="icon">）
	SettingSubscribeDomain = "subscribe_domain"  // 订阅域名（预留：多域名分发 P2）
	SettingSubscribeURL    = "subscribe_url"     // 订阅访问根 URL（如 https://sub.example.com）
	SettingSubscribePath   = "subscribe_path"    // 订阅路径前缀（如 /sub，/link）
	SettingSubscribePort   = "subscribe_port"    // 独立订阅监听端口（如 5000，0/空为禁用）
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
	SettingSubscribeDomain, SettingSubscribeURL, SettingSubscribePath, SettingSubscribePort,
	SettingSubCleanUA, SettingSubStrictUA, SettingSubBlockedUA,
	SettingTOSURL, SettingStopRegister, SettingCurrency, SettingCurrencySymbol,
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

var webBaseRe = regexp.MustCompile(`^/[a-zA-Z0-9/_-]*$`)

// SiteService 站点设置（管理端「设置」页）。当前支持 web_base：
// 空 = 根路径；/panel 等 = 面板挂在 /panel/... 下。DB 值优先，config.yaml 为兜底默认。
type SiteService struct {
	DB         *gorm.DB
	cfgDefault string // config.yaml app.web_base（兜底）
	mu         sync.RWMutex
	webBase    string // 规范化后的当前值（如 /panel；空=根路径）
}

// NewSiteService 构造并加载当前 web base（DB 优先，config 兜底）。
func NewSiteService(db *gorm.DB, cfg *config.Config) *SiteService {
	s := &SiteService{DB: db, cfgDefault: normalizeWebBase(cfg.App.WebBase)}
	s.reload()
	return s
}

// normalizeWebBase 规范化：确保以 / 开头、去尾斜杠；空或 / 视为根路径（返回空串）。
func normalizeWebBase(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "/" {
		return ""
	}
	if !strings.HasPrefix(v, "/") {
		v = "/" + v
	}
	return strings.TrimRight(v, "/")
}

func (s *SiteService) reload() {
	base := s.cfgDefault
	var set models.Setting
	if err := s.DB.Where("key = ?", SettingWebBase).First(&set).Error; err == nil {
		// DB 行存在时以其为准（显式清空=根路径，覆盖 config）
		base = normalizeWebBase(set.Value)
	}
	s.mu.Lock()
	s.webBase = base
	s.mu.Unlock()
}

// WebBase 返回当前 web base（如 /panel；空=根路径）。
func (s *SiteService) WebBase() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.webBase
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
			if v != "" && !strings.HasPrefix(v, "/") {
				return errors.New("订阅路径必须以 / 开头（例如 /sub 或 /link）")
			}
			if len(v) > 128 {
				return errors.New("订阅路径过长（最多 128 字符）")
			}
		case SettingSubscribePort:
			v = strings.TrimSpace(v)
			if v != "" && v != "0" {
				p, err := strconv.Atoi(v)
				if err != nil || p < 1 || p > 65535 {
					return errors.New("订阅端口必须为 1~65535 的合法端口号（或 0/留空禁用独立端口）")
				}
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

// SetWebBase 校验并保存 web base（写入 DB，立即生效，无需重启）。
func (s *SiteService) SetWebBase(v string) error {
	n := normalizeWebBase(v)
	if n != "" && !webBaseRe.MatchString(n) {
		return errors.New("web base 需以 / 开头，且仅含字母、数字、/、_、-")
	}
	for _, reserved := range []string{"/api", "/sub", "/node", "/assets", "/healthz", "/admin", "/user"} {
		if n != "" && (n == reserved || strings.HasPrefix(n, reserved+"/")) {
			return errors.New("web base 不能与保留路径冲突: " + reserved)
		}
	}
	var set models.Setting
	err := s.DB.Where("key = ?", SettingWebBase).First(&set).Error
	if err == nil {
		if uerr := s.DB.Model(&set).Update("value", n).Error; uerr != nil {
			return uerr
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		if cerr := s.DB.Create(&models.Setting{Key: SettingWebBase, Value: n}).Error; cerr != nil {
			return cerr
		}
	} else {
		return err
	}
	s.mu.Lock()
	s.webBase = n
	s.mu.Unlock()
	return nil
}
