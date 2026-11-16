package models

import (
	"fmt"

	"gorm.io/gorm"
)

// v7（2026-11-16）：退役 inbounds 上的四个历史列。
//
//   - share_security —— 对外安全层改由接入层决议（不挂层 = 跟随 streamSettings），
//     该列自 2026-10 起既不读也不写（前端保存时已明确不参与回写）。
//   - share_host / share_path —— HTTP Host 与客户端 path 的唯一来源改为
//     streamSettings.xhttpSettings.host / .path（订阅与反代必须同值），该两列同样不再读写。
//   - previous_type —— relay 模型退役后仅作「自动标记前的类型」备份，启动迁移早已清空，
//     通篇零引用。
//
// 四列都只剩字段声明，属于纯残留：保留只会让 GORM SELECT * 多拖无意义负载，
// 并在「原始 JSON 编辑」里透出已经无效的键，故随 v7 一并删除。
//
// 这是一次**向后不兼容**变更（旧面板的 inbounds INSERT/SELECT 会显式带这些列，
// 删列后报 "no such column"），因此 DBMinCompatibleVersion 与 DBSchemaVersion 同步提升到 7。
//
// 用原生 ALTER TABLE DROP COLUMN 而不是 glebarez 的 DropColumn：后者在 SQLite 上
// 静默失效（与 dropLegacyAccessPointHostPort 同一处理方式）。幂等——列不存在即跳过。
func dropLegacyInboundColumns(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable("inbounds") {
		return nil
	}
	table := "inbounds"
	for _, col := range []string{"share_security", "share_host", "share_path", "previous_type"} {
		if !db.Migrator().HasColumn(table, col) {
			continue
		}
		if err := db.Exec("ALTER TABLE `" + table + "` DROP COLUMN `" + col + "`").Error; err != nil {
			return fmt.Errorf("删除 inbounds 遗留列 %s 失败: %w", col, err)
		}
	}
	return nil
}
