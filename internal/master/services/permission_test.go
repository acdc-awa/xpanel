package services

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func u64(v uint64) *uint64 { return &v }

// AP 单点授权派生：用户可见入站集合 = 生效组命中的启用接入点解析结果（直连 / 经 L4 转发）。
func TestBatchInboundAuthorizedGroupIDs_APDerived(t *testing.T) {
	db := testDB(t)

	group1 := models.PermissionGroup{Name: "VIP 1"}
	group2 := models.PermissionGroup{Name: "VIP 2"}
	db.Create(&group1)
	db.Create(&group2)

	plan1 := models.Plan{Name: "p1", PermissionGroupID: group1.ID}
	db.Create(&plan1)

	// 接入点 1：直连入站 101，开放给 group1+group2
	ap1 := models.UserAccessPoint{Name: "ap1", Enabled: true, TargetType: "inbound", TargetInboundID: u64(101)}
	db.Create(&ap1)
	// 接入点 2：直连入站 102，仅开放给 group2
	ap2 := models.UserAccessPoint{Name: "ap2", Enabled: true, TargetType: "inbound", TargetInboundID: u64(102)}
	db.Create(&ap2)
	// 接入点 3：直连入站 103 + 端点覆写（L4 退役后的中转表达），开放给 group1
	ap3 := models.UserAccessPoint{Name: "ap3", Enabled: true, TargetType: "inbound", TargetInboundID: u64(103), CustomHost: "gz.relay.com", CustomPort: 30001}
	db.Create(&ap3)
	// 接入点 4：禁用状态，即使绑定 group1 也不生效（指向入站 104）
	// （GORM default:true 陷阱：零值 false 入库会被默认值覆盖，需创建后显式禁用）
	ap4 := models.UserAccessPoint{Name: "ap4", Enabled: true, TargetType: "inbound", TargetInboundID: u64(104)}
	db.Create(&ap4)
	db.Model(&ap4).Update("enabled", false)
	// 接入点 5：启用但未绑定权限组 = 全员不可见（零信任，指向入站 105）
	ap5 := models.UserAccessPoint{Name: "ap5", Enabled: true, TargetType: "inbound", TargetInboundID: u64(105)}
	db.Create(&ap5)

	if err := SyncAccessPointPermissionGroups(db, ap1.ID, []uint64{group1.ID, group2.ID}); err != nil {
		t.Fatal(err)
	}
	if err := SyncAccessPointPermissionGroups(db, ap2.ID, []uint64{group2.ID}); err != nil {
		t.Fatal(err)
	}
	if err := SyncAccessPointPermissionGroups(db, ap3.ID, []uint64{group1.ID}); err != nil {
		t.Fatal(err)
	}
	if err := SyncAccessPointPermissionGroups(db, ap4.ID, []uint64{group1.ID}); err != nil {
		t.Fatal(err)
	}

	// 入站授权组映射（配置注入同源）
	m := BatchInboundAuthorizedGroupIDs(db, []uint64{101, 102, 103, 104, 105})
	if len(m[101]) != 2 {
		t.Errorf("入站 101 授权组应为 [g1 g2], got %v", m[101])
	}
	if len(m[102]) != 1 || m[102][0] != group2.ID {
		t.Errorf("入站 102 授权组应为 [g2], got %v", m[102])
	}
	if len(m[103]) != 1 || m[103][0] != group1.ID {
		t.Errorf("入站 103 应经接入点派生授权组 [g1], got %v", m[103])
	}
	if len(m[104]) != 0 || len(m[105]) != 0 {
		t.Errorf("禁用/未授权接入点不应产生授权组: 104=%v 105=%v", m[104], m[105])
	}
}

// 入口服务器集合：入口 = 目标入站所在服务器（端点覆写不改变入口归属）。
func TestAuthorizedEntryServerIDs_APDerived(t *testing.T) {
	db := testDB(t)

	group := models.PermissionGroup{Name: "g1"}
	db.Create(&group)
	user := models.User{Username: "u1", PermissionGroupID: group.ID}
	db.Create(&user)

	// 目标入站：服务器 2
	db.Create(&models.Inbound{ID: 201, ServerID: 2, Tag: "in-201", Type: models.InboundTypeUser, Enabled: true})

	apDirect := models.UserAccessPoint{Name: "d", Enabled: true, TargetType: "inbound", TargetInboundID: u64(201)}
	db.Create(&apDirect)
	// 覆写端点（原 L4 中转表达）同样归属目标入站所在服务器
	apOverride := models.UserAccessPoint{Name: "o", Enabled: true, TargetType: "inbound", TargetInboundID: u64(201), CustomHost: "gz.relay.com", CustomPort: 30001}
	db.Create(&apOverride)
	_ = SyncAccessPointPermissionGroups(db, apDirect.ID, []uint64{group.ID})
	_ = SyncAccessPointPermissionGroups(db, apOverride.ID, []uint64{group.ID})

	set := AuthorizedEntryServerIDs(db, &user)
	if !set[2] {
		t.Errorf("接入点应暴露目标入站所在服务器 2，got %v", set)
	}
	if len(set) != 1 {
		t.Errorf("多余入口服务器（不应包含中转机），got %v", set)
	}
}

