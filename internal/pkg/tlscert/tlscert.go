// Package tlscert 提供证书/私钥校验与解析（主控 certs API 与 agent push_cert 落盘共用）。
package tlscert

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
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
