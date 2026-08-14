package services

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/config"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// TOTP 2FA 工具（2026-08-14 方向③，Xboard 无此功能自建最小闭环）：
//   - secret AES-GCM 加密入库（key = sha256(totp.encrypt_key 或 jwt.secret 派生)）
//   - 备份码 8 个，bcrypt 哈希存 JSON 数组，一次性消费
//   - 失败锁定：连续错 ≥5 次锁 30 分钟（Xboard PASSWORD_ERROR_LIMIT 模式）
//   - 校验窗口 ±1 步（30s×3 窗口，容忍时钟偏移）

const (
	totpIssuer            = "XrayPanel"
	totpMaxFailed         = 5
	totpLockDuration      = 30 * time.Minute
	totpBackupCodeCount   = 8
	totpBackupCodeLen     = 10
)

// TOTPError 业务错误。
var (
	ErrTOTPNotEnabled = errors.New("未开启两步验证")
	ErrTOTPAlreadyOn  = errors.New("已开启两步验证，请先解绑")
	ErrTOTPCodeInvalid = errors.New("验证码错误")
	ErrTOTPLocked     = errors.New("验证码错误次数过多，请 30 分钟后再试")
	ErrBackupInvalid  = errors.New("恢复码无效或已使用")
)

// OTPService TOTP 2FA 服务。
type OTPService struct {
	DB      *gorm.DB
	Encrypt []byte // AES-256 key
}

// NewOTPService 构造（encryptKey 为空回退 jwt secret 派生）。
func NewOTPService(db *gorm.DB, cfg *config.Config) *OTPService {
	key := cfg.Totp.EncryptKey
	if key == "" {
		key = cfg.JWT.Secret
	}
	sum := sha256.Sum256([]byte(key))
	return &OTPService{DB: db, Encrypt: sum[:]}
}

// Setup 生成新 TOTP secret 与 otpauth URL（用户登录后调用；已开启则拒绝）。
func (s *OTPService) Setup(userID uint64, email string) (secret, otpauthURL string, err error) {
	var user models.User
	if err := s.DB.First(&user, userID).Error; err != nil {
		return "", "", errors.New("用户不存在")
	}
	if user.TotpEnabled {
		return "", "", ErrTOTPAlreadyOn
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: email,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// Confirm 校验绑定验证码并启用（写入加密 secret + 生成备份码）。
func (s *OTPService) Confirm(userID uint64, secret, code string) (backupCodes []string, err error) {
	var user models.User
	if err := s.DB.First(&user, userID).Error; err != nil {
		return nil, errors.New("用户不存在")
	}
	if user.TotpEnabled {
		return nil, ErrTOTPAlreadyOn
	}
	if !totp.Validate(code, secret) {
		return nil, ErrTOTPCodeInvalid
	}
	enc, err := s.encrypt(secret)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, totpBackupCodeCount)
	hashes := make([]string, 0, totpBackupCodeCount)
	for i := 0; i < totpBackupCodeCount; i++ {
		bc, err := util.RandomHex(totpBackupCodeLen)
		if err != nil {
			return nil, err
		}
		h, err := argon2id.CreateHash(bc, argon2id.DefaultParams)
		if err != nil {
			return nil, err
		}
		codes = append(codes, bc)
		hashes = append(hashes, h)
	}
	raw, _ := json.Marshal(hashes)
	if err := s.DB.Model(&user).Updates(map[string]any{
		"totp_secret":      enc,
		"totp_enabled":     true,
		"totp_failed_count": 0,
		"totp_locked_until": nil,
		"backup_codes":      string(raw),
	}).Error; err != nil {
		return nil, err
	}
	return codes, nil
}

// Disable 解绑（需正确验证码或恢复码，或正确密码——由调用方传入已验凭证）。
func (s *OTPService) Disable(userID uint64) error {
	var user models.User
	if err := s.DB.First(&user, userID).Error; err != nil {
		return errors.New("用户不存在")
	}
	return s.DB.Model(&user).Updates(map[string]any{
		"totp_secret":       "",
		"totp_enabled":      false,
		"totp_failed_count": 0,
		"totp_locked_until": nil,
		"backup_codes":      "",
	}).Error
}

// VerifyCode 校验 TOTP 验证码（含失败锁定与计数）。lockedUntil 返回锁定剩余描述用时间。
func (s *OTPService) VerifyCode(user *models.User, code string) error {
	if !user.TotpEnabled {
		return ErrTOTPNotEnabled
	}
	if user.TotpLockedUntil != nil && time.Now().Before(*user.TotpLockedUntil) {
		return ErrTOTPLocked
	}
	secret, err := s.decrypt(user.TotpSecret)
	if err != nil {
		return err
	}
	if totp.Validate(code, secret) {
		// 成功后清零失败计数
		if user.TotpFailedCount != 0 || user.TotpLockedUntil != nil {
			_ = s.DB.Model(user).Updates(map[string]any{
				"totp_failed_count": 0,
				"totp_locked_until": nil,
			})
		}
		return nil
	}
	// 失败计数 + 锁定
	failed := user.TotpFailedCount + 1
	updates := map[string]any{"totp_failed_count": failed}
	if failed >= totpMaxFailed {
		until := time.Now().Add(totpLockDuration)
		updates["totp_locked_until"] = until
		updates["totp_failed_count"] = 0
		_ = s.DB.Model(user).Updates(updates)
		return ErrTOTPLocked
	}
	_ = s.DB.Model(user).Update("totp_failed_count", failed)
	return ErrTOTPCodeInvalid
}

// VerifyBackupCode 校验恢复码（bcrypt 比对，命中即删除该码——一次性）。
func (s *OTPService) VerifyBackupCode(user *models.User, code string) error {
	if user.BackupCodes == "" {
		return ErrBackupInvalid
	}
	var hashes []string
	if err := json.Unmarshal([]byte(user.BackupCodes), &hashes); err != nil {
		return ErrBackupInvalid
	}
	for i, h := range hashes {
		ok, err := argon2id.ComparePasswordAndHash(code, h)
		if err != nil {
			continue
		}
		if ok {
			rest := append(hashes[:i], hashes[i+1:]...)
			raw, _ := json.Marshal(rest)
			_ = s.DB.Model(user).Update("backup_codes", string(raw))
			return nil
		}
	}
	return ErrBackupInvalid
}

// encrypt AES-GCM 加密 secret（输出 base64）。
func (s *OTPService) encrypt(plain string) (string, error) {
	block, err := aes.NewCipher(s.Encrypt)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (s *OTPService) decrypt(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.Encrypt)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("bad ciphertext")
	}
	nonce, sealed := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
