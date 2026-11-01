package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/contracts"
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

// 大文本（订阅模板/权限组正文）v5 起原样落库：正文要可检索、可取证。
func TestAuditLargeTextPreserved(t *testing.T) {
	db := setupAuditMWDB(t)
	long := strings.Repeat(`line`, 60) // 240 字符（原用例的超长文本）
	log := auditRequest(t, db, http.MethodPut, "/api/v1/admin/permission-groups/:id", "/api/v1/admin/permission-groups/7",
		`{"name":"g","clash_template":"`+long+`"}`)

	var env map[string]any
	assert.NoError(t, json.Unmarshal([]byte(log.Detail), &env))
	body := env["body"].(map[string]any)
	tpl, ok := body["clash_template"].(string)
	assert.True(t, ok, "正文应原样保留，实际类型 %T", body["clash_template"])
	assert.Equal(t, long, tpl)
}

// base64/data-URI 转 sha256 指纹：既不落 1.4 MB 的 logo，又保留「内容是否被改过」。
func TestAuditBinaryDigested(t *testing.T) {
	db := setupAuditMWDB(t)
	payload := strings.Repeat("A", 2000)
	log := auditRequest(t, db, http.MethodPut, "/api/v1/admin/settings", "/api/v1/admin/settings",
		`{"logo":"data:image/png;base64,`+payload+`"}`)

	var env map[string]any
	assert.NoError(t, json.Unmarshal([]byte(log.Detail), &env))
	body := env["body"].(map[string]any)
	bin, ok := body["logo"].(map[string]any)
	assert.True(t, ok, "data URI 应转成 __binary 摘要")
	meta := bin["__binary"].(map[string]any)
	assert.Equal(t, "image/png", meta["type"])
	assert.Equal(t, float64(1500), meta["bytes"], "bytes 是解码后的字节数")
	assert.Len(t, meta["sha256"], 64)
	assert.NotContains(t, log.Detail, payload[:50], "原始 base64 不得落库")

	assert.True(t, isBase64Like(strings.Repeat("QUJD", 400)))
	assert.False(t, isBase64Like(strings.Repeat("这是一段中文", 100)))
}

