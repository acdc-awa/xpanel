package contracts

import (
	"context"
	"time"

	"github.com/acdc/xray-panel/internal/models"
	"github.com/acdc/xray-panel/internal/pkg/protocol"
)

// Token 类型
const (
	TokenAccess  = "access"
	TokenRefresh = "refresh"
)

// JWTClaims JWT 载荷的纯数据契约。
type JWTClaims struct {
	UserID  uint64
	Role    string
	Type    string // access | refresh
	Version uint32 // token_version（会话吊销）
	TwoFA   bool   // 已通过 2FA
	Pending bool   // 2FA 待验证临时 access
}

// RegisterRequest 注册请求（纯数据契约，供 AuthService/API 共用）。
type RegisterRequest struct {
	Email          string `json:"email" binding:"required,email,max=128"`
	Password       string `json:"password" binding:"required,min=8,max=72"`
	InviteCode     string `json:"invite_code" binding:"required"`
	TurnstileToken string `json:"turnstile_token"`
}

// BackupInfo 备份文件元信息。
type BackupInfo struct {
	File      string    `json:"file"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// JWTManager JWT 签发与校验接口。
type JWTManager interface {
	Generate(userID uint64, role, typ string, version uint32) (string, error)
	GeneratePending2FA(userID uint64, role string, version uint32) (string, error)
	GenerateVerified(userID uint64, role string, version uint32) (string, error)
	Parse(token string) (*JWTClaims, error)
}

// AuthService 认证/用户身份服务接口。
type AuthService interface {
	AdminSetPassword(ctx context.Context, userID uint64, newPwd string) error
	ChangePassword(ctx context.Context, userID uint64, oldPwd, newPwd string) error
	ForgotPassword(ctx context.Context, email string) (bool, error)
	IssueTokens(user *models.User) (access, refresh string, err error)
	Login(ctx context.Context, username, password string) (*models.User, error)
	Refresh(ctx context.Context, refreshToken string) (string, error)
	Register(ctx context.Context, req *RegisterRequest) (*models.User, error)
	ResetSubscribeToken(ctx context.Context, userID uint64) (string, error)
	VerifyPassword(userID uint64, password string) (bool, error)
}

// OTPService 两步验证服务接口。
type OTPService interface {
	Confirm(userID uint64, secret, code string) (backupCodes []string, err error)
	Disable(userID uint64) error
	ResetPassword(ctx context.Context, email, code, newPwd string) error
	Setup(userID uint64, email string) (secret, otpauthURL string, err error)
	VerifyBackupCode(user *models.User, code string) error
	VerifyCode(user *models.User, code string) error
}

// TrafficService 流量统计服务接口。
type TrafficService interface {
	Save(tr protocol.TrafficReportPayload, serverID uint64) error
	UserUsed(userID uint64) (up, down int64, err error)
}

// OrderService 订单/余额支付服务接口。
type OrderService interface {
	ListByUser(userID uint64) ([]models.Order, error)
	PayWithBalance(userID, planID uint64) (*models.Order, error)
}

// AuditService 审计日志服务接口。
type AuditService interface {
	Log(opType string, opID uint64, action, detail, ip string)
}

// ConfigService 配置生成与待推送服务接口。
type ConfigService interface {
	Generate(serverID uint64) (string, error)
	GetPending(serverID uint64) (*models.PendingConfig, error)
	GetValidUsers(serverID uint64) (map[string][]protocol.User, error)
	MarkPushedByServerIfSame(serverID uint64, configJSON string) (bool, error)
	MarkPushedIfSame(id uint64, configJSON string) (bool, error)
	PreviewUsers(inb *models.Inbound, groupIDs []uint64) []protocol.User
	SavePending(serverID uint64, configJSON string) error
}

// SiteService 站点设置服务接口。
type SiteService interface {
	SetSiteGroup(vals map[string]string) error
	SetWebBase(v string) error
	SiteGroup() map[string]string
	WebBase() string
}

// GiftCardService 礼品卡/余额账户服务接口。
type GiftCardService interface {
	AdminAdjustBalance(adminID, targetUserID uint64, deltaCents int64, remark string) (int64, error)
	BatchGenerate(adminID uint64, count int, name string, faceValueCents int64, expiresAt *time.Time) ([]models.GiftCard, error)
	DisableOrDelete(cardID uint64) error
	ListBalanceLogs(userID uint64, page, size int) ([]models.BalanceLog, int64, error)
	ListCards(page, size int, status, search string) ([]models.GiftCard, int64, error)
	Redeem(userID uint64, code string) (*models.GiftCard, int64, error)
}

// BackupService 备份服务接口。
type BackupService interface {
	Snapshot() (BackupInfo, error)
	List() ([]BackupInfo, error)
	OpenFile(name string) (string, error)
}
