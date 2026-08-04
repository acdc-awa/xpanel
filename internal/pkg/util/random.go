package util

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
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

// RandomID 生成短随机 ID（请求配对用，n 字节 hex）。
func RandomID(n int) string {
	s, err := RandomHex(n)
	if err != nil {
		// crypto/rand 几乎不会失败，兜底用时间戳
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return s
}