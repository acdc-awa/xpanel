package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/middleware"
	"github.com/zhx/xray-panel/internal/master/services"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// Register POST /api/v1/auth/register
func (d *Deps) Register(c *gin.Context) {
	var req services.RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	// 人机验证（方向②：captcha_enable 开启时校验）
	if err := services.VerifyCaptcha(d.DB, req.TurnstileToken, util.ClientIPFromContext(c), c.Request.Host, "register"); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	user, err := d.Auth.Register(c.Request.Context(), &req)
	if err != nil {
		code, msg := authError(err)
		util.Fail(c, code, msg)
		return
	}
	access, refresh, err := d.Auth.IssueTokens(user)
	if err != nil {
		util.ServerError(c, "签发令牌失败")
		return
	}
	d.setAuthCookies(c, access, refresh)
	util.OK(c, gin.H{
		"user": d.userView(user),
	})
}

// Login POST /api/v1/auth/login —— 密码校验；已开启 TOTP 的用户返回 twofa_required + 临时 pending token。
func (d *Deps) Login(c *gin.Context) {
	var req struct {
		Username       string `json:"username" binding:"required"`
		Password       string `json:"password" binding:"required"`
		TurnstileToken string `json:"turnstile_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
		return
	}
	// 人机验证（方向②：登录也校验）
	if err := services.VerifyCaptcha(d.DB, req.TurnstileToken, util.ClientIPFromContext(c), c.Request.Host, "login"); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	user, err := d.Auth.Login(c.Request.Context(), req.Username, req.Password)
	if err != nil {
		code, msg := authError(err)
		util.Fail(c, code, msg)
		return
	}
	// 2FA：已开启 → 签发 2 分钟 pending access，前端二次验证后换发完整令牌
	if user.TotpEnabled {
		pending, err := d.JWT.GeneratePending2FA(user.ID, user.Role, user.TokenVersion)
		if err != nil {
			util.ServerError(c, "签发令牌失败")
			return
		}
		d.setAuthCookies(c, pending, "")
		util.OK(c, gin.H{"twofa_required": true})
		return
	}
	d.finishLogin(c, user)
}

// TwoFAVerify POST /api/v1/auth/2fa/verify —— 登录第二步：TOTP/恢复码验证 → 换发完整 access+refresh。
func (d *Deps) TwoFAVerify(c *gin.Context) {
	claims := middleware.CurrentClaims(c)
	if claims == nil || !claims.Pending {
		util.Unauthorized(c, "请先完成密码登录")
		return
	}
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
		return
	}
	var user models.User
	if err := d.DB.First(&user, claims.UserID).Error; err != nil {
		util.Unauthorized(c, "用户不存在")
		return
	}
	if user.TokenVersion != claims.Version {
		util.Unauthorized(c, "会话已失效，请重新登录")
		return
	}
	if !user.TotpEnabled {
		util.BadRequest(c, "未开启两步验证")
		return
	}
	if err := d.OTP.VerifyCode(&user, req.Code); err != nil {
		if errors.Is(err, services.ErrTOTPCodeInvalid) {
			if berr := d.OTP.VerifyBackupCode(&user, req.Code); berr != nil {
				util.BadRequest(c, "验证码错误")
				return
			}
		} else {
			util.BadRequest(c, err.Error())
			return
		}
	}
	d.Audit.Log("user", user.ID, "auth.login_2fa", "两步验证登录成功", util.ClientIPFromContext(c))
	d.finishLogin(c, &user)
}

// finishLogin 签发完整令牌并写入 cookie + 审计。
// ISSUE-02：TOTP 用户必须签发 TwoFA=true 的完整 access；普通用户签普通 access。
func (d *Deps) finishLogin(c *gin.Context, user *models.User) {
	var access string
	var err error
	if user.TotpEnabled {
		access, err = d.JWT.GenerateVerified(user.ID, user.Role, user.TokenVersion)
	} else {
		access, err = d.JWT.Generate(user.ID, user.Role, services.TokenAccess, user.TokenVersion)
	}
	if err != nil {
		util.ServerError(c, "签发令牌失败")
		return
	}
	refresh, err := d.JWT.Generate(user.ID, user.Role, services.TokenRefresh, user.TokenVersion)
	if err != nil {
		util.ServerError(c, "签发令牌失败")
		return
	}
	d.Audit.Log("user", user.ID, "auth.login", "登录成功", util.ClientIPFromContext(c))
	d.setAuthCookies(c, access, refresh)
	util.OK(c, gin.H{
		"user": d.userView(user),
	})
}

// ForgotPassword POST /api/v1/auth/forgot —— 忘记密码第一步：提交邮箱。
// 统一文案防枚举；已绑定 TOTP 的账号可自助重置（第二步 /auth/reset），否则提示联系管理员。
func (d *Deps) ForgotPassword(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
		return
	}
	_, _ = d.Auth.ForgotPassword(c.Request.Context(), req.Email)
	// 统一响应（不区分邮箱是否存在/是否绑定 TOTP）
	util.OK(c, gin.H{
		"message": "若该邮箱已注册且已开启两步验证，请使用验证码完成重置；未开启两步验证的账号请联系管理员重置密码",
	})
}

// ResetPassword POST /api/v1/auth/reset —— 忘记密码第二步：TOTP/恢复码 + 新密码。
func (d *Deps) ResetPassword(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Code     string `json:"code" binding:"required"`
		Password string `json:"password" binding:"required,min=8,max=72"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := d.OTP.ResetPassword(c.Request.Context(), req.Email, req.Code, req.Password); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	util.OK(c, gin.H{"ok": true, "message": "密码已重置，请重新登录"})
}

// Refresh POST /api/v1/auth/refresh
func (d *Deps) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		util.Unauthorized(c, "缺少刷新令牌")
		return
	}
	access, err := d.Auth.Refresh(c.Request.Context(), refreshToken)
	if err != nil {
		d.clearAuthCookies(c)
		util.Unauthorized(c, "刷新令牌无效或已过期")
		return
	}
	// J19-② refresh 轮换：每次刷新签发新 refresh 覆盖 cookie（缩短旧 refresh 复用窗口）
	var newRefresh string
	if claims, perr := d.JWT.Parse(refreshToken); perr == nil {
		newRefresh, _ = d.JWT.Generate(claims.UserID, claims.Role, services.TokenRefresh, claims.Version)
	}
	d.setAuthCookies(c, access, newRefresh)
	util.OK(c, gin.H{"ok": true})
}

