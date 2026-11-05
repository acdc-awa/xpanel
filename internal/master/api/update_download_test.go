package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/acdc-awa/xpanel/internal/config"
)

// updateSources：update.mirror 置顶 + 内置候选，压缩包与 .sha256 同源派生。
func TestUpdateSources(t *testing.T) {
	// 用一个不在内置链里的前缀，才测得出「mirror 置顶 + 内置 4 个」= 5 个候选
	d := &Deps{Cfg: &config.Config{Update: config.Update{Mirror: "https://mirror.example.com/https://github.com/"}}}
	srcs := d.updateSources("v9.9.9")
	wantLen := 1 + len(updateBuiltinMirrors)
	if len(srcs) != wantLen {
		t.Fatalf("候选数 = %d, want %d", len(srcs), wantLen)
	}
	if srcs[0].Base != "https://mirror.example.com/https://github.com" {
		t.Fatalf("mirror 应置顶，got %q", srcs[0].Base)
	}
	asset := fmt.Sprintf("xpanel-master-v9.9.9-linux-%s.tar.gz", runtime.GOARCH)
	if !strings.HasSuffix(srcs[0].Tarball, asset) {
		t.Fatalf("压缩包名不对: %q", srcs[0].Tarball)
	}
	if srcs[0].Sha256 != srcs[0].Tarball+".sha256" {
		t.Fatalf("校验和必须与压缩包同源: %q vs %q", srcs[0].Sha256, srcs[0].Tarball)
	}
	if srcs[1].Base != "https://github.com" {
		t.Fatalf("内置候选首位应为 GitHub 直连，got %q", srcs[1].Base)
	}
}

// 未配置 mirror 时只有内置候选；mirror 与内置重复时去重。
func TestUpdateSourcesDefaultAndDedup(t *testing.T) {
	d := &Deps{Cfg: &config.Config{}}
	if got := len(d.updateSources("v1.0.0")); got != len(updateBuiltinMirrors) {
		t.Fatalf("默认候选数 = %d, want %d", got, len(updateBuiltinMirrors))
	}
	d.Cfg.Update.Mirror = "https://github.com"
	if got := len(d.updateSources("v1.0.0")); got != len(updateBuiltinMirrors) {
		t.Fatalf("mirror 与内置重复应去重，候选数 = %d", got)
	}
}

// 签名直链的查询串不得出现在日志/界面文案里。
func TestSanitizeURLText(t *testing.T) {
	in := `Get "https://release-assets.githubusercontent.com/x.tar.gz?sp=r&sig=SECRET": net/http: TLS handshake timeout`
	got := sanitizeURLText(in)
	if strings.Contains(got, "SECRET") || strings.Contains(got, "sp=r") {
		t.Fatalf("查询串没抹掉: %q", got)
	}
	if !strings.Contains(got, "release-assets.githubusercontent.com/x.tar.gz") {
		t.Fatalf("路径不该被吃掉: %q", got)
	}
	if !strings.Contains(got, "TLS handshake timeout") {
		t.Fatalf("原始原因不该被吃掉: %q", got)
	}
}

// 版本标签：必须挡住路径穿越/URL 注入。
func TestUpdateTagPattern(t *testing.T) {
	ok := []string{"v0.2.5", "v1.2.3", "1.2.3", "v1.2.3-rc1"}
	bad := []string{"", "v1", "../../etc/passwd", "v1.2.3?x=1", "v1.2.3/../../x"}
	for _, v := range ok {
		if !updateTagPattern.MatchString(v) {
			t.Errorf("%q 应通过", v)
		}
	}
	for _, v := range bad {
		if updateTagPattern.MatchString(v) {
			t.Errorf("%q 应被拒绝", v)
		}
	}
}

// 4xx 多为「该源没有这个资产」→ 换源，不重试；5xx/网络错误 → 重试。
func TestUpdateRetryable(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{&updateHTTPError{URL: "https://x/y", Status: http.StatusNotFound}, false},
		{&updateHTTPError{URL: "https://x/y", Status: http.StatusForbidden}, false},
		{&updateHTTPError{URL: "https://x/y", Status: http.StatusBadGateway}, true},
		{&updateHTTPError{URL: "https://x/y", Status: http.StatusTooManyRequests}, true},
		{context.DeadlineExceeded, false},
		{context.Canceled, false},
		{errors.New("net/http: TLS handshake timeout"), true},
		{nil, false},
	}
	for _, c := range cases {
		if got := updateRetryable(c.err); got != c.want {
			t.Errorf("updateRetryable(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

// 同源重试：前两次 502，第三次成功 → 3 次请求、整体成功。
func TestFetchWithRetryRetriesThenSucceeds(t *testing.T) {
	restore := shortenRetryBackoff(t)
	defer restore()

	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var buf strings.Builder
	if err := fetchWithRetry(context.Background(), "测试下载", func() error {
		buf.Reset()
		return downloadTo(context.Background(), srv.URL, &buf)
	}); err != nil {
		t.Fatalf("应重试后成功, got %v", err)
	}
	if n.Load() != 3 {
		t.Fatalf("请求次数 = %d, want 3", n.Load())
	}
	if buf.String() != "ok" {
		t.Fatalf("重试后正文 = %q（上一次的半截内容必须被清掉）", buf.String())
	}
}

// 404 不重试：该源没有资产，直接交给调用方换源。
func TestFetchWithRetryNoRetryOn404(t *testing.T) {
	restore := shortenRetryBackoff(t)
	defer restore()

	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	var buf strings.Builder
	err := fetchWithRetry(context.Background(), "测试下载", func() error {
		buf.Reset()
		return downloadTo(context.Background(), srv.URL, &buf)
	})
	if err == nil {
		t.Fatal("404 应返回错误")
	}
	if n.Load() != 1 {
		t.Fatalf("404 不该重试, 请求次数 = %d", n.Load())
	}
}

// 失败文案里不能带签名查询串。
func TestDownloadToSanitizesQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	var buf strings.Builder
	err := downloadTo(context.Background(), srv.URL+"/pkg.tar.gz?sig=SECRET&sp=r", &buf)
	if err == nil {
		t.Fatal("500 应返回错误")
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("签名外泄: %v", err)
	}
}

// shortenRetryBackoff 把 2s/5s 的退避压到毫秒级，避免测试空等。
func shortenRetryBackoff(t *testing.T) func() {
	t.Helper()
	old := updateRetryBackoff
	updateRetryBackoff = []time.Duration{time.Millisecond, time.Millisecond}
	return func() { updateRetryBackoff = old }
}
