package services

import (
	"context"
	"errors"
	"strings"
	"sync"
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
	ErrRegisterClosed = errors.New("注册已关闭")
)

// AuthService 注册/登录/令牌签发。
type AuthService struct {
	DB  *gorm.DB
	JWT *JWTManager
}

// RegisterReq 注册请求（2026-08-14 方向①：用户名=邮箱必填 + 邀请码必填；方向②：turnstile_token 人机验证）。
type RegisterReq struct {
	Email          string `json:"email" binding:"required,email,max=128"`
	Password       string `json:"password" binding:"required,min=8,max=72"`
	InviteCode     string `json:"invite_code" binding:"required"`
	TurnstileToken string `json:"turnstile_token"`
}

func (s *AuthService) Register(ctx context.Context, req *RegisterReq) (*models.User, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	// 站点开关：stop_register=1 关闭注册（设置页「站点」tab，单一入口）
	if StopRegister(s.DB) {
		return nil, ErrRegisterClosed
	}
	var user *models.User
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 邀请码必填（2026-08-14 起硬编码，无开关）
		if err := s.consumeInviteTx(ctx, tx, req.InviteCode); err != nil {
			return err
		}

		var cnt int64
		if err := tx.Model(&models.User{}).
			Where("username = ? OR email = ?", email, email).Count(&cnt).Error; err != nil {
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
			Username:       email, // 用户名=邮箱（同值双写，登录兼容）
			Email:          email,
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

// consumeInviteTx 校验并核销邀请码（事务内：状态校验 → 条件更新，防并发双注册）。
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
	// 条件更新：仅 unused 可核销（并发安全，避免 select-then-update 竞态）
	res := tx.WithContext(ctx).Model(&models.InvitationCode{}).
		Where("id = ? AND status = ?", inv.ID, models.InviteUnused).
		Updates(map[string]any{
			"status":  models.InviteUsed,
			"used_at": time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrInviteInvalid
	}
	return nil
}

// loginFailures 按邮箱的账号级失败计数（J19-③，Xboard PASSWORD_ERROR_LIMIT 模式：
// 5 次/30 分钟锁定；与路由层 IP 限流互补，防代理池暴力破解）。内存实现，多实例需换 Redis。
var loginFailures = struct {
	sync.Mutex
	m map[string]loginFail
}{m: make(map[string]loginFail)}

type loginFail struct {
	count int
	until time.Time
}

const (
	loginFailLimit  = 5
	loginFailWindow = 30 * time.Minute
)

// Login 校验用户名密码与账号状态（含账号级失败锁定）。
func (s *AuthService) Login(ctx context.Context, username, password string) (*models.User, error) {
	key := strings.ToLower(strings.TrimSpace(username))

	loginFailures.Lock()
	f, locked := loginFailures.m[key]
	now := time.Now()
	if locked && now.Before(f.until) {
		loginFailures.Unlock()
		return nil, errors.New("密码错误次数过多，请 30 分钟后再试")
	}
	if locked && !now.Before(f.until) {
		delete(loginFailures.m, key)
	}
	loginFailures.Unlock()

	var user models.User
	if err := s.DB.WithContext(ctx).Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCreds
		}
		return nil, err
	}
	match, err := argon2id.ComparePasswordAndHash(password, user.PasswordHash)
	if err != nil || !match {
		loginFailures.Lock()
		cur := loginFailures.m[key]
		cur.count++
		if cur.count >= loginFailLimit {
			cur.until = now.Add(loginFailWindow)
			cur.count = 0
		}
		loginFailures.m[key] = cur
		loginFailures.Unlock()
		return nil, ErrInvalidCreds
	}
	// 登录成功：清零失败计数
	loginFailures.Lock()
	delete(loginFailures.m, key)
	loginFailures.Unlock()

	if user.Status != models.StatusActive {
		return nil, ErrUserDisabled
	}
	return &user, nil
}

