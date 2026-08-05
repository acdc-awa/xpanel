package upgrade

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0.1", "v1.0.0", 1},
		{"v1.1.0", "v1.0.9", 1},
		{"v2.0.0", "v1.9.9", 1},
		{"v1.0.0", "v1.0.1", -1},
		{"1.0.0", "v1.0.0", 0},
		{"dev", "v1.0.0", -1}, // 非法归一为 "0"，0 < 1.0.0 → -1
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestEnsureURL(t *testing.T) {
	cases := map[string]string{
		"wss://panel.example.com/api/v1/node/ws": "https://panel.example.com/api/v1/node/ws",
		"ws://127.0.0.1:18080/api/v1/node/ws":    "http://127.0.0.1:18080/api/v1/node/ws",
	}
	for in, want := range cases {
		if got := EnsureURL(in); got != want {
			t.Errorf("EnsureURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFetcherLatestAndDownload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/download/agent" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("X-Agent-Version", "v1.2.3")
		w.Header().Set("X-Agent-Sha256", Sha256Hex([]byte("agent-binary")))
		w.Write([]byte("agent-binary"))
	}))
	defer srv.Close()

	f := &Fetcher{BaseURL: srv.URL}
	v, err := f.Latest()
	if err != nil || v != "v1.2.3" {
		t.Fatalf("Latest = %q, %v", v, err)
	}
	data, sum, err := f.Download()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "agent-binary" {
		t.Errorf("data = %q", data)
	}
	if sum != Sha256Hex(data) {
		t.Errorf("sha256 头与数据不匹配")
	}
}

func TestFetcherNoVersionHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("x"))
	}))
	defer srv.Close()
	f := &Fetcher{BaseURL: srv.URL}
	if _, err := f.Latest(); err == nil {
		t.Error("无版本头应报错")
	}
}

func TestSha256Hex(t *testing.T) {
	sum := Sha256Hex([]byte("abc"))
	if !strings.HasPrefix(sum, "ba7816bf") {
		t.Errorf("sha256(abc) = %s", sum)
	}
}
