package xray

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"strings"
	"testing"
)

// TestGenerateKeys_Format 验证生成的 REALITY 密钥与 `xray x25519` 输出格式一致
// （RawURLEncoding：43 字符、无填充、无 +/ 与 = 字符），并确保能解码回 32 字节。
func TestGenerateKeys_Format(t *testing.T) {
	priv, pub, err := GenerateKeys()
	if err != nil {
		t.Fatalf("GenerateKeys() 返回错误: %v", err)
	}
	if priv == "" || pub == "" {
		t.Fatalf("GenerateKeys() 返回空密钥: priv=%q pub=%q", priv, pub)
	}
	for name, key := range map[string]string{"private": priv, "public": pub} {
		if strings.ContainsAny(key, "+/=") {
			t.Errorf("%s 密钥含 StdEncoding 特征字符，xray REALITY 将拒绝: %q", name, key)
		}
		if len(key) != 43 {
			t.Errorf("%s 密钥长度 = %d，应为 43（RawURLEncoding 32 字节）: %q", name, len(key), key)
		}
		b, err := base64.RawURLEncoding.DecodeString(key)
		if err != nil {
			t.Errorf("%s 密钥无法按 RawURLEncoding 解码: %v", name, err)
			continue
		}
		if len(b) != 32 {
			t.Errorf("%s 密钥解码后长度 = %d，应为 32", name, len(b))
		}
	}
}

// TestGenerateKeys_KeyPairMatch 验证私钥与公钥属于同一 X25519 密钥对（xray 校验私钥+公钥一致性）。
func TestGenerateKeys_KeyPairMatch(t *testing.T) {
	priv, pub, err := GenerateKeys()
	if err != nil {
		t.Fatalf("GenerateKeys() 返回错误: %v", err)
	}
	privB, err := base64.RawURLEncoding.DecodeString(priv)
	if err != nil {
		t.Fatalf("私钥解码失败: %v", err)
	}
	pubB, err := base64.RawURLEncoding.DecodeString(pub)
	if err != nil {
		t.Fatalf("公钥解码失败: %v", err)
	}
	k, err := ecdh.X25519().NewPrivateKey(privB)
	if err != nil {
		t.Fatalf("私钥解析失败: %v", err)
	}
	if got := k.PublicKey().Bytes(); !bytes.Equal(got, pubB) {
		t.Errorf("公钥与私钥不匹配: 由私钥推导 %x，返回 %x", got, pubB)
	}
}
