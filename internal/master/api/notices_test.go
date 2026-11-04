package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/config"
	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

func setupNoticesTest(t *testing.T) (*gin.Engine, *gorm.DB, *Deps, string, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite memory db: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}

	cfg := &config.Config{
		JWT: config.JWT{
			Secret:     "notices-test-secret-key-32bytes-123456",
			AccessTTL:  2 * time.Hour,
			RefreshTTL: 7 * 24 * time.Hour,
		},
	}
	jwt := services.NewJWTManager(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	auditSvc := &services.AuditService{DB: db}
	deps := &Deps{
		DB:    db,
		Cfg:   cfg,
		JWT:   jwt,
		Audit: auditSvc,
	}

	u1, _ := util.NewUUID()
	u2, _ := util.NewUUID()
	sub1, _ := util.RandomHex(32)
	sub2, _ := util.RandomHex(32)
	adminUser := models.User{UUID: u1, Email: "admin@test.com", SubscribeToken: sub1, Role: models.RoleAdmin, Status: models.StatusActive}
	normalUser := models.User{UUID: u2, Email: "user@test.com", SubscribeToken: sub2, Role: models.RoleUser, Status: models.StatusActive}
	db.Create(&adminUser)
	db.Create(&normalUser)

	adminToken, _ := jwt.GenerateVerified(adminUser.ID, adminUser.Role, adminUser.TokenVersion)
	userToken, _ := jwt.GenerateVerified(normalUser.ID, normalUser.Role, normalUser.TokenVersion)

	r := gin.New()
	adminGroup := r.Group("/api/v1/admin", middleware.AuthRequired(jwt, db), middleware.RequireRole("admin"))
	{
		adminGroup.GET("/notices", deps.AdminListNotices)
		adminGroup.POST("/notices", deps.AdminCreateNotice)
		adminGroup.PUT("/notices/order", deps.AdminReorderNotices)
		adminGroup.PUT("/notices/:id", deps.AdminUpdateNotice)
		adminGroup.DELETE("/notices/:id", deps.AdminDeleteNotice)
		adminGroup.POST("/notices/:id/toggle", deps.AdminToggleNotice)
	}
	userGroup := r.Group("/api/v1/user", middleware.AuthRequired(jwt, db))
	{
		userGroup.GET("/notices", deps.UserListNotices)
	}

	return r, db, deps, adminToken, userToken
}

func TestNotice_AdminCRUD_And_UserQuery(t *testing.T) {
	r, db, _, adminToken, userToken := setupNoticesTest(t)

	// 1. 管理端创建两条公告
	body1, _ := json.Marshal(map[string]any{
		"title":     "欢迎使用 Xray 面板",
		"content":   "这是普通公告内容",
		"is_pinned": false,
		"is_popup":  false,
		"status":    1,
	})
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/notices", bytes.NewReader(body1))
	req1.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("create notice 1 failed: code=%d body=%s", w1.Code, w1.Body.String())
	}

	var resp1 struct {
		Code int           `json:"code"`
		Data models.Notice `json:"data"`
	}
	json.Unmarshal(w1.Body.Bytes(), &resp1)
	n1ID := resp1.Data.ID

	// 创建第 2 条：置顶 + 弹窗
	body2, _ := json.Marshal(map[string]any{
		"title":     "重要维护通知",
		"content":   "今晚 12 点进行节点升级维护",
		"is_pinned": true,
		"is_popup":  true,
		"status":    1,
	})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/admin/notices", bytes.NewReader(body2))
	req2.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("create notice 2 failed: code=%d body=%s", w2.Code, w2.Body.String())
	}

	// 2. 用户端查询公告列表（置顶排在最前）
	reqU := httptest.NewRequest(http.MethodGet, "/api/v1/user/notices", nil)
	reqU.AddCookie(&http.Cookie{Name: "access_token", Value: userToken})
	wU := httptest.NewRecorder()
	r.ServeHTTP(wU, reqU)
	if wU.Code != http.StatusOK {
		t.Fatalf("user list notices failed: code=%d body=%s", wU.Code, wU.Body.String())
	}
	var uListResp struct {
		Code int             `json:"code"`
		Data []models.Notice `json:"data"`
	}
	json.Unmarshal(wU.Body.Bytes(), &uListResp)
	if len(uListResp.Data) != 2 {
		t.Fatalf("want 2 notices, got %d", len(uListResp.Data))
	}
	// 置顶必须排在第一位
	if uListResp.Data[0].Title != "重要维护通知" || !uListResp.Data[0].IsPinned || !uListResp.Data[0].IsPopup {
		t.Errorf("first notice should be pinned: %+v", uListResp.Data[0])
	}

	// 3. 管理端切换公告 1 为隐藏 (toggle status)
	toggleBody, _ := json.Marshal(map[string]any{
		"field": "status",
	})
	reqT := httptest.NewRequest(http.MethodPost, "/api/v1/admin/notices/"+strconv.FormatUint(n1ID, 10)+"/toggle", bytes.NewReader(toggleBody))
	reqT.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	reqT.Header.Set("Content-Type", "application/json")
	wT := httptest.NewRecorder()
	r.ServeHTTP(wT, reqT)
	if wT.Code != http.StatusOK {
		t.Fatalf("toggle status failed: code=%d body=%s", wT.Code, wT.Body.String())
	}

	// 用户端再次查询，只剩一条启用的公告
	wU2 := httptest.NewRecorder()
	reqU2 := httptest.NewRequest(http.MethodGet, "/api/v1/user/notices", nil)
	reqU2.AddCookie(&http.Cookie{Name: "access_token", Value: userToken})
	r.ServeHTTP(wU2, reqU2)
	var uListResp2 struct {
		Data []models.Notice `json:"data"`
	}
	json.Unmarshal(wU2.Body.Bytes(), &uListResp2)
	if len(uListResp2.Data) != 1 {
		t.Fatalf("want 1 active notice, got %d", len(uListResp2.Data))
	}

	// 4. 管理端删除公告 1
	reqD := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/notices/"+strconv.FormatUint(n1ID, 10), nil)
	reqD.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	wD := httptest.NewRecorder()
	r.ServeHTTP(wD, reqD)
	if wD.Code != http.StatusOK {
		t.Fatalf("delete notice failed: code=%d", wD.Code)
	}

	var count int64
	db.Model(&models.Notice{}).Count(&count)
	if count != 1 {
		t.Fatalf("want 1 notice remaining in DB, got %d", count)
	}
}

