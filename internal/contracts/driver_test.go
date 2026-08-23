package contracts

import (
	"context"
	"errors"
	"testing"
)

type fakeDriver struct {
	name string
}

func (f fakeDriver) Name() string { return f.name }
func (f fakeDriver) Generate(context.Context, *GenerateInput) ([]byte, error) {
	return []byte(f.name), nil
}
func (f fakeDriver) ValidateConfig(context.Context, []byte) error { return nil }

func TestDriverRegistry(t *testing.T) {
	r := NewDriverRegistry()
	if r.Default() != nil {
		t.Fatal("空注册表 Default 应为 nil")
	}
	if got := r.Find("xray"); got != nil {
		t.Fatal("空注册表 Find 应为 nil")
	}

	r.Register(fakeDriver{name: "xray"})
	r.Register(fakeDriver{name: "sing-box"})
	if got := r.Find("xray"); got == nil || got.Name() != "xray" {
		t.Fatal("Find(xray) 未命中")
	}
	if got := r.Default(); got == nil || got.Name() != "xray" {
		t.Fatalf("Default 应为首个注册 xray, got %v", got)
	}

	// 同名覆盖
	r.Register(fakeDriver{name: "xray"})
	if len(r.drivers) != 2 {
		t.Fatalf("同名注册应覆盖而非追加, drivers=%d", len(r.drivers))
	}

	// nil 注册忽略
	r.Register(nil)
	if len(r.drivers) != 2 {
		t.Fatalf("nil 注册应被忽略, drivers=%d", len(r.drivers))
	}

	// Generate 透传
	out, err := r.Find("sing-box").Generate(context.Background(), &GenerateInput{})
	if err != nil || string(out) != "sing-box" {
		t.Fatalf("Generate 透传错误: out=%q err=%v", out, err)
	}
}

// TestCoreDriverInterface 编译期确认 fakeDriver 实现接口（防接口签名漂移）。
func TestCoreDriverInterface(t *testing.T) {
	var d CoreDriver = fakeDriver{name: "x"}
	if d.Name() != "x" {
		t.Fatal(errors.New("Name 不匹配"))
	}
}
