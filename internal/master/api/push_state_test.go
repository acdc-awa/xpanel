package api

import (
	"testing"
)

func TestDerivePushState(t *testing.T) {
	cases := []struct {
		name         string
		configDrift  bool
		hasPend      bool
		configStatus string
		pushError    string
		serverStatus int
		wantState    string
	}{
		{
			name:        "1. 磁盘偏离优先级最高 (drift)",
			configDrift: true,
			hasPend:     true, configStatus: "pushed", pushError: "", serverStatus: 1,
			wantState: "drift",
		},
		{
			name:        "1b. 磁盘偏离即使推失败也显偏离 (drift > rejected)",
			configDrift: true,
			hasPend:     true, configStatus: "pending", pushError: "xray panic", serverStatus: 1,
			wantState: "drift",
		},
		{
			name:        "2. 无待推送配置记录 (none)",
			configDrift: false,
			hasPend:     false, configStatus: "", pushError: "", serverStatus: 1,
			wantState: "none",
		},
		{
			name:        "3. 已同步且未偏离 (synced)",
			configDrift: false,
			hasPend:     true, configStatus: "pushed", pushError: "", serverStatus: 1,
			wantState: "synced",
		},
		{
			name:        "4. 在线且推送失败拒绝 (rejected)",
			configDrift: false,
			hasPend:     true, configStatus: "pending", pushError: "端口已被占用", serverStatus: 1,
			wantState: "rejected",
		},
		{
			name:        "5. 离线导致推送失败为待推送而非拒绝 (pending)",
			configDrift: false,
			hasPend:     true, configStatus: "pending", pushError: "服务器离线，等待上线自动补推", serverStatus: 0,
			wantState: "pending",
		},
		{
			name:        "6. 正常待推送无报错 (pending)",
			configDrift: false,
			hasPend:     true, configStatus: "pending", pushError: "", serverStatus: 1,
			wantState: "pending",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pushStateOf(tc.configDrift, tc.hasPend, tc.configStatus, tc.pushError, tc.serverStatus)
			if got != tc.wantState {
				t.Errorf("pushStateOf() = %q, want %q", got, tc.wantState)
			}
		})
	}
}
