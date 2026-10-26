package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
)

// callAdmin 直测 handler：body 为空时不带 Content-Type；params 用于 :id 等路径参数。
func callAdmin(t *testing.T, d *Deps, method, path, body string, params map[string]string, fn func(*gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	for k, v := range params {
		c.Params = append(c.Params, gin.Param{Key: k, Value: v})
	}
	fn(c)
	return r
}

// TestPermissionGroupTemplateBinding 绑定/解绑/保存自动解绑/删除模板四个写路径。
func TestPermissionGroupTemplateBinding(t *testing.T) {
	db := apiTestDB(t)
	d := &Deps{DB: db}

	tpl := models.SubTemplate{Name: "模板A", Content: "proxies:\n$PROXIES$\n"}
	if err := db.Create(&tpl).Error; err != nil {
		t.Fatal(err)
	}
	g := models.PermissionGroup{Name: "组A", ClashTemplate: "old-copy"}
	if err := db.Create(&g).Error; err != nil {
		t.Fatal(err)
	}
	reload := func() models.PermissionGroup {
		var got models.PermissionGroup
		if err := db.First(&got, g.ID).Error; err != nil {
			t.Fatal(err)
		}
		return got
	}

	// 绑定：写入 ID + 模板内容快照。
	r := callAdmin(t, d, http.MethodPut, "/admin/permission-groups/1/sub-template",
		`{"sub_template_id":1}`, map[string]string{"id": "1"}, d.AdminBindPermissionGroupSubTemplate)
	if r.Code != http.StatusOK {
		t.Fatalf("bind failed: %d %s", r.Code, r.Body.String())
	}
	got := reload()
	if got.SubTemplateID == nil || *got.SubTemplateID != tpl.ID {
		t.Fatalf("绑定未落库: %+v", got)
	}
	if got.ClashTemplate != tpl.Content {
		t.Fatalf("绑定时应写入模板快照，实际 %q", got.ClashTemplate)
	}

	// 组编辑器保存（带 clash_template）：自动解绑，转为自定义。
	r = callAdmin(t, d, http.MethodPut, "/admin/permission-groups/1",
		`{"clash_template":"edited-copy"}`, map[string]string{"id": "1"}, d.AdminUpdatePermissionGroup)
	if r.Code != http.StatusOK {
		t.Fatalf("update failed: %d %s", r.Code, r.Body.String())
	}
	got = reload()
	if got.SubTemplateID != nil {
		t.Fatalf("保存组模板后应自动解绑，实际 %v", *got.SubTemplateID)
	}
	if got.ClashTemplate != "edited-copy" {
		t.Fatalf("组模板未保存: %q", got.ClashTemplate)
	}

	// 只改名称/备注不应解绑。
	if r = callAdmin(t, d, http.MethodPut, "/admin/permission-groups/1/sub-template",
		`{"sub_template_id":1}`, map[string]string{"id": "1"}, d.AdminBindPermissionGroupSubTemplate); r.Code != http.StatusOK {
		t.Fatalf("re-bind failed: %d %s", r.Code, r.Body.String())
	}
	if r = callAdmin(t, d, http.MethodPut, "/admin/permission-groups/1",
		`{"remark":"只改备注"}`, map[string]string{"id": "1"}, d.AdminUpdatePermissionGroup); r.Code != http.StatusOK {
		t.Fatalf("update remark failed: %d %s", r.Code, r.Body.String())
	}
	if got = reload(); got.SubTemplateID == nil || *got.SubTemplateID != tpl.ID {
		t.Fatalf("只改备注不应解绑: %+v", got)
	}

	// 显式转自定义（空键）：先把当前模板内容落快照，再清空绑定。
	if r = callAdmin(t, d, http.MethodDelete, "/admin/permission-groups/1/sub-template",
		"", map[string]string{"id": "1"}, d.AdminClearPermissionGroupSubTemplate); r.Code != http.StatusOK {
		t.Fatalf("clear failed: %d %s", r.Code, r.Body.String())
	}
	got = reload()
	if got.SubTemplateID != nil {
		t.Fatalf("解绑后 sub_template_id 应为 NULL")
	}
	if got.ClashTemplate != tpl.Content {
		t.Fatalf("解绑应保留当前模板内容，实际 %q", got.ClashTemplate)
	}

	// 删除被绑定模板：允许，组转为自定义并保留模板最新内容。
	if r = callAdmin(t, d, http.MethodPut, "/admin/permission-groups/1/sub-template",
		`{"sub_template_id":1}`, map[string]string{"id": "1"}, d.AdminBindPermissionGroupSubTemplate); r.Code != http.StatusOK {
		t.Fatalf("re-bind failed: %d %s", r.Code, r.Body.String())
	}
	tpl.Content = "latest-content"
	if err := db.Save(&tpl).Error; err != nil {
		t.Fatal(err)
	}
	r = callAdmin(t, d, http.MethodDelete, "/admin/sub-templates/1",
		"", map[string]string{"id": "1"}, d.AdminDeleteSubTemplate)
	if r.Code != http.StatusOK {
		t.Fatalf("delete template failed: %d %s", r.Code, r.Body.String())
	}
	var resp struct {
		Data struct {
			Deleted            bool  `json:"deleted"`
			DetachedGroupCount int64 `json:"detached_group_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v %s", err, r.Body.String())
	}
	if !resp.Data.Deleted || resp.Data.DetachedGroupCount != 1 {
		t.Fatalf("删除模板应解绑 1 个组，实际 %+v", resp.Data)
	}
	got = reload()
	if got.SubTemplateID != nil {
		t.Fatalf("模板删除后组应转为自定义")
	}
	if got.ClashTemplate != "latest-content" {
		t.Fatalf("模板删除后应保留最新内容，实际 %q", got.ClashTemplate)
	}

	// 绑定不存在的模板：404。
	r = callAdmin(t, d, http.MethodPut, "/admin/permission-groups/1/sub-template",
		`{"sub_template_id":999}`, map[string]string{"id": "1"}, d.AdminBindPermissionGroupSubTemplate)
	if r.Code != http.StatusNotFound {
		t.Fatalf("绑定不存在的模板应 404，实际 %d %s", r.Code, r.Body.String())
	}

	// 绑定内容为空的模板：400（解析器会把空内容当未配置，绑定它只会制造「显示已绑定但走默认」的歧义）。
	empty := models.SubTemplate{Name: "空模板", Content: "   "}
	if err := db.Create(&empty).Error; err != nil {
		t.Fatal(err)
	}
	r = callAdmin(t, d, http.MethodPut, "/admin/permission-groups/1/sub-template",
		fmt.Sprintf(`{"sub_template_id":%d}`, empty.ID), map[string]string{"id": "1"}, d.AdminBindPermissionGroupSubTemplate)
	if r.Code != http.StatusBadRequest {
		t.Fatalf("绑定空内容模板应 400，实际 %d %s", r.Code, r.Body.String())
	}
}

// TestPermissionGroupTemplateListMeta 列表返回绑定模式/来源/名称与模板使用计数。
func TestPermissionGroupTemplateListMeta(t *testing.T) {
	db := apiTestDB(t)
	d := &Deps{DB: db}

	tpl := models.SubTemplate{Name: "模板A", Content: "tpl-content"}
	if err := db.Create(&tpl).Error; err != nil {
		t.Fatal(err)
	}
	bound := models.PermissionGroup{Name: "绑定组", SubTemplateID: &tpl.ID, ClashTemplate: "snapshot"}
	custom := models.PermissionGroup{Name: "自定义组", ClashTemplate: "copy"}
	empty := models.PermissionGroup{Name: "默认组"}
	for _, g := range []*models.PermissionGroup{&bound, &custom, &empty} {
		if err := db.Create(g).Error; err != nil {
			t.Fatal(err)
		}
	}

	r := callAdmin(t, d, http.MethodGet, "/admin/permission-groups", "", nil, d.AdminPermissionGroups)
	if r.Code != http.StatusOK {
		t.Fatalf("list groups failed: %d %s", r.Code, r.Body.String())
	}
	body := r.Body.String()
	for _, want := range []string{
		`"template_mode":"bound"`, `"template_source":"bound"`, `"sub_template_name":"模板A"`,
		`"template_mode":"custom"`, `"template_source":"custom"`, `"template_source":"default"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("权限组列表缺少 %s: %s", want, body)
		}
	}

	r = callAdmin(t, d, http.MethodGet, "/admin/sub-templates", "", nil, d.AdminListSubTemplates)
	if r.Code != http.StatusOK {
		t.Fatalf("list templates failed: %d %s", r.Code, r.Body.String())
	}
	body = r.Body.String()
	if !strings.Contains(body, `"usage_count":1`) || !strings.Contains(body, `"group_names":["绑定组"]`) {
		t.Fatalf("模板列表应带使用计数与组名: %s", body)
	}
}
