package api

import (
	"os"
	"testing"
	"time"
)

// TestMain 与 cmd/master/main.go 保持一致：测试进程同样把本地时区固定为 UTC。
// 否则 time.Now()/GORM 自动时间戳会随开发机时区（如 +08:00）落库为本地偏移文本，
// 而查询侧按 UTC 绑定，范围筛选类用例会随时区漂移、真机行为也无法复现。
func TestMain(m *testing.M) {
	time.Local = time.UTC
	os.Exit(m.Run())
}
