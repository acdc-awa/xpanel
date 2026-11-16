package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// TestPurgeAuditNoise 一次性清理：只删清单内的噪音 action，真实配置变更一行不动；
// 写完 settings 标记后再次执行是空操作；清理本身留一条自述审计。
func TestPurgeAuditNoise(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&AuditLog{}, &Setting{}))

	keep := []AuditLog{
		{Action: "inbounds.:id", OperatorType: "admin"},
		{Action: "servers.:id.command", OperatorType: "admin"},
		{Action: "settings", OperatorType: "admin"},
		{Action: "auth.login", OperatorType: "user"},
		{Action: "auth.login_locked", OperatorType: "user"},
		{Action: "users.:id.toggle", OperatorType: "admin"},
	}
	noise := []AuditLog{
		{Action: "auth.login_failed"},
		{Action: "auth.2fa_failed"},
		{Action: "auth.logout"},
		{Action: "audit-logs.export"},
		{Action: "topology-layout"},
		{Action: "notices.order"},
		{Action: "plans.order"},
		{Action: "servers.order"},
		{Action: "permission-groups.order"},
		{Action: "notifications.:key"},
		{Action: "permission-groups.:id.preview-template"},
		{Action: "xray.preview-config"},
	}
	for _, r := range append(append([]AuditLog{}, keep...), noise...) {
		assert.NoError(t, db.Create(&r).Error)
	}

	assert.NoError(t, PurgeAuditNoise(db))

	var remained []string
	assert.NoError(t, db.Model(&AuditLog{}).Order("id").Pluck("action", &remained).Error)
	// 6 条真实行 + 1 条自述行
	assert.Len(t, remained, len(keep)+1)
	for _, a := range remained {
		assert.NotContains(t, mapKeys(noiseActionsMap()), a, "噪音 %q 应已删除", a)
	}
	assert.Contains(t, remained, "audit.noise_purge")
	assert.Equal(t, "1", readSettingValue(db, auditNoisePurgeKey))

	var ev AuditLog
	assert.NoError(t, db.Where("action = ?", "audit.noise_purge").Take(&ev).Error)
	assert.Equal(t, "system", ev.OperatorType)
	assert.Equal(t, AuditCategorySettings, ev.Category)
	assert.Contains(t, ev.Detail, `"rows":12`)

	// 幂等：标记已写 → 不再删除、不再追加自述行
	var before int64
	assert.NoError(t, db.Model(&AuditLog{}).Count(&before).Error)
	assert.NoError(t, PurgeAuditNoise(db))
	var after int64
	assert.NoError(t, db.Model(&AuditLog{}).Count(&after).Error)
	assert.Equal(t, before, after)
}

// TestPurgeAuditNoiseNoRowWhenNothingDeleted 全新库 / 已清过的库不该凭空多一条「清理了 0 条」。
func TestPurgeAuditNoiseNoRowWhenNothingDeleted(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&AuditLog{}, &Setting{}))
	assert.NoError(t, db.Create(&AuditLog{Action: "inbounds.:id"}).Error)

	assert.NoError(t, PurgeAuditNoise(db))

	var n int64
	assert.NoError(t, db.Model(&AuditLog{}).Where("action = ?", "audit.noise_purge").Count(&n).Error)
	assert.Equal(t, int64(0), n, "没删到行就不写自述行")
	assert.Equal(t, "1", readSettingValue(db, auditNoisePurgeKey), "即使一行没删也要写标记，否则每次启动都扫一遍")
}

// noiseActionsMap 清单 → 集合（断言更方便）。
func noiseActionsMap() map[string]bool {
	m := make(map[string]bool, len(auditNoiseActions))
	for _, a := range auditNoiseActions {
		m[a] = true
	}
	return m
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
