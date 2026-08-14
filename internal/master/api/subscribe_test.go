package api

import (
	"testing"

	"github.com/zhx/xray-panel/internal/models"
)

// TestShareAddrOf 订阅分享地址三态（与 xray 监听解耦：custom 时用 ShareAddr+SharePort）。
func TestShareAddrOf(t *testing.T) {
	srv := &models.Server{Host: "node.example.com"}
	inb := &models.Inbound{Port: 8443}

	// 默认 node → 服务器 Host + 入站端口
	if h, p := shareAddrOf(srv, inb); h != "node.example.com" || p != 8443 {
		t.Errorf("node 默认: %s:%d", h, p)
	}
	// custom + SharePort=0 → ShareAddr + 入站端口
	inb.ShareAddrStrategy = "custom"
	inb.ShareAddr = "cdn.example.com"
	inb.SharePort = 0
	if h, p := shareAddrOf(srv, inb); h != "cdn.example.com" || p != 8443 {
		t.Errorf("custom 无端口: %s:%d", h, p)
	}
	// custom + SharePort=443 → 转发端点
	inb.SharePort = 443
	if h, p := shareAddrOf(srv, inb); h != "cdn.example.com" || p != 443 {
		t.Errorf("custom 转发端点: %s:%d", h, p)
	}
	// custom 但地址空 → 回退 node
	inb.ShareAddr = ""
	if h, p := shareAddrOf(srv, inb); h != "node.example.com" || p != 8443 {
		t.Errorf("custom 空地址应回退: %s:%d", h, p)
	}
	// listen + 内网监听 → Listen + 入站端口
	inb.ShareAddrStrategy = "listen"
	inb.Listen = "127.0.0.1"
	if h, p := shareAddrOf(srv, inb); h != "127.0.0.1" || p != 8443 {
		t.Errorf("listen: %s:%d", h, p)
	}
	// listen + 0.0.0.0 → 回退 node
	inb.Listen = "0.0.0.0"
	if h, p := shareAddrOf(srv, inb); h != "node.example.com" || p != 8443 {
		t.Errorf("listen 0.0.0.0 应回退: %s:%d", h, p)
	}
}

// TestSubscribeFlow 订阅 flow 计算（与生成侧 buildClients 同源）：
// 入站级（none 禁用自动注入）→ TCP+REALITY 自动 vision（UserInbound 覆盖已随批2 冻结删除）。
func TestSubscribeFlow(t *testing.T) {
	cases := []struct {
		name       string
		inbFlow    string
		tcpReality bool
		wantFlow   string
		wantNo     bool
	}{
		{"自动: tcp+reality", "", true, "xtls-rprx-vision", false},
		{"自动: 非 reality 不注入", "", false, "", false},
		{"入站级开启", "xtls-rprx-vision", true, "xtls-rprx-vision", false},
		{"入站级 none 禁用", "none", true, "", true},
		{"入站级 none + 非 reality", "none", false, "", true},
	}
	for _, c := range cases {
		flow, no := subscribeFlow(c.inbFlow, c.tcpReality)
		if flow != c.wantFlow || no != c.wantNo {
			t.Errorf("%s: got flow=%q noAuto=%v, want flow=%q noAuto=%v",
				c.name, flow, no, c.wantFlow, c.wantNo)
		}
	}
}
