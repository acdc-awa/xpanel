// Package services 承载主控业务逻辑。
package services

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Token 类型
const (
	TokenAccess  = "access"
	TokenRefresh = "refresh"
)

// Claims JWT 载荷。
type Claims struct {
	UserID uint64 `json:"uid"`
	Role   string `json:"role"`
	Type   string `json:"typ"` // access | refresh
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

// Generate 签发指定类型 token。
func (m *JWTManager) Generate(userID uint64, role, typ string) (string, error) {
	ttl := m.accessTTL
	if typ == TokenRefresh {
		ttl = m.refreshTTL
	}
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Role:   role,
		Type:   typ,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			Issuer:    "xray-panel",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// GeneratePair 同时签发 access + refresh。
func (m *JWTManager) GeneratePair(userID uint64, role string) (access, refresh string, err error) {
	access, err = m.Generate(userID, role, TokenAccess)
	if err != nil {
		return "", "", err
	}
	refresh, err = m.Generate(userID, role, TokenRefresh)
	if err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

// Parse 校验并解析 token。
func (m *JWTManager) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
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
	return claims, nil
}