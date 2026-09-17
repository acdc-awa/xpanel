package models

import (
	"strconv"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newSchemaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

// 全新库放行；迁移并记录后版本信息可读、同版本再检查仍放行。
func TestSchemaCompatFreshAndRecorded(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := CheckSchemaCompat(db); err != nil {
		t.Fatalf("全新库应放行: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := RecordSchemaVersion(db, "v1.0.0"); err != nil {
		t.Fatalf("record: %v", err)
	}
	si := ReadSchemaInfo(db)
	if si.Version != strconv.Itoa(DBSchemaVersion) || si.MinCompatible != strconv.Itoa(DBMinCompatibleVersion) ||
		si.MigratedBy != "v1.0.0" || si.Expected != DBSchemaVersion {
		t.Fatalf("schema info = %+v", si)
	}
	if err := CheckSchemaCompat(db); err != nil {
		t.Fatalf("同版本应放行: %v", err)
	}
}

// 版本号引入前的老库（有 settings 表但无版本记录）必须放行，交由 AutoMigrate 向前迁移。
func TestSchemaCompatLegacyDBAllowed(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := db.AutoMigrate(&Setting{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	if err := db.Create(&Setting{Key: "some_legacy_key", Value: "x"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := CheckSchemaCompat(db); err != nil {
		t.Fatalf("老库应放行: %v", err)
	}
}

// 库被更新版本面板迁移过（最低兼容版本更高）时必须拒绝旧面板启动。
func TestSchemaCompatRejectsNewerDB(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := upsertSetting(db, settingSchemaMinCompatible, strconv.Itoa(DBSchemaVersion+1)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := CheckSchemaCompat(db); err == nil {
		t.Fatalf("库由更新版本迁移时应拒绝启动")
	}
}

// v2 起护栏首次实际生效：v2 面板迁移并记录的库，用 v1 面板（模拟 checkSchemaCompat(db, 1)）读必须被
// 拒绝，且文案要给出出路。这是「删列 + 一次性语义回填」不可回滚的唯一防线。
func TestSchemaCompatRejectsRollbackToV1(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := RecordSchemaVersion(db, "v2.0.0"); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := CheckSchemaCompat(db); err != nil {
		t.Fatalf("当前面板读自己迁移的库应放行: %v", err)
	}
	if DBMinCompatibleVersion <= 1 {
		t.Fatalf("DBMinCompatibleVersion = %d：未排除 v1 面板，回滚护栏形同虚设", DBMinCompatibleVersion)
	}
	err := checkSchemaCompat(db, 1)
	if err == nil {
		t.Fatal("v1 面板读 v2 库应被拒绝启动")
	}
	if !strings.Contains(err.Error(), "备份") {
		t.Fatalf("拒绝文案未给出恢复出路: %v", err)
	}
}

// 记录幂等：重复记录只更新 migrated_by，不产生重复键。
func TestRecordSchemaVersionIdempotent(t *testing.T) {
	db := newSchemaTestDB(t)
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := RecordSchemaVersion(db, "v1"); err != nil {
		t.Fatalf("record #1: %v", err)
	}
	if err := RecordSchemaVersion(db, "v2"); err != nil {
		t.Fatalf("record #2: %v", err)
	}
	if si := ReadSchemaInfo(db); si.MigratedBy != "v2" {
		t.Fatalf("migrated_by = %q, want v2", si.MigratedBy)
	}
	var n int64
	if err := db.Model(&Setting{}).Where("key = ?", settingSchemaVersion).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("schema_version 记录应唯一, got %d", n)
	}
}