// PreviewUsers（与 GetValidUsers 同规则）：入站注入用户由启用接入点白名单派生。
func TestProtoUsersFor_APDerived(t *testing.T) {
	db := testDB(t)
	_ = db.AutoMigrate(&models.Inbound{}, &models.Order{}, &models.TrafficDaily{})

	cfgSvc := &ConfigService{DB: db}

	group := models.PermissionGroup{Name: "VIP 1"}
	db.Create(&group)

	user := models.User{
		Username:          "active_user",
		UUID:              "11111111-2222-3333-4444-555555555555",
		Email:             "active_user@test.local",
		Status:            models.StatusActive,
		PermissionGroupID: group.ID,
	}
	db.Create(&user)

	inb := models.Inbound{
		ID:       201,
		ServerID: 1,
		Tag:      "vless-test",
		Type:     models.InboundTypeUser,
		Enabled:  true,
	}
	db.Create(&inb)

	// 1. 无任何接入点指向该入站时，不对任何用户开放
	if got := cfgSvc.PreviewUsers(&inb); len(got) != 0 {
		t.Fatalf("无接入点引用的入站应返回 0 个用户（不对任何人开放），got %d", len(got))
	}

	// 2. 建立指向该入站且绑定 group 的启用接入点后，命中用户注入
	ap := models.UserAccessPoint{Name: "ap", Enabled: true, TargetType: "inbound", TargetInboundID: u64(inb.ID)}
	db.Create(&ap)
	if err := SyncAccessPointPermissionGroups(db, ap.ID, []uint64{group.ID}); err != nil {
		t.Fatal(err)
	}
	usersAllowed := cfgSvc.PreviewUsers(&inb)
	if len(usersAllowed) != 1 || usersAllowed[0].UUID != user.UUID {
		t.Fatalf("接入点授权组命中后应返回用户，got %+v", usersAllowed)
	}

	// 3. 接入点禁用后不再注入
	db.Model(&ap).Update("enabled", false)
	if got := cfgSvc.PreviewUsers(&inb); len(got) != 0 {
		t.Fatalf("接入点禁用后应返回 0 个用户，got %d", len(got))
	}
}

// 权限组内接入点优先级（2026-09-03）：
// ① 组侧 SetGroupAccessPointOrder 全量重排（sort_order=下标）；
// ② 接入点侧同步保留既有排序、新绑定追加到组内末尾。
func TestGroupAccessPointOrderAndSyncPreserve(t *testing.T) {
	db := testDB(t)
	g := models.PermissionGroup{Name: "g"}
	if err := db.Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	group2 := models.PermissionGroup{Name: "g2"}
	if err := db.Create(&group2).Error; err != nil {
		t.Fatal(err)
	}

	// ① 组视角：有序列表 [7,3,9] → sort_order = 下标
	if err := SetGroupAccessPointOrder(db, g.ID, []uint64{7, 3, 9}); err != nil {
		t.Fatalf("SetGroupAccessPointOrder: %v", err)
	}
	var links []models.PermissionGroupAccessPoint
	if err := db.Where("permission_group_id = ?", g.ID).Order("sort_order ASC, access_point_id ASC").Find(&links).Error; err != nil {
		t.Fatal(err)
	}
	want := []uint64{7, 3, 9}
	if len(links) != 3 {
		t.Fatalf("期望 3 条绑定，实际 %d", len(links))
	}
	for i, l := range links {
		if l.AccessPointID != want[i] || l.SortOrder != i {
			t.Fatalf("第 %d 项期望 ap=%d so=%d，实际 ap=%d so=%d", i, want[i], i, l.AccessPointID, l.SortOrder)
		}
	}

	// ② 接入点侧新增绑定：加入已排序组 → 追加到末尾（so=3）；加入空组 → so=0
	apID := uint64(11)
	if err := SyncAccessPointPermissionGroups(db, apID, []uint64{g.ID, group2.ID}); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	// 注意：Model(空结构) 避免 GORM 按非零模型主键注入复合主键条件
	var l models.PermissionGroupAccessPoint
	if err := db.Model(&models.PermissionGroupAccessPoint{}).Where("permission_group_id = ? AND access_point_id = ?", g.ID, apID).First(&l).Error; err != nil {
		t.Fatal(err)
	}
	if l.SortOrder != 3 {
		t.Fatalf("新绑定应追加到组内末尾 so=3，实际 %d", l.SortOrder)
	}
	l = models.PermissionGroupAccessPoint{} // 清空，防 GORM First 按目标主键注入条件
	if err := db.Model(&models.PermissionGroupAccessPoint{}).Where("permission_group_id = ? AND access_point_id = ?", group2.ID, apID).First(&l).Error; err != nil {
		t.Fatal(err)
	}
	if l.SortOrder != 0 {
		t.Fatalf("空组新绑定应 so=0，实际 %d", l.SortOrder)
	}
	l = models.PermissionGroupAccessPoint{} // 清空，防 GORM First 按目标主键注入条件

	// ③ 接入点侧移除后重挂：组内既有排序（7,3,9）不被删除/改动
	l = models.PermissionGroupAccessPoint{}
	apOld := uint64(3)
	if err := SyncAccessPointPermissionGroups(db, 3, []uint64{g.ID, group2.ID}); err != nil {
		t.Fatalf("Sync old ap: %v", err)
	}
	if err := db.Model(&models.PermissionGroupAccessPoint{}).Where("permission_group_id = ? AND access_point_id = ?", g.ID, apOld).First(&l).Error; err != nil {
		t.Fatal(err)
	}
	if l.SortOrder != 1 {
		t.Fatalf("既有绑定重申应保留原排序 so=1，实际 %d", l.SortOrder)
	}
}
