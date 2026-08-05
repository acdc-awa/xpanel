// Package upgrade 提供 agent 自升级：版本查询/比较/下载/校验/替换（CLI 与未来 WS 推送共用）。
package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version 当前 agent 版本（构建期 -ldflags -X 注入；"dev" 为默认开发值）。
var Version = "dev"

// CurrentVersion 返回当前 agent 版本。
func CurrentVersion() string { return Version }

// normVersion 归一化版本串：去掉 v 前缀，非法段视为 0。
func normVersion(s string) string {
	s = strings.TrimSpace(strings.TrimPrefix(s, "v"))
	if s == "" {
		return "0"
	}
	for _, seg := range strings.Split(s, ".") {
		if seg == "" {
			return "0"
		}
		if _, err := strconv.Atoi(seg); err != nil {
			return "0"
		}
	}
	return s
}

// Compare 语义化比较版本：-1 a<b / 0 相等 / 1 a>b。非法版本归一为 "0"。
func Compare(a, b string) int {
	na := normVersion(a)
	nb := normVersion(b)
	as := strings.Split(na, ".")
	bs := strings.Split(nb, ".")
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := 0; i < n; i++ {
		ai, bi := 0, 0
		if i < len(as) {
			ai, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			bi, _ = strconv.Atoi(bs[i])
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

// Fetcher 从主控下载 agent 二进制并解析版本/sha256 响应头。
type Fetcher struct {
	BaseURL string // 主控 http(s) 地址，如 https://panel.example.com
	Client  *http.Client
}

// Latest 返回主控当前 agent 版本（读 X-Agent-Version 头）。
func (f *Fetcher) Latest() (string, error) {
	resp, err := f.get()
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	v := resp.Header.Get("X-Agent-Version")
	if v == "" {
		return "", fmt.Errorf("主控未提供 X-Agent-Version（非内嵌构建）")
	}
	return v, nil
}

// Download 下载二进制，返回数据与声明的 sha256（hex）。
func (f *Fetcher) Download() ([]byte, string, error) {
	resp, err := f.get()
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("X-Agent-Sha256"), nil
}

// get 发起 GET 下载请求（头信息由 Download/Latest 复用）。
func (f *Fetcher) get() (*http.Response, error) {
	cli := f.Client
	if cli == nil {
		cli = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := cli.Get(f.BaseURL + "/api/v1/download/agent")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("下载失败: HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

// Sha256Hex 计算数据 sha256 的 hex 串。
func Sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// EnsureURL 把 ws(s) 地址转成 http(s)（与 install-agent.sh 的转换一致）。
func EnsureURL(masterURL string) string {
	switch {
	case strings.HasPrefix(masterURL, "wss://"):
		return "https://" + strings.TrimPrefix(masterURL, "wss://")
	case strings.HasPrefix(masterURL, "ws://"):
		return "http://" + strings.TrimPrefix(masterURL, "ws://")
	}
	return strings.TrimSuffix(masterURL, "/")
}

// ErrUpToDate 已是最新版本。
var ErrUpToDate = errors.New("已是最新版本")

// Apply 完整升级流程：查版本 → 比较 → 下载 → sha256 校验 → 原子替换 → 重启。
// exePath 为目标二进制路径（通常 os.Executable()）；restart 由调用方注入（systemd 重启或手动提示）。
func Apply(f *Fetcher, exePath string, restart func() error, out io.Writer) error {
	latest, err := f.Latest()
	if err != nil {
		return err
	}
	if Compare(CurrentVersion(), latest) >= 0 {
		fmt.Fprintf(out, "当前版本 %s，已是最新（主控: %s）\n", CurrentVersion(), latest)
		return ErrUpToDate
	}
	fmt.Fprintf(out, "发现新版本 %s（当前 %s），开始升级...\n", latest, CurrentVersion())

	data, wantSum, err := f.Download()
	if err != nil {
		return err
	}
	if wantSum != "" && !strings.EqualFold(wantSum, Sha256Hex(data)) {
		return fmt.Errorf("sha256 校验失败: 声明 %s 实际 %s", wantSum, Sha256Hex(data))
	}

	tmp := exePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, exePath); err != nil {
		os.Remove(tmp)
		return err
	}
	fmt.Fprintf(out, "二进制已替换（%s）\n", exePath)

	if err := restart(); err != nil {
		return fmt.Errorf("重启失败（新二进制已就位，可手动重启）: %w", err)
	}
	fmt.Fprintln(out, "重启完成，升级成功")
	return nil
}
