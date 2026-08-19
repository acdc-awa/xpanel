package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/master/services"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// 常见自动化探测与爬虫 UA 特征（转小写后匹配）
var defaultBlockedUAPatterns = []string{
	"curl/", "wget/", "python-requests", "python-urllib", "python",
	"go-http-client", "java/", "apache-httpclient", "okhttp",
	"urllib", "aiohttp", "httpx", "postman", "insomnia",
	"bot", "spider", "crawler", "headless", "phantomjs", "selenium",
	"scan", "feedly", "universalfeedparser",
}

// 常见主流合法代理客户端与网页导入 UA 包含特征（转小写后匹配）
var allowedClientUAPatterns = []string{
	"clash", "mihomo", "shadowrocket", "surge", "sing-box", "singbox",
	"stash", "quantumult", "v2rayn", "v2ray", "nekobox", "nekoray",
	"hiddify", "loon", "surfboard", "passwall", "ssr", "trojan",
	"fair", "foxray", "streisand", "v2box", "karing", "flclash",
	"mozilla", "chrome", "safari", "webkit", "edge", "firefox",
}

// SubSieveMiddleware 订阅请求清洗与防探测中间件。
// 动态从 settings 表（或传入的读取函数）读取清洗开关。
func SubSieveMiddleware(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		cleanUA := services.GetSetting(db, services.SettingSubCleanUA) == "1"
		strictUA := services.GetSetting(db, services.SettingSubStrictUA) == "1"
		customBlocked := services.GetSetting(db, services.SettingSubBlockedUA)

		// 若未启用任何清洗设置，直接放行
		if !cleanUA && !strictUA && customBlocked == "" {
			c.Next()
			return
		}

		rawUA := strings.TrimSpace(c.GetHeader("User-Agent"))
		ua := strings.ToLower(rawUA)

		// 1. 若开启智能清洗：空 UA 立即拒绝
		if cleanUA && ua == "" {
			util.Fail(c, http.StatusForbidden, "Access Denied: Empty User-Agent")
			c.Abort()
			return
		}

		// 2. 自定义封禁 UA 关键词检查
		if customBlocked != "" {
			for _, kw := range strings.Split(customBlocked, ",") {
				kw = strings.TrimSpace(strings.ToLower(kw))
				if kw != "" && strings.Contains(ua, kw) {
					util.Fail(c, http.StatusForbidden, "Access Denied: Blocked User-Agent")
					c.Abort()
					return
				}
			}
		}

		// 3. 智能爬虫 UA 特征过滤
		if cleanUA && ua != "" {
			for _, pattern := range defaultBlockedUAPatterns {
				if strings.Contains(ua, pattern) {
					util.Fail(c, http.StatusForbidden, "Access Denied: Automated Client Prohibited")
					c.Abort()
					return
				}
			}
		}

		// 4. 严格模式：仅允许知名代理客户端与主流浏览器
		if strictUA && ua != "" {
			matched := false
			for _, pattern := range allowedClientUAPatterns {
				if strings.Contains(ua, pattern) {
					matched = true
					break
				}
			}
			if !matched {
				util.Fail(c, http.StatusForbidden, "Access Denied: Unrecognized Proxy Client")
				c.Abort()
				return
			}
		}

		c.Next()
	}
}
