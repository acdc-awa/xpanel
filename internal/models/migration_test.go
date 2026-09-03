package models

import (
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestTrafficLogLegacyIndexMigration 从旧 schema（单列唯一索引 idx_traffic_period）启动迁移：
// 旧索引必须被删除，并建立 (user_id, inbound_id, period_start) 复合唯一索引；
// 同一 period 的多个用户流量可同时入库（ISSUE-04 回归）。
func TestTrafficLogLegacyIndexMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// 旧 schema：与旧版模型一致，period_start 单列唯一
	oldDDL := `
CREATE TABLE traffic_logs (
	id integer PRIMARY KEY AUTOINCREMENT,
	user_id integer NOT NULL,
	inbound_id integer,
	up_bytes integer NOT NULL,
	down_bytes integer NOT NULL,
	period_start datetime,
	period_end datetime,
	created_at datetime
);
CREATE UNIQUE INDEX idx_traffic_period ON traffic_logs(period_start);
`
	for _, stmt := range strings.Split(oldDDL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create old schema: %v", err)
		}
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate from legacy schema: %v", err)
	}

	if db.Migrator().HasIndex(&TrafficLog{}, "idx_traffic_period") {
		t.Fatal("旧单列唯一索引 idx_traffic_period 应被删除")
	}
	if !db.Migrator().HasIndex(&TrafficLog{}, "idx_traffic_uid_inb_period") {
		t.Fatal("复合唯一索引 idx_traffic_uid_inb_period 应已建立")
	}

	period := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	if err := db.Create(&TrafficLog{UserID: 1, InboundID: 1, UpBytes: 10, DownBytes: 20, PeriodStart: period, PeriodEnd: period.Add(time.Minute)}).Error; err != nil {
		t.Fatalf("first user traffic create: %v", err)
	}
	if err := db.Create(&TrafficLog{UserID: 2, InboundID: 1, UpBytes: 30, DownBytes: 40, PeriodStart: period, PeriodEnd: period.Add(time.Minute)}).Error; err != nil {
		t.Fatalf("second user same period create should succeed: %v", err)
	}

	// 重复同键创建必须仍被复合唯一索引拒绝（幂等由上层 Save 合并处理）
	if err := db.Create(&TrafficLog{UserID: 1, InboundID: 1, UpBytes: 5, DownBytes: 6, PeriodStart: period, PeriodEnd: period.Add(time.Minute)}).Error; err == nil {
		t.Fatal("重复 (user,inbound,period) 应违反复合唯一索引")
	}
}

// TestTrafficLogMigrationIdempotent 全新库与再次启动均幂等。
func TestTrafficLogMigrationIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second migrate should be idempotent: %v", err)
	}
	if !db.Migrator().HasIndex(&TrafficLog{}, "idx_traffic_uid_inb_period") {
		t.Fatal("复合唯一索引应存在")
	}
}

// TestLegacyAccessPointHostPortMigration 旧 schema 遗留 host NOT NULL / port 列必须被删除，
// 否则创建接入点会触发 NOT NULL constraint failed（回归：创建接入点 500）。
func TestLegacyAccessPointHostPortMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	oldDDL := `
CREATE TABLE user_access_points (
	id integer PRIMARY KEY AUTOINCREMENT,
	name varchar(128) NOT NULL,
	host varchar(255) NOT NULL,
	port integer NOT NULL DEFAULT 0,
	target_type varchar(32) NOT NULL DEFAULT '',
	target_inbound_id integer,
	target_l4_rule_id integer,
	enabled numeric NOT NULL DEFAULT 1,
	remark varchar(255),
	created_at datetime,
	updated_at datetime
);
`
	for _, stmt := range strings.Split(oldDDL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("create old schema: %v", err)
		}
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate from legacy schema: %v", err)
	}

	for _, col := range []string{"host", "port"} {
		if db.Migrator().HasColumn(&UserAccessPoint{}, col) {
			t.Fatalf("遗留列 %s 应被删除", col)
		}
	}

	// 删除后创建接入点必须成功（不再触发 host NOT NULL）
	if err := db.Create(&UserAccessPoint{Name: "测试接入点"}).Error; err != nil {
		t.Fatalf("create access point after migration: %v", err)
	}

	// 再次启动迁移应幂等
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second migrate should be idempotent: %v", err)
	}
}

