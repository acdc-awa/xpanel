package tlscert

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateSelfSigned(t *testing.T) {
	certPEM, keyPEM, err := GenerateSelfSigned("relay-01.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	// 生成的证书/私钥必须过现有配对校验（agent 落盘前也走它）
	if err := ValidatePair(certPEM, keyPEM); err != nil {
		t.Fatalf("ValidatePair: %v", err)
	}
	leaf, err := ParseLeaf(certPEM)
	if err != nil {
		t.Fatalf("ParseLeaf: %v", err)
	}
	if !leaf.IsCA {
		t.Error("自签证书应为 IsCA 形态")
	}
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != "relay-01.example.com" {
		t.Errorf("SAN DNSNames = %v", leaf.DNSNames)
	}
	if d := time.Until(leaf.NotAfter); d < 9*365*24*time.Hour {
		t.Errorf("有效期应约 10 年, 剩 %v", d)
	}

	// IP 域名进 IPAddresses 而非 DNSNames
	ipCert, _, err := GenerateSelfSigned("10.0.0.1")
	if err != nil {
		t.Fatalf("GenerateSelfSigned IP: %v", err)
	}
	ipLeaf, _ := ParseLeaf(ipCert)
	if len(ipLeaf.IPAddresses) != 1 {
		t.Errorf("IP 证书 SAN IPAddresses = %v", ipLeaf.IPAddresses)
	}

	// 非法域名拒绝
	if _, _, err := GenerateSelfSigned("bad/domain"); err == nil {
		t.Error("非法域名应被拒绝")
	}
}

func TestPinSHA256Hex(t *testing.T) {
	certPEM, _, err := GenerateSelfSigned("pin.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	pin1, err := PinSHA256Hex(certPEM)
	if err != nil {
		t.Fatalf("PinSHA256Hex: %v", err)
	}
	if len(pin1) != 64 {
		t.Fatalf("pin 应为 64 位 hex, got %d", len(pin1))
	}
	if strings.ToLower(pin1) != pin1 {
		t.Error("pin 应为小写 hex")
	}
	// 同证书幂等
	pin2, _ := PinSHA256Hex(certPEM)
	if pin1 != pin2 {
		t.Error("同证书 pin 应一致")
	}
	// 不同证书 pin 不同
	other, _, _ := GenerateSelfSigned("pin2.example.com")
	pin3, _ := PinSHA256Hex(other)
	if pin1 == pin3 {
		t.Error("不同证书 pin 应不同")
	}
	// 非法 PEM 报错
	if _, err := PinSHA256Hex("not a pem"); err == nil {
		t.Error("非法 PEM 应报错")
	}
}
