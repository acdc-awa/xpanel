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
