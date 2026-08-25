package api

import (
	"regexp"
	"strings"
	"testing"
)

// TestMaskSensitivePath 订阅 token 路径必须脱敏，其他路径原样保留。
func TestMaskSensitivePath(t *testing.T) {
	if got := maskSensitivePath("/api/v1/sub/EVAL_SECRET_TOKEN_123456"); got != "/api/v1/sub/***" {
		t.Fatalf("sub path = %q, want masked", got)
	}
	if got := maskSensitivePath("/api/v1/user/me"); got != "/api/v1/user/me" {
		t.Fatalf("normal path should not change, got %q", got)
	}
	if got := maskSensitivePath("/api/v1/sub"); got != "/api/v1/sub" {
		t.Fatalf("exact sub path should not change, got %q", got)
	}
}

// TestInjectSiteHeadTitle DB 配置的 app_name 必须替换静态 <title>（而非追加成双 title——
// 浏览器只取第一个，追加会让静态默认永远盖过 DB 配置）。
func TestInjectSiteHeadTitle(t *testing.T) {
	const staticHTML = "<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><title>Xray 面板</title></head><body><div id=\"app\"></div></body></html>"

	// 1. DB 有 app_name：静态 title 被替换，全文档只保留一个 title
	out := injectSiteHead(staticHTML, map[string]string{"app_name": "PerlicaCloud"})
	titles := reTitle.FindAllString(out, -1)
	if len(titles) != 1 {
		t.Fatalf("title 数量 = %d, want 1; out=%s", len(titles), out)
	}
	if !strings.Contains(out, "<title>PerlicaCloud</title>") {
		t.Fatalf("title 未替换为 DB app_name; out=%s", out)
	}
	if strings.Contains(out, "<title>Xray 面板</title>") {
		t.Fatalf("静态 title 仍残留; out=%s", out)
	}

	// 2. DB 无 app_name：保留静态 title 兜底
	out2 := injectSiteHead(staticHTML, map[string]string{})
	if !strings.Contains(out2, "<title>Xray 面板</title>") {
		t.Fatalf("app_name 为空时应保留静态 title; out=%s", out2)
	}

	// 3. 无静态 title 时插入 DB app_name
	noTitle := "<!DOCTYPE html><html><head></head><body></body></html>"
	out3 := injectSiteHead(noTitle, map[string]string{"app_name": "Cloud"})
	if !strings.Contains(out3, "<head><title>Cloud</title>") {
		t.Fatalf("无静态 title 时应插入; out=%s", out3)
	}

	// 4. 特殊字符必须 HTML 转义（html.EscapeString：& < > " → &amp; &lt; &gt; &#34;）
	out4 := injectSiteHead(noTitle, map[string]string{"app_name": `A&B<C>"D`})
	if !strings.Contains(out4, "<title>A&amp;B&lt;C&gt;&#34;D</title>") {
		t.Fatalf("app_name 未转义; out=%s", out4)
	}
}

// TestInjectSiteHeadAssets favicon 与站点设置脚本注入，且不破坏 title 替换。
func TestInjectSiteHeadAssets(t *testing.T) {
	const staticHTML = "<!DOCTYPE html><html><head><title>默认</title></head><body></body></html>"
	out := injectSiteHead(staticHTML, map[string]string{
		"app_name":   "Cloud",
		"favicon":    "data:image/png;base64,AAAA",
		"stop_register": "1",
	})
	if !strings.Contains(out, `<link rel="icon" href="data:image/png;base64,AAAA">`) {
		t.Fatalf("favicon 未注入; out=%s", out)
	}
	if !strings.Contains(out, "window.__PANEL_SETTINGS__") || !strings.Contains(out, `"stop_register":"1"`) {
		t.Fatalf("站点设置脚本未注入; out=%s", out)
	}
	if strings.Contains(out, "<title>默认</title>") {
		t.Fatalf("title 未被替换; out=%s", out)
	}
	if !regexp.MustCompile(`</head>`).MatchString(out) {
		t.Fatalf("</head> 被破坏; out=%s", out)
	}
}

