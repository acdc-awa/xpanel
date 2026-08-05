// Package util 提供统一响应与随机串等公共工具。
package util

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构：code=0 表示成功，非 0 为 HTTP 状态码（简化前端判断）。
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// OK 成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "ok", Data: data})
}

// Fail 失败响应（status 同时作为 HTTP 状态码与业务码）。
func Fail(c *gin.Context, status int, msg string) {
	c.JSON(status, Response{Code: status, Message: msg})
}

// BadRequest 400 快捷方式。
func BadRequest(c *gin.Context, msg string) { Fail(c, http.StatusBadRequest, msg) }

// Unauthorized 401 快捷方式。
func Unauthorized(c *gin.Context, msg string) { Fail(c, http.StatusUnauthorized, msg) }

// Forbidden 403 快捷方式。
func Forbidden(c *gin.Context, msg string) { Fail(c, http.StatusForbidden, msg) }

// ServerError 500 快捷方式。
func ServerError(c *gin.Context, msg string) { Fail(c, http.StatusInternalServerError, msg) }
