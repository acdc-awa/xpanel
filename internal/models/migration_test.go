package models

import (
	"encoding/json"
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

// TestDefaultOutboundDSIntoOutboundsMigration 服务器级出站解析策略并退出站迁移（2026-09-17 唯一入口收口）：
// servers.default_outbound_domain_strategy（及 2026-08-31 前的遗留列 default_outbound_ds）已从模型移除，
// 出站域名解析策略唯一入口收口到出站编辑器。存量迁移必须「行为等价」——旧生成器只在该列非 AsIs 且
// 目标出站自有值为空/AsIs 时注入，故迁移须按同一优先级写入出站 settings_json，否则升级后这些
// 服务器的域名解析行为会静默退回 AsIs。列在读完后删除；幂等（重跑不改动已写入的值）。
func TestDefaultOutboundDSIntoOutboundsMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Server{}, &ServerOutbound{}); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	// 模拟升级前状态：补出服务器级策略列
	if err := db.Exec("ALTER TABLE servers ADD COLUMN default_outbound_domain_strategy varchar(16) DEFAULT 'AsIs'").Error; err != nil {
		t.Fatalf("add legacy column: %v", err)
	}

	// 服务器 A：存量 UseIP，direct 出站自有 AsIs → 应被写入 UseIP
	a := Server{ServerType: ServerTypeXray, Name: "香港01", Host: "hk.node.com", NodeID: "node-hk", Secret: "s", DefaultOutboundTag: "direct"}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("create server A: %v", err)
	}
	obA := ServerOutbound{ServerID: a.ID, Tag: "direct", Protocol: "freedom", SettingsJSON: `{"domainStrategy":"AsIs"}`, Enabled: true}
	if err := db.Create(&obA).Error; err != nil {
		t.Fatalf("create outbound A: %v", err)
	}
	if err := db.Exec("UPDATE servers SET default_outbound_domain_strategy = 'UseIP' WHERE id = ?", a.ID).Error; err != nil {
		t.Fatalf("seed legacy value A: %v", err)
	}

	// 服务器 B：存量 UseIP，direct 出站自有 UseIPv4（非 AsIs）→ 出站自有值优先，不被覆盖
	b := Server{ServerType: ServerTypeXray, Name: "日本01", Host: "jp.node.com", NodeID: "node-jp", Secret: "s", DefaultOutboundTag: "direct"}
	if err := db.Create(&b).Error; err != nil {
		t.Fatalf("create server B: %v", err)
	}
	obB := ServerOutbound{ServerID: b.ID, Tag: "direct", Protocol: "freedom", SettingsJSON: `{"domainStrategy":"UseIPv4"}`, Enabled: true}
	if err := db.Create(&obB).Error; err != nil {
		t.Fatalf("create outbound B: %v", err)
	}
	if err := db.Exec("UPDATE servers SET default_outbound_domain_strategy = 'UseIP' WHERE id = ?", b.ID).Error; err != nil {
		t.Fatalf("seed legacy value B: %v", err)
	}

	// 服务器 C：存量 UseIP，默认出口指向 vless 中转出站（非 freedom）→ 旧注入同样不生效，不动数据
	c := Server{ServerType: ServerTypeXray, Name: "美国01", Host: "us.node.com", NodeID: "node-us", Secret: "s", DefaultOutboundTag: "relay"}
	if err := db.Create(&c).Error; err != nil {
		t.Fatalf("create server C: %v", err)
	}
	obC := ServerOutbound{ServerID: c.ID, Tag: "relay", Protocol: "vless", SettingsJSON: `{"vnext":[]}`, Enabled: true}
	if err := db.Create(&obC).Error; err != nil {
		t.Fatalf("create outbound C: %v", err)
	}
	if err := db.Exec("UPDATE servers SET default_outbound_domain_strategy = 'UseIP' WHERE id = ?", c.ID).Error; err != nil {
		t.Fatalf("seed legacy value C: %v", err)
	}

	// 服务器 D：存量为 AsIs（默认）→ 不注入，出站保持原样
	d := Server{ServerType: ServerTypeXray, Name: "新加坡01", Host: "sg.node.com", NodeID: "node-sg", Secret: "s", DefaultOutboundTag: "direct"}
	if err := db.Create(&d).Error; err != nil {
		t.Fatalf("create server D: %v", err)
	}
	obD := ServerOutbound{ServerID: d.ID, Tag: "direct", Protocol: "freedom", SettingsJSON: `{"domainStrategy":"AsIs"}`, Enabled: true}
	if err := db.Create(&obD).Error; err != nil {
		t.Fatalf("create outbound D: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	// 列已删除（含遗留别名）
	for _, col := range []string{"default_outbound_domain_strategy", "default_outbound_ds"} {
		if db.Migrator().HasColumn(&Server{}, col) {
			t.Fatalf("服务器级解析策略列 %s 应被删除", col)
		}
	}

	dsOf := func(id uint64) string {
		t.Helper()
		var ob ServerOutbound
		if err := db.First(&ob, id).Error; err != nil {
			t.Fatalf("load outbound %d: %v", id, err)
		}
		var settings map[string]any
		if err := json.Unmarshal([]byte(ob.SettingsJSON), &settings); err != nil {
			t.Fatalf("unmarshal outbound %d settings: %v", id, err)
		}
		ds, _ := settings["domainStrategy"].(string)
		return ds
	}
	if got := dsOf(obA.ID); got != "UseIP" {
		t.Fatalf("存量 UseIP 应并入 direct 出站，got %q", got)
	}
	if got := dsOf(obB.ID); got != "UseIPv4" {
		t.Fatalf("出站自有非 AsIs 值不得被覆盖，got %q", got)
	}
	// 非 freedom 目标出站不带 domainStrategy 字段（旧注入同样跳过），迁移不改动其 settings
	if got := dsOf(obC.ID); got != "" {
		t.Fatalf("非 freedom 目标出站不应被写入 domainStrategy，got %q", got)
	}
	if got := dsOf(obD.ID); got != "AsIs" {
		t.Fatalf("存量 AsIs 不应改动出站，got %q", got)
	}

	// 幂等：再次迁移不报错、值不变
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second migrate should be idempotent: %v", err)
	}
	if got := dsOf(obA.ID); got != "UseIP" {
		t.Fatalf("重复迁移改动存量值: UseIP → %q", got)
	}
	if got := dsOf(obB.ID); got != "UseIPv4" {
		t.Fatalf("重复迁移改动出站自有值: UseIPv4 → %q", got)
	}
}

