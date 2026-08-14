package services

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/models"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Plan{}, &models.PermissionGroup{}, &models.PermissionGroupInbound{},
		&models.User{}, &models.UserInbound{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func TestGroupInboundIDs(t *testing.T) {
	db := testDB(t)

	group := models.PermissionGroup{Name: "g1"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.Plan{Name: "p1", PriceCents: 100, TrafficGB: 10, DurationDays: 30, PermissionGroupID: group.ID}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	db.Create(&models.PermissionGroupInbound{PermissionGroupID: group.ID, InboundID: 11})
	db.Create(&models.PermissionGroupInbound{PermissionGroupID: group.ID, InboundID: 12})

	// 套餐绑定权限组 → 集合
	ids := GroupInboundIDs(db, plan.ID)
	if len(ids) != 2 {
		t.Fatalf("ids = %v, want [11 12]", ids)
	}
	// planID=0 → 空
	if got := GroupInboundIDs(db, 0); got != nil {
		t.Errorf("planID=0 应为空: %v", got)
	}
	// 未绑定权限组 → 空
	plan2 := models.Plan{Name: "p2", PriceCents: 100, TrafficGB: 1, DurationDays: 1}
	db.Create(&plan2)
	if got := GroupInboundIDs(db, plan2.ID); len(got) != 0 {
		t.Errorf("未绑定权限组应为空: %v", got)
	}
}

func TestAuthorizedInboundSet_UnifiedPermissionGroup(t *testing.T) {
	db := testDB(t)

	group1 := models.PermissionGroup{Name: "VIP 1"}
	group2 := models.PermissionGroup{Name: "VIP 2"}
	db.Create(&group1)
	db.Create(&group2)

	plan1 := models.Plan{Name: "p1", PermissionGroupID: group1.ID}
	db.Create(&plan1)

	// 入站 101 开放给 group1 和 group2
	SyncInboundPermissionGroups(db, 101, []uint64{group1.ID, group2.ID})
	// 入站 102 仅开放给 group2
	SyncInboundPermissionGroups(db, 102, []uint64{group2.ID})

	// 用户 u1 购买 plan1（继承 group1）
	user1 := models.User{Username: "u1", PlanID: plan1.ID}
	db.Create(&user1)

	set1 := AuthorizedInboundSet(db, &user1)
	if !set1[101] {
		t.Errorf("u1 应可访问入站 101")
	}
	if set1[102] {
		t.Errorf("u1 不应访问仅开放给 VIP2 的入站 102")
	}

	// 管理员手动将 user1 权限组覆盖为 group2
	db.Model(&user1).Update("permission_group_id", group2.ID)
	user1.PermissionGroupID = group2.ID

	set2 := AuthorizedInboundSet(db, &user1)
	if !set2[101] || !set2[102] {
		t.Errorf("u1 覆盖为 VIP2 后应可访问 101 与 102, got %v", set2)
	}

	// 批量查询测试
	inboundGroups := BatchInboundPermissionGroupIDs(db, []uint64{101, 102})
	if len(inboundGroups[101]) != 2 || len(inboundGroups[102]) != 1 {
		t.Errorf("BatchInboundPermissionGroupIDs returned unexpected: %v", inboundGroups)
	}
}