// 数组上限 200，且尾部留一条显式标记说明还有多少项（旧版静默丢弃）。
func TestAuditArrayCapWithMarker(t *testing.T) {
	db := setupAuditMWDB(t)
	items := make([]string, 0, 205)
	for i := 0; i < 205; i++ {
		items = append(items, `{"email":"u`+strconv.Itoa(i)+`@x.com"}`)
	}
	body := `{"clients":[` + strings.Join(items, ",") + `]}`
	log := auditRequest(t, db, http.MethodPut, "/api/v1/admin/inbounds/:id", "/api/v1/admin/inbounds/1", body)

	var env map[string]any
	assert.NoError(t, json.Unmarshal([]byte(log.Detail), &env))
	arr := env["body"].(map[string]any)["clients"].([]any)
	assert.Len(t, arr, 201, "超 200 条时保留前 200 条 + 一条标记")
	mark := arr[200].(map[string]any)["__omitted"].(map[string]any)
	assert.Equal(t, float64(5), mark["items"])
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

// 凭据类字段全覆盖：私钥 PEM、Hysteria auth、HTTP/SOCKS pass、VLESS 客户端 UUID、
// 用户 UUID；公钥/CA 证书保持可读（2026-10-28 审查 P2-2）。
func TestAuditSanitizeCredentialMatrix(t *testing.T) {
	db := setupAuditMWDB(t)
	log := auditRequest(t, db, http.MethodPost, "/api/v1/admin/inbounds", "/api/v1/admin/inbounds",
		`{"ca_key_pem":"-----BEGIN EC PRIVATE KEY-----\nCAKEYMARK","ca_pem":"PUBLICCA","public_key":"PUBKEY",`+
			`"uuid":"USERUUIDMARK",`+
			`"settings_json":"{\"users\":[{\"auth\":\"HYSECRETMARK\",\"email\":\"a@b.c\"}],\"clients\":[{\"id\":\"VLESSUUIDMARK\",\"email\":\"u@x\"}],\"accounts\":[{\"user\":\"u\",\"pass\":\"PASSMARK\"}]}"}`)

	var env map[string]any
	assert.NoError(t, json.Unmarshal([]byte(log.Detail), &env))
	body := env["body"].(map[string]any)

	assert.Equal(t, "***", body["ca_key_pem"], "CA 私钥 PEM 不得明文落审计")
	assert.Equal(t, "***", body["uuid"], "用户连接凭据 UUID 不得明文落审计")
	assert.Equal(t, "***", body["public_key"], "密钥类字段统一脱敏")
	assert.Equal(t, "PUBLICCA", body["ca_pem"], "CA 证书本身是公开材料，保持可读")

	settings, ok := body["settings_json"].(map[string]any)
	assert.True(t, ok, "settings_json 应被解析为对象")
	users := settings["users"].([]any)
	assert.Equal(t, "***", users[0].(map[string]any)["auth"], "Hysteria 用户密码不得明文落审计")
	clients := settings["clients"].([]any)
	assert.Equal(t, "***", clients[0].(map[string]any)["id"], "VLESS clients[].id（UUID）不得明文落审计")
	accounts := settings["accounts"].([]any)
	assert.Equal(t, "***", accounts[0].(map[string]any)["pass"], "HTTP/SOCKS 用户密码不得明文落审计")

	for _, marker := range []string{"CAKEYMARK", "USERUUIDMARK", "HYSECRETMARK", "VLESSUUIDMARK", "PASSMARK"} {
		assert.NotContains(t, log.Detail, marker, "脱敏标记 %s 不应出现在审计 detail", marker)
	}
}

// 超过嵌套上限的 JSON 串不落原文（内部可能含凭据，按 key 脱敏只对可解析对象生效）。
func TestAuditOversizeJSONNotStoredRaw(t *testing.T) {
	db := setupAuditMWDB(t)

	bigBlob := `{"clients":[{"id":"BIGBLOBUUID","email":"u@x"}],"pad":"` + strings.Repeat("x", 40000) + `"}`
	blobJSON, _ := json.Marshal(bigBlob)
	log := auditRequest(t, db, http.MethodPost, "/api/v1/admin/inbounds", "/api/v1/admin/inbounds",
		`{"tag":"a","settings_json":`+string(blobJSON)+`}`)
	assert.NotContains(t, log.Detail, "BIGBLOBUUID")
	assert.Contains(t, log.Detail, "__omitted")

	// 信封超过 32 KB：按最长字段优先省略，小字段全部保留，并记录 truncated_fields。
	db2 := setupAuditMWDB(t)
	fields := []string{`"password":"OVERSIZEPASS"`, `"keep":"small-value"`}
	for i := 0; i < 6; i++ {
		// 中文不算 base64 字符：确保走「超限省略」而不是二进制摘要
		fields = append(fields, `"f`+strconv.Itoa(i)+`":"`+strings.Repeat("日志内容", 3000)+`"`)
	}
	bigBody := `{` + strings.Join(fields, ",") + `}`
	log2 := auditRequest(t, db2, http.MethodPost, "/api/v1/admin/inbounds", "/api/v1/admin/inbounds", bigBody)
	var env map[string]any
	assert.NoError(t, json.Unmarshal([]byte(log2.Detail), &env))
	assert.Equal(t, true, env["__truncated"])
	assert.NotEmpty(t, env["truncated_fields"])
	assert.NotContains(t, log2.Detail, "OVERSIZEPASS")
	body := env["body"].(map[string]any)
	assert.Equal(t, "small-value", body["keep"], "小字段不该被一起丢掉")
	assert.LessOrEqual(t, len(log2.Detail), auditDetailCap+1024)
}

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

// 目标快照：删除/启停等无 body 上下文的操作，执行前预读实体显示名；
// v5 起类型/主键/显示名落到 audit_logs 的独立列（不再塞进 detail 信封）。
func TestAuditTargetRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newDB := func(t *testing.T) *gorm.DB {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		assert.NoError(t, err)
		assert.NoError(t, db.AutoMigrate(&models.AuditLog{}, &models.Notice{}, &models.User{}))
		assert.NoError(t, db.Create(&models.Notice{Title: "公告A", Content: "c"}).Error)
		assert.NoError(t, db.Create(&models.User{Email: "a@x.com", UUID: "u1", SubscribeToken: "t1"}).Error)
		return db
	}
	lastLog := func(t *testing.T, db *gorm.DB) models.AuditLog {
		t.Helper()
		var log models.AuditLog
		assert.NoError(t, db.Order("id DESC").First(&log).Error)
		return log
	}
	call := func(db *gorm.DB, method, pattern, path, body string) {
		t.Helper()
		r := gin.New()
		r.Use(Audit(db))
		h := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) }
		switch method {
		case http.MethodPost:
			r.POST(pattern, h)
		case http.MethodDelete:
			r.DELETE(pattern, h)
		}
		var req *http.Request
		if body != "" {
			req = httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	// DELETE：注册表预读标题（记录删除后仍可读）
	db := newDB(t)
	call(db, http.MethodDelete, "/api/v1/admin/notices/:id", "/api/v1/admin/notices/1", "")
	got := lastLog(t, db)
	assert.Equal(t, "notice", got.TargetType)
	assert.EqualValues(t, 1, got.TargetID)
	assert.Equal(t, "公告A", got.TargetName)
	assert.NotContains(t, got.Detail, `"target"`, "目标不再重复写进 detail")

	// toggle：请求体只有字段名，目标名靠注册表
	db = newDB(t)
	call(db, http.MethodPost, "/api/v1/admin/notices/:id/toggle", "/api/v1/admin/notices/1/toggle", `{"field":"is_popup"}`)
	got = lastLog(t, db)
	assert.Equal(t, "公告A", got.TargetName)

	// 目标不存在：不报错，只留类型与主键
	db = newDB(t)
	call(db, http.MethodDelete, "/api/v1/admin/notices/:id", "/api/v1/admin/notices/999", "")
	got = lastLog(t, db)
	assert.Equal(t, "notice", got.TargetType)
	assert.EqualValues(t, 999, got.TargetID)
	assert.Equal(t, "", got.TargetName)

	// 创建类（无 :id）：类型来自路径，显示名从 body 推断
	db = newDB(t)
	call(db, http.MethodPost, "/api/v1/admin/notices", "/api/v1/admin/notices", `{"title":"新公告"}`)
	got = lastLog(t, db)
	assert.Equal(t, "notice", got.TargetType)
	assert.EqualValues(t, 0, got.TargetID)
	assert.Equal(t, "新公告", got.TargetName)

	// 无目标对象的模块：类型留空
	db = newDB(t)
	call(db, http.MethodPost, "/api/v1/admin/settings", "/api/v1/admin/settings", `{"site_name":"x"}`)
	got = lastLog(t, db)
	assert.Equal(t, "", got.TargetType)
	assert.Equal(t, "", got.TargetName)

	// status 落列：handler 返回 400 时记为失败，可按结果筛选
	db = newDB(t)
	r := gin.New()
	r.Use(Audit(db))
	r.POST("/api/v1/admin/plans", func(c *gin.Context) { c.JSON(http.StatusBadRequest, gin.H{"error": "x"}) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/admin/plans", strings.NewReader(`{"name":"P"}`)))
	got = lastLog(t, db)
	assert.Equal(t, http.StatusBadRequest, got.Status)
	assert.Equal(t, "plan", got.TargetType)
	assert.Equal(t, "P", got.TargetName)
}

// 创建类操作（无 :id）：成功后按显示名回查主键，让「同邮箱重建」的两段历史能按 id 区分；
// 名称不唯一时不猜主键。
func TestAuditCreateFillsTargetID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.AuditLog{}, &models.User{}, &models.Notice{}))
	// 同名公告：创建类操作无法确定新行，只能留空
	assert.NoError(t, db.Create(&models.Notice{Title: "重名", Content: "c"}).Error)
	assert.NoError(t, db.Create(&models.Notice{Title: "重名", Content: "c"}).Error)

	r := gin.New()
	r.Use(Audit(db))
	r.POST("/api/v1/admin/users", func(c *gin.Context) {
		u := models.User{Email: "new@x.com", UUID: "u-new", SubscribeToken: "t-new"}
		assert.NoError(t, db.Create(&u).Error)
		c.JSON(http.StatusOK, gin.H{"id": u.ID})
	})
	r.POST("/api/v1/admin/notices", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/admin/users",
		strings.NewReader(`{"email":"new@x.com"}`)))

	var log models.AuditLog
	assert.NoError(t, db.Order("id DESC").First(&log).Error)
	assert.Equal(t, "user", log.TargetType)
	assert.Equal(t, "new@x.com", log.TargetName)
	assert.NotZero(t, log.TargetID, "创建成功应回填新行主键")
	var created models.User
	assert.NoError(t, db.First(&created, log.TargetID).Error)
	assert.Equal(t, "new@x.com", created.Email)

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/admin/notices",
		strings.NewReader(`{"title":"重名"}`)))
	var noticeLog models.AuditLog
	assert.NoError(t, db.Order("id DESC").First(&noticeLog).Error)
	assert.Equal(t, "notice", noticeLog.TargetType)
	assert.EqualValues(t, 0, noticeLog.TargetID, "同名目标不猜主键")
}

// 操作人快照：写入时记 users.email 到 operator_name，改邮箱后历史行不变。
func TestAuditOperatorSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.AuditLog{}, &models.User{}))
	admin := models.User{Email: "old@panel.local", UUID: "u-adm", SubscribeToken: "t-adm", Role: models.RoleAdmin}
	assert.NoError(t, db.Create(&admin).Error)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set(CtxClaimsKey, &contracts.JWTClaims{UserID: admin.ID})
		c.Next()
	})
	r.Use(Audit(db))
	r.POST("/api/v1/admin/plans", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/admin/plans", strings.NewReader(`{"name":"P"}`)))

	var log models.AuditLog
	assert.NoError(t, db.First(&log).Error)
	assert.Equal(t, "old@panel.local", log.OperatorName)
	assert.Equal(t, "admin", log.OperatorType)

	// 管理员改邮箱：历史行仍是写入时的快照
	assert.NoError(t, db.Model(&models.User{}).Where("id = ?", admin.ID).Update("email", "new@panel.local").Error)
	assert.NoError(t, db.First(&log).Error)
	assert.Equal(t, "old@panel.local", log.OperatorName)
}
