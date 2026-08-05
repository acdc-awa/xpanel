package util

import (
	"crypto/sha256"
	"encoding/hex"
)

// HashSecret 对节点密钥做 sha256 摘要存储（管理端只在创建时返回一次明文）。
func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}
