package xray_test

// Stage 4：CoreDriver 的 Xray 实现测试——等价性（driver.Generate 与直接 Generate 字节一致）
// 与金标准校验（ValidateConfig 走官方二进制 xray -test）。

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/acdc/xray-panel/internal/contracts"
	"github.com/acdc/xray-panel/internal/master/xray"
	"github.com/acdc/xray-panel/internal/models"
)

// testXrayBin 定位仓库内锁定的官方 xray 二进制（不存在则跳过金标准校验测试）。
func testXrayBin(t *testing.T) string {
	t.Helper()
	bin, err := filepath.Abs(filepath.Join("..", "..", "..", "tools", "xray-windows-64", "xray.exe"))
	if err != nil {
		t.Skipf("解析 xray 路径失败: %v", err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("xray 二进制不存在（%s），跳过金标准校验", bin)
	}
	return bin
}

func driverGoldenInbound(t *testing.T) []models.Inbound {
	t.Helper()
	priv, pub, err := xray.GenerateKeys()
	if err != nil {
		t.Fatalf("GenerateKeys failed: %v", err)
	}
	_ = pub
	return []models.Inbound{{
		ID: 1, ServerID: 1, Tag: "vless-in", Protocol: "vless", Port: 443,
		StreamSettings: `{"network":"tcp","security":"reality","realitySettings":{"serverNames":["example.com"],"privateKey":"` + priv + `","shortIds":["12345678"],"dest":"example.com:443"}}`,
		Enabled:        true,
	}}
}

// TestDriverGenerateEquivalence driver.Generate 必须与包级 Generate 字节级一致（行为不变验收）。
func TestDriverGenerateEquivalence(t *testing.T) {
	inbounds := driverGoldenInbound(t)
	users := vlessUsers("vless-in")

	direct, err := xray.Generate(inbounds, nil, nil, users, nil, "", "")
	if err != nil {
		t.Fatalf("直接 Generate 失败: %v", err)
	}

	drv := xray.NewDriver()
	viaDriver, err := drv.Generate(context.Background(), &contracts.GenerateInput{
		Inbounds:   inbounds,
		UsersByTag: users,
	})
	if err != nil {
		t.Fatalf("driver.Generate 失败: %v", err)
	}
	if string(direct) != string(viaDriver) {
		t.Fatal("driver.Generate 与直接 Generate 输出不一致")
	}
	if drv.Name() != "xray" {
		t.Fatalf("驱动名 = %q, 期望 xray", drv.Name())
	}
}

// TestDriverValidateConfigGolden 金标准入测：生成配置必须过官方 xray -test。
func TestDriverValidateConfigGolden(t *testing.T) {
	bin := testXrayBin(t)
	drv := &xray.Driver{TestBin: bin}

	raw, err := drv.Generate(context.Background(), &contracts.GenerateInput{
		Inbounds:   driverGoldenInbound(t),
		UsersByTag: vlessUsers("vless-in"),
	})
	if err != nil {
		t.Fatalf("Generate 失败: %v", err)
	}
	if err := drv.ValidateConfig(context.Background(), raw); err != nil {
		t.Fatalf("生成配置未通过 xray -test: %v", err)
	}

	// 负例：非法配置必须报错
	if err := drv.ValidateConfig(context.Background(), []byte(`{"inbounds": "broken"}`)); err == nil {
		t.Fatal("非法配置应被 xray -test 拒绝")
	}
}

// TestDriverValidateConfigSkip 无二进制/路径不存在时校验跳过（返回 nil），不硬失败。
func TestDriverValidateConfigSkip(t *testing.T) {
	drv := &xray.Driver{TestBin: filepath.Join(os.TempDir(), "no-such-xray-binary.exe")}
	if err := drv.ValidateConfig(context.Background(), []byte(`{}`)); err != nil {
		t.Fatalf("二进制缺失时应跳过校验, got %v", err)
	}
}
