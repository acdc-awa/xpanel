package stats

import "testing"

func TestOnlineCounterRegex(t *testing.T) {
	cases := map[string]bool{
		"user>>>a@b.com>>>online":                true,
		"user>>>user-1@panel.local>>>online":     true,
		"user>>>a@b.com>>>traffic>>>uplink":      false,
		"inbound>>>vless-in>>>traffic>>>uplink":  false,
	}
	for name, want := range cases {
		if got := onlineRe.MatchString(name); got != want {
			t.Errorf("onlineRe(%q) = %v, want %v", name, got, want)
		}
	}
}