// Logout POST /api/v1/auth/logout
func (d *Deps) Logout(c *gin.Context) {
	d.clearAuthCookies(c)
	util.OK(c, gin.H{"ok": true})
}

// setAuthCookies 签发认证 cookie（J19-① 加固）：
// SameSite=Strict 防 CSRF；Secure 仅在反代标记 HTTPS 时开启（生产 Caddy TLS 反代带 X-Forwarded-Proto）。
func (d *Deps) setAuthCookies(c *gin.Context, access, refresh string) {
	secure := c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("access_token", access, int(d.Cfg.JWT.AccessTTL.Seconds()), "/", "", secure, true)
	if refresh != "" {
		c.SetCookie("refresh_token", refresh, int(d.Cfg.JWT.RefreshTTL.Seconds()), "/", "", secure, true)
	}
}

func (d *Deps) clearAuthCookies(c *gin.Context) {
	secure := c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("access_token", "", -1, "/", "", secure, true)
	c.SetCookie("refresh_token", "", -1, "/", "", secure, true)
}

func authError(err error) (int, string) {
	switch {
	case errors.Is(err, services.ErrUserExists):
		return 409, err.Error()
	case errors.Is(err, services.ErrInviteInvalid):
		return 400, err.Error()
	case errors.Is(err, services.ErrInvalidCreds), errors.Is(err, services.ErrInvalidRefresh):
		return 401, err.Error()
	case errors.Is(err, services.ErrUserDisabled), errors.Is(err, services.ErrRegisterClosed):
		return 403, err.Error()
	default:
		return 500, "服务器内部错误"
	}
}

// userSummary 用户公开信息（不含密码哈希与订阅 token 之外的敏感字段）。
func userSummary(u *models.User) gin.H {
	return gin.H{
		"id":              u.ID,
		"username":        u.Username,
		"email":           u.Email,
		"role":            u.Role,
		"status":          u.Status,
		"plan_id":         u.PlanID,
		"expire_at":       u.ExpireAt,
		"created_at":      u.CreatedAt,
		"must_change_pwd": u.MustChangePwd,
	}
}
