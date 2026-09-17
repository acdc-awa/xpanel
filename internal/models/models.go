// Package models 定义全部 GORM 模型，对应《系统设计方案》§5 数据库设计。
package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

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
		&SubTemplate{},
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
	if err := migrateDefaultOutboundDSIntoOutbounds(db); err != nil {
		return err
	}
	// 必须紧跟上一行：种子形态判定要看到并退后的最终 domainStrategy。
	if err := migrateFreedomFinalRules(db); err != nil {
		return err
	}
	if err := backfillPlanSnapshots(db); err != nil {
		return err
	}
	// 必须紧跟 backfillPlanSnapshots：归位判定要读快照列 plan_group_id，快照未回填时无从比较。
	if err := backfillUserFollowPlanGroup(db); err != nil {
		return err
	}
	if err := backfillTrafficBilled(db); err != nil {
		return err
	}
	if err := migratePlanSaleFlags(db); err != nil {
		return err
	}
	// 时间归一置于最后：需待全部表建好、各列迁移完成后再统一存量时间口径。
	if err := NormalizeTimesToUTC(db); err != nil {
		return err
	}
	return migrateUserSubscribeTokens(db)
}

// backfillTrafficBilled 流量计费两列一次性回填（2026-09-06 倍率计费）：
// 存量行按 1:1 回填（billed = 原始字节，等价倍率 1）——历史消费不追溯倍率。
// settings 标记保证只跑一次：新版本落库路径恒写两列（含 ratio=0 的免费行 billed=0），
// 重跑会把免费行错误抬回原值。幂等。
func backfillTrafficBilled(db *gorm.DB) error {
	var mark Setting
	err := db.Where("key = ?", "traffic_billed_backfilled").First(&mark).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err := db.Exec("UPDATE traffic_logs SET billed_up = up_bytes, billed_down = down_bytes").Error; err != nil {
		return fmt.Errorf("回填流量计费两列失败: %w", err)
	}
	return db.Create(&Setting{Key: "traffic_billed_backfilled", Value: "1"}).Error
}

// migratePlanSaleFlags 套餐销售两属性一次性迁移（2026-09-03 enabled → purchasable/renewable）：
// 旧库按 enabled 原值映射（1 → 双 true，0 → 双 false，语义无损），随后删除遗留 enabled 列。
// settings 标记保证映射只跑一次（AutoMigrate 已先补出两列，默认 false）。幂等。
func migratePlanSaleFlags(db *gorm.DB) error {
	var mark Setting
	err := db.Where("key = ?", "plan_sale_flags_backfilled").First(&mark).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if db.Migrator().HasTable(&Plan{}) && db.Migrator().HasColumn(&Plan{}, "enabled") {
			if err := db.Exec("UPDATE plans SET purchasable = 1, renewable = 1 WHERE enabled = 1").Error; err != nil {
				return fmt.Errorf("回填套餐销售两属性失败: %w", err)
			}
		}
		if err := db.Create(&Setting{Key: "plan_sale_flags_backfilled", Value: "1"}).Error; err != nil {
			return err
		}
	}
	m := db.Migrator()
	if !m.HasTable(&Plan{}) || !m.HasColumn(&Plan{}, "enabled") {
		return nil
	}
	// DROP COLUMN 写法同 migrateDefaultOutboundDSIntoOutbounds（glebarez Migrator().DropColumn 对裸列名静默失效）
	if err := db.Exec("ALTER TABLE plans DROP COLUMN enabled").Error; err != nil {
		return fmt.Errorf("删除 plans 遗留列 enabled 失败: %w", err)
	}
	return nil
}

