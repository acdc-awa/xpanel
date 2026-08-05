package services

import (
	"errors"
	"regexp"
	"strings"
	"sync"

	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/config"
	"github.com/zhx/xray-panel/internal/models"
)

// SettingWebBase 站点设置键：自定义 Web 访问路径前缀。
const SettingWebBase = "web_base"

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