// VerifyPassword 校验用户当前密码（2FA 解绑等场景用）。
func (s *AuthService) VerifyPassword(userID uint64, password string) (bool, error) {
	var user models.User
	if err := s.DB.First(&user, userID).Error; err != nil {
		return false, err
	}
	return argon2id.ComparePasswordAndHash(password, user.PasswordHash)
}

// IssueTokens 签发 access + refresh（携带 token_version，会话吊销基准）。
func (s *AuthService) IssueTokens(user *models.User) (access, refresh string, err error) {
	return s.JWT.GeneratePair(user.ID, user.Role, user.TokenVersion)
}

// ChangePassword 修改密码（校验旧密码；成功后 bump token_version 吊销旧会话）。
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
		"token_version":   gorm.Expr("token_version + 1"),
	}).Error
}

// AdminSetPassword 管理员改密/重置密码（bump token_version 吊销旧会话）。
func (s *AuthService) AdminSetPassword(ctx context.Context, userID uint64, newPwd string) error {
	hash, err := argon2id.CreateHash(newPwd, argon2id.DefaultParams)
	if err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).Updates(map[string]any{
		"password_hash":   hash,
		"must_change_pwd": false,
		"token_version":   gorm.Expr("token_version + 1"),
	}).Error
}

// Refresh 用 refresh token 换取新 access token（校验 token_version：改密/封禁后旧 refresh 立即失效）。
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (string, error) {
	claims, err := s.JWT.Parse(refreshToken)
	if err != nil || claims.Type != TokenRefresh {
		return "", ErrInvalidRefresh
	}
	// 校验用户仍存在、可用且版本号匹配（会话吊销）
	var user models.User
	if err := s.DB.WithContext(ctx).First(&user, claims.UserID).Error; err != nil {
		return "", ErrInvalidRefresh
	}
	if user.Status != models.StatusActive {
		return "", ErrUserDisabled
	}
	if user.TokenVersion != claims.Version {
		return "", ErrInvalidRefresh
	}
	return s.JWT.Generate(user.ID, user.Role, TokenAccess, user.TokenVersion)
}

// ForgotPassword 忘记密码第一步：校验邮箱存在（统一文案防枚举）。
// 返回 true = 邮箱存在且已绑定 TOTP（可自助重置）；false = 不存在或未绑定（走管理员）。
func (s *AuthService) ForgotPassword(ctx context.Context, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	var user models.User
	if err := s.DB.WithContext(ctx).Where("email = ? OR username = ?", email, email).First(&user).Error; err != nil {
		return false, nil // 不存在：统一返回，防枚举
	}
	return user.TotpEnabled, nil
}

// ResetPassword 忘记密码第二步：TOTP/恢复码验证通过后重置密码 + bump token_version 吊销旧会话。
func (s *OTPService) ResetPassword(ctx context.Context, email, code, newPwd string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	var user models.User
	if err := s.DB.WithContext(ctx).Where("email = ? OR username = ?", email, email).First(&user).Error; err != nil {
		return ErrInvalidCreds
	}
	if !user.TotpEnabled {
		return ErrTOTPNotEnabled // 未绑定 TOTP 无法自助重置（走管理员）
	}
	if err := s.VerifyCode(&user, code); err != nil {
		if errors.Is(err, ErrTOTPCodeInvalid) {
			// 尝试恢复码
			if berr := s.VerifyBackupCode(&user, code); berr != nil {
				return ErrTOTPCodeInvalid
			}
		} else {
			return err
		}
	}
	hash, err := argon2id.CreateHash(newPwd, argon2id.DefaultParams)
	if err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Model(&user).Updates(map[string]any{
		"password_hash":   hash,
		"must_change_pwd": false,
		"token_version":   gorm.Expr("token_version + 1"),
	}).Error
}

// ResetSubscribeToken 重置用户订阅密钥（旧订阅链接即刻失效）。
func (s *AuthService) ResetSubscribeToken(ctx context.Context, userID uint64) (string, error) {
	token, err := util.NewSubscribeToken()
	if err != nil {
		return "", err
	}
	if err := s.DB.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).
		Update("subscribe_token", token).Error; err != nil {
		return "", err
	}
	return token, nil
}
