package util

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/gin-gonic/gin/binding"
)

func TestTranslateBindError(t *testing.T) {
	type form struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8,max=72"`
		Count    int    `json:"count" binding:"required,min=1"`
	}

	// 非法 JSON 语法
	if got := TranslateBindError(errors.New("invalid character 'x' looking for beginning of value")); got != "参数错误：请求体格式不正确" {
		t.Errorf("syntax error = %q", got)
	}
	// 空/截断请求体不透传原文
	if got := TranslateBindError(io.EOF); got != "参数错误：请求体不能为空" {
		t.Errorf("EOF = %q", got)
	}
	if got := TranslateBindError(io.ErrUnexpectedEOF); got != "参数错误：请求体不能为空" {
		t.Errorf("ErrUnexpectedEOF = %q", got)
	}
	// 校验错误：已知字段译中文（gin 验证器注册了 json tag 名），且不透传英文字段名/规则名
	err := binding.Validator.ValidateStruct(form{Email: "bad", Password: "short", Count: 0})
	if err == nil {
		t.Fatal("want validation error, got nil")
	}
	got := TranslateBindError(err)
	for _, want := range []string{"邮箱", "密码", "数量"} {
		if !strings.Contains(got, want) {
			t.Errorf("translate = %q, want contains %q", got, want)
		}
	}
	for _, leak := range []string{"email", "password", "required", "min", "max"} {
		if strings.Contains(got, leak) {
			t.Errorf("translate leaks raw validator text: %q", got)
		}
	}
	// 无 json tag 的结构体回退 Go 字段名展示
	bareErr := binding.Validator.ValidateStruct(struct {
		Email string `binding:"required,email"`
	}{Email: "bad"})
	if got := TranslateBindError(bareErr); got != "参数错误：字段「Email」格式不正确" {
		t.Errorf("no-json-tag fallback = %q", got)
	}
}