// TestL4RuleAccessPointMigration L4 建模退役迁移（2026-08-24 拍板，选项 A）：
// l4_rule 型接入点折转为「直连目标入站 + CustomHost/CustomPort 覆写为转发端点」，
// 曾被 L4 指向的挂层入站解挂层，l4_relay 服务器退役删除，随后 l4_port_rules 表整体 Drop。
func TestL4RuleAccessPointMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// 标准服务器/入站/接入点表（与生产旧库一致：glebarez 自建、列类型匹配）；
	// 再手工补旧版遗留列 target_l4_rule_id 与旧表 l4_port_rules（模拟 L4 退役前 schema）
	if err := db.AutoMigrate(&Server{}, &Inbound{}, &UserAccessPoint{}); err != nil {
		t.Fatalf("create standard tables: %v", err)
	}
	if err := db.Exec("ALTER TABLE user_access_points ADD COLUMN target_l4_rule_id integer").Error; err != nil {
		t.Fatalf("add legacy column: %v", err)
	}
	if err := db.Exec(`CREATE TABLE l4_port_rules (
		"id" integer PRIMARY KEY AUTOINCREMENT,
		"server_id" integer NOT NULL,
		"listen_port" integer NOT NULL,
		"target_server_id" integer NOT NULL,
		"target_inbound_id" integer NOT NULL,
		"remark" varchar(255),
		"enabled" numeric NOT NULL DEFAULT 1,
		"created_at" datetime,
		"updated_at" datetime
	)`).Error; err != nil {
		t.Fatalf("create l4_port_rules: %v", err)
	}

	// 旧数据：落地机 + 挂层入站 + l4_relay 中转机 + L4 规则 + l4_rule 型接入点
	hkSrv := Server{ServerType: ServerTypeXray, Name: "香港01", Host: "hk.node.com", NodeID: "node-hk", Secret: "s"}
	if err := db.Create(&hkSrv).Error; err != nil {
		t.Fatalf("create hk server: %v", err)
	}
	layerID := uint64(7) // 入站挂的对外接入层（L4 链从未消费，应解挂）
	inb := Inbound{ServerID: hkSrv.ID, Tag: "xhttp-web", Protocol: "vless", Port: 10086, LayerID: &layerID, Type: InboundTypeUser, Enabled: true}
	if err := db.Create(&inb).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	relay := Server{ServerType: "l4_relay", Name: "广州中转", Host: "gz.relay.com", NodeID: "node-l4", Secret: "s"}
	if err := db.Create(&relay).Error; err != nil {
		t.Fatalf("create l4 relay server: %v", err)
	}
	l4 := legacyL4Rule{ServerID: relay.ID, ListenPort: 30001, TargetInboundID: inb.ID}
	if err := db.Table("l4_port_rules").Create(&l4).Error; err != nil {
		t.Fatalf("create l4 rule: %v", err)
	}
	apL4ID := l4.ID
	// 模型已移除 TargetL4RuleID 字段，故用裸表结构写入存量 l4_rule 型接入点
	if err := db.Table("user_access_points").Create(map[string]any{
		"name": "香港·广州中转", "target_type": "l4_rule", "target_l4_rule_id": apL4ID, "enabled": true,
	}).Error; err != nil {
		t.Fatalf("create l4 access point: %v", err)
	}
	// 第二个 l4_rule 型接入点：已有自定义覆写 → 折转时覆写保留、不覆盖
	if err := db.Table("user_access_points").Create(map[string]any{
		"name": "自定覆写", "target_type": "l4_rule", "target_l4_rule_id": apL4ID,
		"custom_host": "my.edge.com", "custom_port": 8443, "enabled": true,
	}).Error; err != nil {
		t.Fatalf("create custom-override l4 access point: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate from L4 legacy schema: %v", err)
	}

	// 1. 折转：target_type=inbound + 指向规则目标入站，target_l4_rule_id 清空
	var after UserAccessPoint
	if err := db.First(&after, 1).Error; err != nil {
		t.Fatalf("load migrated ap: %v", err)
	}
	if after.TargetType != "inbound" || after.TargetInboundID == nil || *after.TargetInboundID != inb.ID {
		t.Fatalf("AP 应折转为直连入站 %d, got type=%q inbound=%v", inb.ID, after.TargetType, after.TargetInboundID)
	}
	var afterRaw legacyL4RuleAP
	if err := db.Table("user_access_points").First(&afterRaw, 1).Error; err != nil {
		t.Fatalf("load migrated ap raw: %v", err)
	}
	if afterRaw.TargetL4RuleID != nil {
		t.Fatal("target_l4_rule_id 应清空")
	}
	// 2. 覆写缺省 = 中转机 Host + 监听端口（订阅输出与旧 L4 链逐字段等价）
	if after.CustomHost != "gz.relay.com" || after.CustomPort != 30001 {
		t.Fatalf("覆写缺省应为中转端点, got host=%q port=%d", after.CustomHost, after.CustomPort)
	}
	if after.Remark == "" {
		t.Fatal("无覆写/备注的接入点应折入可追溯备注")
	}
	// 3. 已有自定义覆写保留
	var afterCust UserAccessPoint
	if err := db.First(&afterCust, 2).Error; err != nil {
		t.Fatalf("load migrated custom ap: %v", err)
	}
	if afterCust.CustomHost != "my.edge.com" || afterCust.CustomPort != 8443 {
		t.Fatalf("已有覆写应保留, got host=%q port=%d", afterCust.CustomHost, afterCust.CustomPort)
	}
	// 4. 选项 A：L4 目标入站挂层解挂
	var inbAfter Inbound
	if err := db.First(&inbAfter, inb.ID).Error; err != nil {
		t.Fatalf("load inbound: %v", err)
	}
	if inbAfter.LayerID != nil {
		t.Fatal("曾被 L4 指向的挂层入站应解挂层（层从未沿四层链路生效）")
	}
	// 5. l4_relay 服务器退役删除，l4_port_rules 表整体 Drop
	var relayN int64
	if err := db.Model(&Server{}).Where("id = ?", relay.ID).Count(&relayN).Error; err != nil {
		t.Fatalf("count relay: %v", err)
	}
	if relayN != 0 {
		t.Fatal("l4_relay 服务器应随迁移退役删除")
	}
	if db.Migrator().HasTable("l4_port_rules") {
		t.Fatal("l4_port_rules 表应被删除")
	}
	var hkN int64
	if err := db.Model(&Server{}).Where("id = ?", hkSrv.ID).Count(&hkN).Error; err != nil {
		t.Fatalf("count hk: %v", err)
	}
	if hkN != 1 {
		t.Fatal("非 l4_relay 服务器不得被误删")
	}

	// 6. 再次迁移幂等
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second migrate should be idempotent: %v", err)
	}
}