// backfillPlanSnapshots 套餐快照一次性回填（2026-09-01 Xboard 式隔离）：
// 存量用户按其当前套餐写入快照三列（plan_traffic_bytes/plan_device_limit/plan_group_id），
// 与新购/续费/分配路径同口径。settings 标记保证只跑一次——若每次启动都回填，
// 「改套餐未同步 + 面板重启」会把存量快照冲掉，隔离语义失效。幂等。
func backfillPlanSnapshots(db *gorm.DB) error {
	var mark Setting
	err := db.Where("key = ?", "plan_snapshot_backfilled").First(&mark).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var plans []Plan
	if err := db.Find(&plans).Error; err != nil {
		return err
	}
	for _, p := range plans {
		updates := PlanSnapshotColumns(&p)
		updates["plan_id"] = p.ID
		if err := db.Model(&User{}).Where("plan_id = ?", p.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("回填套餐 %d 用户快照失败: %w", p.ID, err)
		}
	}
	return db.Create(&Setting{Key: "plan_snapshot_backfilled", Value: "1"}).Error
}

// backfillUserFollowPlanGroup 权限组跟随套餐一次性归位（2026-09-17）：
// 旧购买路径把 plan.PermissionGroupID 直接写入用户的 permission_group_id（自定义列），
// 于是「套餐绑组」的用户被写成了与套餐同值的「假自定义」——生效组当时正确，但此后管理员
// 改套餐权限组时，该用户因 EffectiveGroupID 的「自定义优先」不再跟随（面板恒显示「(自定义)」）。
// 归位条件严格限定 permission_group_id = plan_group_id（套餐快照）：归零后生效组由快照回落，
// 权限完全不变，仅显示语义回到「(套餐继承)」；两值不同者为管理员真实自定义分组，保持不动。
// settings 标记保证只跑一次；幂等。
func backfillUserFollowPlanGroup(db *gorm.DB) error {
	var mark Setting
	err := db.Where("key = ?", "user_follow_plan_group_backfilled").First(&mark).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	m := db.Migrator()
	// 空库/仅迁移了部分表时无用户列可比，直接落标记。
	if m.HasTable(&User{}) && m.HasColumn(&User{}, "permission_group_id") && m.HasColumn(&User{}, "plan_group_id") {
		if err := db.Model(&User{}).
			Where("permission_group_id > 0 AND permission_group_id = plan_group_id").
			Update("permission_group_id", 0).Error; err != nil {
			return fmt.Errorf("权限组跟随套餐归位失败: %w", err)
		}
	}
	return db.Create(&Setting{Key: "user_follow_plan_group_backfilled", Value: "1"}).Error
}

// legacyServerDSRow 迁移读取结构：服务器级出站解析策略列已从模型移除，用裸表结构读存量值。
type legacyServerDSRow struct {
	ID                 uint64
	DefaultOutboundTag string
	DS                 string
}

