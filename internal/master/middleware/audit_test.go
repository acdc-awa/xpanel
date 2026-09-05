package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func setupAuditMWDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.AuditLog{}))
	return db
}

// auditRequest 按 routePattern 注册路由、请求 reqPath，返回落库的第一条审计记录。
func auditRequest(t *testing.T, db *gorm.DB, method, routePattern, reqPath, body string) *models.AuditLog {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Audit(db))
	handler := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }
	switch method {
	case http.MethodPost:
		r.POST(routePattern, handler)
	case http.MethodPut:
		r.PUT(routePattern, handler)
	case http.MethodDelete:
		r.DELETE(routePattern, handler)
	case http.MethodGet:
		r.GET(routePattern, handler)
	}
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, reqPath, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, reqPath, nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var log models.AuditLog
	assert.NoError(t, db.First(&log).Error)
	return &log
}

// JSON body → v2 信封：真实路径/参数/状态 + 脱敏 body。
func TestAuditEnvelopeStructure(t *testing.T) {
	db := setupAuditMWDB(t)
	log := auditRequest(t, db, http.MethodPut, "/api/v1/admin/plans/:id", "/api/v1/admin/plans/3",
		`{"name":"VIP","price_cents":2500,"password":"p@ss"}`)

	var env map[string]any
	assert.NoError(t, json.Unmarshal([]byte(log.Detail), &env))
	assert.Equal(t, float64(2), env["v"])
	assert.Equal(t, http.MethodPut, env["method"])
	assert.Equal(t, "/api/v1/admin/plans/3", env["path"])
	assert.Equal(t, float64(http.StatusOK), env["status"])
	assert.Equal(t, map[string]any{"id": "3"}, env["params"])

	body, ok := env["body"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "VIP", body["name"])
	assert.Equal(t, float64(2500), body["price_cents"])
	assert.Equal(t, "***", body["password"])
}

// 组合键名（turnstile_secret）也应脱敏；嵌套 JSON 字符串剥一层。
func TestAuditSanitizeNestedAndSecrets(t *testing.T) {
	db := setupAuditMWDB(t)
	log := auditRequest(t, db, http.MethodPost, "/api/v1/admin/inbounds", "/api/v1/admin/inbounds",
		`{"tag":"a","stream_settings":"{\"network\":\"tcp\",\"security\":\"reality\",\"turnstile_secret\":\"xyz\",\"realitySettings\":{\"privateKey\":\"REALITYSECRET\"}}","key_pem":"PRIVATE"}`)

	var env map[string]any
	assert.NoError(t, json.Unmarshal([]byte(log.Detail), &env))
	body := env["body"].(map[string]any)
	assert.Equal(t, "***", body["key_pem"])
	stream, ok := body["stream_settings"].(map[string]any)
	assert.True(t, ok, "嵌套 JSON 字符串应被解析为对象")
	assert.Equal(t, "tcp", stream["network"])
	assert.Equal(t, "reality", stream["security"])
	assert.Equal(t, "***", stream["turnstile_secret"])
	reality, ok := stream["realitySettings"].(map[string]any)
	assert.True(t, ok, "REALITY 设置应被解析为对象")
	assert.Equal(t, "***", reality["privateKey"], "驼峰 privateKey 应脱敏（REALITY 私钥不得明文落审计）")
	assert.NotContains(t, log.Detail, "REALITYSECRET")
}

// 超长文本摘要化：__text 标记（预览 200 字符 + 行数 + 字符数），不落全文。
func TestAuditLargeTextSummarized(t *testing.T) {
	db := setupAuditMWDB(t)
	long := strings.Repeat(`line\n`, 60) // JSON 转义后 300 字符、60 个换行
	log := auditRequest(t, db, http.MethodPut, "/api/v1/admin/permission-groups/:id", "/api/v1/admin/permission-groups/7",
		`{"name":"g","clash_template":"`+long+`"}`)

	var env map[string]any
	assert.NoError(t, json.Unmarshal([]byte(log.Detail), &env))
	body := env["body"].(map[string]any)
	tpl, ok := body["clash_template"].(map[string]any)
	assert.True(t, ok, "大文本应为 __text 摘要")
	mark := tpl["__text"].(map[string]any)
	assert.Equal(t, float64(61), mark["lines"])
	assert.Equal(t, float64(300), mark["chars"])
	assert.Len(t, mark["preview"], 200)
	// 全文超出预览的部分不应落库
	tailJSON, _ := json.Marshal(long[250:])
	assert.NotContains(t, log.Detail, strings.Trim(string(tailJSON), `"`))
}

