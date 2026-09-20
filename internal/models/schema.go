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
// 首次实际提升发生在 v2（2026-09-17，删列 + 两处一次性语义回填）；v1 时期的迁移（删表/删列）
// 都发生在版本号引入之前，当时无记录可写，故历史上的库与面板一律记为 v1。
//
// 启动顺序：CheckSchemaCompat（迁移前，拒绝旧面板跑新库）→ DeclareSchemaMinCompatible（迁移前，
// 先落下护栏标记，失败致命）→ AutoMigrate → RecordSchemaVersion（补记版本号与写入者，失败仅记日志）。
const (
	// DBSchemaVersion 当前面板写入的数据库 schema 版本。变更表结构/数据语义时 +1。
	//
	// v2（2026-09-17）：servers.default_outbound_domain_strategy 退役（存量非 AsIs 值并入目标
	// freedom 出站的 settings.domainStrategy 后删列），并新增两处一次性数据语义回填——
	// freedom 出站 finalRules 归一（migrateFreedomFinalRules）、users.permission_group_id
	// 假自定义归位（backfillUserFollowPlanGroup）。
	//
	// v3（2026-09-20，流量计费审计 F1/F2/F3）：traffic_logs 加 cycle_id 列、唯一索引由
	// (user_id, inbound_id, period_start) 扩为含 cycle_id 的四列，新增 traffic_batches 去重表，
	// users 加 traffic_cycle_id 并回填为 1。计费口径从「period_start >= traffic_cycle_start」
	// 改为「cycle_id = 用户当前账期」（cycle_id=0 的存量/旧 agent 行零值回退，见 models.CycleUsageSQL）。
	DBSchemaVersion = 3

	// DBMinCompatibleVersion 能安全读取当前 schema 的最老面板 schema 版本。
	// 注意：做了不向后兼容的迁移时才与 DBSchemaVersion 同步提升。
	//
	// v2 与 DBSchemaVersion 同步提升，依据是「回滚后无法自愈」而非「v1 读不了」：v1 面板实测
	// 仍能启动并读写该库（GORM 会把被删的列补回来），但一次回滚会留下两处再无人修的分叉——
	//  ① v1 购买路径重新写入 permission_group_id 假自定义，而归位迁移的 settings 标记已消费，
	//     再升级不会重跑；
	//  ② 回滚期间写入的服务器级解析策略，会在下次升级时被「出站自有值优先」跳过，随后列被删除，
	//     该值静默消失（出站卡片不展示生效值，界面无从察觉）。
	// 按取舍从严：宁可拒绝回滚，也不接受静默分叉。代价是「在面板里安装历史版本」降到 v1 时会被
	// 本护栏拒绝启动；出路是前进到 v2，或从升级前备份恢复（备份恢复路径同样比对本版本号，
	// 见 backup/restore.go）。
	//
	// v3 与 DBSchemaVersion 同步提升，依据是**硬故障**而非静默分叉：v2 面板的落库 upsert 以
	// 三列唯一索引为冲突目标，而 v3 迁移把该索引换成了四列——v2 面板回滚上来后每次流量写入都会
	// 报 "ON CONFLICT clause does not match any PRIMARY KEY or UNIQUE constraint"，节点流量
	// 全部丢弃。同时 v2 面板写入的行不带 cycle_id（恒 0），在 v3 口径下按「未知账期」回退归属，
	// 会与切换后的新账期混算。故拒绝回滚，出路是前进到 v3 或从升级前备份恢复。
	DBMinCompatibleVersion = 3
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
	return checkSchemaCompat(db, DBSchemaVersion)
}

// checkSchemaCompat 按指定的面板 schema 版本执行校验，供测试模拟「旧面板读新库」。
func checkSchemaCompat(db *gorm.DB, panelVersion int) error {
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
	if min > panelVersion {
		return fmt.Errorf(
			"数据库 schema 版本不兼容：该库由更新版本的面板迁移过（最低兼容 schema v%d），当前面板 schema v%d 过旧，"+
				"继续运行可能读错或写坏数据。请升级面板至不低于该版本，或从升级前的数据库备份恢复后再启动",
			min, panelVersion)
	}
	return nil
}

// DeclareSchemaMinCompatible 在**执行任何结构迁移之前**落下最低兼容版本标记（= 本面板的
// DBMinCompatibleVersion）。调用方须在 CheckSchemaCompat 通过之后、AutoMigrate 之前调用，
// 且**必须把失败当作致命错误**：标记写不上却继续改结构，等于先把护栏关掉再动手。
//
// 为什么必须早于迁移：CheckSchemaCompat 读的正是这个键，它是「旧面板读新库」的唯一拦截点。
// 若沿用「迁移全部跑完才写」的顺序，就存在一个「结构已改成 v3、标记仍是 v2」的窗口，窗口内
// 启动的旧面板会被护栏放行，而它的三列 upsert 在四列唯一索引上找不到冲突目标，每次流量写入
// 都硬失败（见 dropLegacyTrafficUniqueIndexes 的说明）。
//
// 这个窗口不是理论问题：面板内自更新的 entrypoint 会在新版本启动失败时自动回滚旧二进制
// （deploy/master/entrypoint.sh），于是「新面板迁移到一半失败 → 自动回滚 → 旧面板带病启动」
// 是一条真实路径；此外 RecordSchemaVersion 的失败只记日志不中止启动，进程在两步之间被 kill
// 也会留下同一状态。先声明即 fail-closed：崩溃后留下的库旧面板读不了（拒绝启动并给出出路），
// 而新面板能幂等续跑完迁移。
//
// 代价是「声明已写但迁移未跑完」时旧面板同样被拒——这正是不可逆迁移的应有语义（见 UPGRADE.md
// §4「跨越 DBMinCompatibleVersion 提升的版本不可回退」），出路是前进到新版或从升级前备份恢复。
//
// 幂等。settings 表不存在（全新库）时先建该表：它是与版本无关的键值表，提前建出不参与任何
// 迁移判定，也不会影响随后 AutoMigrate 的结果。
func DeclareSchemaMinCompatible(db *gorm.DB) error {
	if !db.Migrator().HasTable(&Setting{}) {
		if err := db.AutoMigrate(&Setting{}); err != nil {
			return fmt.Errorf("建 settings 表失败: %w", err)
		}
	}
	if err := upsertSetting(db, settingSchemaMinCompatible, strconv.Itoa(DBMinCompatibleVersion)); err != nil {
		return fmt.Errorf("声明最低兼容 schema 版本失败: %w", err)
	}
	return nil
}

// RecordSchemaVersion 在 AutoMigrate 成功后写入/更新版本记录（幂等 upsert）。
// appVersion 记录写入者身份，便于排查「哪次升级改了库」。
//
// 承重的 settingSchemaMinCompatible 已由 DeclareSchemaMinCompatible 在迁移前落下（且失败致命），
// 故本函数写的是「迁移已跑完」的最终状态与展示信息；它的失败只需记日志，不会打开护栏缺口。
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
