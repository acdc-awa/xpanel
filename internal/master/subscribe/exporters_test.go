package subscribe

import (
	"context"
	"testing"

	"github.com/acdc/xray-panel/internal/contracts"
)

// TestDefaultRegistry_ClashAndBase64 注册表调度与导出一致性：
// 导出器原生消费 contracts.ProxyNodeDTO（协议插件决议语义），UA 命中 Clash、兜底 base64。
func TestDefaultRegistry_ClashAndBase64(t *testing.T) {
	uuid := "11111111-2222-3333-4444-555555555555"
	dto := contracts.ProxyNodeDTO{
		ID: 1, Name: "香港01", ServerHost: "hk.node.local", ServerPort: 10086, Protocol: "vless",
		Transport: &contracts.TransportOptions{Network: "xhttp", Mode: "auto", Path: "/xhttp", Host: "hk.node.local"},
		Security:  &contracts.SecurityOptions{Type: "tls"},
		Auth:      &contracts.ClientCredentialDTO{UUID: uuid},
	}
	dtos := []contracts.ProxyNodeDTO{dto}
	summary := contracts.UserSummaryDTO{UUID: uuid}

	reg := DefaultRegistry()

	// 1. Clash by UA
	clashContent, _, err := reg.Export(context.Background(), "clash.meta", summary, dtos, contracts.ExportOptions{Template: "", PanelHost: "panel.test"})
	if err != nil {
		t.Fatalf("Clash export failed: %v", err)
	}
	expectedClash := BuildClashWithTemplate(dtos, "", "panel.test")
	if clashContent != expectedClash {
		t.Errorf("Clash exporter mismatch:\n got: %s\nwant: %s", clashContent, expectedClash)
	}

	// 2. Base64 fallback
	base64Exporter := reg.Find("base64")
	if base64Exporter == nil {
		t.Fatal("base64 exporter not registered")
	}
	b64Content, _, err := base64Exporter.Export(context.Background(), summary, dtos, contracts.ExportOptions{})
	if err != nil {
		t.Fatalf("Base64 export failed: %v", err)
	}
	expectedB64 := BuildBase64(dtos)
	if b64Content != expectedB64 {
		t.Errorf("Base64 exporter mismatch:\n got: %s\nwant: %s", b64Content, expectedB64)
	}
}
