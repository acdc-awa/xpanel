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

// TestDeclareSchemaMinCompatibleBlocksOlderPanelBeforeMigration 回归：护栏标记必须在**迁移之前**
// 就已生效。
//
// 缺陷形态：标记原先只在迁移全部跑完后由 RecordSchemaVersion 写（且失败仅记日志），于是存在
// 「结构已改成 v3、标记仍是 v2」的窗口。面板内自更新的 entrypoint 会在新版本启动失败时自动
// 回滚旧二进制（deploy/master/entrypoint.sh），窗口内回滚上来的旧面板会被 CheckSchemaCompat
// 放行，而它的三列 upsert 在四列唯一索引上找不到冲突目标 ⇒ 每次流量写入硬失败，且旧面板不发
// traffic_ack、节点发完即删 ⇒ 流量记账静默停摆、配额判定不再累积。
func TestDeclareSchemaMinCompatibleBlocksOlderPanelBeforeMigration(t *testing.T) {
	db := newSchemaTestDB(t)
	// 前置：模拟一个已被上一版面板迁移过的生产库——settings 表存在且 min_compatible 记录为低一版。
	// 必须这样铺，否则「无 settings 表 ⇒ CheckSchemaCompat 直接放行」的早返回会掩盖真正要测的路径。
	if err := db.AutoMigrate(&Setting{}); err != nil {
		t.Fatalf("migrate settings: %v", err)
	}
	if err := upsertSetting(db, settingSchemaMinCompatible, strconv.Itoa(DBSchemaVersion-1)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := CheckSchemaCompat(db); err != nil {
		t.Fatalf("旧库对本面板应放行: %v", err)
	}
	if err := DeclareSchemaMinCompatible(db); err != nil {
		t.Fatalf("declare: %v", err)
	}

	// 关键断言：此刻**尚未执行 AutoMigrate**，模拟「声明已落、迁移未跑完（或跑到一半失败、进程被杀）」
	// 的窗口。旧面板（schema 低一版）必须已被拒绝——这正是修复前不成立的断言。
	if err := checkSchemaCompat(db, DBSchemaVersion-1); err == nil {
		t.Fatal("声明之后、迁移之前旧面板就必须被拒绝；否则自动回滚上来的旧面板会带病启动")
	}
	// 本面板自己仍放行，保证声明后能幂等续跑完迁移
	if err := checkSchemaCompat(db, DBSchemaVersion); err != nil {
		t.Fatalf("同版本应放行（声明不得锁死自己）: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := checkSchemaCompat(db, DBSchemaVersion-1); err == nil {
		t.Fatal("迁移完成后旧面板仍须被拒绝")
	}
}

// TestDeclareSchemaMinCompatibleFreshDBAndIdempotent 全新库（尚无 settings 表）也能落下标记，
// 且重复调用无副作用、不影响随后的 AutoMigrate。
func TestDeclareSchemaMinCompatibleFreshDBAndIdempotent(t *testing.T) {
	db := newSchemaTestDB(t)
	if db.Migrator().HasTable(&Setting{}) {
		t.Fatal("前置条件：全新库不应存在 settings 表")
	}
	for i := 1; i <= 3; i++ {
		if err := DeclareSchemaMinCompatible(db); err != nil {
			t.Fatalf("declare #%d: %v", i, err)
		}
	}
	if got := readSettingValue(db, settingSchemaMinCompatible); got != strconv.Itoa(DBMinCompatibleVersion) {
		t.Fatalf("min_compatible = %q, want %d", got, DBMinCompatibleVersion)
	}
	var n int64
	if err := db.Model(&Setting{}).Where("key = ?", settingSchemaMinCompatible).Count(&n).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("min_compatible 记录应唯一, got %d", n)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("提前建出的 settings 表不得影响 AutoMigrate: %v", err)
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
