// Package models 定义全部 GORM 模型，对应《系统设计方案》§5 数据库设计。
package models

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// 角色 / 状态常量
const (
	RoleAdmin = "admin"
	RoleUser  = "user"

	StatusActive   = 1 // 正常
	StatusDisabled = 0 // 禁用

	InviteUnused   = 0 // 邀请码未使用
	InviteUsed     = 1 // 邀请码已使用
	InviteDisabled = 2 // 邀请码已禁用/过期

	OrderPaid = "paid" // 订单状态（余额直付即时生效，无 pending/人工确认）

	// Phase T：入站二态（面向终端用户 / 内部链式代理落地）
	InboundTypeUser  = "user"  // 进订阅、参与用户授权与 SyncUsers（默认）
	InboundTypeRelay = "relay" // 内部转发入站，被出站 InboundRef 引用，clients 固定为 InternalUUID
)

// All 返回全部模型，供 AutoMigrate 使用。
func All() []any {
	return []any{
		&User{}, &InvitationCode{}, &GiftCard{}, &BalanceLog{},
		&Server{}, &Inbound{}, &PendingConfig{}, &PendingCert{},
		&ServerOutbound{}, &ServerRoutingRule{},
		&Plan{}, &Order{}, &Cert{},
		&TrafficLog{}, &TrafficDaily{}, &NodeReport{},
		&AuditLog{}, &Setting{},
		&PermissionGroup{},
		&L4PortRule{},
		&UserAccessPoint{}, &PermissionGroupAccessPoint{},
		&Notice{},
	}
}

// AutoMigrate 建表/补列（生产环境由启动时执行，后续可切换为显式迁移）。
// ISSUE-04：先显式删除旧版 traffic_logs 的单列唯一索引 idx_traffic_period，
// 再执行 AutoMigrate 建立 (user_id, inbound_id, period_start) 复合唯一索引。
// 2026-08-23 访问控制单点化：退役 InboundEndpoint / 入站·L4 权限白名单三表，
// 授权收口为「用户接入点（UserAccessPoint）权限组白名单」单点，旧表显式删除（GORM 只增不删）。
func AutoMigrate(db *gorm.DB) error {
	if err := dropLegacyTrafficPeriodIndex(db); err != nil {
		return err
	}
	if err := dropRetiredAccessControlTables(db); err != nil {
		return err
	}
	return db.AutoMigrate(All()...)
}

// dropRetiredAccessControlTables 幂等删除已退役的接入控制旧表（授权单点化迁移）。
func dropRetiredAccessControlTables(db *gorm.DB) error {
	for _, table := range []string{
		"inbound_endpoints",
		"permission_group_endpoints",
		"permission_group_l4_rules",
		"permission_group_inbounds",
	} {
		if err := db.Migrator().DropTable(table); err != nil {
			return fmt.Errorf("删除已退役表 %s 失败: %w", table, err)
		}
	}
	return nil
}

// dropLegacyTrafficPeriodIndex 幂等删除旧库中的单列唯一索引（GORM AutoMigrate 只增不删）。
func dropLegacyTrafficPeriodIndex(db *gorm.DB) error {
	const legacy = "idx_traffic_period"
	if !db.Migrator().HasTable(&TrafficLog{}) {
		return nil
	}
	if !db.Migrator().HasIndex(&TrafficLog{}, legacy) {
		return nil
	}
	if err := db.Migrator().DropIndex(&TrafficLog{}, legacy); err != nil {
		return fmt.Errorf("删除旧唯一索引 %s 失败: %w", legacy, err)
	}
	return nil
}

// 时间辅助
func Now() time.Time { return time.Now() }
