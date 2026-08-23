package api

import (
	"testing"

	"github.com/acdc/xray-panel/internal/master/subscribe"
	"github.com/acdc/xray-panel/internal/models"
)

// TestShareAddrOf 订阅分享地址三态（与 xray 监听解耦：custom 时用 ShareAddr+SharePort）。
func TestShareAddrOf(t *testing.T) {
	srv := &models.Server{Host: "node.example.com"}
	inb := &models.Inbound{Port: 8443}

	// 默认 node → 服务器 Host + 入站端口
	if h, p := subscribe.ShareAddrOf(srv, inb); h != "node.example.com" || p != 8443 {
		t.Errorf("node 默认: %s:%d", h, p)
	}
	// custom + SharePort=0 → ShareAddr + 入站端口
	inb.ShareAddrStrategy = "custom"
	inb.ShareAddr = "cdn.example.com"
	inb.SharePort = 0
	if h, p := subscribe.ShareAddrOf(srv, inb); h != "cdn.example.com" || p != 8443 {
		t.Errorf("custom 无端口: %s:%d", h, p)
	}
	// custom + SharePort=443 → 转发端点
	inb.SharePort = 443
	if h, p := subscribe.ShareAddrOf(srv, inb); h != "cdn.example.com" || p != 443 {
		t.Errorf("custom 转发端点: %s:%d", h, p)
	}
	// custom 但地址空 → 回退 node
	inb.ShareAddr = ""
	if h, p := subscribe.ShareAddrOf(srv, inb); h != "node.example.com" || p != 443 {
		t.Errorf("custom 空地址应回退: %s:%d", h, p)
	}
	// listen + 内网监听 → Listen + 入站端口
	inb.ShareAddrStrategy = "listen"
	inb.Listen = "127.0.0.1"
	if h, p := subscribe.ShareAddrOf(srv, inb); h != "127.0.0.1" || p != 443 {
		t.Errorf("listen: %s:%d", h, p)
	}
	// listen + 0.0.0.0 → 回退 node
	inb.Listen = "0.0.0.0"
	if h, p := subscribe.ShareAddrOf(srv, inb); h != "node.example.com" || p != 443 {
		t.Errorf("listen 0.0.0.0 应回退: %s:%d", h, p)
	}
}

