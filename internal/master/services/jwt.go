// Package services 承载主控业务逻辑。
package services

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/acdc/xray-panel/internal/contracts"
)

// Token 类型（保留 services 别名，兼容既有调用）。
const (
	TokenAccess  = contracts.TokenAccess
	TokenRefresh = contracts.TokenRefresh
)

// claims 内部 JWT 载荷（transient），解析后对外统一返回 contracts.JWTClaims。
type claims struct {
	UserID  uint64 `json:"uid"`
	Role    string `json:"role"`
	Type    string `json:"typ"`  // access | refresh
	Version uint32 `json:"tv"`   // token_version（会话吊销：改密/重置密码/封禁后 bump，refresh 时校验）
	TwoFA   bool   `json:"tfa"`  // 已通过 2FA（完整 access 标记）
	Pending bool   `json:"pend"` // 2FA 待验证临时 access（仅可调 /auth/2fa/verify）
	jwt.RegisteredClaims
}

// JWTManager 负责 access/refresh token 的签发与校验。
type JWTManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewJWTManager 构造 JWT 管理器。
func NewJWTManager(secret string, accessTTL, refreshTTL time.Duration) *JWTManager {
	return &JWTManager{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

// Generate 签发指定类型 token（version = users.token_version，用于会话吊销校验）。
func (m *JWTManager) Generate(userID uint64, role, typ string, version uint32) (string, error) {
	return m.generate(userID, role, typ, version, false, false)
}

// GeneratePending2FA 签发 2FA 待验证临时 access（短 TTL，仅可调 verify 接口）。
func (m *JWTManager) GeneratePending2FA(userID uint64, role string, version uint32) (string, error) {
	return m.generate(userID, role, TokenAccess, version, false, true)
}

// GenerateVerified 签发已通过 2FA 的完整 access。
func (m *JWTManager) GenerateVerified(userID uint64, role string, version uint32) (string, error) {
	return m.generate(userID, role, TokenAccess, version, true, false)
}

func (m *JWTManager) generate(userID uint64, role, typ string, version uint32, twoFA, pending bool) (string, error) {
	ttl := m.accessTTL
	if typ == TokenRefresh {
		ttl = m.refreshTTL
	}
	if pending {
		ttl = 2 * time.Minute
	}
	now := time.Now()
	c := claims{
		UserID:  userID,
		Role:    role,
		Type:    typ,
		Version: version,
		TwoFA:   twoFA,
		Pending: pending,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    "xray-panel",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.secret)
}

// GeneratePair 同时签发 access + refresh。
func (m *JWTManager) GeneratePair(userID uint64, role string, version uint32) (access, refresh string, err error) {
	access, err = m.generate(userID, role, TokenAccess, version, false, false)
	if err != nil {
		return "", "", err
	}
	refresh, err = m.generate(userID, role, TokenRefresh, version, false, false)
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

// Parse 校验并解析 token。
func (m *JWTManager) Parse(tokenStr string) (*contracts.JWTClaims, error) {
	raw := &claims{}
	token, err := jwt.ParseWithClaims(tokenStr, raw, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return &contracts.JWTClaims{
		UserID:  raw.UserID,
		Role:    raw.Role,
		Type:    raw.Type,
		Version: raw.Version,
		TwoFA:   raw.TwoFA,
		Pending: raw.Pending,
	}, nil
}