// body 非 JSON → body_raw 回退（正则脱敏 JSON 形态的敏感键值）。
func TestAuditNonJSONBodyFallback(t *testing.T) {
	db := setupAuditMWDB(t)
	// 截断的 JSON（解析失败）但含敏感键值对
	log := auditRequest(t, db, http.MethodPost, "/api/v1/admin/misc", "/api/v1/admin/misc", `{"password":"abc"`)

	var env map[string]any
	assert.NoError(t, json.Unmarshal([]byte(log.Detail), &env))
	assert.Contains(t, env["body_raw"], "***")
	assert.NotContains(t, env["body_raw"], "abc")
}

// DELETE 无 body：信封含真实路径与参数；GET 不记审计。
func TestAuditDeleteEnvelopeAndSkipGET(t *testing.T) {
	db := setupAuditMWDB(t)
	log := auditRequest(t, db, http.MethodDelete, "/api/v1/admin/users/:id", "/api/v1/admin/users/9", "")
	var env map[string]any
	assert.NoError(t, json.Unmarshal([]byte(log.Detail), &env))
	assert.Equal(t, http.MethodDelete, env["method"])
	assert.Equal(t, "/api/v1/admin/users/9", env["path"])
	assert.Equal(t, map[string]any{"id": "9"}, env["params"])
	_, hasBody := env["body"]
	assert.False(t, hasBody)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Audit(db))
	r.GET("/api/v1/admin/plans", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/plans", nil))
	var cnt int64
	db.Model(&models.AuditLog{}).Count(&cnt)
	assert.Equal(t, int64(1), cnt, "GET 不应新增审计记录")
}

// 旧版正则回退路径保持可用（注：匹配后冒号后的空格会被一并消费）。
func TestRedactKeyLegacy(t *testing.T) {
	assert.Equal(t, `{"password":"***","x":1}`, redactKey(`{"password":"abc","x":1}`, "password"))
	assert.Equal(t, `{"token":"***"}`, redactKey(`{"token": "abc"}`, "token"))
}

// 目标名注册表：删除/启停等无 body 上下文的操作，执行前预读实体显示名进 envelope.target。
func TestAuditTargetRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newDB := func(t *testing.T) *gorm.DB {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		assert.NoError(t, err)
		assert.NoError(t, db.AutoMigrate(&models.AuditLog{}, &models.Notice{}, &models.User{}))
		assert.NoError(t, db.Create(&models.Notice{Title: "公告A", Content: "c"}).Error)
		assert.NoError(t, db.Create(&models.User{Email: "a@x.com", Username: "a@x.com", UUID: "u1", SubscribeToken: "t1"}).Error)
		return db
	}
	getEnvelope := func(t *testing.T, db *gorm.DB) map[string]any {
		t.Helper()
		var log models.AuditLog
		assert.NoError(t, db.First(&log).Error)
		var env map[string]any
		assert.NoError(t, json.Unmarshal([]byte(log.Detail), &env))
		return env
	}

	// DELETE：注册表预读标题（记录删除后仍可读）
	db := newDB(t)
	r := gin.New()
	r.Use(Audit(db))
	r.DELETE("/api/v1/admin/notices/:id", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/notices/1", nil))
	env := getEnvelope(t, db)
	assert.Equal(t, "公告A", env["target"])

	// toggle：请求体只有字段名，目标名靠注册表
	db = newDB(t)
	r = gin.New()
	r.Use(Audit(db))
	r.POST("/api/v1/admin/notices/:id/toggle", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/notices/1/toggle", strings.NewReader(`{"field":"is_popup"}`)))
	env = getEnvelope(t, db)
	assert.Equal(t, "公告A", env["target"])

	// 目标不存在：不报错，envelope 无 target 键
	db = newDB(t)
	r = gin.New()
	r.Use(Audit(db))
	r.DELETE("/api/v1/admin/notices/:id", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/notices/999", nil))
	env = getEnvelope(t, db)
	_, hasTarget := env["target"]
	assert.False(t, hasTarget)

	// 未注册路由：不产生 target
	db = newDB(t)
	r = gin.New()
	r.Use(Audit(db))
	r.POST("/api/v1/admin/notices", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/notices", strings.NewReader(`{"title":"x"}`)))
	env = getEnvelope(t, db)
	_, hasTarget = env["target"]
	assert.False(t, hasTarget)
}