// TestDefaultOutboundDSColumnMigration 默认出口出站解析策略列名修正迁移（2026-08-31）：
// 旧库列名 default_outbound_ds（GORM 对字段名缩写 DS 不展开）与更新接口手写的
// default_outbound_domain_strategy 不一致 → PUT /admin/servers/:id 恒 500「no such column」。
// 模型列名显式统一为 API 同名后，旧列存量值必须搬入新列再删除旧列（回归：路由页保存 500）。
func TestDefaultOutboundDSColumnMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// 新模型建表（只有新列 default_outbound_domain_strategy），再手工补旧库遗留列并写入存量值（模拟升级前状态）
	if err := db.AutoMigrate(&Server{}); err != nil {
		t.Fatalf("create servers table: %v", err)
	}
	if err := db.Exec("ALTER TABLE servers ADD COLUMN default_outbound_ds varchar(16) DEFAULT 'AsIs'").Error; err != nil {
		t.Fatalf("add legacy column: %v", err)
	}
	legacy := Server{ServerType: ServerTypeXray, Name: "香港01", Host: "hk.node.com", NodeID: "node-hk", Secret: "s"}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create server: %v", err)
	}
	if err := db.Exec("UPDATE servers SET default_outbound_ds = 'UseIP' WHERE id = ?", legacy.ID).Error; err != nil {
		t.Fatalf("seed legacy value: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	if db.Migrator().HasColumn(&Server{}, "default_outbound_ds") {
		t.Fatal("遗留列 default_outbound_ds 应被删除")
	}
	var got Server
	if err := db.First(&got, legacy.ID).Error; err != nil {
		t.Fatalf("load server: %v", err)
	}
	if got.DefaultOutboundDS != "UseIP" {
		t.Fatalf("旧列存量值应搬入新列 default_outbound_domain_strategy，got %q", got.DefaultOutboundDS)
	}

	// 幂等：再次迁移不报错、值不变
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second migrate should be idempotent: %v", err)
	}
	var again Server
	if err := db.First(&again, legacy.ID).Error; err != nil {
		t.Fatalf("reload server: %v", err)
	}
	if again.DefaultOutboundDS != "UseIP" {
		t.Fatalf("重复迁移改动存量值: UseIP → %q", again.DefaultOutboundDS)
	}
}

