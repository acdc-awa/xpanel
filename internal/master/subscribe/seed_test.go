package subscribe

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func seedTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SubTemplate{}, &models.Setting{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() }) // Windows 文件锁：必须在 TempDir 清理前关闭
	return db
}

// TestSeedBuiltinSubTemplate_Once 首次播种写入一条普通模板库记录，重复调用不产生副本。
func TestSeedBuiltinSubTemplate_Once(t *testing.T) {
	db := seedTestDB(t)

	if err := SeedBuiltinSubTemplate(db); err != nil {
		t.Fatalf("首次播种失败: %v", err)
	}
	var list []models.SubTemplate
	if err := db.Find(&list).Error; err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("播种后应恰好 1 条模板，实际 %d", len(list))
	}
	if list[0].Name != BuiltinSeedSubTemplateName {
		t.Errorf("播种名 = %q，期望 %q", list[0].Name, BuiltinSeedSubTemplateName)
	}
	if list[0].Content != BuiltinSeedSubTemplate {
		t.Error("播种正文与 BuiltinSeedSubTemplate 不一致")
	}

	// 幂等：再跑两次仍是同一条
	for i := 0; i < 2; i++ {
		if err := SeedBuiltinSubTemplate(db); err != nil {
			t.Fatalf("第 %d 次重复播种报错: %v", i+2, err)
		}
	}
	if err := db.Find(&list).Error; err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("重复播种产生了副本，模板数 = %d", len(list))
	}
}

// TestSeedBuiltinSubTemplate_NoResurrectAfterDelete 播种行与自建模板同权：
// 删除后再次启动（重跑播种）不得复活——否则管理员删掉的内置模板会反复出现。
func TestSeedBuiltinSubTemplate_NoResurrectAfterDelete(t *testing.T) {
	db := seedTestDB(t)
	if err := SeedBuiltinSubTemplate(db); err != nil {
		t.Fatal(err)
	}

	var seeded models.SubTemplate
	if err := db.Where("name = ?", BuiltinSeedSubTemplateName).First(&seeded).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&models.SubTemplate{}, seeded.ID).Error; err != nil {
		t.Fatal(err)
	}

	if err := SeedBuiltinSubTemplate(db); err != nil {
		t.Fatalf("删除后重跑播种报错: %v", err)
	}
	var count int64
	if err := db.Model(&models.SubTemplate{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("被删除的播种模板复活了，模板数 = %d", count)
	}
}

// TestBuiltinSeedSubTemplate_Shape 播种正文是一份可用的起点：
// 带节点池与全节点占位符、多一条面板域名防回环规则，且与回落默认模板共用同一正文主体。
func TestBuiltinSeedSubTemplate_Shape(t *testing.T) {
	for _, ph := range []string{"$PROXIES$", "$ALL_PROXIES$", "$PANEL_HOST$"} {
		if !strings.Contains(BuiltinSeedSubTemplate, ph) {
			t.Errorf("播种模板缺少占位符 %s", ph)
		}
	}
	if !strings.Contains(BuiltinSeedSubTemplate, "DOMAIN,$PANEL_HOST$,DIRECT") {
		t.Error("播种模板缺少面板域名防回环规则")
	}
	// 与默认模板共用主体：去掉 rules 段后两份正文必须完全一致（防止再次各写一份全文而漂移）
	body := func(s string) string { return s[:strings.Index(s, "rules:\n")] }
	if body(BuiltinSeedSubTemplate) != body(BuiltinDefaultClashTemplate) {
		t.Error("播种模板与回落默认模板的正文主体已分叉")
	}
	// 回落默认模板保持原样：不带防回环规则，避免改动存量权限组的订阅输出
	if strings.Contains(BuiltinDefaultClashTemplate, "DOMAIN,$PANEL_HOST$,DIRECT") {
		t.Error("回落默认模板不应包含防回环规则（会改变空模板权限组的订阅输出）")
	}
	// 两份模板都必须是合法 YAML 骨架（占位符按编辑器同规则中性化后解析）
	for name, tmpl := range map[string]string{
		"BuiltinDefaultClashTemplate": BuiltinDefaultClashTemplate,
		"BuiltinSeedSubTemplate":      BuiltinSeedSubTemplate,
	} {
		rendered := BuildClashWithTemplate(nil, tmpl, "panel.example.com")
		if strings.Contains(rendered, "$") {
			t.Errorf("%s 渲染后仍残留未替换的占位符:\n%s", name, rendered)
		}
	}
}
