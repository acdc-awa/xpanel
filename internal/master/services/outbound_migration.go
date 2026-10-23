package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
)

// MigrateLegacyProxyOutbounds 把旧版「外部代理节点」出站折叠为自定义 JSON。
//
// 旧模型用 protocol + settings_json/stream_settings_json 描述外部代理节点（editor 的
// 「外部代理节点」场景）。新模型只保留三种形态：基础出口（freedom/blackhole）、引用入站
// （仅 inbound_ref）、自定义 JSON（仅 outbound_json）。本迁移把旧行的两列合并为一份完整
// outbound JSON，行协议记为 custom，清空旧列。
//
// 幂等：outbound_json 非空的行不会再被选中；单行解析失败只记日志跳过，不阻断启动。
// 旧面板同样支持 outbound_json + protocol=custom，因此降级仍可读写（无 schema 版本提升）。
func MigrateLegacyProxyOutbounds(db *gorm.DB) error {
	var rows []models.ServerOutbound
	err := db.Where(
		"inbound_ref IS NULL AND (outbound_json IS NULL OR outbound_json = '') AND owner_inbound_id = 0 " +
			"AND protocol NOT IN ('freedom','blackhole') AND (settings_json != '' OR stream_settings_json != '')",
	).Find(&rows).Error
	if err != nil {
		return err
	}

	migrated, skipped := 0, 0
	for i := range rows {
		ob := &rows[i]
		raw, foldErr := foldLegacyOutboundJSON(ob)
		if foldErr != nil {
			skipped++
			log.Printf("出站 #%d %q 旧版代理配置迁移失败（保留原样）: %v", ob.ID, ob.Tag, foldErr)
			continue
		}
		if err := db.Model(&models.ServerOutbound{}).Where("id = ?", ob.ID).Updates(map[string]any{
			"outbound_json":        raw,
			"protocol":             "custom",
			"settings_json":        "",
			"stream_settings_json": "",
		}).Error; err != nil {
			return fmt.Errorf("迁移出站 #%d 失败: %w", ob.ID, err)
		}
		migrated++
	}
	if migrated > 0 || skipped > 0 {
		log.Printf("旧版代理出站迁移：已转换 %d 条，跳过 %d 条", migrated, skipped)
	}
	return nil
}

// foldLegacyOutboundJSON 按旧生成器的合并语义把 settings/stream 折叠成完整出站对象。
func foldLegacyOutboundJSON(ob *models.ServerOutbound) (string, error) {
	item := map[string]any{}
	if raw := strings.TrimSpace(ob.SettingsJSON); raw != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return "", fmt.Errorf("settings_json 非法: %w", err)
		}
		// 旧生成器：含 protocol/settings 键视为完整对象并摊到顶层，否则视为裸 settings。
		_, hasProto := parsed["protocol"]
		_, hasSettings := parsed["settings"]
		if hasProto || hasSettings {
			for k, v := range parsed {
				item[k] = v
			}
		} else {
			item["settings"] = parsed
		}
	}
	if raw := strings.TrimSpace(ob.StreamSettingsJSON); raw != "" {
		var stream map[string]any
		if err := json.Unmarshal([]byte(raw), &stream); err != nil {
			return "", fmt.Errorf("stream_settings_json 非法: %w", err)
		}
		// 出站是客户端角色：保留 fingerprint/publicKey/spiderX/hysteria auth 等字段。
		item["streamSettings"] = xray.SanitizeOutboundStreamMap(stream)
	}
	if _, ok := item["protocol"]; !ok {
		item["protocol"] = ob.Protocol
	}
	out, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}
