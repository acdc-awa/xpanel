package api

import "testing"

// TestNewWebRouterRegistersWithoutPanic 路由注册期冲突（如静态段与 :param 同层）会直接 panic，
// 用一次性构建覆盖新增的备份上传/恢复路由与分组 body 上限。
func TestNewWebRouterRegistersWithoutPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("路由注册 panic: %v", r)
		}
	}()
	if r := NewWebRouter(&Deps{}); r == nil {
		t.Fatal("NewWebRouter 返回 nil")
	}
}
