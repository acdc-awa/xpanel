package certutil

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strings"
	"time"
)

// 支持的密钥类型常量
const (
	KeyTypeECDSAP256 = "ecdsa_p256"
	KeyTypeECDSAP384 = "ecdsa_p384"
	KeyTypeRSA2048   = "rsa_2048"
	KeyTypeRSA4096   = "rsa_4096"
	KeyTypeEd25519   = "ed25519"
)

// CAGenRequest 生成根 CA 请求
type CAGenRequest struct {
	CommonName   string `json:"common_name"`
	Organization string `json:"organization"`
	Country      string `json:"country"`
	KeyType      string `json:"key_type"`   // 默认 ecdsa_p256
	ValidDays    int    `json:"valid_days"` // 默认 3650 (10年)
}

// CAGenResult 生成根 CA 返回结果
type CAGenResult struct {
	CertPEM      string    `json:"cert_pem"`
	KeyPEM       string    `json:"key_pem"`
	CommonName   string    `json:"common_name"`
	Organization string    `json:"organization"`
	Country      string    `json:"country"`
	NotAfter     time.Time `json:"not_after"`
	KeyType      string    `json:"key_type"`
}

// CertSignRequest 基于 CA 签发服务端证书请求
type CertSignRequest struct {
	CommonName   string   `json:"common_name"`
	SANDNS       []string `json:"san_dns"` // DNSNames
	SANIP        []string `json:"san_ip"`  // IPAddresses
	Organization string   `json:"organization"`
	Country      string   `json:"country"`
	KeyType      string   `json:"key_type"`   // 默认 ecdsa_p256
	ValidDays    int      `json:"valid_days"` // 默认 365
}

// CertSignResult 签发服务端证书返回结果
type CertSignResult struct {
	FullchainPEM string    `json:"fullchain_pem"` // leaf + ca
	LeafCertPEM  string    `json:"leaf_cert_pem"` // 仅 leaf
	KeyPEM       string    `json:"key_pem"`
	PinSHA256    string    `json:"pin_sha256"` // leaf DER SHA-256 (hex)
	NotAfter     time.Time `json:"not_after"`
	KeyType      string    `json:"key_type"`
}

// SelfSignedRequest 自签证书高级生成请求
type SelfSignedRequest struct {
	CommonName   string   `json:"common_name"`
	SANDNS       []string `json:"san_dns"`
	SANIP        []string `json:"san_ip"`
	Organization string   `json:"organization"`
	Country      string   `json:"country"`
	KeyType      string   `json:"key_type"`   // 默认 ecdsa_p256
	ValidDays    int      `json:"valid_days"` // 默认 3650
}

// CAInfo 解析 CA 证书信息
type CAInfo struct {
	CommonName   string
	Organization string
	Country      string
	NotAfter     time.Time
	KeyType      string
	IsCA         bool
}

// generateKeyPair 根据 KeyType 生成私钥与公钥
func generateKeyPair(keyType string) (crypto.PrivateKey, crypto.PublicKey, string, error) {
	norm := strings.ToLower(strings.TrimSpace(keyType))
	if norm == "" {
		norm = KeyTypeECDSAP256
	}
	switch norm {
	case KeyTypeECDSAP256:
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, "", err
		}
		return k, &k.PublicKey, KeyTypeECDSAP256, nil
	case KeyTypeECDSAP384:
		k, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
		if err != nil {
			return nil, nil, "", err
		}
		return k, &k.PublicKey, KeyTypeECDSAP384, nil
	case KeyTypeRSA2048:
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, nil, "", err
		}
		return k, &k.PublicKey, KeyTypeRSA2048, nil
	case KeyTypeRSA4096:
		k, err := rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return nil, nil, "", err
		}
		return k, &k.PublicKey, KeyTypeRSA4096, nil
	case KeyTypeEd25519:
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, nil, "", err
		}
		return priv, pub, KeyTypeEd25519, nil
	default:
		return nil, nil, "", fmt.Errorf("不支持的密钥类型: %s", keyType)
	}
}

// encodePrivateKeyToPEM 将私钥序列化为 PKCS8 PEM 格式
func encodePrivateKeyToPEM(key crypto.PrivateKey) (string, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", fmt.Errorf("序列化私钥失败: %w", err)
	}
	block := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	}
	return string(pem.EncodeToMemory(block)), nil
}

// ParsePrivateKey 解析私钥 PEM（兼容 PKCS8, PKCS1, EC）
func ParsePrivateKey(keyPEM string) (crypto.PrivateKey, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, errors.New("私钥 PEM 解析失败，未找到有效块")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	return nil, errors.New("无法识别的私钥格式（支持 PKCS8/PKCS1/EC）")
}

// ParseCertificate 解析首张证书
func ParseCertificate(certPEM string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("证书 PEM 解析失败，未找到有效 CERTIFICATE 块")
	}
	return x509.ParseCertificate(block.Bytes)
}

