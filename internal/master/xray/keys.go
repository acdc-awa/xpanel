package xray

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// GenerateKeys 生成 REALITY 的 X25519 密钥对（RawURLEncoding base64，与 `xray x25519` 输出一致；
// xray-core REALITY 要求无填充 URL-safe 编码，StdEncoding 会被拒绝）。
func GenerateKeys() (privateKey, publicKey string, err error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("生成 X25519 密钥失败: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(priv.Bytes()),
		base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()), nil
}