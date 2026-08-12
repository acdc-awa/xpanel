package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// genCert 生成自签证书（ECDSA P-256，测试用）。
func genCert(t *testing.T, cn string) (certPEM, keyPEM string, leaf *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		IsCA:         false,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM, leaf
}

func TestValidatePair(t *testing.T) {
	certPEM, keyPEM, _ := genCert(t, "test.local")
	if err := ValidatePair(certPEM, keyPEM); err != nil {
		t.Fatalf("匹配的证书/私钥应通过: %v", err)
	}
}

func TestValidatePairMismatch(t *testing.T) {
	certPEM, _, _ := genCert(t, "a.local")
	_, keyPEM2, _ := genCert(t, "b.local")
	if err := ValidatePair(certPEM, keyPEM2); err == nil {
		t.Error("不匹配的证书/私钥应报错")
	}
}

func TestValidatePairBadPEM(t *testing.T) {
	if err := ValidatePair("not-pem", "not-key"); err == nil {
		t.Error("非法 PEM 应报错")
	}
}

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	certPEM, keyPEM, _ := genCert(t, "test.local")

	if err := Write(dir, "test.local", certPEM, keyPEM); err != nil {
		t.Fatalf("Write: %v", err)
	}
	sub := filepath.Join(dir, "test.local")
	fullchain := filepath.Join(sub, "fullchain.pem")
	key := filepath.Join(sub, "key.pem")
	for _, p := range []string{fullchain, key} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("缺少 %s: %v", p, err)
		}
	}
	// POSIX 权限位仅 Linux 有效（Windows 恒 0666）
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(key); info.Mode().Perm() != 0o600 {
			t.Errorf("key.pem 权限 = %o, want 600", info.Mode().Perm())
		}
		if info, _ := os.Stat(fullchain); info.Mode().Perm() != 0o644 {
			t.Errorf("fullchain.pem 权限 = %o, want 644", info.Mode().Perm())
		}
	}
	// 内容一致
	data, _ := os.ReadFile(fullchain)
	if string(data) != certPEM {
		t.Error("fullchain 内容不一致")
	}
}

func TestWriteRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	certPEM, keyPEM, _ := genCert(t, "x.local")
	for _, domain := range []string{"../evil", "a/b", "..", "/etc/passwd", ""} {
		if err := Write(dir, domain, certPEM, keyPEM); err == nil {
			t.Errorf("非法 domain %q 应报错", domain)
		}
	}
	// 未发生越界写入
	if _, err := os.Stat(filepath.Join(dir, "evil")); !os.IsNotExist(err) {
		t.Error("不应创建越界目录")
	}
}

func TestSanitizeDomain(t *testing.T) {
	if got := SanitizeDomain("a\nb"); strings.Contains(got, "\n") {
		t.Errorf("SanitizeDomain 未去除控制字符: %q", got)
	}
}
