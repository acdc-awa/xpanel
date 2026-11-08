package xray

import "testing"

func TestValidateXHTTPXmux(t *testing.T) {
	tests := []struct {
		name    string
		stream  string
		wantErr bool
	}{
		{"空 streamSettings", "", false},
		{"非 xhttp", `{"network":"tcp","security":"none"}`, false},
		{"xhttp 无 xmux", `{"network":"xhttp","xhttpSettings":{"path":"/"}}`, false},
		{"maxConnections 单值", `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":"3"}}}`, false},
		{"maxConcurrency 区间", `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConcurrency":"16-32"}}}`, false},
		{"maxConnections=0 不冲突", `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":"0","maxConcurrency":"8"}}}`, false},
		{"两项同时 >0", `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":"2-4","maxConcurrency":"8"}}}`, true},
		{"负数", `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":"-1"}}}`, true},
		{"倒序区间", `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":"4-2"}}}`, true},
		{"超 int32", `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":"2147483648"}}}`, true},
		{"int32 上限", `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":"2147483647"}}}`, false},
		{"浮点数", `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":1.5}}}`, true},
		{"JSON 3.0 不被 int 接受", `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":3.0}}}`, true},
		{"JSON 整数", `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":3}}}`, false},
		{"hKeepAlive -1", `{"network":"xhttp","xhttpSettings":{"xmux":{"hKeepAlivePeriod":-1}}}`, false},
		{"hKeepAlive 字符串", `{"network":"xhttp","xhttpSettings":{"xmux":{"hKeepAlivePeriod":"0"}}}`, true},
		{"hKeepAlive 非整数", `{"network":"xhttp","xhttpSettings":{"xmux":{"hKeepAlivePeriod":1.5}}}`, true},
		{"hKeepAlive 超界", `{"network":"xhttp","xhttpSettings":{"xmux":{"hKeepAlivePeriod":2147483648}}}`, true},
		{"xmux 非对象", `{"network":"xhttp","xhttpSettings":{"xmux":"x"}}`, true},
		{"未知子键忽略", `{"network":"xhttp","xhttpSettings":{"xmux":{"futureKey":"x"}}}`, false},
		{"downloadSettings 嵌套校验", `{"network":"xhttp","xhttpSettings":{"downloadSettings":{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":"3","maxConcurrency":"8"}}}}}`, true},
		{"extra 对象", `{"network":"xhttp","xhttpSettings":{"extra":{"xmux":{"maxConnections":"3","maxConcurrency":"8"}}}}`, true},
		{"extra 字符串", `{"network":"xhttp","xhttpSettings":{"extra":"{\"xmux\":{\"maxConnections\":\"3\",\"maxConcurrency\":\"8\"}}"}}`, true},
		{"空 xmux 对象", `{"network":"xhttp","xhttpSettings":{"mode":"stream-one","xmux":{}}}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateXHTTPXmux(tt.stream)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateXHTTPXmux() error = %v, wantErr %v\nstream: %s", err, tt.wantErr, tt.stream)
			}
		})
	}
}

func TestValidateInbound_RejectsInvalidXmux(t *testing.T) {
	stream := `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":"3","maxConcurrency":"8"}}}`
	if err := ValidateInbound(`{"clients":[]}`, stream, ""); err == nil {
		t.Fatal("ValidateInbound 应拒绝互斥的 xmux")
	}
}

func TestValidateOutbound_RejectsInvalidXmux(t *testing.T) {
	stream := `{"network":"xhttp","xhttpSettings":{"xmux":{"maxConnections":"3","maxConcurrency":"8"}}}`
	if err := ValidateOutbound(`{}`, stream); err == nil {
		t.Fatal("ValidateOutbound 应拒绝互斥的 xmux")
	}
}
