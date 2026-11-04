package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestNormalizeNoticeSortOrder 覆盖升级时的一次性归一：按旧口径 (is_pinned DESC, sort_order DESC, id DESC)
// 的可见顺序重写 0..N-1，且只执行一次（标记存在时不再反转管理员后来拖好的顺序）。
func TestNormalizeNoticeSortOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:notice_sort?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite memory db: %v", err)
	}
	if err := db.AutoMigrate(&Notice{}, &Setting{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	// 旧数据：id 越大越新；sort_order 当作权重（越大越靠前）；#4 置顶。
	rows := []Notice{
		{Title: "A", Content: "x", SortOrder: 0},
		{Title: "B", Content: "x", SortOrder: 5},
		{Title: "C", Content: "x", SortOrder: 0},
		{Title: "P", Content: "x", SortOrder: 0, IsPinned: true},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create notice %d: %v", i, err)
		}
	}

	if err := normalizeNoticeSortOrder(db); err != nil {
		t.Fatalf("normalizeNoticeSortOrder: %v", err)
	}

	var list []Notice
	if err := db.Order("is_pinned DESC, sort_order ASC, id ASC").Find(&list).Error; err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []string{"P", "B", "C", "A"}
	if len(list) != len(want) {
		t.Fatalf("want %d notices, got %d", len(want), len(list))
	}
	for i, title := range want {
		if list[i].Title != title {
			t.Fatalf("第 %d 位期望 %s，实际 %s", i, title, list[i].Title)
		}
		if list[i].SortOrder != i {
			t.Fatalf("%s 的 sort_order 期望 %d，实际 %d", title, i, list[i].SortOrder)
		}
	}

	// 模拟管理员拖拽后的顺序：只改 sort_order，再跑一次归一必须不再生效。
	for i := range list {
		if err := db.Model(&Notice{}).Where("id = ?", list[i].ID).UpdateColumn("sort_order", i).Error; err != nil {
			t.Fatalf("update sort_order: %v", err)
		}
	}
	if err := db.Model(&Notice{}).Where("title = ?", "A").UpdateColumn("sort_order", 0).Error; err != nil {
		t.Fatalf("move A to top: %v", err)
	}
	if err := normalizeNoticeSortOrder(db); err != nil {
		t.Fatalf("normalizeNoticeSortOrder second run: %v", err)
	}
	var a Notice
	if err := db.Where("title = ?", "A").First(&a).Error; err != nil {
		t.Fatalf("load A: %v", err)
	}
	if a.SortOrder != 0 {
		t.Fatalf("标记存在时不应再次归一，A 的 sort_order 实际 %d", a.SortOrder)
	}
}
