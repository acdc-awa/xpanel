package services

import (
	"context"
	"errors"
	"time"

	"github.com/alexedwards/argon2id"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// 业务错误（handler 层映射为 HTTP 状态码）
var (
	ErrUserExists     = errors.New("用户名或邮箱已存在")
	ErrInviteInvalid  = errors.New("邀请码无效或已使用")
	ErrInvalidCreds   = errors.New("用户名或密码错误")
	ErrUserDisabled   = errors.New("账号已被禁用")
	ErrInvalidRefresh = errors.New("无效的刷新令牌")
)

// AuthService 注册/登录/令牌签发。
type AuthService struct {
	DB             *gorm.DB
	JWT            *JWTManager
	InviteRequired bool
}

// RegisterReq 注册请求。
type RegisterReq struct {
	Username   string `json:"username" binding:"required,min=3,max=32"`
	Email      string `json:"email" binding:"omitempty,email,max=128"`
	Password   string `json:"password" binding:"required,min=8,max=72"`
	InviteCode string `json:"invite_code"`
}

// Register 注册新用户（邀请码制）。
func (s *AuthService) Register(ctx context.Context, req *RegisterReq) (*models.User, error) {
	// 邀请码校验（若开启）
	if s.InviteRequired || req.InviteCode != "" {
		if err := s.consumeInvite(ctx, req.InviteCode); err != nil {
			return nil, err
		}
	}

	// 用户名/邮箱查重
	var cnt int64
	if err := s.DB.WithContext(ctx).Model(&models.User{}).
		Where("username = ? OR email = ?", req.Username, req.Email).Count(&cnt).Error; err != nil {
		return nil, err
	}
	if cnt > 0 {
		return nil, ErrUserExists
	}

	hash, err := argon2id.CreateHash(req.Password, argon2id.DefaultParams)
	if err != nil {
		return nil, err
	}
	token, err := util.NewSubscribeToken()
	if err != nil {
		return nil, err
	}
	uuid, err := util.NewUUID()
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Username:       req.Username,
		Email:          req.Email,
		UUID:           uuid,
		PasswordHash:   hash,
		Role:           models.RoleUser,
		Status:         models.StatusActive,
		SubscribeToken: token,
	}
	if err := s.DB.WithContext(ctx).Create(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

// consumeInvite 校验并核销邀请码（事务内：状态校验 → 使用）。
func (s *AuthService) consumeInvite(ctx context.Context, code string) error {
	if code == "" {
		return ErrInviteInvalid
	}
	var inv models.InvitationCode
	if err := s.DB.WithContext(ctx).Where("code = ?", code).First(&inv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInviteInvalid
		}
		return err
	}
	if inv.Status != models.InviteUnused {
		return ErrInviteInvalid
	}
	if inv.ExpiresAt != nil && time.Now().After(*inv.ExpiresAt) {
		return ErrInviteInvalid
	}
	return s.DB.WithContext(ctx).Model(&inv).Updates(map[string]any{
		"status":   models.InviteUsed,
		"used_at":  time.Now(),
	}).Error
}

// Login 校验用户名密码与账号状态。
func (s *AuthService) Login(ctx context.Context, username, password string) (*models.User, error) {
	var user models.User
	if err := s.DB.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCreds
		}
		return nil, err
	}
	match, err := argon2id.ComparePasswordAndHash(password, user.PasswordHash)
	if err != nil || !match {
		return nil, ErrInvalidCreds
	}
	if user.Status != models.StatusActive {
		return nil, ErrUserDisabled
	}
	return &user, nil
}

// IssueTokens 签发 access + refresh。
func (s *AuthService) IssueTokens(user *models.User) (access, refresh string, err error) {
	return s.JWT.GeneratePair(user.ID, user.Role)
}

// Refresh 用 refresh token 换取新 access token。
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (string, error) {
	claims, err := s.JWT.Parse(refreshToken)
	if err != nil || claims.Type != TokenRefresh {
		return "", ErrInvalidRefresh
	}
	// 校验用户仍存在且可用
	var user models.User
	if err := s.DB.WithContext(ctx).First(&user, claims.UserID).Error; err != nil {
		return "", ErrInvalidRefresh
	}
	if user.Status != models.StatusActive {
		return "", ErrUserDisabled
	}
	return s.JWT.Generate(user.ID, user.Role, TokenAccess)
}