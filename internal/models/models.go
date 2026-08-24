// Package models 定义全部 GORM 模型，对应《系统设计方案》§5 数据库设计。
package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/pkg/util"
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
		&AccessLayer{},
		&UserAccessPoint{}, &PermissionGroupAccessPoint{},
		&Notice{},
	}
}

// AutoMigrate 建表/补列（生产环境由启动时执行，后续可切换为显式迁移）。
// ISSUE-04：先显式删除旧版 traffic_logs 的单列唯一索引 idx_traffic_period，
// 再执行 AutoMigrate 建立 (user_id, inbound_id, period_start) 复合唯一索引。
// 2026-08-23 访问控制单点化：退役 InboundEndpoint / 入站·L4 权限白名单三表，
// 授权收口为「用户接入点（UserAccessPoint）权限组白名单」单点，旧表显式删除（GORM 只增不删）。
// 2026-08-24 L4 建模退役：l4_rule 型接入点折转为「直连目标入站 + 端点覆写」，随后删除 l4_port_rules 表。
func AutoMigrate(db *gorm.DB) error {
	if err := dropLegacyTrafficPeriodIndex(db); err != nil {
		return err
	}
	if err := dropRetiredAccessControlTables(db); err != nil {
		return err
	}
	if err := dropLegacyAccessPointHostPort(db); err != nil {
		return err
	}
	if err := migrateL4RuleAccessPoints(db); err != nil {
		return err
	}
	if err := dropRetiredL4Tables(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(All()...); err != nil {
		return err
	}
	return migrateUserSubscribeTokens(db)
}

// legacyL4RuleAP 退役 L4 建模迁移的临时读取结构（user_access_points 中 l4_rule 型存量记录）。
// UserAccessPoint 模型已移除 TargetL4RuleID 字段（数据库列保留、迁移时清空），故用裸表结构读取。
type legacyL4RuleAP struct {
	ID              uint64
	CustomHost      string
	CustomPort      int
	Remark          string
	TargetL4RuleID  *uint64 `gorm:"column:target_l4_rule_id"`
	TargetInboundID *uint64 `gorm:"column:target_inbound_id"`
}

// legacyL4Rule 退役 L4 建模迁移的临时读取结构（l4_port_rules 存量表）。
type legacyL4Rule struct {
	ID              uint64
	ServerID        uint64
	ListenPort      int
	TargetServerID  uint64
	TargetInboundID uint64
}

// migrateL4RuleAccessPoints L4 建模退役一次性迁移（2026-08-24 拍板，选项 A：中转入站解挂层）：
// L4 中转语义由「AP 直连目标入站 + CustomHost/CustomPort 覆写为转发端点」等价表达——
// 原链路 L4 决议的 (中转机 Host, 监听端口) 本就是 AP 覆写的缺省值，且 L4 不改变任何客户端流参数
// （security/sni/path 全继承目标入站），故折转后订阅输出逐字段一致。
// 附：迁入站若是曾挂层的 L4 目标，解挂该层（层从未沿四层链路生效，避免折直连后层接管 security/SNI 造成行为漂移）；
// 退役存量 l4_relay 服务器（带外设施不再登记进面板，端点信息已折入 AP）。幂等。
func migrateL4RuleAccessPoints(db *gorm.DB) error {
	if !db.Migrator().HasTable("l4_port_rules") {
		return nil
	}
	var aps []legacyL4RuleAP
	if err := db.Table("user_access_points").Where("target_type = ?", "l4_rule").Find(&aps).Error; err != nil {
		return fmt.Errorf("查询 l4_rule 型接入点失败: %w", err)
	}
	for i := range aps {
		ap := &aps[i]
		updates := map[string]any{"target_l4_rule_id": nil}
		var rule legacyL4Rule
		if ap.TargetL4RuleID != nil {
			err := db.Table("l4_port_rules").First(&rule, *ap.TargetL4RuleID).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("查询 L4 规则 %d 失败: %w", *ap.TargetL4RuleID, err)
			}
		}
		// 规则或目标入站缺失 → 折为「待连线」空 AP，不产出错误节点
		if rule.TargetInboundID == 0 {
			updates["target_type"] = ""
			updates["target_inbound_id"] = nil
			if err := db.Table("user_access_points").Where("id = ?", ap.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("折转接入点 %d 失败: %w", ap.ID, err)
			}
			continue
		}
		var inb Inbound
		if err := db.First(&inb, rule.TargetInboundID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			updates["target_type"] = ""
			updates["target_inbound_id"] = nil
			if err := db.Table("user_access_points").Where("id = ?", ap.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("折转接入点 %d 失败: %w", ap.ID, err)
			}
			continue
		}
		updates["target_type"] = "inbound"
		updates["target_inbound_id"] = rule.TargetInboundID
		var l4Srv Server
		if err := db.First(&l4Srv, rule.ServerID).Error; err == nil {
			if ap.CustomHost == "" {
				updates["custom_host"] = l4Srv.Host
			}
			if ap.CustomPort == 0 {
				updates["custom_port"] = rule.ListenPort
			}
			if strings.TrimSpace(ap.CustomHost) == "" && ap.CustomPort == 0 && ap.Remark == "" {
				// 可追溯性：原带外中转信息折入备注（仅原本无覆写/备注的接入点）
				updates["remark"] = fmt.Sprintf("原L4中转：%s（%s:%d）", l4Srv.Name, l4Srv.Host, rule.ListenPort)
			}
		}
		if err := db.Table("user_access_points").Where("id = ?", ap.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("折转接入点 %d 失败: %w", ap.ID, err)
		}
		// 选项 A：解挂被 L4 指向的入站所挂接入层
		if inb.LayerID != nil {
			if err := db.Model(&Inbound{}).Where("id = ?", inb.ID).Update("layer_id", nil).Error; err != nil {
				return fmt.Errorf("解挂入站 %d 接入层失败: %w", inb.ID, err)
			}
		}
	}

	if err := retireL4RelayServers(db); err != nil {
		return err
	}
	return nil
}