// ParseCertificateChain 解析证书链
func ParseCertificateChain(certPEM string) ([]*x509.Certificate, error) {
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
			return nil, fmt.Errorf("证书链解析失败: %w", err)
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, errors.New("未找到有效证书")
	}
	return certs, nil
}

// DetectKeyType 判断公钥类型
func DetectKeyType(pub crypto.PublicKey) string {
	switch p := pub.(type) {
	case *ecdsa.PublicKey:
		if p.Curve == elliptic.P256() {
			return KeyTypeECDSAP256
		} else if p.Curve == elliptic.P384() {
			return KeyTypeECDSAP384
		}
		return "ecdsa"
	case *rsa.PublicKey:
		if p.N.BitLen() == 2048 {
			return KeyTypeRSA2048
		} else if p.N.BitLen() == 4096 {
			return KeyTypeRSA4096
		}
		return fmt.Sprintf("rsa_%d", p.N.BitLen())
	case ed25519.PublicKey:
		return KeyTypeEd25519
	default:
		return "unknown"
	}
}

// GenerateCA 生成根 CA
func GenerateCA(req CAGenRequest) (*CAGenResult, error) {
	priv, pub, actualKeyType, err := generateKeyPair(req.KeyType)
	if err != nil {
		return nil, err
	}

	days := req.ValidDays
	if days <= 0 {
		days = 3650 // 默认 10 年
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("生成序列号失败: %w", err)
	}

	subject := pkix.Name{
		CommonName: req.CommonName,
	}
	if req.Organization != "" {
		subject.Organization = []string{req.Organization}
	}
	if req.Country != "" {
		subject.Country = []string{req.Country}
	}

	notBefore := time.Now().Add(-1 * time.Hour)
	notAfter := time.Now().Add(time.Duration(days) * 24 * time.Hour)

	tmpl := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        false,
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, pub, priv)
	if err != nil {
		return nil, fmt.Errorf("签发 CA 证书失败: %w", err)
	}

	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM, err := encodePrivateKeyToPEM(priv)
	if err != nil {
		return nil, err
	}

	return &CAGenResult{
		CertPEM:      certPEM,
		KeyPEM:       keyPEM,
		CommonName:   req.CommonName,
		Organization: req.Organization,
		Country:      req.Country,
		NotAfter:     notAfter,
		KeyType:      actualKeyType,
	}, nil
}

// SignServerCert 使用 CA 签发服务端证书
func SignServerCert(caCertPEM, caKeyPEM string, req CertSignRequest) (*CertSignResult, error) {
	caCert, err := ParseCertificate(caCertPEM)
	if err != nil {
		return nil, fmt.Errorf("解析 CA 证书失败: %w", err)
	}
	if !caCert.IsCA {
		return nil, errors.New("提供的证书不是 CA 证书（IsCA 为 false）")
	}

	caKey, err := ParsePrivateKey(caKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("解析 CA 私钥失败: %w", err)
	}

	leafPriv, leafPub, actualKeyType, err := generateKeyPair(req.KeyType)
	if err != nil {
		return nil, err
	}

	days := req.ValidDays
	if days <= 0 {
		days = 365
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("生成序列号失败: %w", err)
	}

	subject := pkix.Name{
		CommonName: req.CommonName,
	}
	if req.Organization != "" {
		subject.Organization = []string{req.Organization}
	}
	if req.Country != "" {
		subject.Country = []string{req.Country}
	}

	notBefore := time.Now().Add(-1 * time.Hour)
	notAfter := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	if notAfter.After(caCert.NotAfter) {
		notAfter = caCert.NotAfter // 不能超过 CA 到期时间
	}

	// 整理 SAN
	dnsNames := make([]string, 0, len(req.SANDNS)+1)
	if req.CommonName != "" && !strings.Contains(req.CommonName, " ") && net.ParseIP(req.CommonName) == nil {
		dnsNames = append(dnsNames, req.CommonName)
	}
	for _, d := range req.SANDNS {
		trimmed := strings.TrimSpace(d)
		if trimmed != "" && !contains(dnsNames, trimmed) {
			dnsNames = append(dnsNames, trimmed)
		}
	}

	var ipAddresses []net.IP
	if ip := net.ParseIP(req.CommonName); ip != nil {
		ipAddresses = append(ipAddresses, ip)
	}
	for _, ipStr := range req.SANIP {
		trimmed := strings.TrimSpace(ipStr)
		if trimmed != "" {
			if ip := net.ParseIP(trimmed); ip != nil {
				ipAddresses = append(ipAddresses, ip)
			}
		}
	}

	tmpl := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, caCert, leafPub, caKey)
	if err != nil {
		return nil, fmt.Errorf("签发服务端证书失败: %w", err)
	}

	leafCertPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM, err := encodePrivateKeyToPEM(leafPriv)
	if err != nil {
		return nil, err
	}

	// 组装 fullchain：leaf + CA 证书
	fullchainPEM := strings.TrimSpace(leafCertPEM) + "\n" + strings.TrimSpace(caCertPEM) + "\n"

	// 计算 leaf 证书 PinSHA256 (hex)
	sum := sha256.Sum256(der)
	pinHex := hex.EncodeToString(sum[:])

	return &CertSignResult{
		FullchainPEM: fullchainPEM,
		LeafCertPEM:  leafCertPEM,
		KeyPEM:       keyPEM,
		PinSHA256:    pinHex,
		NotAfter:     notAfter,
		KeyType:      actualKeyType,
	}, nil
}

