package subscribe

import (
	"context"
	"testing"

	"github.com/acdc/xray-panel/internal/contracts"
	"github.com/acdc/xray-panel/internal/models"
)

func TestDefaultRegistry_ClashAndBase64(t *testing.T) {
	srv := &models.Server{Name: "香港01", Host: "hk.node.local"}
	inb := &models.Inbound{
		Tag:            "vless-xhttp",
		Protocol:       "vless",
		Port:           10086,
		StreamSettings: `{"network":"xhttp","security":"tls","xhttpSettings":{"mode":"auto","path":"/xhttp","host":"hk.node.local"}}`,
	}
	uuid := "11111111-2222-3333-4444-555555555555"
	item := BuildProxyItem(srv, inb, uuid)
	items := []ProxyItem{item}
	user := &models.User{UUID: uuid}
	summary := UserToSummaryDTO(user)
	dtos := ProxyItemsToDTOs(items)

	reg := DefaultRegistry()

	// 1. Clash by UA
	clashContent, _, err := reg.Export(context.Background(), "clash.meta", summary, dtos, contracts.ExportOptions{Template: "", PanelHost: "panel.test"})
	if err != nil {
		t.Fatalf("Clash export failed: %v", err)
	}
	expectedClash := BuildClashWithTemplate(user, items, "", "panel.test")
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
	expectedB64 := BuildBase64(user, items)
	if b64Content != expectedB64 {
		t.Errorf("Base64 exporter mismatch:\n got: %s\nwant: %s", b64Content, expectedB64)
	}
}
