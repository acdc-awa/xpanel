package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// BodyLimit 限制请求体大小（P2-2）。Content-Length 超限直接 413；
// 分块传输超限由 MaxBytesReader 在读取阶段报错。
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > maxBytes {
			util.Fail(c, http.StatusRequestEntityTooLarge, "请求体过大")
			c.Abort()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
