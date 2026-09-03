package api

import (
	"testing"
)

func TestNormAgentVersion(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"v0.1.8-1-gabcdef", "0.1.8"},
		{"v0.1.0", "0.1.0"},
		{"", "0"},
		{"invalid", "0"},
	}
	for _, tc := range cases {
		got := NormAgentVersion(tc.in)
		if got != tc.want {
			t.Errorf("NormAgentVersion(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestCompareAgentVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.1.7", "v0.1.8", -1},
		{"v0.1.8", "v0.1.8", 0},
		{"v0.2.0", "v0.1.8", 1},
		{"0.1.8", "v0.1.8", 0},
		{"v1.0.0", "v0.9.9", 1},
		{"v0.1.8-alpha", "v0.1.8", 0},
		{"v0.1.7-dev", "v0.1.8", -1},
	}
	for _, tc := range cases {
		got := CompareAgentVersion(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("CompareAgentVersion(%q, %q) = %d; want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
