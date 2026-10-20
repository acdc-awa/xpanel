package services

import (
	"testing"
	"time"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
)

// TestBuildGenerateContextCAFileByAgentCap：机间 CA 验签材料按「消费方 agent 能力」二选一——
// 声明 push_ca 的新 agent 用文件路径（push_ca 已落盘），旧 agent 退回内联 PEM。
func TestBuildGenerateContextCAFileByAgentCap(t *testing.T) {
	db := equivDB(t)
	src := models.Server{ID: 11, Name: "src", Host: "src.example.com", NodeID: "node-src"}
	dst := models.Server{ID: 12, Name: "dst", Host: "dst.example.com", NodeID: "node-dst"}
	if err := db.Create(&src).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&dst).Error; err != nil {
		t.Fatal(err)
	}

	const caPEM = "-----BEGIN CERTIFICATE-----\nMIIC-probe\n-----END CERTIFICATE-----\n"
	ca := models.CACert{ID: 7, Name: "root", CACertPEM: caPEM, HasKey: true, NotAfter: time.Now().Add(24 * time.Hour)}
	if err := db.Create(&ca).Error; err != nil {
		t.Fatal(err)
	}
	cert := models.Cert{ID: 9, Domain: "land.example.com", CertPEM: "leaf", KeyPEM: "key",
		NotAfter: time.Now().Add(24 * time.Hour), CAID: &ca.ID}
	if err := db.Create(&cert).Error; err != nil {
		t.Fatal(err)
	}
	target := models.Inbound{ID: 201, ServerID: dst.ID, Tag: "land", Protocol: models.ProtocolVless,
		Port: 443, CertID: &cert.ID, Enabled: true}
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	ref := target.ID
	ob := models.ServerOutbound{ID: 301, ServerID: src.ID, Tag: "to-land", Protocol: models.ProtocolVless,
		InboundRef: &ref, Enabled: true}
	if err := db.Create(&ob).Error; err != nil {
		t.Fatal(err)
	}

	// 1. 旧 agent（无能力位）→ 内联 PEM
	topo, err := buildGenerateContext(db, &src, nil, []models.ServerOutbound{ob})
	if err != nil {
		t.Fatal(err)
	}
	rt, ok := topo.RefTargets[target.ID]
	if !ok {
		t.Fatal("缺少 RefTarget")
	}
	if rt.CACert != caPEM {
		t.Errorf("旧 agent 应内联 CA PEM，got %q", rt.CACert)
	}
	if rt.CACertFile != "" {
		t.Errorf("旧 agent 不应引用 CA 文件，got %q", rt.CACertFile)
	}
	if rt.CACertID != ca.ID {
		t.Errorf("CACertID = %d, want %d", rt.CACertID, ca.ID)
	}

	// 2. 新 agent（声明 push_ca）→ 文件路径，不再内联
	src.AgentCaps = protocol.CapPushCA
	topo2, err := buildGenerateContext(db, &src, nil, []models.ServerOutbound{ob})
	if err != nil {
		t.Fatal(err)
	}
	rt2 := topo2.RefTargets[target.ID]
	if rt2.CACertFile != contracts.CACertPath(ca.ID) {
		t.Errorf("CACertFile = %q, want %q", rt2.CACertFile, contracts.CACertPath(ca.ID))
	}
	if rt2.CACert != "" {
		t.Errorf("新 agent 不应内联 CA PEM，got %q", rt2.CACert)
	}
}
