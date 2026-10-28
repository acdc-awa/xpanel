package models

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestNormalizeServerSortOrder 老库重复/稀疏排序值的归一：
// 可见顺序必须逐台不变（仍按 sort_order, id 排），软删除行不参与，重复执行幂等。
func TestNormalizeServerSortOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:mem_sort_order?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	mk := func(name string, sort int) uint64 {
		s := Server{Name: name, Host: "h", NodeID: "node-" + name, Secret: "sec", SortOrder: sort}
		if err := db.Create(&s).Error; err != nil {
			t.Fatalf("create server: %v", err)
		}
		return s.ID
	}
	first := mk("a", 0)
	second := mk("b", 0)
	third := mk("c", 0)
	fourth := mk("d", 2)
	archived := mk("gone", 0)
	if err := db.Delete(&Server{}, archived).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	// 软删除行归一前是什么就保持什么
	var beforeArchived Server
	if err := db.Unscoped().First(&beforeArchived, archived).Error; err != nil {
		t.Fatalf("read archived: %v", err)
	}

	if err := normalizeServerSortOrder(db); err != nil {
		t.Fatalf("normalize: %v", err)
	}

	got := map[uint64]int{}
	var rows []Server
	if err := db.Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, row := range rows {
		got[row.ID] = row.SortOrder
	}
	want := []uint64{first, second, third, fourth}
	for i, id := range want {
		if got[id] != i {
			t.Fatalf("server %d sort_order = %d, want %d (all=%v)", id, got[id], i, got)
		}
	}
	if _, ok := got[archived]; ok {
		t.Fatalf("软删除服务器不应出现在列表里: %v", got)
	}
	var afterArchived Server
	if err := db.Unscoped().First(&afterArchived, archived).Error; err != nil {
		t.Fatalf("read archived after: %v", err)
	}
	if afterArchived.SortOrder != beforeArchived.SortOrder {
		t.Fatalf("软删除行的排序被改动: %d → %d", beforeArchived.SortOrder, afterArchived.SortOrder)
	}

	// 幂等：再跑一次不应改变任何值。
	if err := normalizeServerSortOrder(db); err != nil {
		t.Fatalf("normalize second run: %v", err)
	}
	var again []Server
	if err := db.Order("sort_order ASC, id ASC").Find(&again).Error; err != nil {
		t.Fatalf("list again: %v", err)
	}
	for i, row := range again {
		if row.SortOrder != i {
			t.Fatalf("second run changed order: server %d sort_order = %d, want %d", row.ID, row.SortOrder, i)
		}
	}
}

// TestNormalizeServerSortOrderEmptyTable 空表/无 servers 表时必须是无害空操作。
func TestNormalizeServerSortOrderEmptyTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:mem_sort_empty_%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := normalizeServerSortOrder(db); err != nil {
		t.Fatalf("missing table should be a no-op: %v", err)
	}
	if err := db.AutoMigrate(&Server{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	if err := normalizeServerSortOrder(db); err != nil {
		t.Fatalf("empty table should be a no-op: %v", err)
	}
}
