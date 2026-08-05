// Package upgrade 提供 agent 自升级：版本查询/比较/下载/校验/替换（CLI 与未来 WS 推送共用）。
package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
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