// TestNotice_ReorderAndAppendOrder 覆盖拖拽排序：新建追加末尾、全量重排生效、过期列表被拒。
func TestNotice_ReorderAndAppendOrder(t *testing.T) {
	r, _, _, adminToken, _ := setupNoticesTest(t)

	post := func(title string) models.Notice {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"title": title, "content": "x", "status": 1})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/notices", bytes.NewReader(body))
		req.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("create %s failed: code=%d body=%s", title, w.Code, w.Body.String())
		}
		var resp struct {
			Data models.Notice `json:"data"`
		}
		json.Unmarshal(w.Body.Bytes(), &resp)
		return resp.Data
	}

	a := post("A")
	b := post("B")
	c := post("C")
	if a.SortOrder != 0 || b.SortOrder != 1 || c.SortOrder != 2 {
		t.Fatalf("新建公告应追加到末尾，实际 sort_order = %d/%d/%d", a.SortOrder, b.SortOrder, c.SortOrder)
	}

	// C 置顶后列表变成 [C, A, B]；拖拽把 B 移到 A 前面，提交全量顺序。
	reqPin := httptest.NewRequest(http.MethodPost, "/api/v1/admin/notices/"+itoa(c.ID)+"/toggle",
		bytes.NewReader([]byte(`{"field":"is_pinned"}`)))
	reqPin.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	reqPin.Header.Set("Content-Type", "application/json")
	wPin := httptest.NewRecorder()
	r.ServeHTTP(wPin, reqPin)
	if wPin.Code != http.StatusOK {
		t.Fatalf("pin notice failed: code=%d body=%s", wPin.Code, wPin.Body.String())
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/notices/order",
		bytes.NewReader([]byte(`{"ids":[`+itoa(c.ID)+`,`+itoa(b.ID)+`,`+itoa(a.ID)+`]}`)))
	req.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reorder failed: code=%d body=%s", w.Code, w.Body.String())
	}

	// 用真实的列表接口（带鉴权）核对返回顺序
	reqL := httptest.NewRequest(http.MethodGet, "/api/v1/admin/notices", nil)
	reqL.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	wL := httptest.NewRecorder()
	r.ServeHTTP(wL, reqL)
	var listResp struct {
		Data []models.Notice `json:"data"`
	}
	json.Unmarshal(wL.Body.Bytes(), &listResp)
	list := listResp.Data
	if len(list) != 3 {
		t.Fatalf("want 3 notices, got %d", len(list))
	}
	want := []string{"C", "B", "A"}
	for i, title := range want {
		if list[i].Title != title {
			t.Fatalf("第 %d 位期望 %s，实际 %s（顺序 %v）", i, title, list[i].Title, titles(list))
		}
	}

	// 列表过期（少一个 id）必须被拒，避免部分重排打乱顺序
	reqBad := httptest.NewRequest(http.MethodPut, "/api/v1/admin/notices/order",
		bytes.NewReader([]byte(`{"ids":[`+itoa(c.ID)+`]}`)))
	reqBad.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	reqBad.Header.Set("Content-Type", "application/json")
	wBad := httptest.NewRecorder()
	r.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("过期排序列表应返回 400，实际 %d body=%s", wBad.Code, wBad.Body.String())
	}
}

// TestNotice_CreateDraftKeepsStatus 覆盖 GORM default:1 的零值陷阱：
// 草稿（status=0）在创建时不能被数据库默认值改成已发布，且不能出现在用户端。
func TestNotice_CreateDraftKeepsStatus(t *testing.T) {
	r, _, _, adminToken, userToken := setupNoticesTest(t)

	body, _ := json.Marshal(map[string]any{"title": "草稿公告", "content": "x", "status": 0})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/notices", bytes.NewReader(body))
	req.AddCookie(&http.Cookie{Name: "access_token", Value: adminToken})
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create draft failed: code=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data models.Notice `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.Status != 0 {
		t.Fatalf("草稿应保持 status=0，实际 %d", resp.Data.Status)
	}

	// 用户端看不到草稿
	reqU := httptest.NewRequest(http.MethodGet, "/api/v1/user/notices", nil)
	reqU.AddCookie(&http.Cookie{Name: "access_token", Value: userToken})
	wU := httptest.NewRecorder()
	r.ServeHTTP(wU, reqU)
	var listResp struct {
		Data []models.Notice `json:"data"`
	}
	json.Unmarshal(wU.Body.Bytes(), &listResp)
	if len(listResp.Data) != 0 {
		t.Fatalf("草稿不应出现在用户端，实际 %d 条", len(listResp.Data))
	}
}

func itoa(v uint64) string { return strconv.FormatUint(v, 10) }

func titles(list []models.Notice) []string {
	out := make([]string, 0, len(list))
	for _, n := range list {
		out = append(out, n.Title)
	}
	return out
}
