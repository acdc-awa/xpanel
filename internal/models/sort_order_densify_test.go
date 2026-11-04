package models

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func sortOrderTestDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite memory db: %v", err)
	}
	if err := db.AutoMigrate(&Plan{}, &PermissionGroup{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

// TestNormalizePlanSortOrder 套餐归一必须按商城现行可见顺序 (sort_order, price_cents, id)：
// 升级瞬间商城逐条不变，同时把「管理端同值回退 id」与「商城同值回退价格」的旧口径差消掉。
// 已归档套餐一并参与（不能被软删除 WHERE 过滤掉），否则它们会与新序号撞车。
func TestNormalizePlanSortOrder(t *testing.T) {
	db := sortOrderTestDB(t, "file:plan_sort?mode=memory&cache=shared")

	mk := func(name string, sort int, price int64) Plan {
		p := Plan{Name: name, PriceCents: price, TrafficGB: 10, DurationDays: 30, SortOrder: sort}
		if err := db.Create(&p).Error; err != nil {
			t.Fatalf("create plan %s: %v", name, err)
		}
		return p
	}
	// 旧口径：同一 sort_order 下商城按价格升序。归档套餐也保留自己的排序值。
	pA := mk("A", 0, 3000)
	pB := mk("B", 0, 1000)
	pC := mk("C", 5, 2000)
	pD := mk("D-archived", 0, 500)
	if err := db.Delete(&Plan{}, pD.ID).Error; err != nil {
		t.Fatalf("archive D: %v", err)
	}

	if err := normalizePlanSortOrder(db); err != nil {
		t.Fatalf("normalizePlanSortOrder: %v", err)
	}

	var rows []Plan
	if err := db.Unscoped().Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []uint64{pD.ID, pB.ID, pA.ID, pC.ID}
	if len(rows) != len(want) {
		t.Fatalf("want %d plans, got %d", len(want), len(rows))
	}
	for i, id := range want {
		if rows[i].ID != id {
			t.Fatalf("第 %d 位期望 id=%d，实际 id=%d", i, id, rows[i].ID)
		}
		if rows[i].SortOrder != i {
			t.Fatalf("%s 的 sort_order 期望 %d，实际 %d", rows[i].Name, i, rows[i].SortOrder)
		}
	}

	// 模拟拖拽：把 A 拖到 B 前面（写的是稠密的新序号），再次归一必须保留拖拽结果。
	if err := db.Model(&Plan{}).Where("id = ?", pA.ID).UpdateColumn("sort_order", 1).Error; err != nil {
		t.Fatalf("move A: %v", err)
	}
	if err := db.Model(&Plan{}).Where("id = ?", pB.ID).UpdateColumn("sort_order", 2).Error; err != nil {
		t.Fatalf("move B: %v", err)
	}
	if err := normalizePlanSortOrder(db); err != nil {
		t.Fatalf("normalizePlanSortOrder second run: %v", err)
	}
	var a, b Plan
	if err := db.First(&a, pA.ID).Error; err != nil {
		t.Fatalf("load A: %v", err)
	}
	if err := db.First(&b, pB.ID).Error; err != nil {
		t.Fatalf("load B: %v", err)
	}
	if a.SortOrder >= b.SortOrder {
		t.Fatalf("再次归一不应打乱拖拽结果：A=%d B=%d", a.SortOrder, b.SortOrder)
	}
}

// TestNormalizePermissionGroupSortOrder 权限组新列默认 0，首次归一等价于旧 id 顺序；之后幂等。
func TestNormalizePermissionGroupSortOrder(t *testing.T) {
	db := sortOrderTestDB(t, "file:pg_sort?mode=memory&cache=shared")

	names := []string{"a", "b", "c"}
	ids := make([]uint64, 0, len(names))
	for _, name := range names {
		g := PermissionGroup{Name: name}
		if err := db.Create(&g).Error; err != nil {
			t.Fatalf("create group %s: %v", name, err)
		}
		ids = append(ids, g.ID)
	}

	if err := normalizePermissionGroupSortOrder(db); err != nil {
		t.Fatalf("normalizePermissionGroupSortOrder: %v", err)
	}
	var rows []PermissionGroup
	if err := db.Order("sort_order ASC, id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("list: %v", err)
	}
	for i, row := range rows {
		if row.ID != ids[i] {
			t.Fatalf("首次归一应按 id 顺序：第 %d 位期望 id=%d，实际 id=%d", i, ids[i], row.ID)
		}
		if row.SortOrder != i {
			t.Fatalf("第 %d 位 sort_order 期望 %d，实际 %d", i, i, row.SortOrder)
		}
	}

	// 模拟拖拽：把 a 拖到最后。再次归一必须保留。
	if err := db.Model(&PermissionGroup{}).Where("id = ?", ids[0]).UpdateColumn("sort_order", 2).Error; err != nil {
		t.Fatalf("move a: %v", err)
	}
	if err := db.Model(&PermissionGroup{}).Where("id = ?", ids[1]).UpdateColumn("sort_order", 0).Error; err != nil {
		t.Fatalf("move b: %v", err)
	}
	if err := db.Model(&PermissionGroup{}).Where("id = ?", ids[2]).UpdateColumn("sort_order", 1).Error; err != nil {
		t.Fatalf("move c: %v", err)
	}
	if err := normalizePermissionGroupSortOrder(db); err != nil {
		t.Fatalf("normalizePermissionGroupSortOrder second run: %v", err)
	}
	var a PermissionGroup
	if err := db.First(&a, ids[0]).Error; err != nil {
		t.Fatalf("load a: %v", err)
	}
	if a.SortOrder != 2 {
		t.Fatalf("再次归一不应覆盖拖拽结果，a 的 sort_order 实际 %d", a.SortOrder)
	}
}