// migrateDefaultOutboundDSIntoOutbounds 服务器级出站解析策略并退出站（2026-09-17 唯一入口收口）：
// 出站域名解析策略（freedom settings.domainStrategy）是出站级属性，唯一入口已收口到出站编辑器，
// servers.default_outbound_domain_strategy 列移除。迁移按「行为等价」原则处理存量：
//   - 旧生成器只在该列非空且非 AsIs 时，才把值注入 default_outbound_tag 指向的 freedom 出站，
//     且仅当该出站自己的 domainStrategy 为空或 AsIs（出站自有值优先）。此处用同一优先级写入，
//     保证升级前后的下发配置逐字节一致——直接删列会让这些服务器的域名解析行为静默退回 AsIs。
//   - default_outbound_tag 指向的出站非 freedom（如 vless 中转）、或为停用（旧生成器不合并停用出站）、
//     或 settings_json 为空/无 settings（旧注入同样跳过）时，该值本就未生效，迁移不动数据。
//   - 该列非 AsIs 只可能由 PUT /admin/servers/:id 写入，而路由页加载即建默认出站行（EnsureDefaultServerOutbounds），
//     故目标出站行必然存在；确无匹配行则跳过（该服务器此前也读不到该列的值）。
//
// 两个历史列名都要处理：default_outbound_domain_strategy（现行）与 default_outbound_ds（2026-08-31 前的
// GORM 缩写推导列）。优先读现行列，回填完成后两列一并删除。幂等（列不存在即跳过，不重复注入）。
func migrateDefaultOutboundDSIntoOutbounds(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasTable(&Server{}) {
		return nil
	}
	// 取值列：现行列优先，退化为 2026-08-31 前的遗留列
	dsCol := ""
	for _, col := range []string{"default_outbound_domain_strategy", "default_outbound_ds"} {
		if m.HasColumn(&Server{}, col) {
			dsCol = col
			break
		}
	}
	if dsCol != "" {
		var rows []legacyServerDSRow
		q := "SELECT id, default_outbound_tag, " + dsCol + " AS ds FROM servers WHERE " + dsCol + " IS NOT NULL AND " + dsCol + " != '' AND " + dsCol + " != 'AsIs'"
		if err := db.Raw(q).Scan(&rows).Error; err != nil {
			return fmt.Errorf("读取 servers.%s 存量值失败: %w", dsCol, err)
		}
		for _, r := range rows {
			if err := moveServerDSIntoOutbound(db, r); err != nil {
				return err
			}
		}
	}
	// 删列：glebarez Migrator().DropColumn 对裸列名静默失效，须原生 ALTER（同 migratePlanSaleFlags）
	for _, col := range []string{"default_outbound_domain_strategy", "default_outbound_ds"} {
		if !m.HasColumn(&Server{}, col) {
			continue
		}
		if err := db.Exec("ALTER TABLE servers DROP COLUMN " + col).Error; err != nil {
			return fmt.Errorf("删除 servers 遗留列 %s 失败: %w", col, err)
		}
	}
	return nil
}

// moveServerDSIntoOutbound 把单台服务器的存量服务器级解析策略并入其默认出口出站。
func moveServerDSIntoOutbound(db *gorm.DB, r legacyServerDSRow) error {
	tag := r.DefaultOutboundTag
	if tag == "" {
		tag = "direct"
	}
	var obs []ServerOutbound
	if err := db.Where("server_id = ? AND tag = ? AND protocol = ? AND enabled = ?", r.ID, tag, "freedom", true).
		Order("id ASC").Find(&obs).Error; err != nil {
		return fmt.Errorf("查询服务器 %d 的默认出口出站失败: %w", r.ID, err)
	}
	if len(obs) == 0 {
		// 该值本就未生效（旧生成器同样跳过），但列删除后这份存量偏好再无痕迹，留一行日志备查。
		log.Printf("[migrate] 服务器 %d 的服务器级解析策略 %s 无可用 freedom 出口出站（tag=%s），随列删除丢弃",
			r.ID, r.DS, tag)
		return nil
	}
	ob := obs[0]
	if strings.TrimSpace(ob.SettingsJSON) == "" {
		return nil
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(ob.SettingsJSON), &settings); err != nil {
		return nil // 非法 JSON：旧生成器同样不会写入，交由出站编辑器修正
	}
	if cur, _ := settings["domainStrategy"].(string); cur != "" && cur != "AsIs" {
		// 出站自有非 AsIs 值优先，旧生成器亦不覆盖。此处必须记日志：该值随后随列删除消失，
		// 而出站卡片不展示生效值，界面无从察觉——静默丢弃是这条路径最容易埋下的坑。
		log.Printf("[migrate] 服务器 %d 的服务器级解析策略 %s 与出站 %d(#%s) 自有值 %s 冲突，按出站优先丢弃",
			r.ID, r.DS, ob.ID, ob.Tag, cur)
		return nil
	}
	settings["domainStrategy"] = r.DS
	return updateOutboundSettings(db, ob.ID, settings)
}

