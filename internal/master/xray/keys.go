package xray

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// GenerateKeys 生成 REALITY 的 X25519 密钥对（RawURLEncoding base64）。
func GenerateKeys() (privateKey, publicKey string, err error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("生成 X25519 密钥失败: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(priv.Bytes()),
		base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()), nil
}

// GenerateShortID 生成 REALITY 的 8 字节随机 shortId（16 个 hex 字符）。
func GenerateShortID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "6ba7b8109abc4def" // 兜底
	}
	return hex.EncodeToString(b)
}