// TestFreedomFinalRulesMigration 私网拦截下沉迁移（2026-09-17 路由层私网规则移除）：
//   - 种子形态（settings 仅 domainStrategy 一个键）→ 回填 canonical（私网 block + allow 兜底），
//     并保留其 domainStrategy（含被上一迁移改成非 AsIs 的行）；
//   - 已有 finalRules 且私网 block 缺 blockDelay → 补 "0"（维持原先路由层「立即断开」的快速失败）；
//   - 已有 blockDelay 的规则、非 freedom 出站、设置了面板开关（finalRules 已存在）的行 → 不动。
func TestFreedomFinalRulesMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Server{}, &ServerOutbound{}); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	mk := func(tag, proto, settings string) ServerOutbound {
		ob := ServerOutbound{ServerID: 1, Tag: tag, Protocol: proto, SettingsJSON: settings, Enabled: true}
		if err := db.Create(&ob).Error; err != nil {
			t.Fatalf("create outbound %s: %v", tag, err)
		}
		return ob
	}
	// 种子形态（RouterDomainStrategy 已被上一迁移改成 UseIP 的场景同样属种子形态）
	stub := mk("direct", "freedom", `{"domainStrategy":"AsIs"}`)
	stubMigrated := mk("direct2", "freedom", `{"domainStrategy":"UseIP"}`)
	// 缺 blockDelay 的私网拦截规则 → 补 0
	noDelay := mk("legacy-panel", "freedom", `{"domainStrategy":"AsIs","finalRules":[{"action":"block","ip":["geoip:private"]},{"action":"allow"}]}`)
	// 已显式指定 blockDelay → 不动
	hasDelay := mk("explicit", "freedom", `{"domainStrategy":"AsIs","finalRules":[{"action":"block","ip":["geoip:private"],"blockDelay":"30-90"},{"action":"allow"}]}`)
	// 非 freedom 出站 → 不动
	relay := mk("relay", "vless", `{"vnext":[]}`)

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	settingsOf := func(id uint64) map[string]any {
		t.Helper()
		var ob ServerOutbound
		if err := db.First(&ob, id).Error; err != nil {
			t.Fatalf("load outbound %d: %v", id, err)
		}
		var s map[string]any
		if err := json.Unmarshal([]byte(ob.SettingsJSON), &s); err != nil {
			t.Fatalf("unmarshal outbound %d: %v", id, err)
		}
		return s
	}
	privateBlockDelay := func(s map[string]any) (string, bool) {
		rules, _ := s["finalRules"].([]any)
		for _, r := range rules {
			m, _ := r.(map[string]any)
			if action, _ := m["action"].(string); action != "block" {
				continue
			}
			if ips, _ := m["ip"].([]any); len(ips) > 0 && ips[0] == "geoip:private" {
				d, _ := m["blockDelay"].(string)
				return d, true
			}
		}
		return "", false
	}

	// 种子行：回填 canonical 且保留 domainStrategy
	s := settingsOf(stub.ID)
	if ds, _ := s["domainStrategy"].(string); ds != "AsIs" {
		t.Errorf("种子行 domainStrategy 被改动: %q", ds)
	}
	if d, ok := privateBlockDelay(s); !ok || d != "0" {
		t.Errorf("种子行应回填私网 block 且 blockDelay=0, got (%q,%v): %v", d, ok, s)
	}
	rules, _ := s["finalRules"].([]any)
	if len(rules) != 2 {
		t.Errorf("种子行 finalRules 应为 block + allow 两条: %v", rules)
	} else if action, _ := rules[1].(map[string]any)["action"].(string); action != "allow" {
		t.Errorf("种子行 finalRules 末位应为 allow: %v", rules)
	}

	// 上一迁移改成 UseIP 的种子行：回填时须保留 UseIP
	s2 := settingsOf(stubMigrated.ID)
	if ds, _ := s2["domainStrategy"].(string); ds != "UseIP" {
		t.Errorf("种子行回填丢失 domainStrategy: %q", ds)
	}
	if d, ok := privateBlockDelay(s2); !ok || d != "0" {
		t.Errorf("种子行应回填私网 block: (%q,%v)", d, ok)
	}

	// 缺 blockDelay → 补 0
	if d, ok := privateBlockDelay(settingsOf(noDelay.ID)); !ok || d != "0" {
		t.Errorf("缺 blockDelay 的私网规则应补 0, got (%q,%v)", d, ok)
	}
	// 已显式指定 → 不覆盖
	if d, _ := privateBlockDelay(settingsOf(hasDelay.ID)); d != "30-90" {
		t.Errorf("显式 blockDelay 不应被覆盖, got %q", d)
	}
	// 非 freedom → 不动
	if _, ok := privateBlockDelay(settingsOf(relay.ID)); ok {
		t.Error("非 freedom 出站不应被写入 finalRules")
	}

	// 幂等
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second migrate should be idempotent: %v", err)
	}
	if d, _ := privateBlockDelay(settingsOf(stub.ID)); d != "0" {
		t.Errorf("重复迁移改动种子行: %q", d)
	}
	if d, _ := privateBlockDelay(settingsOf(hasDelay.ID)); d != "30-90" {
		t.Errorf("重复迁移覆盖显式 blockDelay: %q", d)
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

// TestUserFollowPlanGroupBackfill 权限组跟随套餐一次性归位（2026-09-17）：
// 旧购买路径把套餐权限组写进用户自定义列，形成「与套餐同值的假自定义」。
// 归位只清 permission_group_id == plan_group_id 的行（生效组由快照回落，权限不变）；
// 取值不同的行是管理员真实自定义分组，必须保持原样。
func TestUserFollowPlanGroupBackfill(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &Plan{}, &Setting{}); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	plan := Plan{Name: "p1", PriceCents: 1000, TrafficGB: 100, DurationDays: 30, PermissionGroupID: 5, Purchasable: true, Renewable: true}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	// dirty：旧购买路径写下的假自定义（自定义列 == 套餐快照）
	dirty := User{Username: "dirty@x.com", Email: "dirty@x.com", UUID: "uuid-d", PasswordHash: "h", Role: RoleUser, Status: StatusActive, SubscribeToken: "tok-d", PlanID: plan.ID, PermissionGroupID: 5, PlanGroupID: 5}
	// custom：管理员显式设的自定义分组（与套餐快照不同值）——必须保持
	custom := User{Username: "custom@x.com", Email: "custom@x.com", UUID: "uuid-c", PasswordHash: "h", Role: RoleUser, Status: StatusActive, SubscribeToken: "tok-c", PlanID: plan.ID, PermissionGroupID: 9, PlanGroupID: 5}
	if err := db.Create(&dirty).Error; err != nil {
		t.Fatalf("create dirty user: %v", err)
	}
	if err := db.Create(&custom).Error; err != nil {
		t.Fatalf("create custom user: %v", err)
	}

	if err := backfillUserFollowPlanGroup(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var gotDirty User
	if err := db.First(&gotDirty, dirty.ID).Error; err != nil {
		t.Fatalf("load dirty: %v", err)
	}
	if gotDirty.PermissionGroupID != 0 {
		t.Fatalf("假自定义应归位为 0，实际 %d", gotDirty.PermissionGroupID)
	}
	if got := gotDirty.EffectiveGroupID(); got != 5 {
		t.Fatalf("归位后生效组须仍为快照 5（权限不变），实际 %d", got)
	}

	var gotCustom User
	if err := db.First(&gotCustom, custom.ID).Error; err != nil {
		t.Fatalf("load custom: %v", err)
	}
	if gotCustom.PermissionGroupID != 9 {
		t.Fatalf("真实自定义分组不得被归位，实际 %d", gotCustom.PermissionGroupID)
	}

	// 幂等：重复执行不改动已归位的行（且标记存在时直接短路）
	if err := backfillUserFollowPlanGroup(db); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if err := db.First(&gotCustom, custom.ID).Error; err != nil {
		t.Fatalf("reload custom: %v", err)
	}
	if gotCustom.PermissionGroupID != 9 {
		t.Fatalf("重复归位改动了自定义分组，实际 %d", gotCustom.PermissionGroupID)
	}
}

// TestTrafficBilledBackfill 流量计费两列一次性回填（2026-09-06 倍率计费）：
// 存量行按 1:1 回填（billed = 原始字节，等价倍率 1）；settings 标记只跑一次——
// 回填后新语义落库的免费行（ratio=0：raw>0、billed=0）不得被重启重跑错误抬回原值。
func TestTrafficBilledBackfill(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&TrafficLog{}, &Setting{}); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	if err := db.Create(&TrafficLog{UserID: 1, InboundID: 1, UpBytes: 100, DownBytes: 200, PeriodStart: time.Now()}).Error; err != nil {
		t.Fatalf("create legacy log: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate should backfill billed: %v", err)
	}
	var got TrafficLog
	if err := db.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.BilledUp != 100 || got.BilledDown != 200 {
		t.Fatalf("回填 billed = %d/%d, want 100/200", got.BilledUp, got.BilledDown)
	}

	// 模拟新版本落库的免费入站行（ratio=0：raw>0、billed=0），重启重跑不得抬回原值
	if err := db.Create(&TrafficLog{UserID: 2, InboundID: 1, UpBytes: 500, DownBytes: 0, PeriodStart: time.Now()}).Error; err != nil {
		t.Fatalf("create free log: %v", err)
	}
	// 人工清零 billed 模拟免费行（上面 Create 默认 billed=0，无需额外处理）
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second migrate should be idempotent: %v", err)
	}
	var free TrafficLog
	if err := db.Where("user_id = ?", 2).First(&free).Error; err != nil {
		t.Fatal(err)
	}
	if free.BilledUp != 0 || free.BilledDown != 0 {
		t.Fatalf("重跑回填污染了免费行 billed = %d/%d, want 0/0", free.BilledUp, free.BilledDown)
	}
}
