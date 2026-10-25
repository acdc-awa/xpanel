package models

import "testing"

// 存量行的 xray 协议值 dokodemo-door 必须在启动迁移中改名为 tunnel，
// 随后由类型归一迁移补全 type=tunnel；新值与非直通协议不受影响。
func TestMigrateDokodemoProtocolToTunnel(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := db.AutoMigrate(&Inbound{}); err != nil {
		t.Fatalf("migrate schema: %v", err)
	}

	rows := []Inbound{
		{ServerID: 1, Tag: "legacy-channel", Protocol: "dokodemo-door", Port: 9090, Type: InboundTypeChannel},
		{ServerID: 1, Tag: "legacy-empty-type", Protocol: "dokodemo-door", Port: 9091, Type: ""},
		{ServerID: 1, Tag: "new-tunnel", Protocol: ProtocolTunnel, Port: 9092, Type: InboundTypeTunnel},
		{ServerID: 1, Tag: "vless", Protocol: ProtocolVless, Port: 443, Type: InboundTypeUser},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := migrateDokodemoProtocolToTunnel(db); err != nil {
		t.Fatalf("migrate protocol: %v", err)
	}
	if err := migrateTunnelInboundType(db); err != nil {
		t.Fatalf("migrate type: %v", err)
	}

	var got []Inbound
	if err := db.Order("id").Find(&got).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	want := []struct{ tag, protocol, typ string }{
		{"legacy-channel", ProtocolTunnel, InboundTypeTunnel},
		{"legacy-empty-type", ProtocolTunnel, InboundTypeTunnel},
		{"new-tunnel", ProtocolTunnel, InboundTypeTunnel},
		{"vless", ProtocolVless, InboundTypeUser},
	}
	for i, w := range want {
		if got[i].Tag != w.tag || got[i].Protocol != w.protocol || got[i].Type != w.typ {
			t.Fatalf("row %d = {tag:%q protocol:%q type:%q}, want {tag:%q protocol:%q type:%q}",
				i, got[i].Tag, got[i].Protocol, got[i].Type, w.tag, w.protocol, w.typ)
		}
	}

	// 幂等：再跑一次不应改变任何行。
	if err := migrateDokodemoProtocolToTunnel(db); err != nil {
		t.Fatalf("migrate protocol (second): %v", err)
	}
	if err := migrateTunnelInboundType(db); err != nil {
		t.Fatalf("migrate type (second): %v", err)
	}
}

func TestProtocolDisplayName(t *testing.T) {
	cases := map[string]string{
		"dokodemo-door": "tunnel",
		"tunnel":        "tunnel",
		"vless":         "vless",
		"hysteria":      "hysteria",
	}
	for in, want := range cases {
		if got := ProtocolDisplayName(in); got != want {
			t.Fatalf("ProtocolDisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}
