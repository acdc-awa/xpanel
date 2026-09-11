package api

import "testing"

// assetURLForRelease 只认当前架构的资产名，命中时按镜像前缀重写直链。
func TestAssetURLForRelease(t *testing.T) {
	rel := releaseItem{
		TagName: "v1.2.3",
		Assets: []releaseAsset{
			{Name: "xpanel-master-v1.2.3-linux-arm64.tar.gz", BrowserDownloadURL: "https://github.com/o/r/releases/download/v1.2.3/xpanel-master-v1.2.3-linux-arm64.tar.gz"},
			{Name: "xpanel-master-v1.2.3-linux-amd64.tar.gz", BrowserDownloadURL: "https://github.com/o/r/releases/download/v1.2.3/xpanel-master-v1.2.3-linux-amd64.tar.gz"},
			{Name: "xpanel-master-v1.2.3-linux-amd64.tar.gz.sha256", BrowserDownloadURL: "https://github.com/o/r/releases/download/v1.2.3/xpanel-master-v1.2.3-linux-amd64.tar.gz.sha256"},
		},
	}
	got := assetURLForRelease(rel, "amd64", "https://ghproxy.net/https://github.com")
	want := "https://ghproxy.net/https://github.com/o/r/releases/download/v1.2.3/xpanel-master-v1.2.3-linux-amd64.tar.gz"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// 非 .sha256、且架构匹配
	if got := assetURLForRelease(rel, "arm64", "https://github.com"); got == "" {
		t.Fatalf("arm64 应命中")
	}
	// 无当前架构资产 → 空串（不可安装）
	if got := assetURLForRelease(rel, "riscv64", "https://github.com"); got != "" {
		t.Fatalf("缺失架构应返回空串, got %q", got)
	}
}

// 空 tag / 无资产时不得 panic，返回空串。
func TestAssetURLForReleaseEmpty(t *testing.T) {
	if got := assetURLForRelease(releaseItem{}, "amd64", "https://github.com"); got != "" {
		t.Fatalf("got %q", got)
	}
}
