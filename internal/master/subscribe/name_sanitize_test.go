package subscribe

import "testing"

// 2026-10-28 审查 P3-3：接入点名称会进入 Clash 节点名与 VLESS 链接 #fragment，
// 控制字符必须剥离，避免污染 YAML/客户端显示。
func TestSanitizeNodeName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"香港 01", "香港 01"},
		{"香港\n01", "香港 01"},
		{"a\t\tb\r\nc", "a b c"},
		{"\x00\x01hidden\x7f", "hidden"},
		{"  spaced   name  ", "spaced name"},
		{"emoji 🚀 节点", "emoji 🚀 节点"},
		{"", ""},
	}
	for _, c := range cases {
		if got := sanitizeNodeName(c.in); got != c.want {
			t.Errorf("sanitizeNodeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
