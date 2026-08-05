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

func (s *AuthService) Register(ctx context.Context, req *RegisterReq) (*models.User, error) {
	var user *models.User
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if s.InviteRequired || req.InviteCode != "" {
			if err := s.consumeInviteTx(ctx, tx, req.InviteCode); err != nil {
				return err
			}
		}

		var cnt int64
		if err := tx.Model(&models.User{}).
			Where("username = ? OR email = ?", req.Username, req.Email).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			return ErrUserExists
		}

		hash, err := argon2id.CreateHash(req.Password, argon2id.DefaultParams)
		if err != nil {
			return err
		}
		token, err := util.NewSubscribeToken()
		if err != nil {
			return err
		}
		uuid, err := util.NewUUID()
		if err != nil {
			return err
		}

		user = &models.User{
			Username:       req.Username,
			Email:          req.Email,
			UUID:           uuid,
			PasswordHash:   hash,
			Role:           models.RoleUser,
			Status:         models.StatusActive,
			SubscribeToken: token,
		}
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}

// consumeInviteTx 校验并核销邀请码（事务内：状态校验 → 使用）。
func (s *AuthService) consumeInviteTx(ctx context.Context, tx *gorm.DB, code string) error {
	if code == "" {
		return ErrInviteInvalid
	}
	var inv models.InvitationCode
	if err := tx.WithContext(ctx).Where("code = ?", code).First(&inv).Error; err != nil {
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
	return tx.WithContext(ctx).Model(&inv).Updates(map[string]any{
		"status":  models.InviteUsed,
		"used_at": time.Now(),
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

// ChangePassword 修改密码（校验旧密码）。
func (s *AuthService) ChangePassword(ctx context.Context, userID uint64, oldPwd, newPwd string) error {
	var user models.User
	if err := s.DB.WithContext(ctx).First(&user, userID).Error; err != nil {
		return errors.New("用户不存在")
	}
	match, err := argon2id.ComparePasswordAndHash(oldPwd, user.PasswordHash)
	if err != nil || !match {
		return errors.New("当前密码错误")
	}
	hash, err := argon2id.CreateHash(newPwd, argon2id.DefaultParams)
	if err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Model(&user).Updates(map[string]any{
		"password_hash":   hash,
		"must_change_pwd": false,
	}).Error
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
