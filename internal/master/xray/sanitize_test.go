package xray

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeStreamSettings(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want func(t *testing.T, out string)
	}{
		{
			name: "empty passthrough",
			in:   "",
			want: func(t *testing.T, out string) {
				if out != "" {
					t.Errorf("empty should stay empty, got %q", out)
				}
			},
		},
		{
			name: "invalid JSON passthrough",
			in:   `{not json`,
			want: func(t *testing.T, out string) {
				if out != `{not json` {
					t.Errorf("invalid JSON should be returned as-is")
				}
			},
		},
		{
			name: "delete externalProxy",
			in:   `{"network":"tcp","externalProxy":{"tag":"proxy"}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				if _, ok := m["externalProxy"]; ok {
					t.Error("externalProxy should be deleted")
				}
			},
		},
		{
			name: "delete tlsSettings.settings",
			in:   `{"security":"tls","tlsSettings":{"serverName":"x.com","settings":{"allowInsecure":true}}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				tls := m["tlsSettings"].(map[string]any)
				if _, ok := tls["settings"]; ok {
					t.Error("tlsSettings.settings should be deleted")
				}
				if tls["serverName"] != "x.com" {
					t.Error("tlsSettings.serverName should be preserved")
				}
			},
		},
		{
			name: "delete realitySettings.settings",
			in:   `{"security":"reality","realitySettings":{"serverName":"apple.com","settings":{"publicKey":"pk"}}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				r := m["realitySettings"].(map[string]any)
				if _, ok := r["settings"]; ok {
					t.Error("realitySettings.settings should be deleted")
				}
			},
		},
		{
			name: "drop finalmask on reality",
			in:   `{"security":"reality","finalmask":{"rand":[{"length":64}]}}`,
			want: func(t *testing.T, out string) {
				if strings.Contains(out, "finalmask") {
					t.Error("finalmask should be deleted when security=reality")
				}
			},
		},
		{
			name: "keep finalmask on non-reality",
			in:   `{"security":"tls","finalmask":{"rand":[{"length":64}]}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				if _, ok := m["finalmask"]; !ok {
					t.Error("finalmask should be kept when security!=reality")
				}
			},
		},
		{
			name: "drop empty rand packets",
			in:   `{"finalmask":{"rand":[{},{"length":64}]}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				fm := m["finalmask"].(map[string]any)
				rand := fm["rand"].([]any)
				if len(rand) != 1 {
					t.Errorf("empty rand should be dropped, got %d items", len(rand))
				}
			},
		},
		{
			name: "lift xhttp session keys",
			in:   `{"xhttpSettings":{"mode":"stream-up","sessionPlacement":"middle","sessionKey":"mykey","path":"/x"}}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				json.Unmarshal([]byte(out), &m)
				x := m["xhttpSettings"].(map[string]any)
				if _, ok := x["sessionPlacement"]; ok {
					t.Error("sessionPlacement should be lifted to sessionIDPlacement")
				}
				if _, ok := x["sessionKey"]; ok {
					t.Error("sessionKey should be lifted to sessionID")
				}
				if v, _ := x["sessionIDPlacement"].(string); v != "middle" {
					t.Errorf("sessionIDPlacement=%q, want middle", v)
				}
				if v, _ := x["sessionID"].(string); v != "mykey" {
					t.Errorf("sessionID=%q, want mykey", v)
				}
				if x["path"] != "/x" {
					t.Error("unrelated keys should be preserved")
				}
			},
		},
		{
			name: "full sanitize - all rules together",
			in: `{
				"network":"tcp",
				"security":"reality",
				"externalProxy":{"tag":"x"},
				"realitySettings":{"serverName":"a.com","settings":{"foo":1}},
				"finalmask":{"rand":[{},{"length":32}]},
				"xhttpSettings":{"sessionPlacement":"front"}
			}`,
			want: func(t *testing.T, out string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(out), &m); err != nil {
					t.Fatalf("output should be valid JSON: %v", err)
				}
				// externalProxy removed
				if _, ok := m["externalProxy"]; ok {
					t.Error("externalProxy should be deleted")
				}
				// realitySettings.settings removed
				r := m["realitySettings"].(map[string]any)
				if _, ok := r["settings"]; ok {
					t.Error("realitySettings.settings should be deleted")
				}
				// finalmask removed (reality)
				if _, ok := m["finalmask"]; ok {
					t.Error("finalmask should be deleted when security=reality")
				}
				// xhttp keys lifted
				x := m["xhttpSettings"].(map[string]any)
				if _, ok := x["sessionPlacement"]; ok {
					t.Error("sessionPlacement should be lifted")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeStreamSettings(tt.in)
			tt.want(t, got)
		})
	}
}