// retireL4RelayServers 退役存量 l4_relay 服务器：清理其关联数据后删除
// （纯四层中转机为带外设施，删除后带外转发不受影响，面板不再登记）。
func retireL4RelayServers(db *gorm.DB) error {
	var relays []Server
	if err := db.Where("server_type = ?", "l4_relay").Find(&relays).Error; err != nil {
		return err
	}
	for _, r := range relays {
		id := r.ID
		for _, m := range []any{
			&AccessLayer{}, &PendingConfig{}, &PendingCert{}, &NodeReport{},
			&ServerOutbound{}, &ServerRoutingRule{}, &Inbound{},
		} {
			if !db.Migrator().HasTable(m) {
				continue // 迁移先行阶段部分表尚未建立（如全新库），无可清理
			}
			if err := db.Where("server_id = ?", id).Delete(m).Error; err != nil {
				return fmt.Errorf("清理 l4_relay 服务器 %d 关联数据失败: %w", id, err)
			}
		}
		if err := db.Delete(&Server{}, id).Error; err != nil {
			return fmt.Errorf("删除 l4_relay 服务器 %d 失败: %w", id, err)
		}
	}
	return nil
}

// migrateUserSubscribeTokens 订阅 token 回填一次性迁移（2026-08-24）：
// 早期建库的存量用户（含初始管理员）subscribe_token 为空，登录后订阅中心拿不到订阅地址
// （前端 token 为空串时显示「加载中…」）。为所有空 token 用户补齐 64 位 hex token，
// 与注册/新建/受控创建路径（auth.go Register / admin.go / ensureAdmin）同源生成。幂等。
func migrateUserSubscribeTokens(db *gorm.DB) error {
	var ids []uint64
	if err := db.Model(&User{}).Where("subscribe_token = '' OR subscribe_token IS NULL").Pluck("id", &ids).Error; err != nil {
		return err
	}
	for _, id := range ids {
		token, err := util.NewSubscribeToken()
		if err != nil {
			return err
		}
		if err := db.Model(&User{}).Where("id = ?", id).Update("subscribe_token", token).Error; err != nil {
			return fmt.Errorf("回填用户 %d 订阅 token 失败: %w", id, err)
		}
	}
	return nil
}

// dropRetiredL4Tables 幂等删除已退役的 L4 建模表（迁移先行折转，再删表）。
func dropRetiredL4Tables(db *gorm.DB) error {
	if err := db.Migrator().DropTable("l4_port_rules"); err != nil {
		return fmt.Errorf("删除已退役表 l4_port_rules 失败: %w", err)
	}
	return nil
}

// dropLegacyAccessPointHostPort 幂等删除早期接入点模型遗留的 host/port 列
// （管道收口前的中途设计存在 host NOT NULL；GORM AutoMigrate 只增不删，需显式删除）。
// 注意：glebarez/sqlite 的 Migrator().DropColumn 对未加反引号的旧列静默失效，
// 故用原生 ALTER TABLE DROP COLUMN（SQLite ≥3.35 与 MySQL 均支持，幂等由 HasColumn 保证）。
func dropLegacyAccessPointHostPort(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasTable(&UserAccessPoint{}) {
		return nil
	}
	table := (&UserAccessPoint{}).TableName()
	for _, col := range []string{"host", "port"} {
		if !m.HasColumn(&UserAccessPoint{}, col) {
			continue
		}
		if err := db.Exec("ALTER TABLE " + table + " DROP COLUMN " + col).Error; err != nil {
			return fmt.Errorf("删除 user_access_points 遗留列 %s 失败: %w", col, err)
		}
	}
	return nil
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