// migrateFreedomFinalRules freedom 出站私网拦截下沉 + blockDelay 归一（2026-09-17 路由层私网规则移除）：
// 路由层不再注入 {"ip":["geoip:private"],"outboundTag":"blocked"}——路由规则自上而下首个命中生效，
// 该规则不带 inbound/outbound 维度限定，会抢在管理员「入站 → 其他出站」的规则之前拦掉目标为私网的
// 连接（且注入规则在 UI 中不可见，管理员无从排查）。私网拦截改由 freedom 出站 finalRules 承担，故：
//  1. finalRules 已含 block geoip:private 但未指定 blockDelay 的行 → 补 "0"。原先路由层丢给 blocked
//     出站是立即断开，不补会退化为官方默认 30-90s 黑洞挂起（CN 规则有意不补，保抗探测延迟）。
//  2. settings 只有 domainStrategy 一个键的 freedom 出站（从未通过出站编辑器保存过面板开关的种子形态）
//     → 回填 canonical 默认：私网拦截 + allow 兜底，与面板开关默认值（开）及种子行一致。
//     歧义提示：该形状既可能是没编辑过的种子行，也可能是管理员显式关掉「屏蔽内网私有 IP」后保存的结果，
//     二者字节相同、无法区分。此处按「不放宽」取舍——这批服务器此前同样拦私网（路由层规则 + freedom
//     内建安全策略），回填只把拦截位置显式化，不会放宽；确需放行私网的管理员在出站编辑器关掉开关
//     即可（关闭会写入无条件 allow 规则）。
//
// 幂等：已有 finalRules 的非种子行不再改动（第 1 条除外，补齐后再次运行无匹配项）。
func migrateFreedomFinalRules(db *gorm.DB) error {
	if !db.Migrator().HasTable(&ServerOutbound{}) {
		return nil
	}
	var obs []ServerOutbound
	if err := db.Where("protocol = ?", "freedom").Find(&obs).Error; err != nil {
		return fmt.Errorf("查询 freedom 出站失败: %w", err)
	}
	for i := range obs {
		ob := obs[i]
		if strings.TrimSpace(ob.SettingsJSON) == "" {
			continue
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(ob.SettingsJSON), &settings); err != nil {
			continue // 非法 JSON：交由出站编辑器修正
		}
		rules, hasRules := settings["finalRules"].([]any)
		if !hasRules {
			// 种子形态：仅 domainStrategy 一个键 → 回填 canonical（保留其 domainStrategy）
			if len(settings) != 1 {
				continue
			}
			ds, ok := settings["domainStrategy"].(string)
			if !ok {
				continue
			}
			var canon map[string]any
			if err := json.Unmarshal([]byte(DefaultFreedomDirectSettingsJSON), &canon); err != nil {
				return fmt.Errorf("解析 canonical freedom 默认 settings 失败: %w", err)
			}
			canon["domainStrategy"] = ds
			if err := updateOutboundSettings(db, ob.ID, canon); err != nil {
				return err
			}
			continue
		}
		// 补齐私网拦截规则的 blockDelay
		changed := false
		for _, r := range rules {
			m, ok := r.(map[string]any)
			if !ok {
				continue
			}
			if action, _ := m["action"].(string); action != "block" {
				continue
			}
			if _, has := m["blockDelay"]; has {
				continue
			}
			if ips, ok := m["ip"].([]any); !ok || len(ips) == 0 || ips[0] != "geoip:private" {
				continue
			}
			m["blockDelay"] = "0"
			changed = true
		}
		if !changed {
			continue
		}
		settings["finalRules"] = rules
		if err := updateOutboundSettings(db, ob.ID, settings); err != nil {
			return err
		}
	}
	return nil
}

// updateOutboundSettings 序列化并写回出站 settings_json。
func updateOutboundSettings(db *gorm.DB, id uint64, settings map[string]any) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("序列化出站 %d settings 失败: %w", id, err)
	}
	if err := db.Model(&ServerOutbound{}).Where("id = ?", id).Update("settings_json", string(raw)).Error; err != nil {
		return fmt.Errorf("回填出站 %d settings 失败: %w", id, err)
	}
	return nil
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
