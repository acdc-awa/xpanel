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

// NewUUID 生成 UUID v4（用于 VLESS 用户账号）。
func NewUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// NewGiftCardCode 生成礼品卡卡密（形如 GIFT-XXXX-XXXX-XXXX-XXXX）。
func NewGiftCardCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	h := hex.EncodeToString(b)
	return fmt.Sprintf("GIFT-%s-%s-%s-%s", h[0:4], h[4:8], h[8:12], h[12:16]), nil
}

// RandomID 生成短随机 ID（请求配对用，n 字节 hex）。
func RandomID(n int) string {
	s, err := RandomHex(n)
	if err != nil {
		// crypto/rand 几乎不会失败，兜底用时间戳
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return s
}

