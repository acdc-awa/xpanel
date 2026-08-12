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

func TestAuthorizedInboundSet(t *testing.T) {
	db := testDB(t)

	group := models.PermissionGroup{Name: "g2"}
	db.Create(&group)
	plan := models.Plan{Name: "p3", PriceCents: 100, TrafficGB: 10, DurationDays: 30, PermissionGroupID: group.ID}
	db.Create(&plan)
	db.Create(&models.PermissionGroupInbound{PermissionGroupID: group.ID, InboundID: 21})
	db.Create(&models.PermissionGroupInbound{PermissionGroupID: group.ID, InboundID: 22})

	user := models.User{Username: "u1", Email: "u1@example.com", UUID: "uuid-1", PlanID: plan.ID}
	db.Create(&user)
	// 手动授权一个（叠加场景）
	db.Create(&models.UserInbound{UserID: user.ID, InboundID: 23, Enabled: true})
	// 禁用授权不计入（Enabled=false 是零值，GORM INSERT 会省略 → 先建后改）
	dis := models.UserInbound{UserID: user.ID, InboundID: 24, Enabled: true}
	db.Create(&dis)
	db.Model(&dis).Update("enabled", false)

	set := AuthorizedInboundSet(db, &user)
	for _, id := range []uint64{21, 22, 23} {
		if !set[id] {
			t.Errorf("入站 %d 应已授权", id)
		}
	}
	if set[24] {
		t.Error("禁用授权不应计入")
	}
	if len(set) != 3 {
		t.Errorf("集合大小 = %d, want 3 (21,22,23)", len(set))
	}

	// 无 plan 用户：仅手动授权
	user2 := models.User{Username: "u2", Email: "u2@example.com", UUID: "uuid-2"}
	db.Create(&user2)
	set2 := AuthorizedInboundSet(db, &user2)
	if len(set2) != 0 {
		t.Errorf("无授权用户应为空: %v", set2)
	}
}
