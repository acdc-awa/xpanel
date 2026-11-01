package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

func newAuditQueryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.AuditLog{}))
	rows := []models.AuditLog{
		{OperatorType: "admin", OperatorID: 1, OperatorName: "a%b@x.com", Action: "users.:id", Category: "users",
			TargetType: "user", TargetID: 7, TargetName: "target_7@x.com", Status: 200, Detail: `{"v":2}`},
		{OperatorType: "admin", OperatorID: 2, OperatorName: "normal@x.com", Action: "servers.:id", Category: "servers",
			TargetType: "server", TargetID: 3, TargetName: "hk-main", Status: 400, Detail: `{"v":2,"status":400}`},
		{OperatorType: "user", OperatorID: 5, OperatorName: "user@x.com", Action: "auth.login", Category: "auth",
			Status: 200, Detail: `{"v":2,"message":"登录成功"}`},
	}
	require.NoError(t, db.Create(&rows).Error)
	return db
}

func auditQuery(t *testing.T, db *gorm.DB, rawQuery string) []models.AuditLog {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/admin/audit-logs"+rawQuery, nil)
	d := &Deps{DB: db}
	var out []models.AuditLog
	require.NoError(t, d.buildAuditQuery(c).Order("id").Find(&out).Error)
	return out
}

// LIKE 通配符必须转义：输入 % 或 _ 不能变成全表匹配。
func TestAuditKeywordEscaping(t *testing.T) {
	db := newAuditQueryDB(t)

	assert.Len(t, auditQuery(t, db, "?keyword=%25"), 1, "% 应只匹配含字面百分号的行")
	assert.Len(t, auditQuery(t, db, "?keyword=_"), 1, "_ 应只匹配含字面下划线的行")
	assert.Len(t, auditQuery(t, db, "?keyword=a%25b"), 1)
	assert.Len(t, auditQuery(t, db, "?keyword=normal"), 1)
}

// 关键词覆盖操作人快照与目标名快照：卡片上看到的邮箱/目标名都搜得到。
func TestAuditKeywordCoversSnapshots(t *testing.T) {
	db := newAuditQueryDB(t)

	got := auditQuery(t, db, "?keyword=normal@x.com")
	require.Len(t, got, 1)
	assert.Equal(t, "servers.:id", got[0].Action)

	got = auditQuery(t, db, "?keyword=hk-main")
	require.Len(t, got, 1)
	assert.EqualValues(t, 3, got[0].TargetID)
}

// 目标与结果筛选（v5 新增列）。
func TestAuditTargetAndResultFilters(t *testing.T) {
	db := newAuditQueryDB(t)

	assert.Len(t, auditQuery(t, db, "?target_type=user&target_id=7"), 1)
	assert.Len(t, auditQuery(t, db, "?result=fail"), 1)
	assert.Len(t, auditQuery(t, db, "?result=success"), 2)
	assert.Len(t, auditQuery(t, db, "?status=400"), 1)
	assert.Len(t, auditQuery(t, db, "?operator_name=user@x.com"), 1)
}

// 列表截断：超过 8 KB 的 detail 缩到 8 KB 并打标记，完整内容走详情接口。
func TestAuditListDetailTruncated(t *testing.T) {
	long := ""
	for i := 0; i < 9000; i++ {
		long += "日志"
	}
	cut, truncated := truncateAuditDetail(long)
	assert.True(t, truncated)
	assert.LessOrEqual(t, len(cut), auditListDetailCap)
	assert.Equal(t, cut, string([]rune(cut)), "截断不得切出半个汉字")

	short, truncated := truncateAuditDetail(`{"v":2}`)
	assert.False(t, truncated)
	assert.Equal(t, `{"v":2}`, short)
}
