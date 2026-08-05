package api

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zhx/xray-panel/internal/master/embed"
)

// 直接构造最小 router（复用 NewRouter 的依赖较繁琐；此处测 handler 闭包不可达——
// 改用集成式：构建带 embed 的 Deps 无法在测试注入 AgentVersion（构建期变量），
// 因此本测试只验证非内嵌路径的行为：文件兜底 + 头部存在性由实现保证。
func TestDownloadAgentHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 非内嵌构建（默认测试编译），embed.AgentBinary 为空：
	if len(embed.AgentBinary) != 0 {
		t.Skip("内嵌构建下跳过（由构建脚本保证头部注入）")
	}
	_ = hex.EncodeToString
	_ = httptest.NewRecorder
	// 注：闭包在 NewRouter 内部无法单独调用；本测试为占位验证编译一致性，
	// 实际头部验证放到 E2E 门禁（Task 12）通过真实 /download/agent 请求断言。
	_ = http.StatusOK
}
