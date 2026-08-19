package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/pkg/util"
)

// rateLimiter 简易内存滑动窗口限流（单实例够用；多实例部署需换 Redis）。
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{hits: make(map[string][]time.Time), limit: limit, window: window}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-l.window)
	// 清理窗口外记录
	recent := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.limit {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)

	// P2-12：map 大小上限，超过时清理过期 key，避免内存无限增长。
	if len(l.hits) > 10000 {
		for k, ts := range l.hits {
			keep := ts[:0]
			for _, t := range ts {
				if t.After(cutoff) {
					keep = append(keep, t)
				}
			}
			if len(keep) == 0 {
				delete(l.hits, k)
			} else {
				l.hits[k] = keep
			}
		}
	}
	return true
}

// RateLimit 按 客户端IP+路径 限流（如登录 5 次/分钟）。
func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	rl := newRateLimiter(limit, window)
	return func(c *gin.Context) {
		key := util.ClientIPFromContext(c) + "|" + c.FullPath()
		if !rl.allow(key) {
			util.Fail(c, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}
		c.Next()
	}
}

// RateLimitWrite 仅对写请求（非 GET/HEAD/OPTIONS）限流；用于管理端写操作面（P2-3）。
func RateLimitWrite(limit int, window time.Duration) gin.HandlerFunc {
	rl := newRateLimiter(limit, window)
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		key := util.ClientIPFromContext(c) + "|" + c.FullPath()
		if !rl.allow(key) {
			util.Fail(c, http.StatusTooManyRequests, "操作过于频繁，请稍后再试")
			c.Abort()
			return
		}
		c.Next()
	}
}
