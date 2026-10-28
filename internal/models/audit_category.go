package models

import (
	"strings"

	"gorm.io/gorm"
)

// 审计日志模块分类。分类在写入时判定并落库（AuditLog.Category），
// 列表筛选按列精确匹配；前端展示名由 AuditCategoryLabel 提供（与前端 badge 保持一致）。
const (
	AuditCategoryServers  = "servers"  // 服务器 / 节点 / 配置下发
	AuditCategoryUsers    = "users"    // 用户 / 邀请码 / 订阅凭据
	AuditCategoryBilling  = "billing"  // 套餐 / 订单 / 礼品卡 / 自动续费
	AuditCategoryInbounds = "inbounds" // 入站 / 证书 / 权限组 / 接入点 / 订阅模板
	AuditCategorySettings = "settings" // 全局设置 / 公告 / 备份 / 数据运维 / 更新
	AuditCategoryAuth     = "auth"     // 登录 / 两步验证 / 密码
	AuditCategoryOther    = "other"    // 未归类（新增 action 未登记时兜底）
)

var auditCategoryLabels = map[string]string{
	AuditCategoryServers:  "服务器",
	AuditCategoryUsers:    "用户",
	AuditCategoryBilling:  "财务",
	AuditCategoryInbounds: "入站证书",
	AuditCategorySettings: "系统",
	AuditCategoryAuth:     "认证",
	AuditCategoryOther:    "其他",
}

// AuditCategoryLabel 分类展示名（未知分类原样返回，便于排查新增 action）。
func AuditCategoryLabel(code string) string {
	if label, ok := auditCategoryLabels[code]; ok {
		return label
	}
	return code
}

// AuditCategory 由 action 判定模块分类（唯一入口，写入与回填共用）。
//
// action 有两种来源：管理端中间件由路由路径推导（如 users.:id.balance、servers.:id.outbounds），
// 以及业务代码手动打点（auth.login、auto_renew、gift_card.redeem 等）。这里按前缀归类，
// 新增 action 时务必在此登记，否则会落到 other。
func AuditCategory(action string) string {
	a := strings.ToLower(strings.TrimSpace(action))
	switch {
	case a == "":
		return AuditCategoryOther

	// 财务：系统自动续费/退订、用户余额支付与礼品卡兑换，以及管理端套餐/订单/礼品卡。
	case a == "auto_renew", a == "subscription_expired", a == "user.auto_renew",
		a == "gift_card.redeem", a == "order.pay_balance",
		hasAnyPrefix(a, "plans", "orders", "gift-cards", "billing"):
		return AuditCategoryBilling

	// 认证：登录、两步验证、密码修改、订阅凭据重置。
	case hasAnyPrefix(a, "auth", "otp"),
		a == "subscribe.reset_token":
		return AuditCategoryAuth

	case hasAnyPrefix(a, "users", "invitations"):
		return AuditCategoryUsers

	case hasAnyPrefix(a, "servers"):
		return AuditCategoryServers

	// 入站域：入站、证书与 CA、权限组/接入点、订阅模板，以及 xray 工具/协议能力查询。
	case hasAnyPrefix(a, "inbounds", "certs", "ca-certs", "access-points", "permission-groups",
		"sub-templates", "xray", "protocols"):
		return AuditCategoryInbounds

	// 系统域：全局设置、公告（含旧版 notice. 单数）、备份（含备份服务内部 action）、
	// 数据管理、拓扑布局、面板更新与系统状态。
	case hasAnyPrefix(a, "settings", "notices", "notice.", "backup", "topology", "data",
		"update", "system", "dashboard"),
		a == "audit-logs":
		return AuditCategorySettings

	default:
		return AuditCategoryOther
	}
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// BackfillAuditCategories 回填存量审计行的分类（幂等）。v4 新增 category 列后，
// 旧行走一次 action→分类映射；之后新增行由写入方直接填列，本函数每次启动为空操作。
// 无法识别的 action 归入 other（仍可按关键词/操作人检索）。
func BackfillAuditCategories(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	var actions []string
	if err := db.Model(&AuditLog{}).
		Where("category IS NULL OR category = ''").
		Distinct().Pluck("action", &actions).Error; err != nil {
		return err
	}
	for _, a := range actions {
		if err := db.Model(&AuditLog{}).
			Where("action = ? AND (category IS NULL OR category = '')", a).
			Update("category", AuditCategory(a)).Error; err != nil {
			return err
		}
	}
	return nil
}
