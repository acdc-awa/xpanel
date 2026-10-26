package subscribe

import (
	"errors"
	"log"
	"strings"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// 订阅模板的生效模式与来源（2026-10-26 权限组绑定模板库）。
//
// 数据模型只有一列：PermissionGroup.SubTemplateID。
//   - NULL = 自定义（空键），生效内容读 PermissionGroup.ClashTemplate；
//   - 非空 = 绑定模板库条目，生效内容读 SubTemplate.Content（活引用）。
//
// Mode 描述「绑定键的状态」，Source 描述「最终内容从哪来」。二者在正常数据下一致，
// 只有悬挂绑定（模板行已不存在/内容为空）会分叉：Mode=custom + Source=custom/default。
const (
	TemplateModeCustom = "custom"
	TemplateModeBound  = "bound"

	TemplateSourceBound   = "bound"
	TemplateSourceCustom  = "custom"
	TemplateSourceDefault = "default"
)

// TemplateResolution 权限组订阅模板的解析结果。
type TemplateResolution struct {
	Content      string // 最终模板正文；空串表示走内置默认模板
	Mode         string // custom / bound：权限组当前是自定义还是绑定
	Source       string // bound / custom / default：Content 的实际来源
	TemplateID   uint64 // 绑定模板 ID（Mode=bound 时有值）
	TemplateName string // 绑定模板名（Mode=bound 且行存在时有值）
	Missing      bool   // 绑定的模板行不存在（已降级为自定义）
}

// ResolveTemplateFromRows 纯函数：给定权限组与已查出的绑定模板行，按唯一口径解析生效模板。
// tpl 为 nil 表示未绑定或绑定行不存在；批量列表路径用它避免 N+1 查询。
func ResolveTemplateFromRows(pg *models.PermissionGroup, tpl *models.SubTemplate) TemplateResolution {
	res := TemplateResolution{Mode: TemplateModeCustom, Source: TemplateSourceDefault}
	if pg == nil {
		return res
	}
	if pg.SubTemplateID != nil && *pg.SubTemplateID > 0 {
		res.Mode = TemplateModeBound
		res.TemplateID = *pg.SubTemplateID
		if tpl != nil && tpl.ID == *pg.SubTemplateID && strings.TrimSpace(tpl.Content) != "" {
			res.Content = tpl.Content
			res.Source = TemplateSourceBound
			res.TemplateName = tpl.Name
			return res
		}
		// 悬挂绑定（行不存在）或绑定行内容为空：降级为自定义，继续走下面的副本兜底。
		res.Mode = TemplateModeCustom
		res.Missing = tpl == nil
	}
	if strings.TrimSpace(pg.ClashTemplate) != "" {
		res.Content = pg.ClashTemplate
		res.Source = TemplateSourceCustom
		return res
	}
	return res
}

// ResolvePermissionGroupTemplate 订阅/预览的唯一读取入口：按 ID 查绑定模板后走纯函数解析。
// 模板行不存在时静默降级为组内副本（备份恢复或手工改库可能留下悬挂引用），
// 数据库错误只记日志并同样降级——订阅生成不应因模板读取失败而 500。
func ResolvePermissionGroupTemplate(db *gorm.DB, pg *models.PermissionGroup) TemplateResolution {
	var tpl *models.SubTemplate
	if pg != nil && pg.SubTemplateID != nil && *pg.SubTemplateID > 0 {
		var row models.SubTemplate
		err := db.First(&row, *pg.SubTemplateID).Error
		switch {
		case err == nil:
			tpl = &row
		case errors.Is(err, gorm.ErrRecordNotFound):
			// 悬挂绑定：降级
		default:
			log.Printf("[subscribe] 读取绑定模板 %d 失败: %v", *pg.SubTemplateID, err)
		}
	}
	return ResolveTemplateFromRows(pg, tpl)
}
