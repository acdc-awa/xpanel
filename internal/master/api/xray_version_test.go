package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// withGitHubAPIBase 把 REST 基址指向 httptest，返回还原函数。
func withGitHubAPIBase(t *testing.T, base string) {
	t.Helper()
	old := githubAPIBase
	githubAPIBase = base
	t.Cleanup(func() { githubAPIBase = old })
}

// 版本列表必须只保留「发布包齐备」的 release：纯 tag、草稿、资产不成对的一律剔除。
func TestFetchRepoVersionsKeepsOnlyInstallableReleases(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/XTLS/Xray-core/releases", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("per_page") == "" {
			t.Error("缺少 per_page 参数")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[
		  {"tag_name":"v26.10.31","draft":false,"assets":[{"name":"Xray-linux-64.zip"},{"name":"Xray-linux-64.zip.dgst"}]},
		  {"tag_name":"v26.10.29","draft":false,"assets":[]},
		  {"tag_name":"v26.10.21","draft":true,"assets":[{"name":"Xray-linux-64.zip"},{"name":"Xray-linux-64.zip.dgst"}]},
		  {"tag_name":"v26.9.30","draft":false,"assets":[{"name":"Xray-linux-64.zip"}]},
		  {"tag_name":"v26.9.9","draft":false,"assets":[{"name":"Xray-linux-arm64-v8a.zip"},{"name":"Xray-linux-arm64-v8a.zip.dgst"}]}
		]`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	withGitHubAPIBase(t, srv.URL)

	got, err := FetchXrayVersions(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchXrayVersions: %v", err)
	}
	want := []string{"v26.10.31", "v26.9.9"}
	if len(got) != len(want) {
		t.Fatalf("versions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("versions[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Agent 侧口径不同：要有 checksums.txt 且至少一个架构二进制。
func TestFetchAgentVersionsRequiresChecksumsAndBinary(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acdc-awa/XPanel-Node/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[
		  {"tag_name":"v0.3.0","draft":false,"assets":[{"name":"checksums.txt"},{"name":"xray-agent-linux-amd64"}]},
		  {"tag_name":"v0.2.0","draft":false,"assets":[{"name":"xray-agent-linux-amd64"}]},
		  {"tag_name":"v0.1.19","draft":false,"assets":[{"name":"checksums.txt"},{"name":"install-agent.sh"}]}
		]`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	withGitHubAPIBase(t, srv.URL)

	got, err := FetchRepoVersions(context.Background(), srv.URL, defaultAgentRepo)
	if err != nil {
		t.Fatalf("FetchRepoVersions: %v", err)
	}
	if len(got) != 1 || got[0] != "v0.3.0" {
		t.Fatalf("versions = %v, want [v0.3.0]", got)
	}
}

// 一个可用版本都没有时必须报错，不能退回「列一堆纯 tag」。
func TestFetchRepoVersionsEmptyWhenNoInstallable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/XTLS/Xray-core/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"tag_name":"v26.10.29","draft":false,"assets":[]}]`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	withGitHubAPIBase(t, srv.URL)

	if _, err := FetchXrayVersions(context.Background(), srv.URL); err == nil {
		t.Fatal("没有可安装版本时应报错")
	}
}

// REST 不可用（限流 / 内网只能走镜像）时降级读 releases.atom，并去重。
func TestFetchRepoVersionsFallsBackToAtom(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/XTLS/Xray-core/releases", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	})
	mux.HandleFunc("/XTLS/Xray-core/releases.atom", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <link rel="alternate" type="text/html" href="https://github.com/XTLS/Xray-core/releases/tag/v26.10.21"/>
    <link rel="self" href="https://api.github.com/repos/XTLS/Xray-core/releases/1"/>
  </entry>
  <entry>
    <link rel="alternate" type="text/html" href="https://github.com/XTLS/Xray-core/releases/tag/v26.10.21"/>
  </entry>
  <entry>
    <link rel="alternate" type="text/html" href="https://github.com/XTLS/Xray-core/releases/tag/v26.10.20"/>
  </entry>
</feed>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	withGitHubAPIBase(t, srv.URL)

	got, err := FetchXrayVersions(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("FetchXrayVersions: %v", err)
	}
	want := []string{"v26.10.21", "v26.10.20"}
	if len(got) != len(want) {
		t.Fatalf("versions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("versions[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
