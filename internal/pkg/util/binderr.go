// binderr.go 把 gin/validator 的参数校验错误翻译成中文文案，
// 避免校验器英文原文（含字段名与规则）直接透传给用户。
package util

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// gin 默认校验器未注册 TagNameFunc，FieldError.Field() 会返回 Go 字段名（英文驼峰）。
// init 统一注册为 json tag 名，字段中文名映射与请求体字段保持一致。
func init() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "-" {
				return ""
			}
			return name
		})
	}
}

// bindFieldNames 常用请求字段的中文名（键为 json tag 名；无 json tag 的字段回退 Go 字段名）。
var bindFieldNames = map[string]string{
	"username": "用户名", "email": "邮箱", "password": "密码",
	"old_password": "当前密码", "new_password": "新密码", "current_password": "当前密码",
	"code": "验证码", "token": "令牌",
	"name": "名称", "title": "标题", "content": "内容", "remark": "备注",
	"tag": "标签", "protocol": "协议", "port": "端口", "host": "主机地址",
	"domain": "域名", "server_id": "服务器", "plan_id": "套餐", "traffic_gb": "流量额度",
	"price_cents": "价格", "face_value_cents": "面值", "amount_cents": "金额",
	"duration_days": "有效期天数", "count": "数量", "location": "地区",
	"custom_host": "连接地址",
	"cert_pem":    "证书", "key_pem": "私钥", "secret": "密钥",
	"outbound_tag": "出站标签", "type": "类型", "field": "字段",
}

// bindTagText 把单条校验规则翻译成中文短语。
func bindTagText(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "不能为空"
	case "email", "url", "uri", "ip", "ipv4", "ipv6":
		return "格式不正确"
	case "min":
		if fe.Type().Kind() == reflect.String {
			return "长度不能少于 " + fe.Param()
		}
		return "不能小于 " + fe.Param()
	case "max":
		if fe.Type().Kind() == reflect.String {
			return "长度不能超过 " + fe.Param()
		}
		return "不能大于 " + fe.Param()
	case "len":
		return "长度必须为 " + fe.Param() + " 位"
	case "oneof":
		return "取值不合法"
	case "alphanum", "alpha":
		return "仅支持字母数字"
	case "gte":
		return "不能小于 " + fe.Param()
	case "lte":
		return "不能大于 " + fe.Param()
	default:
		return "校验未通过"
	}
}

// TranslateBindError 把 ShouldBindJSON 等返回的错误翻译成用户可读文案。
// 校验错误 → 逐字段中文；JSON 语法错误 → 统一提示；其他错误原样兜底。
func TranslateBindError(err error) string {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		parts := make([]string, 0, len(ve))
		for _, fe := range ve {
			name := bindFieldNames[fe.Field()]
			if name == "" {
				name = bindFieldNames[fe.StructField()] // 无 json tag 的结构体回退 Go 字段名
			}
			if name == "" {
				display := fe.Field()
				if display == "" {
					display = fe.StructField()
				}
				name = "字段「" + display + "」"
			}
			parts = append(parts, name+bindTagText(fe))
		}
		return "参数错误：" + strings.Join(parts, "、")
	}
	var ute *json.UnmarshalTypeError
	if errors.As(err, &ute) {
		return "参数错误：请求体格式不正确"
	}
	// 空/截断请求体（io.EOF 等）不透传原始错误文本。
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return "参数错误：请求体不能为空"
	}
	if strings.Contains(err.Error(), "invalid character") || strings.Contains(err.Error(), "unexpected end of JSON") {
		return "参数错误：请求体格式不正确"
	}
	return "参数错误：" + err.Error()
}

// BindJSON 绑定请求体并在失败时直接写入中文错误响应；返回是否成功。
func BindJSON(c *gin.Context, obj any) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		BadRequest(c, TranslateBindError(err))
		return false
	}
	return true
}
