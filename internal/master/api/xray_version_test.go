package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchXrayVersionsParsesAtomAndDedupes(t *testing.T) {
	mux := http.NewServeMux()
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

func TestFetchXrayVersionsRejectsEmptyFeed(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/XTLS/Xray-core/releases.atom", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<feed xmlns="http://www.w3.org/2005/Atom"></feed>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if _, err := FetchXrayVersions(context.Background(), srv.URL); err == nil {
		t.Fatal("空 feed 应报错")
	}
}