// TestUserSubscribeTokenBackfill 订阅 token 回填迁移（2026-08-24 修复「订阅中心拿不到订阅地址」）：
// 存量用户（早期建库/初始管理员）subscribe_token 为空 → 登录后前端订阅地址显示「加载中…」；
// AutoMigrate 必须为其补齐 64 位 hex token，已有 token 的用户保持不变，重复迁移幂等。
func TestUserSubscribeTokenBackfill(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatalf("create users table: %v", err)
	}

	legacy := User{Username: "admin@panel.local", Email: "admin@panel.local", UUID: "uuid-admin", PasswordHash: "h", Role: RoleAdmin, Status: StatusActive}
	withToken := User{Username: "u@x.com", Email: "u@x.com", UUID: "uuid-user", PasswordHash: "h", Role: RoleUser, Status: StatusActive, SubscribeToken: "already-set-token"}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy user: %v", err)
	}
	if err := db.Create(&withToken).Error; err != nil {
		t.Fatalf("create user with token: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate should backfill tokens: %v", err)
	}

	var gotWith User
	if err := db.First(&gotWith, withToken.ID).Error; err != nil {
		t.Fatalf("load user with token: %v", err)
	}
	if gotWith.SubscribeToken != "already-set-token" {
		t.Fatalf("已有 token 被改动: %q", gotWith.SubscribeToken)
	}

	var gotLegacy User
	if err := db.First(&gotLegacy, legacy.ID).Error; err != nil {
		t.Fatalf("load legacy user: %v", err)
	}
	if len(gotLegacy.SubscribeToken) != 64 {
		t.Fatalf("回填 token 长度应为 64，实际 %d", len(gotLegacy.SubscribeToken))
	}
	for _, ch := range gotLegacy.SubscribeToken {
		if !strings.ContainsRune("0123456789abcdef", ch) {
			t.Fatalf("回填 token 含非法字符: %q", gotLegacy.SubscribeToken)
		}
	}

	// 幂等：再次迁移不得改动任何 token
	tokenBefore := gotLegacy.SubscribeToken
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second migrate should be idempotent: %v", err)
	}
	var again User
	if err := db.First(&again, legacy.ID).Error; err != nil {
		t.Fatalf("reload legacy user: %v", err)
	}
	if again.SubscribeToken != tokenBefore {
		t.Fatalf("重复迁移改动了 token: %q → %q", tokenBefore, again.SubscribeToken)
	}
}

// TestPlanSnapshotBackfill 套餐快照一次性回填（2026-09-01 Xboard 式隔离）：
// 存量用户按当前套餐写入快照三列；settings 标记保证只跑一次——「改套餐未同步 + 重启」
// 不得把存量快照冲掉（这是隔离语义的核心），重复迁移幂等。
func TestPlanSnapshotBackfill(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &Plan{}, &Setting{}); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	plan := Plan{Name: "p1", PriceCents: 1000, TrafficGB: 100, DurationDays: 30, DeviceLimit: 3, PermissionGroupID: 5, Purchasable: true, Renewable: true}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	withPlan := User{Username: "a@x.com", Email: "a@x.com", UUID: "uuid-a", PasswordHash: "h", Role: RoleUser, Status: StatusActive, PlanID: plan.ID, SubscribeToken: "tok-a"}
	noPlan := User{Username: "b@x.com", Email: "b@x.com", UUID: "uuid-b", PasswordHash: "h", Role: RoleUser, Status: StatusActive, SubscribeToken: "tok-b"}
	if err := db.Create(&withPlan).Error; err != nil {
		t.Fatalf("create user with plan: %v", err)
	}
	if err := db.Create(&noPlan).Error; err != nil {
		t.Fatalf("create user without plan: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate should backfill snapshots: %v", err)
	}

	var gotWith User
	if err := db.First(&gotWith, withPlan.ID).Error; err != nil {
		t.Fatalf("load user with plan: %v", err)
	}
	wantBytes := int64(100) * 1024 * 1024 * 1024
	if gotWith.PlanTrafficBytes != wantBytes || gotWith.PlanDeviceLimit != 3 || gotWith.PlanGroupID != 5 {
		t.Fatalf("快照未按套餐回填: traffic=%d device=%d group=%d", gotWith.PlanTrafficBytes, gotWith.PlanDeviceLimit, gotWith.PlanGroupID)
	}
	var gotNo User
	if err := db.First(&gotNo, noPlan.ID).Error; err != nil {
		t.Fatalf("load user without plan: %v", err)
	}
	if gotNo.PlanTrafficBytes != 0 || gotNo.PlanDeviceLimit != 0 || gotNo.PlanGroupID != 0 {
		t.Fatal("无套餐用户快照应保持零值")
	}

	// 关键隔离性质：改套餐（未同步）后再启动，回填不得重跑、存量快照不得被冲掉
	if err := db.Model(&Plan{}).Where("id = ?", plan.ID).Update("traffic_gb", 999).Error; err != nil {
		t.Fatalf("update plan: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var again User
	if err := db.First(&again, withPlan.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if again.PlanTrafficBytes != wantBytes {
		t.Fatalf("重复迁移冲掉了存量快照: %d → %d（隔离语义失效）", wantBytes, again.PlanTrafficBytes)
	}
}
