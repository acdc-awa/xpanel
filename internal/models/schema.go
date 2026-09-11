package models

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// 数据库协议版本（schema versioning）。
//
// 目的有两个：
//  1. 向前兼容：新面板能迁移老库（AutoMigrate 补列/补表 + 幂等回填），老库无需人工干预即可被读取；
//  2. 降级护栏：若某次迁移做了「向后不兼容」的改动（删列/改列语义/删表），把老面板放回新库上运行
//     会读错甚至写坏数据。版本号让老面板在启动时就能识别并拒绝运行，而不是静默损坏。
//
// 两个维度：
//   - DBSchemaVersion：当前代码写入的 schema 版本。任何结构或数据语义变更都 +1。
//   - DBMinCompatibleVersion：能安全读取「该库」的最老面板 schema 版本。仅当这次迁移不向后兼容时
//     才提升到 DBSchemaVersion；纯增量（加列/加表/回填、且老代码能容忍）保持不变，
//     从而仍允许回滚到旧面板。
//
// 启动顺序：CheckSchemaCompat（迁移前，拒绝旧面板跑新库）→ AutoMigrate → RecordSchemaVersion。
const (
	// DBSchemaVersion 当前面板写入的数据库 schema 版本。变更表结构/数据语义时 +1。
	DBSchemaVersion = 1
	// DBMinCompatibleVersion 能安全读取当前 schema 的最老面板 schema 版本。
	// 注意：做了不向后兼容的迁移时才与 DBSchemaVersion 同步提升。
	DBMinCompatibleVersion = 1
)

// settings 键（复用既有 settings 键值表，避免新表）。
const (
	settingSchemaVersion       = "schema_version"
	settingSchemaMinCompatible = "schema_min_compatible"
	settingSchemaMigratedBy    = "schema_migrated_by"
)

// SchemaInfo 数据库记录的协议版本信息（系统状态页展示用；缺失字段为空串）。
type SchemaInfo struct {
	Version       string `json:"version"`        // 库当前 schema 版本
	MinCompatible string `json:"min_compatible"` // 能安全读取该库的最老面板 schema 版本
	MigratedBy    string `json:"migrated_by"`    // 最后写入该库的面板版本
	Expected      int    `json:"expected"`       // 当前面板期望的 schema 版本
}

// CheckSchemaCompat 在 AutoMigrate 之前校验：当前面板能否安全读取该数据库。
// 仅当「库记录的最低兼容版本 > 当前面板 schema 版本」时拒绝启动——这正是「用新库回滚到旧面板」
// 的场景。全新库、以及引入版本号之前的老库（无记录）一律放行，交由 AutoMigrate 向前迁移。
func CheckSchemaCompat(db *gorm.DB) error {
	if !db.Migrator().HasTable(&Setting{}) {
		return nil // 全新库：尚无任何记录
	}
	raw, ok, err := readSetting(db, settingSchemaMinCompatible)
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return nil // 老库（无记录）或读取失败：放行，避免误锁死
	}
	min, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return nil // 记录损坏：放行
	}
	if min > DBSchemaVersion {
		return fmt.Errorf(
			"数据库 schema 版本不兼容：该库由更新版本的面板迁移过（最低兼容 schema v%d），当前面板 schema v%d 过旧，"+
				"继续运行可能读错或写坏数据。请升级面板至不低于该版本，或从升级前的数据库备份恢复后再启动",
			min, DBSchemaVersion)
	}
	return nil
}

// RecordSchemaVersion 在 AutoMigrate 成功后写入/更新版本记录（幂等 upsert）。
// appVersion 记录写入者身份，便于排查「哪次升级改了库」。
func RecordSchemaVersion(db *gorm.DB, appVersion string) error {
	for k, v := range map[string]string{
		settingSchemaVersion:       strconv.Itoa(DBSchemaVersion),
		settingSchemaMinCompatible: strconv.Itoa(DBMinCompatibleVersion),
		settingSchemaMigratedBy:    appVersion,
	} {
		if err := upsertSetting(db, k, v); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", k, err)
		}
	}
	return nil
}

// ReadSchemaInfo 读取库记录的版本信息（供系统状态页展示）。
func ReadSchemaInfo(db *gorm.DB) SchemaInfo {
	return SchemaInfo{
		Version:       readSettingValue(db, settingSchemaVersion),
		MinCompatible: readSettingValue(db, settingSchemaMinCompatible),
		MigratedBy:    readSettingValue(db, settingSchemaMigratedBy),
		Expected:      DBSchemaVersion,
	}
}

func readSettingValue(db *gorm.DB, key string) string {
	raw, ok, err := readSetting(db, key)
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(raw)
}

func readSetting(db *gorm.DB, key string) (string, bool, error) {
	var s Setting
	err := db.Where("key = ?", key).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return s.Value, true, nil
}

func upsertSetting(db *gorm.DB, key, value string) error {
	var s Setting
	err := db.Where("key = ?", key).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&Setting{Key: key, Value: value}).Error
	}
	if err != nil {
		return err
	}
	return db.Model(&Setting{}).Where("key = ?", key).Update("value", value).Error
}
