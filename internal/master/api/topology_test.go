package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel-node/pkg/tlscert"
	"github.com/acdc-awa/xpanel/internal/models"
)

func apiTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// 每个测试独立内存库（cache=shared + 唯一名，避免跨测试串库）
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Server{}, &models.Inbound{}, &models.ServerOutbound{},
		&models.Cert{}, &models.Plan{}, &models.PermissionGroup{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func seedRefGraph(t *testing.T, db *gorm.DB) (s1, s2, aIn, bIn uint64) {
	t.Helper()
	s1 = uint64(1)
	s2 = uint64(2)
	db.Create(&models.Server{ID: s1, Name: "s1", Host: "10.0.0.1", NodeID: "n1", Secret: "x"})
	db.Create(&models.Server{ID: s2, Name: "s2", Host: "10.0.0.2", NodeID: "n2", Secret: "x"})
	a := models.Inbound{ServerID: s1, Tag: "in-a", Protocol: "vless", Port: 44301, Type: models.InboundTypeUser, Enabled: true}
	b := models.Inbound{ServerID: s2, Tag: "in-b", Protocol: "vless", Port: 44302, Type: models.InboundTypeRelay, InternalUUID: "uuid-b", Enabled: true}
	db.Create(&a)
	db.Create(&b)
	return s1, s2, a.ID, b.ID
}

func TestWouldCreateRefCycle(t *testing.T) {
	db := apiTestDB(t)
	s1, s2, aIn, bIn := seedRefGraph(t, db)
	d := &Deps{DB: db}

	// X (S1) → B：合法（无环）
	if msg := d.checkInboundRef(s1, bIn, 0); msg != "" {
		t.Fatalf("X(S1)→B 应通过: %s", msg)
	}
	refB := bIn
	db.Create(&models.ServerOutbound{ServerID: s1, Tag: "x-to-b", Protocol: "vless", InboundRef: &refB, Enabled: true})

	// 再建 Y (S2) → A：从 A 出发可达 B（在 S2 上）→ 环
	if msg := d.checkInboundRef(s2, aIn, 0); msg == "" {
		t.Fatal("Y(S2)→A 应判为环（A→B→A）")
	}

	// 同服务器引用：Z (S1) → A（A 在 S1 上）→ 环
	if msg := d.checkInboundRef(s1, aIn, 0); msg == "" {
		t.Fatal("同服务器引用应判为环")
	}

	// 更新 X 自身（exclude X）：X→B 维持，不因自身产生新环
	if msg := d.checkInboundRef(s1, bIn, 1); msg != "" {
		t.Fatalf("更新 X 自身应通过: %s", msg)
	}
}

func TestRelayMarkLifecycle(t *testing.T) {
	db := apiTestDB(t)
	_, _, aIn, bIn := seedRefGraph(t, db)
	d := &Deps{DB: db}

	// 目标原本是 user：设置引用 → 自动标 relay 并记录 PreviousType=user
	db.Model(&models.Inbound{}).Where("id = ?", bIn).Update("type", models.InboundTypeUser)
	d.ensureRelayMark(bIn)
	var b models.Inbound
	db.First(&b, bIn)
	if b.Type != models.InboundTypeRelay {
		t.Errorf("目标应为 relay, got %s", b.Type)
	}
	if b.PreviousType != models.InboundTypeUser {
		t.Errorf("PreviousType 应记录 user, got %q", b.PreviousType)
	}

	// 仍被引用 → demote 不生效
	refB := bIn
	db.Create(&models.ServerOutbound{ServerID: 1, Tag: "x", Protocol: "vless", InboundRef: &refB, Enabled: true})
	d.demoteIfUnreferenced(bIn)
	db.First(&b, bIn)
	if b.Type != models.InboundTypeRelay {
		t.Error("仍被引用时不应降级")
	}

	// 删除引用后 → 回退到引用前类型（user），并清除 PreviousType
	db.Where("tag = ?", "x").Delete(&models.ServerOutbound{})
	d.demoteIfUnreferenced(bIn)
	db.First(&b, bIn)
	if b.Type != models.InboundTypeUser {
		t.Errorf("无引用应回退到 user, got %s", b.Type)
	}
	if b.PreviousType != "" {
		t.Errorf("回退后 PreviousType 应清空, got %q", b.PreviousType)
	}
	_ = aIn
}

// TestDemoteKeepsManualRelay：原本就是手动 relay（无 PreviousType）的落地入站，解绑后保持 relay 不动。
func TestDemoteKeepsManualRelay(t *testing.T) {
	db := apiTestDB(t)
	_, _, _, bIn := seedRefGraph(t, db)
	d := &Deps{DB: db}
	// 种子 bIn 本就是 relay（无 PreviousType）
	refB := bIn
	db.Create(&models.ServerOutbound{ServerID: 1, Tag: "x", Protocol: "vless", InboundRef: &refB, Enabled: true})
	d.ensureRelayMark(bIn)
	db.Where("tag = ?", "x").Delete(&models.ServerOutbound{})
	d.demoteIfUnreferenced(bIn)
	var b models.Inbound
	db.First(&b, bIn)
	if b.Type != models.InboundTypeRelay {
		t.Errorf("手动 relay 解绑后应保持 relay, got %s", b.Type)
	}
}

func TestCertNotAfter(t *testing.T) {
	// 用 agent certs 测试同款自签证书逻辑不便引入；这里校验 tlscert.NotAfter 对坏输入报错
	if _, err := tlscert.NotAfter("not-a-pem"); err == nil {
		t.Error("非法 PEM 应报错")
	}
}

func TestCheckInboundRefTargets(t *testing.T) {
	db := apiTestDB(t)
	s1, _, aIn, _ := seedRefGraph(t, db)
	d := &Deps{DB: db}

	// 目标不存在
	if msg := d.checkInboundRef(s1, 9999, 0); msg == "" {
		t.Error("引用不存在的入站应报错")
	}
	// 目标停用
	var a models.Inbound
	db.First(&a, aIn)
	db.Model(&a).Updates(map[string]any{"type": models.InboundTypeRelay, "enabled": false})
	if msg := d.checkInboundRef(s1, aIn, 0); msg == "" {
		t.Error("引用停用入站应报错")
	}
	_ = time.Now
}

// TestAdminUpdateOutboundUnbindDemotes 回归：PUT outbound {inbound_ref:0} 解绑后，
// 目标落地入站在无其他引用时应回退到引用前类型（user）。
// 曾因 GORM Updates 回写 struct 字段（解绑后 ob.InboundRef 被置 nil），
// 导致 demote 分支的 oldRef 判断失败、relay 永不降级。
func TestAdminUpdateOutboundUnbindDemotes(t *testing.T) {
	db := apiTestDB(t)
	s1, _, aIn, bIn := seedRefGraph(t, db)
	_ = aIn
	// 目标 B 原为 user（被引用后自动标 relay 并记录 PreviousType=user）
	db.Model(&models.Inbound{}).Where("id = ?", bIn).Update("type", models.InboundTypeUser)
	refB := bIn
	ob := models.ServerOutbound{ServerID: s1, Tag: "x", Protocol: "freedom", SettingsJSON: "{}", InboundRef: &refB, Enabled: true}
	db.Create(&ob)

	d := &Deps{DB: db}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "1"}, {Key: "outbound_id", Value: "1"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/servers/1/outbounds/1", strings.NewReader(`{"inbound_ref":0}`))
	c.Request.Header.Set("Content-Type", "application/json")
	d.AdminUpdateServerOutbound(c)

	if w.Code != http.StatusOK {
		t.Fatalf("解绑请求应 200, got %d body=%s", w.Code, w.Body.String())
	}
	var b models.Inbound
	db.First(&b, bIn)
	if b.Type != models.InboundTypeUser {
		t.Errorf("解绑后目标应回退到 user, got %s", b.Type)
	}
}

// TestTopologyLayoutAPI：画布布局云端同步 GET/PUT 往返（settings 表 upsert）
func TestTopologyLayoutAPI(t *testing.T) {
	db := apiTestDB(t)
	db.AutoMigrate(&models.Setting{})
	d := &Deps{DB: db}
	gin.SetMode(gin.TestMode)

	// 初始为空
	{
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Request = httptest.NewRequest(http.MethodGet, "/admin/topology-layout", nil)
		d.AdminGetTopologyLayout(c)
		if r.Code != 200 || !strings.Contains(r.Body.String(), `"positions":{}`) {
			t.Fatalf("empty layout unexpected: %d %s", r.Code, r.Body.String())
		}
	}

	// PUT 保存
	{
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Request = httptest.NewRequest(http.MethodPut, "/admin/topology-layout", strings.NewReader(
			`{"hash":"h1","positions":{"server-4":{"x":123,"y":456}},"widths":{"server-4":600}}`))
		c.Request.Header.Set("Content-Type", "application/json")
		d.AdminSaveTopologyLayout(c)
		if r.Code != 200 {
			t.Fatalf("save failed: %d %s", r.Code, r.Body.String())
		}
	}

	// GET 回读
	{
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Request = httptest.NewRequest(http.MethodGet, "/admin/topology-layout", nil)
		d.AdminGetTopologyLayout(c)
		body := r.Body.String()
		if !strings.Contains(body, `"hash":"h1"`) || !strings.Contains(body, `"x":123`) || !strings.Contains(body, `"y":456`) || !strings.Contains(body, `"widths":{"server-4":600}`) {
			t.Fatalf("roundtrip mismatch: %s", body)
		}
	}

	// PUT 覆盖（第二次保存走 update 分支）
	{
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Request = httptest.NewRequest(http.MethodPut, "/admin/topology-layout", strings.NewReader(
			`{"hash":"h2","positions":{"server-4":{"x":9,"y":9}},"widths":{}}`))
		c.Request.Header.Set("Content-Type", "application/json")
		d.AdminSaveTopologyLayout(c)
		if r.Code != 200 {
			t.Fatalf("resave failed: %d %s", r.Code, r.Body.String())
		}
		r2 := httptest.NewRecorder()
		c2, _ := gin.CreateTestContext(r2)
		c2.Request = httptest.NewRequest(http.MethodGet, "/admin/topology-layout", nil)
		d.AdminGetTopologyLayout(c2)
		if !strings.Contains(r2.Body.String(), `"hash":"h2"`) || !strings.Contains(r2.Body.String(), `"x":9`) || strings.Contains(r2.Body.String(), `"y":456`) {
			t.Fatalf("overwrite mismatch: %s", r2.Body.String())
		}
	}
}

// TestTopologyLayoutTagOrders：tag_orders 往返 + 旧格式载荷（缺字段）保留旧值 +
// 显式空对象清空 + 软上限 400
func TestTopologyLayoutTagOrders(t *testing.T) {
	db := apiTestDB(t)
	db.AutoMigrate(&models.Setting{})
	d := &Deps{DB: db}
	gin.SetMode(gin.TestMode)

	call := func(method, body string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Request = httptest.NewRequest(method, "/admin/topology-layout", strings.NewReader(body))
		if body != "" {
			c.Request.Header.Set("Content-Type", "application/json")
		}
		if method == http.MethodPut {
			d.AdminSaveTopologyLayout(c)
		} else {
			d.AdminGetTopologyLayout(c)
		}
		return r
	}

	// 带 tag_orders 保存 → GET 往返
	if r := call(http.MethodPut, `{"hash":"h1","positions":{},"widths":{},"tag_orders":{"server-4":{"inbounds":[3,1,2],"outbounds":["7","5"]}}}`); r.Code != 200 {
		t.Fatalf("save failed: %d %s", r.Code, r.Body.String())
	}
	if body := call(http.MethodGet, "").Body.String(); !strings.Contains(body, `"tag_orders":{"server-4":{"inbounds":[3,1,2],"outbounds":["7","5"]}}`) {
		t.Fatalf("tag_orders roundtrip mismatch: %s", body)
	}

	// 旧格式载荷（缺 tag_orders 字段）→ 保留云端旧值，hash 正常更新
	if r := call(http.MethodPut, `{"hash":"h2","positions":{},"widths":{}}`); r.Code != 200 {
		t.Fatalf("legacy save failed: %d %s", r.Code, r.Body.String())
	}
	if body := call(http.MethodGet, "").Body.String(); !strings.Contains(body, `"hash":"h2"`) || !strings.Contains(body, `"inbounds":[3,1,2]`) {
		t.Fatalf("legacy PUT should preserve tag_orders: %s", body)
	}

	// 显式 {} → 清空
	if r := call(http.MethodPut, `{"hash":"h3","positions":{},"widths":{},"tag_orders":{}}`); r.Code != 200 {
		t.Fatalf("clear failed: %d %s", r.Code, r.Body.String())
	}
	if body := call(http.MethodGet, "").Body.String(); strings.Contains(body, `"inbounds"`) {
		t.Fatalf("explicit empty should clear tag_orders: %s", body)
	}

	// 软上限：单盒入站数组超限 → 400
	big := `{"hash":"h4","positions":{},"widths":{},"tag_orders":{"server-4":{"inbounds":[` + strings.Repeat("1,", topoTagOrderMaxPerBox) + `1],"outbounds":[]}}}`
	if r := call(http.MethodPut, big); r.Code != http.StatusBadRequest {
		t.Fatalf("over-limit should 400, got %d %s", r.Code, r.Body.String())
	}
}