// GenerateCustomSelfSigned 生成高级自签证书
func GenerateCustomSelfSigned(req SelfSignedRequest) (*CertSignResult, error) {
	priv, pub, actualKeyType, err := generateKeyPair(req.KeyType)
	if err != nil {
		return nil, err
	}

	days := req.ValidDays
	if days <= 0 {
		days = 3650
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("生成序列号失败: %w", err)
	}

	subject := pkix.Name{
		CommonName: req.CommonName,
	}
	if req.Organization != "" {
		subject.Organization = []string{req.Organization}
	} else {
		subject.Organization = []string{"xray-panel relay"}
	}
	if req.Country != "" {
		subject.Country = []string{req.Country}
	}

	notBefore := time.Now().Add(-1 * time.Hour)
	notAfter := time.Now().Add(time.Duration(days) * 24 * time.Hour)

	dnsNames := make([]string, 0, len(req.SANDNS)+1)
	if req.CommonName != "" && !strings.Contains(req.CommonName, " ") && net.ParseIP(req.CommonName) == nil {
		dnsNames = append(dnsNames, req.CommonName)
	}
	for _, d := range req.SANDNS {
		trimmed := strings.TrimSpace(d)
		if trimmed != "" && !contains(dnsNames, trimmed) {
			dnsNames = append(dnsNames, trimmed)
		}
	}

	var ipAddresses []net.IP
	if ip := net.ParseIP(req.CommonName); ip != nil {
		ipAddresses = append(ipAddresses, ip)
	}
	for _, ipStr := range req.SANIP {
		trimmed := strings.TrimSpace(ipStr)
		if trimmed != "" {
			if ip := net.ParseIP(trimmed); ip != nil {
				ipAddresses = append(ipAddresses, ip)
			}
		}
	}

	tmpl := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject:               subject,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true, // 自签证书标记 IsCA=true 兼容作为信任锚点
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, pub, priv)
	if err != nil {
		return nil, fmt.Errorf("生成自签证书失败: %w", err)
	}

	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM, err := encodePrivateKeyToPEM(priv)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(der)
	pinHex := hex.EncodeToString(sum[:])

	return &CertSignResult{
		FullchainPEM: certPEM,
		LeafCertPEM:  certPEM,
		KeyPEM:       keyPEM,
		PinSHA256:    pinHex,
		NotAfter:     notAfter,
		KeyType:      actualKeyType,
	}, nil
}

// ParseCACertInfo 解析 CA 证书详情
func ParseCACertInfo(certPEM string) (*CAInfo, error) {
	cert, err := ParseCertificate(certPEM)
	if err != nil {
		return nil, err
	}
	org := ""
	if len(cert.Subject.Organization) > 0 {
		org = cert.Subject.Organization[0]
	}
	country := ""
	if len(cert.Subject.Country) > 0 {
		country = cert.Subject.Country[0]
	}
	return &CAInfo{
		CommonName:   cert.Subject.CommonName,
		Organization: org,
		Country:      country,
		NotAfter:     cert.NotAfter,
		KeyType:      DetectKeyType(cert.PublicKey),
		IsCA:         cert.IsCA,
	}, nil
}

// ValidateCAPair 校验证书与私钥匹配性
func ValidateCAPair(certPEM, keyPEM string) error {
	cert, err := ParseCertificate(certPEM)
	if err != nil {
		return err
	}
	if !cert.IsCA {
		return errors.New("该证书未包含 CA 标记 (BasicConstraints.IsCA=true)")
	}
	if strings.TrimSpace(keyPEM) == "" {
		return nil // 无私钥（纯公钥验签 CA 模式）
	}
	key, err := ParsePrivateKey(keyPEM)
	if err != nil {
		return err
	}
	switch k := key.(type) {
	case *rsa.PrivateKey:
		pub, ok := cert.PublicKey.(*rsa.PublicKey)
		if ok && pub.N.Cmp(k.PublicKey.N) == 0 {
			return nil
		}
	case *ecdsa.PrivateKey:
		pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
		if ok && pub.X.Cmp(k.PublicKey.X) == 0 && pub.Y.Cmp(k.PublicKey.Y) == 0 {
			return nil
		}
	case ed25519.PrivateKey:
		pub, ok := cert.PublicKey.(ed25519.PublicKey)
		if ok && pub.Equal(k.Public()) {
			return nil
		}
	default:
		return fmt.Errorf("不支持的私钥类型: %T", key)
	}
	return errors.New("CA 证书与私钥公钥不匹配")
}

func contains(arr []string, s string) bool {
	for _, a := range arr {
		if a == s {
			return true
		}
	}
	return false
}
