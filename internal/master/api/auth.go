package api

import (
	"errors"

	"github.com/gin-gonic/gin"

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
	util.OK(c, gin.H{
		"user":          userSummary(user),
		"access_token":  access,
		"refresh_token": refresh,
	})
}

// Login POST /api/v1/auth/login
func (d *Deps) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
		return
	}
	user, err := d.Auth.Login(c.Request.Context(), req.Username, req.Password)
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
	util.OK(c, gin.H{
		"user":          userSummary(user),
		"access_token":  access,
		"refresh_token": refresh,
	})
}

// Refresh POST /api/v1/auth/refresh
func (d *Deps) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
		return
	}
	access, err := d.Auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		util.Unauthorized(c, "刷新令牌无效或已过期")
		return
	}
	util.OK(c, gin.H{"access_token": access})
}

func authError(err error) (int, string) {
	switch {
	case errors.Is(err, services.ErrUserExists):
		return 409, err.Error()
	case errors.Is(err, services.ErrInviteInvalid):
		return 400, err.Error()
	case errors.Is(err, services.ErrInvalidCreds), errors.Is(err, services.ErrInvalidRefresh):
		return 401, err.Error()
	case errors.Is(err, services.ErrUserDisabled):
		return 403, err.Error()
	default:
		return 500, "服务器内部错误"
	}
}

// userSummary 用户公开信息（不含密码哈希与订阅 token 之外的敏感字段）。
func userSummary(u *models.User) gin.H {
	return gin.H{
		"id":         u.ID,
		"username":   u.Username,
		"email":      u.Email,
		"role":       u.Role,
		"status":     u.Status,
		"plan_id":    u.PlanID,
		"expire_at":  u.ExpireAt,
		"created_at": u.CreatedAt,
	}
}