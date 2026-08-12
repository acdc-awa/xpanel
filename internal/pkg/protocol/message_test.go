package protocol

import (
	"strings"
	"testing"
)

func TestNewPayloadsEncodeDecode(t *testing.T) {
	// setup_internal_account 编解码
	raw, err := Encode(MsgSetupInternalAccount, "req-1", SetupInternalAccountPayload{Tag: "relay-a"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	m, err := Decode(raw)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m.Type != MsgSetupInternalAccount || m.ID != "req-1" {
		t.Errorf("帧字段不符: %+v", m)
	}
	var p SetupInternalAccountPayload
	if err := m.PayloadTo(&p); err != nil {
		t.Fatalf("PayloadTo: %v", err)
	}
	if p.Tag != "relay-a" {
		t.Errorf("Tag = %q", p.Tag)
	}

	// push_cert 编解码（含 PEM 多行内容）
	raw2, err := Encode(MsgPushCert, "req-2", PushCertPayload{
		Domain: "test.local", CertPEM: "-----BEGIN CERTIFICATE-----\nAAA\n-----END CERTIFICATE-----\n", KeyPEM: "-----BEGIN PRIVATE KEY-----\nBBB\n-----END PRIVATE KEY-----\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	m2, _ := Decode(raw2)
	var p2 PushCertPayload
	if err := m2.PayloadTo(&p2); err != nil {
		t.Fatal(err)
	}
	if p2.Domain != "test.local" || !strings.Contains(p2.CertPEM, "BEGIN CERTIFICATE") || !strings.Contains(p2.KeyPEM, "BEGIN PRIVATE KEY") {
		t.Errorf("push_cert 载荷不符: %+v", p2)
	}

	// internal_uuid_report（节点→主控）
	raw3, _ := Encode(MsgInternalUUIDReport, "", InternalUUIDReportPayload{Tag: "relay-a", UUID: "11111111-2222-3333-4444-555555555555"})
	m3, _ := Decode(raw3)
	var p3 InternalUUIDReportPayload
	if err := m3.PayloadTo(&p3); err != nil {
		t.Fatal(err)
	}
	if p3.Tag != "relay-a" || p3.UUID == "" {
		t.Errorf("report 载荷不符: %+v", p3)
	}

	// setup 回执 data
	raw4, _ := Encode(MsgResult, "req-1", ResultPayload{OK: true, Data: SetupInternalResult{Tag: "relay-a", UUID: "u"}})
	m4, _ := Decode(raw4)
	var r ResultPayload
	if err := m4.PayloadTo(&r); err != nil {
		t.Fatal(err)
	}
	if !r.OK {
		t.Error("OK 应为 true")
	}
	data, ok := r.Data.(map[string]any)
	if !ok || data["tag"] != "relay-a" || data["uuid"] != "u" {
		t.Errorf("回执 data 不符: %v", r.Data)
	}
}
