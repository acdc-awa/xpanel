package api

import (
	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// UserOTPSetup POST /api/v1/user/2fa/setup —— 获取 TOTP 绑定参数（otpauth URL + secret）。
func (d *Deps) UserOTPSetup(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var user models.User
	if err := d.DB.First(&user, uid).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	secret, otpauth, err := d.OTP.Setup(uid, user.Email)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	util.OK(c, gin.H{"secret": secret, "otpauth_url": otpauth})
}

// UserOTPConfirm POST /api/v1/user/2fa/confirm —— 绑定验证码确认，启用并返回备份码（仅展示一次）。
func (d *Deps) UserOTPConfirm(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req struct {
		Secret string `json:"secret" binding:"required"`
		Code   string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
		return
	}
	codes, err := d.OTP.Confirm(uid, req.Secret, req.Code)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	d.Audit.Log("user", uid, "otp.enable", "开启两步验证", util.ClientIPFromContext(c))

	// 重新读取自增后的 token_version 并重新下发带 2FA 认证的会话 cookie，防止当前页面因会话失效立即被登出
	var user models.User
	if err := d.DB.First(&user, uid).Error; err == nil {
		access, errA := d.JWT.GenerateVerified(user.ID, user.Role, user.TokenVersion)
		refresh, errR := d.JWT.Generate(user.ID, user.Role, contracts.TokenRefresh, user.TokenVersion)
		if errA == nil && errR == nil {
			d.setAuthCookies(c, access, refresh)
		}
	}

	util.OK(c, gin.H{"backup_codes": codes})
}

// UserOTPDisable POST /api/v1/user/2fa/disable —— 解绑（需 TOTP 验证码、恢复码或当前密码任其一）。
func (d *Deps) UserOTPDisable(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req struct {
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
		return
	}
	var user models.User
	if err := d.DB.First(&user, uid).Error; err != nil {
		util.Fail(c, 404, "用户不存在")
		return
	}
	ok := false
	if req.Code != "" {
		if err := d.OTP.VerifyCode(&user, req.Code); err == nil {
			ok = true
		} else if berr := d.OTP.VerifyBackupCode(&user, req.Code); berr == nil {
			ok = true
		}
	}
	if !ok && req.Password != "" {
		match, err := d.Auth.VerifyPassword(uid, req.Password)
		if err == nil && match {
			ok = true
		}
	}
	if !ok {
		util.BadRequest(c, "验证信息无效（需验证码、恢复码或当前密码）")
		return
	}
	if err := d.OTP.Disable(uid); err != nil {
		util.ServerError(c, "解绑失败")
		return
	}
	d.Audit.Log("user", uid, "otp.disable", "关闭两步验证", util.ClientIPFromContext(c))

	// 重新读取自增后的 token_version 并重新下发普通会话 cookie
	if err := d.DB.First(&user, uid).Error; err == nil {
		access, errA := d.JWT.Generate(user.ID, user.Role, contracts.TokenAccess, user.TokenVersion)
		refresh, errR := d.JWT.Generate(user.ID, user.Role, contracts.TokenRefresh, user.TokenVersion)
		if errA == nil && errR == nil {
			d.setAuthCookies(c, access, refresh)
		}
	}

	util.OK(c, gin.H{"ok": true})
}

// AdminDisableOTP POST /api/v1/admin/users/:id/2fa/disable —— 管理员解绑（兜底，记审计）。
func (d *Deps) AdminDisableOTP(c *gin.Context) {
	id, err := parseUint(c.Param("id"))
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	if err := d.OTP.Disable(id); err != nil {
		util.ServerError(c, "解绑失败")
		return
	}
	util.OK(c, gin.H{"ok": true})
}

// UserResetSubscribe POST /api/v1/user/subscribe/reset —— 重置订阅密钥（旧链接即刻失效）。
func (d *Deps) UserResetSubscribe(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	token, err := d.Auth.ResetSubscribeToken(c.Request.Context(), uid)
	if err != nil {
		util.ServerError(c, "重置失败")
		return
	}
	d.Audit.Log("user", uid, "subscribe.reset_token", "重置订阅密钥", util.ClientIPFromContext(c))
	util.OK(c, gin.H{"subscribe_token": token})
}
