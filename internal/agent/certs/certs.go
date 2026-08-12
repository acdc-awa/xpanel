// Package certs 处理主控 push_cert 下发的 TLS 证书落盘。
//
// 约定（05 号文档 §4）：落盘 `/etc/xray/certs/<domain>/{fullchain.pem,key.pem}`，
// fullchain 0644、key 0600，原子写；xray 默认每小时热重载证书文件，换证不重启。
package certs

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// domainRe 允许的域名/标签字符，杜绝路径穿越。
var domainRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)

// ValidatePair 校验证书与私钥：PEM 可解析、证书与私钥公钥匹配。
func ValidatePair(certPEM, keyPEM string) error {
	leaf, err := parseLeaf(certPEM)
	if err != nil {
		return err
	}
	key, err := parseKey(keyPEM)
	if err != nil {
		return err
	}
	switch k := key.(type) {
	case *rsa.PrivateKey:
		if leaf.PublicKey.(*rsa.PublicKey).N.Cmp(k.PublicKey.N) != 0 {
			return fmt.Errorf("证书与 RSA 私钥不匹配")
		}
	case *ecdsa.PrivateKey:
		if leaf.PublicKey.(*ecdsa.PublicKey).X.Cmp(k.PublicKey.X) != 0 || leaf.PublicKey.(*ecdsa.PublicKey).Y.Cmp(k.PublicKey.Y) != 0 {
			return fmt.Errorf("证书与 ECDSA 私钥不匹配")
		}
	default:
		return fmt.Errorf("不支持的私钥类型 %T", key)
	}
	return nil
}

// Write 校验并落盘 <dir>/<domain>/{fullchain.pem,key.pem}（原子写；key 600）。
func Write(dir, domain, certPEM, keyPEM string) error {
	if !domainRe.MatchString(domain) {
		return fmt.Errorf("非法 domain %q（仅允许字母数字 . -）", domain)
	}
	if err := ValidatePair(certPEM, keyPEM); err != nil {
		return err
	}
	sub := filepath.Join(dir, domain)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return fmt.Errorf("创建证书目录失败: %w", err)
	}
	if err := atomicWrite(filepath.Join(sub, "fullchain.pem"), certPEM, 0o644); err != nil {
		return fmt.Errorf("写 fullchain.pem 失败: %w", err)
	}
	if err := atomicWrite(filepath.Join(sub, "key.pem"), keyPEM, 0o600); err != nil {
		return fmt.Errorf("写 key.pem 失败: %w", err)
	}
	return nil
}

// atomicWrite 临时文件 + rename 原子替换。
func atomicWrite(path, content string, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// parseLeaf 解析证书链中的第一张（leaf）证书。
func parseLeaf(certPEM string) (*x509.Certificate, error) {
	rest := []byte(certPEM)
	for {
		block, r := pem.Decode(rest)
		if block == nil {
			return nil, fmt.Errorf("证书 PEM 解析失败")
		}
		rest = r
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("证书解析失败: %w", err)
		}
		if !cert.IsCA {
			return cert, nil
		}
	}
}

// parseKey 解析私钥（PKCS1/PKCS8/EC）。
func parseKey(keyPEM string) (any, error) {
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
