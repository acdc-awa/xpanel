package subscribe

import (
	"errors"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// subTemplateSeedMarkKey 播种完成标记（settings 表）。
const subTemplateSeedMarkKey = "sub_template_builtin_seeded"

// SeedBuiltinSubTemplate 模板库首次初始化：把「极简基础模板」写成一条普通模板库记录，
// 使模板库开箱即有一份可用的起点，而不必让前端再硬编码一个「快捷加载」预设。
//
// 为什么用 settings 标记而不是「表为空就补种」：播种行与用户自建模板完全同权
// （可改名、可改内容、可删除），若按空表判断，用户删掉它之后重启会被复活。
// 标记写入后本函数即为空操作，播种行被删就是被删了。幂等。
//
// 调用方须保证 SubTemplate / Setting 两表已建（models.AutoMigrate 之后）。
func SeedBuiltinSubTemplate(db *gorm.DB) error {
	var mark models.Setting
	err := db.Where("key = ?", subTemplateSeedMarkKey).First(&mark).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// 标记与条目同批写入：任一步失败则下次启动重试，不会留下「标记在、条目无」的永久空库。
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&models.SubTemplate{
			Name:    BuiltinSeedSubTemplateName,
			Content: BuiltinSeedSubTemplate,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&models.Setting{Key: subTemplateSeedMarkKey, Value: "1"}).Error
	})
}
