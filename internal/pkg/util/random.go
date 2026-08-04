package util

import (
	"crypto/rand"
	"encoding/hex"
)

// RandomHex 生成 n 字节的随机 hex 字符串（2n 个字符）。
func RandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// NewSubscribeToken 生成用户订阅 token（32 字节 = 64 hex）。
func NewSubscribeToken() (string, error) { return RandomHex(32) }

// NewInviteCode 生成邀请码（12 字节 = 24 hex，分组展示由前端处理）。
func NewInviteCode() (string, error) { return RandomHex(12) }

// NewNodeSecret 生成节点密钥（32 字节 = 64 hex）。
func NewNodeSecret() (string, error) { return RandomHex(32) }