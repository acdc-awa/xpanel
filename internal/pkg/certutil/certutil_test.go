package certutil

import (
	"crypto/x509"
	"net"
	"testing"
	"time"
)

func TestGenerateCA_AllKeyTypes(t *testing.T) {
	keyTypes := []string{
		KeyTypeECDSAP256,
		KeyTypeECDSAP384,
		KeyTypeRSA2048,
		KeyTypeEd25519,
	}

	for _, kt := range keyTypes {
		t.Run("KeyType_"+kt, func(t *testing.T) {
			res, err := GenerateCA(CAGenRequest{
				CommonName:   "Test Root CA - " + kt,
				Organization: "XPanel Security",
				Country:      "CN",
				KeyType:      kt,
				ValidDays:    365,
			})
			if err != nil {
				t.Fatalf("GenerateCA failed for %s: %v", kt, err)
			}
			if res.CertPEM == "" || res.KeyPEM == "" {
				t.Fatalf("CertPEM or KeyPEM is empty")
			}
			if res.KeyType != kt {
				t.Errorf("Expected key type %s, got %s", kt, res.KeyType)
			}

			// 验证生成的证书
			info, err := ParseCACertInfo(res.CertPEM)
			if err != nil {
				t.Fatalf("ParseCACertInfo failed: %v", err)
			}
			if !info.IsCA {
				t.Errorf("Generated cert is not CA")
			}
			if info.CommonName != "Test Root CA - "+kt {
				t.Errorf("CN mismatch: %s", info.CommonName)
			}
			if info.Organization != "XPanel Security" {
				t.Errorf("Org mismatch: %s", info.Organization)
			}
			if info.Country != "CN" {
				t.Errorf("Country mismatch: %s", info.Country)
			}

			// 验证公私钥匹配
			if err := ValidateCAPair(res.CertPEM, res.KeyPEM); err != nil {
				t.Errorf("ValidateCAPair failed: %v", err)
			}
		})
	}
}

func TestSignServerCert_VerifyAgainstCA(t *testing.T) {
	// 1. 生成 CA
	ca, err := GenerateCA(CAGenRequest{
		CommonName:   "Corp Root CA",
		Organization: "Corp Inc",
		Country:      "US",
		KeyType:      KeyTypeECDSAP256,
		ValidDays:    3650,
	})
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	// 2. 签发服务端证书
	signReq := CertSignRequest{
		CommonName:   "node-01.corp.internal",
		SANDNS:       []string{"node-01.corp.internal", "*.node.corp.internal"},
		SANIP:        []string{"192.168.1.100", "10.0.0.1"},
		Organization: "Corp Nodes",
		Country:      "US",
		KeyType:      KeyTypeECDSAP256,
		ValidDays:    180,
	}

	leafRes, err := SignServerCert(ca.CertPEM, ca.KeyPEM, signReq)
	if err != nil {
		t.Fatalf("SignServerCert failed: %v", err)
	}

	if leafRes.PinSHA256 == "" || len(leafRes.PinSHA256) != 64 {
		t.Errorf("Invalid PinSHA256: %s", leafRes.PinSHA256)
	}

	// 验证 fullchain 包含两张证书
	chain, err := ParseCertificateChain(leafRes.FullchainPEM)
	if err != nil {
		t.Fatalf("ParseCertificateChain failed: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("Expected 2 certs in fullchain, got %d", len(chain))
	}

	leafCert := chain[0]
	caCert := chain[1]

	// 验证 SAN
	if len(leafCert.DNSNames) < 2 {
		t.Errorf("Expected at least 2 DNS names, got %v", leafCert.DNSNames)
	}
	foundWildcard := false
	for _, dns := range leafCert.DNSNames {
		if dns == "*.node.corp.internal" {
			foundWildcard = true
		}
	}
	if !foundWildcard {
		t.Errorf("Wildcard DNS not found in %v", leafCert.DNSNames)
	}

	foundIP := false
	for _, ip := range leafCert.IPAddresses {
		if ip.Equal(net.ParseIP("192.168.1.100")) {
			foundIP = true
		}
	}
	if !foundIP {
		t.Errorf("IP 192.168.1.100 not found in %v", leafCert.IPAddresses)
	}

	// 使用 x509 官方库建立信任池验证 leaf 证书
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	opts := x509.VerifyOptions{
		Roots:       roots,
		CurrentTime: time.Now(),
		DNSName:     "node-01.corp.internal",
	}

	if _, err := leafCert.Verify(opts); err != nil {
		t.Fatalf("leafCert.Verify failed against CA: %v", err)
	}
}

func TestGenerateCustomSelfSigned(t *testing.T) {
	req := SelfSignedRequest{
		CommonName:   "relay-gw.local",
		SANDNS:       []string{"relay-gw.local", "backup.relay.local"},
		SANIP:        []string{"127.0.0.1"},
		Organization: "Internal Relay",
		KeyType:      KeyTypeRSA2048,
		ValidDays:    365,
	}

	res, err := GenerateCustomSelfSigned(req)
	if err != nil {
		t.Fatalf("GenerateCustomSelfSigned failed: %v", err)
	}

	if res.PinSHA256 == "" || len(res.PinSHA256) != 64 {
		t.Errorf("Invalid PinSHA256: %s", res.PinSHA256)
	}

	cert, err := ParseCertificate(res.FullchainPEM)
	if err != nil {
		t.Fatalf("ParseCertificate failed: %v", err)
	}

	if cert.Subject.CommonName != "relay-gw.local" {
		t.Errorf("CN mismatch: %s", cert.Subject.CommonName)
	}

	if !cert.IsCA {
		t.Errorf("Self-signed cert should have IsCA=true")
	}
}

func TestValidateCAPair_PublicKeyOnly(t *testing.T) {
	ca, err := GenerateCA(CAGenRequest{
		CommonName: "Public Only CA",
		KeyType:    KeyTypeECDSAP256,
	})
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	// 纯公钥（keyPEM 为空）校验应通过
	if err := ValidateCAPair(ca.CertPEM, ""); err != nil {
		t.Errorf("ValidateCAPair with empty key should pass: %v", err)
	}

	// 私钥不匹配应报错
	anotherCA, _ := GenerateCA(CAGenRequest{CommonName: "Another CA"})
	if err := ValidateCAPair(ca.CertPEM, anotherCA.KeyPEM); err == nil {
		t.Errorf("ValidateCAPair with mismatched key should fail")
	}
}

func TestSignServerCert_InvalidCA(t *testing.T) {
	// 使用普通非 CA 证书尝试签发
	selfSigned, err := GenerateCustomSelfSigned(SelfSignedRequest{CommonName: "test.local"})
	if err != nil {
		t.Fatal(err)
	}

	// 改写或者传坏 CA
	_, err = SignServerCert("bad-pem", selfSigned.KeyPEM, CertSignRequest{CommonName: "sub.local"})
	if err == nil {
		t.Errorf("Expected error with bad CA PEM")
	}
}
