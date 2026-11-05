package certutil

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"strings"
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
	chain := parseChainPEM(t, leafRes.FullchainPEM)
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
	if cert.KeyUsage&x509.KeyUsageCertSign != 0 {
		t.Error("自签 leaf 不应带 KeyUsageCertSign（它不签发任何证书）")
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

// mustSelfSignedPEM 造一张指定有效期与 SAN 的自签证书（用于边界用例）。
func mustSelfSignedPEM(t *testing.T, key crypto.Signer, notBefore, notAfter time.Time, dns []string) string {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "edge.local"},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              dns,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func mustKeyPEM(t *testing.T, key crypto.Signer) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// TestSignServerCert_RejectsExpiredCA：CA 已过期时必须报错，而不是夹出一个
// NotAfter 落在过去的 leaf。
func TestSignServerCert_RejectsExpiredCA(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	now := time.Now()
	caPEM := mustSelfSignedPEM(t, key, now.Add(-48*time.Hour), now.Add(-24*time.Hour), nil)

	_, err = SignServerCert(caPEM, mustKeyPEM(t, key), CertSignRequest{
		CommonName: "node.local",
		ValidDays:  90,
	})
	if err == nil {
		t.Fatal("过期 CA 应拒绝签发")
	}
	if !strings.Contains(err.Error(), "过期") {
		t.Fatalf("错误应说明 CA 过期，got: %v", err)
	}
}

// TestSignServerCert_RejectsMismatchedCAKey：证书与私钥不是一对时不能签发。
func TestSignServerCert_RejectsMismatchedCAKey(t *testing.T) {
	ca, err := GenerateCA(CAGenRequest{CommonName: "Pair CA", KeyType: KeyTypeECDSAP256})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	other, err := GenerateCA(CAGenRequest{CommonName: "Other CA", KeyType: KeyTypeECDSAP256})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if _, err := SignServerCert(ca.CertPEM, other.KeyPEM, CertSignRequest{CommonName: "node.local"}); err == nil {
		t.Fatal("CA 证书与私钥不匹配时应拒绝签发")
	}
}

// TestValidateServerCert 覆盖下发前的语义闸门：SAN 覆盖、有效期、密钥强度。
func TestValidateServerCert(t *testing.T) {
	ca, err := GenerateCA(CAGenRequest{CommonName: "Validate CA", KeyType: KeyTypeECDSAP256})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	leaf, err := SignServerCert(ca.CertPEM, ca.KeyPEM, CertSignRequest{
		CommonName: "relay.example.com",
		SANDNS:     []string{"*.relay.example.com"},
		SANIP:      []string{"10.0.0.8"},
		ValidDays:  90,
	})
	if err != nil {
		t.Fatalf("SignServerCert: %v", err)
	}

	okCases := []string{"relay.example.com", "node-1.relay.example.com", "10.0.0.8"}
	for _, domain := range okCases {
		if err := ValidateServerCert(leaf.FullchainPEM, domain); err != nil {
			t.Errorf("ValidateServerCert(%q) 应通过: %v", domain, err)
		}
	}
	badCases := []string{"other.example.com", "deep.node.relay.example.com", "10.0.0.9"}
	for _, domain := range badCases {
		if err := ValidateServerCert(leaf.FullchainPEM, domain); err == nil {
			t.Errorf("ValidateServerCert(%q) 应拒绝（SAN 未覆盖）", domain)
		}
	}

	// 已过期
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now()
	expired := mustSelfSignedPEM(t, key, now.Add(-48*time.Hour), now.Add(-time.Hour), []string{"old.local"})
	if err := ValidateServerCert(expired, "old.local"); err == nil {
		t.Error("过期证书应拒绝")
	}

	// 弱密钥：RSA 1024
	rsaKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	weak := mustSelfSignedPEM(t, rsaKey, now.Add(-time.Hour), now.Add(24*time.Hour), []string{"weak.local"})
	if err := ValidateServerCert(weak, "weak.local"); err == nil {
		t.Error("RSA 1024 应被拒绝")
	}
}

// parseChainPEM 解析 PEM 证书链（测试专用：生产侧只签发/校验单张证书，不需要链解析）。
func parseChainPEM(t *testing.T, certPEM string) []*x509.Certificate {
	t.Helper()
	var certs []*x509.Certificate
	rest := []byte(certPEM)
	for {
		block, r := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = r
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("解析证书链失败: %v", err)
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		t.Fatal("未找到有效证书")
	}
	return certs
}
