package services

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/models"
)

// 人机验证（Cloudflare Turnstile）——2026-08-14 方向②。
// 配置存 DB Setting 表（未来站点设置页统一管理）：
//
//	captcha_enable        bool   验证开关（默认关，内网/开发可关）
//	captcha_type          string 验证类型（turnstile / recaptcha 预留）
//	turnstile_site_key    string 前端 widget site key
//	turnstile_secret_key  string 后端 siteverify secret
const (
	SettingCaptchaEnable     = "captcha_enable"
	SettingCaptchaType       = "captcha_type"
	SettingTurnstileSiteKey  = "turnstile_site_key"
	SettingTurnstileSecret   = "turnstile_secret_key"
	DefaultCaptchaType       = "turnstile"
	turnstileVerifyEndpoint  = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	turnstileHTTPTimeout     = 10 * time.Second
	turnstileTokenTTL        = 10 * time.Minute // 一次性消费窗口
)

// CaptchaConfig 人机验证配置（每次请求读 DB，管理端改动即时生效）。
type CaptchaConfig struct {
	Enabled      bool
	Type         string
	SiteKey      string
	SecretKey    string
}

// LoadCaptchaConfig 从 DB Setting 读取验证配置。
func LoadCaptchaConfig(db *gorm.DB) CaptchaConfig {
	cfg := CaptchaConfig{Type: DefaultCaptchaType}
	get := func(key string) string {
		var s models.Setting
		if err := db.Where("key = ?", key).First(&s).Error; err == nil {
			return s.Value
		}
		return ""
	}
	cfg.Enabled = get(SettingCaptchaEnable) == "true" || get(SettingCaptchaEnable) == "1"
	if t := get(SettingCaptchaType); t != "" {
		cfg.Type = t
	}
	cfg.SiteKey = get(SettingTurnstileSiteKey)
	cfg.SecretKey = get(SettingTurnstileSecret)
	return cfg
}

// CaptchaSettings 读取验证分组全部键（设置页「安全」tab 展示；secret 仅管理端可见）。
func CaptchaSettings(db *gorm.DB) map[string]string {
	keys := []string{SettingCaptchaEnable, SettingCaptchaType, SettingTurnstileSiteKey, SettingTurnstileSecret}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = GetSetting(db, k)
	}
	return out
}

// SaveCaptchaSettings 保存验证分组（captcha_enable 规范化为 1/0；空串=默认关）。
func SaveCaptchaSettings(db *gorm.DB, vals map[string]string) error {
	for k, v := range vals {
		switch k {
		case SettingCaptchaEnable:
			if v != "" && v != "0" && v != "1" {
				return errors.New("验证开关仅接受 1（开）/ 0（关）")
			}
		case SettingTurnstileSecret, SettingTurnstileSiteKey:
			if len(v) > 500 {
				return errors.New("密钥过长")
			}
		}
		if err := SetSetting(db, k, v); err != nil {
			return err
		}
	}
	return nil
}

// captchaUsed 已消费的 turnstile token（一次性防重放；单实例内存，多实例需换 Redis）。
var captchaUsed = struct {
	sync.Mutex
	m map[string]time.Time
}{m: make(map[string]time.Time)}

// VerifyCaptcha 校验人机验证：
//   - captcha_enable=false 直接放行（内网/开发环境）；
//   - token 非空且未消费过 → POST siteverify，校验 success 与 hostname（防跨站复用）；
//   - 失败统一返回 ErrCaptchaFailed（不区分未携带/无效，防探测）。
//
// action 绑定业务场景（login/register），用于审计与未来策略区分。
func VerifyCaptcha(db *gorm.DB, token, remoteIP, host, action string) error {
	cfg := LoadCaptchaConfig(db)
	if !cfg.Enabled {
		return nil
	}
	if cfg.Type != DefaultCaptchaType || cfg.SecretKey == "" {
		return ErrCaptchaFailed
	}
	if token == "" {
		return ErrCaptchaFailed
	}

	// 一次性消费（防 token 重放；Turnstile 官方不保证单次有效）
	captchaUsed.Lock()
	if _, dup := captchaUsed.m[token]; dup {
		captchaUsed.Unlock()
		return ErrCaptchaFailed
	}
	// 清理过期记录
	now := time.Now()
	for k, t := range captchaUsed.m {
		if now.Sub(t) > turnstileTokenTTL {
			delete(captchaUsed.m, k)
		}
	}
	// P2-12：token 消费表设上限，异常流量下不无限增长。
	if len(captchaUsed.m) > 10000 {
		for k := range captchaUsed.m {
			delete(captchaUsed.m, k)
			if len(captchaUsed.m) <= 10000 {
				break
			}
		}
	}
	captchaUsed.m[token] = now
	captchaUsed.Unlock()

	form := url.Values{}
	form.Set("secret", cfg.SecretKey)
	form.Set("response", token)
	if remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	client := &http.Client{Timeout: turnstileHTTPTimeout}
	resp, err := client.PostForm(turnstileVerifyEndpoint, form)
	if err != nil {
		// 网络失败：fail-closed（拒绝而非放行），避免验证被绕过
		return ErrCaptchaFailed
	}
	defer resp.Body.Close()

	var out struct {
		Success    bool     `json:"success"`
		Hostname   string   `json:"hostname"`
		ErrorCodes []string `json:"error-codes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ErrCaptchaFailed
	}
	if !out.Success {
		return ErrCaptchaFailed
	}
	// hostname 校验：token 颁发站点必须与请求 Host 一致（防跨站 token 复用）。
	// 开发环境 Host 为空/IP 直连时跳过（host 无法比对），生产反代场景必校验。
	if out.Hostname != "" && host != "" {
		reqHost := host
		if h, err := splitHostPort(reqHost); err == nil {
			reqHost = h
		}
		if !strings.EqualFold(out.Hostname, reqHost) {
			return ErrCaptchaFailed
		}
	}
	return nil
}

// splitHostPort 拆分 host:port（无端口时返回原值）。
func splitHostPort(h string) (string, error) {
	idx := strings.LastIndex(h, ":")
	if idx < 0 {
		return h, nil
	}
	return h[:idx], nil
}

// ErrCaptchaFailed 人机验证未通过（统一文案，不区分原因）。
var ErrCaptchaFailed = errors.New("人机验证未通过")
