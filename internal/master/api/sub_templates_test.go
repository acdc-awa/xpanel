package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
)

// TestSubTemplateCRUD：模板库最小链路 —— 空白名 400、创建（TrimSpace）→ 列表 →
// 更新 → 删除后列表为空。鉴权由 admin 路由组中间件保证，此处直测 handler。
func TestSubTemplateCRUD(t *testing.T) {
	db := apiTestDB(t)
	db.AutoMigrate(&models.SubTemplate{})
	d := &Deps{DB: db}
	gin.SetMode(gin.TestMode)

	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(r)
		c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			c.Request.Header.Set("Content-Type", "application/json")
		}
		switch method {
		case http.MethodPost:
			d.AdminCreateSubTemplate(c)
		case http.MethodGet:
			d.AdminListSubTemplates(c)
		case http.MethodPut:
			c.Params = gin.Params{{Key: "id", Value: path[strings.LastIndex(path, "/")+1:]}}
			d.AdminUpdateSubTemplate(c)
		case http.MethodDelete:
			c.Params = gin.Params{{Key: "id", Value: path[strings.LastIndex(path, "/")+1:]}}
			d.AdminDeleteSubTemplate(c)
		}
		return r
	}

	// 空白名（全空格）→ 400
	if r := call(http.MethodPost, "/admin/sub-templates", `{"name":"   ","content":"PROXIES"}`); r.Code != http.StatusBadRequest {
		t.Fatalf("blank name should 400, got %d %s", r.Code, r.Body.String())
	}

	// 创建（名带空白）→ TrimSpace 落库
	r := call(http.MethodPost, "/admin/sub-templates", `{"name":" 基础模板 ","content":"$PROXIES$"}`)
	if r.Code != http.StatusOK {
		t.Fatalf("create failed: %d %s", r.Code, r.Body.String())
	}
	if !strings.Contains(r.Body.String(), `"name":"基础模板"`) {
		t.Fatalf("name should be trimmed: %s", r.Body.String())
	}

	// 列表含之
	if body := call(http.MethodGet, "/admin/sub-templates", "").Body.String(); !strings.Contains(body, "基础模板") {
		t.Fatalf("list should contain template: %s", body)
	}

	// 更新
	if r := call(http.MethodPut, "/admin/sub-templates/1", `{"name":"改名模板","content":"$ALL_PROXIES$"}`); r.Code != http.StatusOK {
		t.Fatalf("update failed: %d %s", r.Code, r.Body.String())
	}
	var tpl models.SubTemplate
	if err := db.First(&tpl, 1).Error; err != nil || tpl.Name != "改名模板" {
		t.Fatalf("update not persisted: %+v err=%v", tpl, err)
	}

	// 删除 → 列表为空
	if r := call(http.MethodDelete, "/admin/sub-templates/1", ""); r.Code != http.StatusOK {
		t.Fatalf("delete failed: %d %s", r.Code, r.Body.String())
	}
	var count int64
	db.Model(&models.SubTemplate{}).Count(&count)
	if count != 0 {
		t.Fatalf("template should be deleted, count=%d", count)
	}
}
