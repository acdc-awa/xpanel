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

// GenerateSecurePassword 生成指定长度的高强度安全随机密码（包含大写、小写、数字与特殊符号）。
func GenerateSecurePassword(length int) string {
	if length < 8 {
		length = 16
	}
	const (
		upper   = "ABCDEFGHJKLMNPQRSTUVWXYZ" // 排除易混淆的 I, O
		lower   = "abcdefghijkmnopqrstuvwxyz" // 排除易混淆的 l
		digits  = "23456789"                 // 排除易混淆的 0, 1
		symbols = "!@#$%^&*"
		all     = upper + lower + digits + symbols
	)

	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		// 极端兜底
		return fmt.Sprintf("Xray%d#Admin!", time.Now().Unix())
	}

	for i := range b {
		b[i] = all[int(b[i])%len(all)]
	}

	// 保证各字符集至少出现一次
	b[0] = upper[int(b[0])%len(upper)]
	b[1] = lower[int(b[1])%len(lower)]
	b[2] = digits[int(b[2])%len(digits)]
	b[3] = symbols[int(b[3])%len(symbols)]

	return string(b)
}

