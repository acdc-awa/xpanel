package subscribe

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func resolveTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resolve.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SubTemplate{}, &models.PermissionGroup{}); err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func ptr(v uint64) *uint64 { return &v }

// TestResolveTemplateFromRows 纯函数口径：绑定优先于组内副本；悬挂/空内容降级。
func TestResolveTemplateFromRows(t *testing.T) {
	tpl := &models.SubTemplate{ID: 7, Name: "模板A", Content: "from-template"}

	cases := []struct {
		name        string
		pg          *models.PermissionGroup
		tpl         *models.SubTemplate
		wantContent string
		wantMode    string
		wantSource  string
		wantName    string
		wantMissing bool
	}{
		{
			name:        "未绑定且有副本",
			pg:          &models.PermissionGroup{ClashTemplate: "from-copy"},
			wantContent: "from-copy",
			wantMode:    TemplateModeCustom,
			wantSource:  TemplateSourceCustom,
		},
		{
			name:       "未绑定且副本为空",
			pg:         &models.PermissionGroup{},
			wantMode:   TemplateModeCustom,
			wantSource: TemplateSourceDefault,
		},
		{
			name:        "绑定且模板存在",
			pg:          &models.PermissionGroup{SubTemplateID: ptr(7), ClashTemplate: "stale-copy"},
			tpl:         tpl,
			wantContent: "from-template",
			wantMode:    TemplateModeBound,
			wantSource:  TemplateSourceBound,
			wantName:    "模板A",
		},
		{
			name:        "绑定行不存在，降级到副本",
			pg:          &models.PermissionGroup{SubTemplateID: ptr(9), ClashTemplate: "from-copy"},
			wantContent: "from-copy",
			wantMode:    TemplateModeCustom,
			wantSource:  TemplateSourceCustom,
			wantMissing: true,
		},
		{
			name:        "绑定行不存在且无副本，回落默认",
			pg:          &models.PermissionGroup{SubTemplateID: ptr(9)},
			wantMode:    TemplateModeCustom,
			wantSource:  TemplateSourceDefault,
			wantMissing: true,
		},
		{
			name:        "绑定行内容为空，降级到副本",
			pg:          &models.PermissionGroup{SubTemplateID: ptr(7), ClashTemplate: "from-copy"},
			tpl:         &models.SubTemplate{ID: 7, Name: "空模板"},
			wantContent: "from-copy",
			wantMode:    TemplateModeCustom,
			wantSource:  TemplateSourceCustom,
		},
		{
			name:       "绑定 ID 为 0 视为未绑定",
			pg:         &models.PermissionGroup{SubTemplateID: ptr(0)},
			wantMode:   TemplateModeCustom,
			wantSource: TemplateSourceDefault,
		},
		{
			name:       "nil 权限组",
			pg:         nil,
			wantMode:   TemplateModeCustom,
			wantSource: TemplateSourceDefault,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveTemplateFromRows(tc.pg, tc.tpl)
			if got.Content != tc.wantContent {
				t.Fatalf("Content=%q want %q", got.Content, tc.wantContent)
			}
			if got.Mode != tc.wantMode {
				t.Fatalf("Mode=%q want %q", got.Mode, tc.wantMode)
			}
			if got.Source != tc.wantSource {
				t.Fatalf("Source=%q want %q", got.Source, tc.wantSource)
			}
			if got.TemplateName != tc.wantName {
				t.Fatalf("TemplateName=%q want %q", got.TemplateName, tc.wantName)
			}
			if got.Missing != tc.wantMissing {
				t.Fatalf("Missing=%v want %v", got.Missing, tc.wantMissing)
			}
		})
	}
}

// TestResolvePermissionGroupTemplateDB 走数据库的读取入口：绑定优先、悬挂降级。
func TestResolvePermissionGroupTemplateDB(t *testing.T) {
	db := resolveTestDB(t)
	tpl := models.SubTemplate{Name: "模板A", Content: "from-template"}
	if err := db.Create(&tpl).Error; err != nil {
		t.Fatal(err)
	}
	bound := models.PermissionGroup{Name: "绑定组", ClashTemplate: "stale-copy", SubTemplateID: &tpl.ID}
	custom := models.PermissionGroup{Name: "自定义组", ClashTemplate: "from-copy"}
	if err := db.Create(&bound).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&custom).Error; err != nil {
		t.Fatal(err)
	}

	if got := ResolvePermissionGroupTemplate(db, &bound); got.Content != "from-template" || got.Mode != TemplateModeBound {
		t.Fatalf("绑定组应使用模板内容，实际 %+v", got)
	}
	if got := ResolvePermissionGroupTemplate(db, &custom); got.Content != "from-copy" || got.Mode != TemplateModeCustom {
		t.Fatalf("自定义组应使用副本，实际 %+v", got)
	}

	// 删除模板行（模拟备份恢复/手工改库留下的悬挂引用）：降级到副本。
	if err := db.Delete(&models.SubTemplate{}, tpl.ID).Error; err != nil {
		t.Fatal(err)
	}
	got := ResolvePermissionGroupTemplate(db, &bound)
	if got.Content != "stale-copy" || got.Mode != TemplateModeCustom || !got.Missing {
		t.Fatalf("悬挂绑定应降级到副本，实际 %+v", got)
	}
}
