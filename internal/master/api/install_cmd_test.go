package api

import (
	"strings"
	"testing"
)

// TestInstallCmdScriptFromGitHub 一键安装命令的脚本必须来自 XPanel-Node GitHub Releases
// （仓库拆分收口：面板不再充当安装脚本下载源），--master 才指向面板。
func TestInstallCmdScriptFromGitHub(t *testing.T) {
	cmd := installCmd("https://panel.example.com", "", "", "node-1", "sec_x")
	if !strings.HasPrefix(cmd, "bash <(curl -fsSL "+AgentInstallScriptURL+")") {
		t.Fatalf("安装脚本来源应为 GitHub Releases，实际命令: %s", cmd)
	}
	if !strings.Contains(cmd, "--master wss://panel.example.com/node/ws --node-id node-1 --secret sec_x") {
		t.Fatalf("节点 WS 网关地址/参数错误: %s", cmd)
	}
}

// TestInstallCmdPublicURLFallback public_url 为空时回退请求 Host，且按 http 推导 ws 协议。
func TestInstallCmdPublicURLFallback(t *testing.T) {
	cmd := installCmd("", "", "192.0.2.10:18080", "1", "s")
	if !strings.Contains(cmd, "--master ws://192.0.2.10:18080/node/ws --node-id 1 --secret s") {
		t.Fatalf("Host 回退命令错误: %s", cmd)
	}
	if !strings.Contains(cmd, AgentInstallScriptURL) {
		t.Fatalf("脚本来源丢失: %s", cmd)
	}
}

// TestInstallCmdWSPublicURLOverride APP_WS_PUBLIC_URL 提供时整体覆盖 --master
// （独立 WS 域名/任意路径场景，与面板 public_url 无关）。
func TestInstallCmdWSPublicURLOverride(t *testing.T) {
	cmd := installCmd("https://panel.example.com", "wss://ws.example.com/node/ws", "", "2", "s2")
	if !strings.Contains(cmd, "--master wss://ws.example.com/node/ws --node-id 2 --secret s2") {
		t.Fatalf("WS 独立地址覆盖错误: %s", cmd)
	}
}