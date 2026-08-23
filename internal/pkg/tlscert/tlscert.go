// Package tlscert 提供证书/私钥校验与解析（主控 certs API 与 agent push_cert 落盘共用）。
package tlscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"regexp"
	"strings"
	"time"
)

// DomainRe 允许的域名/标签字符，杜绝路径穿越。
var DomainRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)

// ValidatePair 校验证书链与私钥：PEM 可解析、链中存在与私钥公钥匹配的证书
// （自签证书 IsCA=true 也是合法 leaf，不能按 IsCA 跳过）。
func ValidatePair(certPEM, keyPEM string) error {
	certs, err := ParseChain(certPEM)
	if err != nil {
		return err
	}
	key, err := ParseKey(keyPEM)
	if err != nil {
		return err
	}
	for _, leaf := range certs {
		switch k := key.(type) {
		case *rsa.PrivateKey:
			pub, ok := leaf.PublicKey.(*rsa.PublicKey)
			if ok && pub.N.Cmp(k.PublicKey.N) == 0 {
				return nil
			}
		case *ecdsa.PrivateKey:
			pub, ok := leaf.PublicKey.(*ecdsa.PublicKey)
			if ok && pub.X.Cmp(k.PublicKey.X) == 0 && pub.Y.Cmp(k.PublicKey.Y) == 0 {
				return nil
			}
		default:
			return fmt.Errorf("不支持的私钥类型 %T", key)
		}
	}
	return fmt.Errorf("证书与私钥不匹配")
}

// ParseChain 解析证书链中的全部 CERTIFICATE 块。
func ParseChain(certPEM string) ([]*x509.Certificate, error) {
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
			return nil, fmt.Errorf("证书解析失败: %w", err)
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("证书 PEM 解析失败")
	}
	return certs, nil
}

// ParseLeaf 解析证书链中的第一张证书（leaf；自签/CA 标记不影响）。
func ParseLeaf(certPEM string) (*x509.Certificate, error) {
	certs, err := ParseChain(certPEM)
	if err != nil {
		return nil, err
	}
	return certs[0], nil
}

// NotAfter 返回证书链 leaf 的到期时间（用于 certs 表）。
func NotAfter(certPEM string) (time.Time, error) {
	leaf, err := ParseLeaf(certPEM)
	if err != nil {
		return time.Time{}, err
	}
	return leaf.NotAfter, nil
}

// ParseKey 解析私钥（PKCS1/PKCS8/EC）。
func ParseKey(keyPEM string) (any, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, fmt.Errorf("私钥 PEM 解析失败")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("私钥解析失败（支持 PKCS1/PKCS8/EC）")
}

// SanitizeDomain 供日志使用：去除换行等控制字符。
func SanitizeDomain(domain string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, domain)
}

// GenerateSelfSigned 生成自签证书（链式代理 TLS + pinnedPeerCertSha256 场景）：
// ECDSA P-256、10 年期、IsCA 自签根形态。安全性由中转出站 pin 哈希保证（pin 命中即
// 验证通过，不依赖 CA 信任链），自签不降低链路安全性。
func GenerateSelfSigned(domain string) (certPEM, keyPEM string, err error) {
	if !DomainRe.MatchString(domain) {
		return "", "", fmt.Errorf("非法 domain %q（仅允许字母数字 . -）", domain)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("生成私钥失败: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("生成序列号失败: %w", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: domain, Organization: []string{"xray-panel relay"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	if ip := net.ParseIP(domain); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{domain}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("签发证书失败: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("编码私钥失败: %w", err)
	}
	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	return certPEM, keyPEM, nil
}

// PinSHA256Hex 计算证书链 leaf 的 SHA-256（DER 原始字节，小写 hex）——
// 与 xray tlsSettings.pinnedPeerCertSha256 的格式约定一致（v26.6.27 实测）。
func PinSHA256Hex(certPEM string) (string, error) {
	leaf, err := ParseLeaf(certPEM)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(leaf.Raw)
	return hex.EncodeToString(sum[:]), nil
}
