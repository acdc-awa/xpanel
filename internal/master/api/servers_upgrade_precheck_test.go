package api

import (
	"strings"
	"testing"

	"github.com/acdc-awa/xpanel/internal/models"
)

// 同版本 + force 是「重装」，不能被预检当成无效回滚跳过（bug：面板里点「重新安装」直接报
// 「当前已是版本 X，无需回滚」，指令根本没下发）。
func TestAgentUpgradeSkipReasonSameVersionReinstall(t *testing.T) {
	srv := models.Server{AgentVersion: "v0.3.0"}
	if r := agentUpgradeSkipReason(srv, "v0.3.0", true); r != "" {
		t.Fatalf("同版本 force 应放行重装，却跳过: %q", r)
	}
	// 非 force 的同版本才是「已是最新版本」
	if r := agentUpgradeSkipReason(srv, "v0.3.0", false); !strings.Contains(r, "已是最新版本") {
		t.Fatalf("非 force 同版本应跳过为已是最新，got %q", r)
	}
}

func TestAgentUpgradeSkipReasonMatrix(t *testing.T) {
	cases := []struct {
		name    string
		current string
		target  string
		force   bool
		wantTag string // 期望原因包含的关键字，空 = 应放行
	}{
		{"升级", "v0.2.0", "v0.3.0", false, ""},
		{"已是最新", "v0.3.0", "v0.3.0", false, "已是最新版本"},
		{"降级不带 force", "v0.3.0", "v0.2.0", false, "已是最新版本"},
		{"force 降级", "v0.3.0", "v0.2.0", true, ""},
		{"force 重装", "v0.3.0", "v0.3.0", true, ""},
		{"旧 agent force", "v0.1.16", "v0.1.16", true, "不支持在线回滚"},
		{"版本未知", "", "v0.3.0", false, ""},
		{"dev 构建 force", "dev", "v0.3.0", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := models.Server{AgentVersion: tc.current}
			got := agentUpgradeSkipReason(srv, tc.target, tc.force)
			if tc.wantTag == "" {
				if got != "" {
					t.Fatalf("应放行，却跳过: %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantTag) {
				t.Fatalf("跳过原因 = %q, 期望包含 %q", got, tc.wantTag)
			}
		})
	}
}
