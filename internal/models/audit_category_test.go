package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestAuditCategory(t *testing.T) {
	cases := map[string]string{
		// 管理端路径推导
		"servers":                            AuditCategoryServers,
		"servers.:id.command":                AuditCategoryServers,
		"servers.:id.xray_start_failed":      AuditCategoryServers,
		"users":                              AuditCategoryUsers,
		"users.:id.balance":                  AuditCategoryUsers,
		"invitations.:id":                    AuditCategoryUsers,
		"plans.:id":                          AuditCategoryBilling,
		"orders":                             AuditCategoryBilling,
		"gift-cards":                         AuditCategoryBilling,
		"inbounds.:id.toggle":                AuditCategoryInbounds,
		"certs.self-signed":                  AuditCategoryInbounds,
		"ca-certs.:id.issue":                 AuditCategoryInbounds,
		"access-points.:id.target":           AuditCategoryInbounds,
		"permission-groups.:id.sub-template": AuditCategoryInbounds,
		"sub-templates":                      AuditCategoryInbounds,
		"xray.preview-config":                AuditCategoryInbounds,
		"protocols":                          AuditCategoryInbounds,
		"settings":                           AuditCategorySettings,
		"notices.:id.toggle":                 AuditCategorySettings,
		"notice.toggle":                      AuditCategorySettings, // 旧版手动打点
		"backup.create":                      AuditCategorySettings, // 备份服务内部 action
		"backup.upload":                      AuditCategorySettings,
		"topology-layout":                    AuditCategorySettings,
		"data.logs.cleanup":                  AuditCategorySettings,
		"update.apply":                       AuditCategorySettings,
		"system.status":                      AuditCategorySettings,
		"dashboard":                          AuditCategorySettings,
		// 业务代码手动打点
		"auth.login":            AuditCategoryAuth,
		"auth.login_2fa":        AuditCategoryAuth,
		"auth.change_password":  AuditCategoryAuth,
		"otp.enable":            AuditCategoryAuth,
		"subscribe.reset_token": AuditCategoryAuth,
		"gift_card.redeem":      AuditCategoryBilling,
		"order.pay_balance":     AuditCategoryBilling,
		"auto_renew":            AuditCategoryBilling,
		"subscription_expired":  AuditCategoryBilling,
		"user.auto_renew":       AuditCategoryBilling,
		// 未登记 action 兜底
		"":                   AuditCategoryOther,
		"some_future_action": AuditCategoryOther,
	}
	for action, want := range cases {
		assert.Equal(t, want, AuditCategory(action), "action=%q", action)
	}
}

func TestAuditCategoryLabel(t *testing.T) {
	assert.Equal(t, "服务器", AuditCategoryLabel(AuditCategoryServers))
	assert.Equal(t, "入站证书", AuditCategoryLabel(AuditCategoryInbounds))
	assert.Equal(t, "其他", AuditCategoryLabel(AuditCategoryOther))
	assert.Equal(t, "unknown", AuditCategoryLabel("unknown"), "未知分类原样返回，便于排查")
}

func TestBackfillAuditCategories(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&AuditLog{}))

	rows := []AuditLog{
		{Action: "servers.:id.command", Detail: "a"},
		{Action: "auth.login", Detail: "b", OperatorType: "user", OperatorID: 7},
		{Action: "auto_renew", Detail: "c", OperatorType: "system"},
		{Action: "servers.:id", Detail: "d"}, // 同一 action 多行，验证批量更新
	}
	assert.NoError(t, db.Create(&rows).Error)

	assert.NoError(t, BackfillAuditCategories(db))

	var got []AuditLog
	assert.NoError(t, db.Order("id").Find(&got).Error)
	assert.Equal(t, AuditCategoryServers, got[0].Category)
	assert.Equal(t, AuditCategoryAuth, got[1].Category)
	assert.Equal(t, AuditCategoryBilling, got[2].Category)
	assert.Equal(t, AuditCategoryServers, got[3].Category)

	// 幂等：再跑一次不改变任何行（也不会报错）。
	assert.NoError(t, BackfillAuditCategories(db))
	var count int64
	assert.NoError(t, db.Model(&AuditLog{}).Where("category = '' OR category IS NULL").Count(&count).Error)
	assert.Zero(t, count)
}
