package xray

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/acdc/xray-panel/internal/contracts"
)

// Driver Xray 核心驱动：Generate 复用模板生成器，ValidateConfig 走官方二进制 xray -test。
// TestBin 为 xray 二进制路径；为空时读 XRAY_BIN 环境变量；再为空则跳过二进制校验（返回 nil），
// 此时金标准校验仍由 CI/手工 `xray -test -config` 承担（行为与旧版一致）。
type Driver struct {
	TestBin string
}

// NewDriver 构造 Xray 驱动。
func NewDriver() *Driver { return &Driver{} }

// 编译期接口断言。
var _ contracts.CoreDriver = (*Driver)(nil)

// Name 实现 contracts.CoreDriver。
func (d *Driver) Name() string { return "xray" }

// Generate 实现 contracts.CoreDriver：中立输入 → 现模板生成器（行为不变）。
func (d *Driver) Generate(_ context.Context, in *contracts.GenerateInput) ([]byte, error) {
	if in == nil {
		return nil, fmt.Errorf("生成输入为空")
	}
	var ds []string
	if in.DefaultOutboundDS != "" {
		ds = append(ds, in.DefaultOutboundDS)
	}
	return Generate(in.Inbounds, in.Outbounds, in.RoutingRules, in.UsersByTag, in.Topology,
		in.DefaultOutboundTag, in.RoutingDomainStrategy, ds...)
}

// ValidateConfig 实现 contracts.CoreDriver：写入临时文件后执行 `xray -test -config`。
func (d *Driver) ValidateConfig(ctx context.Context, rawConfig []byte) error {
	bin := d.TestBin
	if bin == "" {
		bin = os.Getenv("XRAY_BIN")
	}
	if bin == "" {
		return nil // 无二进制：跳过（金标准校验仍由 CI/手工执行）
	}
	if runtime.GOOS == "windows" && filepath.Ext(bin) == "" {
		bin += ".exe"
	}
	if _, err := os.Stat(bin); err != nil {
		return nil // 二进制不存在：跳过而非硬失败（生产 master 容器通常无 xray）
	}

	tmp, err := os.CreateTemp("", "xray-validate-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(rawConfig); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, bin, "-test", "-config", tmp.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xray -test 校验失败: %v: %s", err, string(out))
	}
	return nil
}
