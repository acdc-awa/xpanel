package api

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/certutil"
)

// selfSignedForTest 造一张自签服务端证书（测试用，SAN 与有效期可控）。
func selfSignedForTest(t *testing.T, key crypto.Signer, dns []string, notBefore, notAfter time.Time) (certPEM, keyPEM string) {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: dns[0]},
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
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM
}

func postCert(t *testing.T, deps *Deps, form certForm) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(form)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/certs", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	deps.AdminCreateCert(c)
	return w
}

// TestAdminCreateCert_RejectsSANMismatch：上传的证书必须覆盖它要挂到的域名，
// 否则会推出一张「域名 a、证书 b」的错配记录。
func TestAdminCreateCert_RejectsSANMismatch(t *testing.T) {
	deps, _ := setupTestDeps(t)
	gin.SetMode(gin.TestMode)

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now()
	certPEM, keyPEM := selfSignedForTest(t, key, []string{"good.local"}, now.Add(-time.Hour), now.Add(24*time.Hour))

	w := postCert(t, deps, certForm{Domain: "wrong.local", CertPEM: certPEM, KeyPEM: keyPEM})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("SAN 不匹配应 400，got %d body=%s", w.Code, w.Body.String())
	}
}

// TestAdminCreateCert_RejectsExpired：过期证书不应入库并下发。
func TestAdminCreateCert_RejectsExpired(t *testing.T) {
	deps, _ := setupTestDeps(t)
	gin.SetMode(gin.TestMode)

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now()
	certPEM, keyPEM := selfSignedForTest(t, key, []string{"old.local"}, now.Add(-48*time.Hour), now.Add(-time.Hour))

	w := postCert(t, deps, certForm{Domain: "old.local", CertPEM: certPEM, KeyPEM: keyPEM})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("过期证书应 400，got %d body=%s", w.Code, w.Body.String())
	}
}

// TestAdminCreateCert_RejectsWeakRSA：RSA 1024 不应通过。
func TestAdminCreateCert_RejectsWeakRSA(t *testing.T) {
	deps, _ := setupTestDeps(t)
	gin.SetMode(gin.TestMode)

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	now := time.Now()
	certPEM, keyPEM := selfSignedForTest(t, key, []string{"weak.local"}, now.Add(-time.Hour), now.Add(24*time.Hour))

	w := postCert(t, deps, certForm{Domain: "weak.local", CertPEM: certPEM, KeyPEM: keyPEM})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("RSA 1024 应 400，got %d body=%s", w.Code, w.Body.String())
	}
}

// TestAdminCreateCert_AcceptsMatching：合法证书照常入库（护栏不能误杀）。
func TestAdminCreateCert_AcceptsMatching(t *testing.T) {
	deps, db := setupTestDeps(t)
	gin.SetMode(gin.TestMode)

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now()
	certPEM, keyPEM := selfSignedForTest(t, key, []string{"good.local", "*.good.local"}, now.Add(-time.Hour), now.Add(24*time.Hour))

	w := postCert(t, deps, certForm{Domain: "node-1.good.local", CertPEM: certPEM, KeyPEM: keyPEM})
	if w.Code != http.StatusOK {
		t.Fatalf("通配覆盖应通过，got %d body=%s", w.Code, w.Body.String())
	}
	var count int64
	db.Model(&models.Cert{}).Count(&count)
	if count != 1 {
		t.Fatalf("证书应入库, count=%d", count)
	}
}

// TestAdminIssueCertFromCA_RejectsBadDomain：签发路径也要挡住会落成节点目录名的非法域名。
func TestAdminIssueCertFromCA_RejectsBadDomain(t *testing.T) {
	deps, db := setupTestDeps(t)
	gin.SetMode(gin.TestMode)

	ca, err := certutil.GenerateCA(certutil.CAGenRequest{CommonName: "Issue CA", KeyType: certutil.KeyTypeECDSAP256})
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	row := models.CACert{Name: "issue-ca", CACertPEM: ca.CertPEM, CAKeyPEM: ca.KeyPEM, HasKey: true, NotAfter: ca.NotAfter, KeyType: ca.KeyType}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create CA: %v", err)
	}

	body, _ := json.Marshal(issueCertFromCAForm{Domain: "bad/domain"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/ca-certs/1/issue", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	deps.AdminIssueCertFromCA(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法域名应 400，got %d body=%s", w.Code, w.Body.String())
	}
}
