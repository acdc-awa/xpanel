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
