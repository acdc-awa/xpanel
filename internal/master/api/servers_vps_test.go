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
	"github.com/acdc-awa/xpanel/internal/models"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return db
}

// vpsDeps 构造带 Cfg 的 Deps：创建/重置密钥端点要用 Cfg.App.PublicURL 拼安装命令，
// 与同包其它测试一致（不给 Cfg 会让 handler 空指针，不该靠生产代码加守卫来掩盖）。
func vpsDeps(t *testing.T) *Deps {
	t.Helper()
	return &Deps{DB: setupTestDB(t), Cfg: config.Default()}
}

type vpsServerResp struct {
	Code int `json:"code"`
	Data struct {
		Server serverView `json:"server"`
	} `json:"data"`
}

func createServerReq(t *testing.T, deps *Deps, body map[string]any) (int, serverView) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal create body: %v", err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/admin/servers", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	deps.AdminCreateServer(c)

	var resp vpsServerResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp.Data.Server
}

func updateServerReq(t *testing.T, deps *Deps, id uint64, body map[string]any) (int, serverView) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal update body: %v", err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(id, 10)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/admin/servers/"+strconv.FormatUint(id, 10), bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	deps.AdminUpdateServer(c)

	var resp vpsServerResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp.Data.Server
}

func TestServerVPSFields_CreateAndUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deps := vpsDeps(t)

	// 1. 创建带有 VPS 账单与 IDC 信息的服务器
	code, srv := createServerReq(t, deps, map[string]any{
		"name":          "Tokyo-01",
		"host":          "tokyo01.example.com",
		"location":      "日本东京",
		"remark":        "软银线路",
		"expire_at":     "2026-10-15",
		"billing_cycle": "年付",
		"price":         "$49.99/年",
		"idc_address":   "https://bwh88.net",
	})
	if code != http.StatusOK {
		t.Fatalf("AdminCreateServer status = %d", code)
	}
	if srv.Name != "Tokyo-01" {
		t.Errorf("expected name Tokyo-01, got %s", srv.Name)
	}
	if srv.BillingCycle != "年付" {
		t.Errorf("expected billing_cycle 年付, got %s", srv.BillingCycle)
	}
	if srv.Price != "$49.99/年" {
		t.Errorf("expected price $49.99/年, got %s", srv.Price)
	}
	if srv.IDCAddress != "https://bwh88.net" {
		t.Errorf("expected idc_address https://bwh88.net, got %s", srv.IDCAddress)
	}
	// 到期日原样存储：不做任何时区换算，输入什么就是什么
	if srv.ExpireAt != "2026-10-15" {
		t.Errorf("expected expire_at 2026-10-15, got %q", srv.ExpireAt)
	}
	serverID := srv.ID

	// 2. 更新 VPS 字段
	code, srvUpdated := updateServerReq(t, deps, serverID, map[string]any{
		"billing_cycle": "月付",
		"price":         "¥35.00",
		"idc_address":   "https://console.cloud.tencent.com",
		"expire_at":     "2026-11-01",
	})
	if code != http.StatusOK {
		t.Fatalf("AdminUpdateServer status = %d", code)
	}
	if srvUpdated.BillingCycle != "月付" {
		t.Errorf("expected billing_cycle 月付, got %s", srvUpdated.BillingCycle)
	}
	if srvUpdated.Price != "¥35.00" {
		t.Errorf("expected price ¥35.00, got %s", srvUpdated.Price)
	}
	if srvUpdated.IDCAddress != "https://console.cloud.tencent.com" {
		t.Errorf("expected idc_address https://console.cloud.tencent.com, got %s", srvUpdated.IDCAddress)
	}
	if srvUpdated.ExpireAt != "2026-11-01" {
		t.Errorf("expected expire_at 2026-11-01, got %q", srvUpdated.ExpireAt)
	}

	// 3. 清空到期日（expire_at: null）——空串表示未设置
	code, cleared := updateServerReq(t, deps, serverID, map[string]any{"expire_at": nil})
	if code != http.StatusOK {
		t.Fatalf("clear expire_at status = %d", code)
	}
	if cleared.ExpireAt != "" {
		t.Errorf("expected expire_at cleared to empty, got %q", cleared.ExpireAt)
	}

	// 4. 三元语义：重新设置后，未提及 expire_at 的更新不应改动它
	code, _ = updateServerReq(t, deps, serverID, map[string]any{"expire_at": "2026-12-31"})
	if code != http.StatusOK {
		t.Fatalf("re-set expire_at status = %d", code)
	}
	code, untouched := updateServerReq(t, deps, serverID, map[string]any{"remark": "只改备注"})
	if code != http.StatusOK {
		t.Fatalf("partial update status = %d", code)
	}
	if untouched.ExpireAt != "2026-12-31" {
		t.Errorf("partial update must not touch expire_at, got %q", untouched.ExpireAt)
	}

	// 5. 列表接口回显（走 DB 往返，确认落库形态就是纯日期）
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/servers", nil)
	deps.AdminServers(c)
	if w.Code != http.StatusOK {
		t.Fatalf("AdminServers status = %d, body = %s", w.Code, w.Body.String())
	}
	var listResp struct {
		Code int `json:"code"`
		Data struct {
			Items []serverView `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list resp: %v", err)
	}
	if len(listResp.Data.Items) == 0 {
		t.Fatalf("expected items in list, got 0")
	}
	found := listResp.Data.Items[0]
	if found.IDCAddress != "https://console.cloud.tencent.com" {
		t.Errorf("list item idc_address mismatch: %s", found.IDCAddress)
	}
	if found.ExpireAt != "2026-12-31" {
		t.Errorf("list item expire_at mismatch: %q", found.ExpireAt)
	}
}

// TestServerVPSFields_DateNormalization 到期日是日历日：各写法都归一为 YYYY-MM-DD。
func TestServerVPSFields_DateNormalization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"标准写法", "2026-10-15", "2026-10-15"},
		{"未补零", "2026-1-5", "2026-01-05"},
		{"斜杠分隔", "2026/10/15", "2026-10-15"},
		{"两侧空格", "  2026-10-15  ", "2026-10-15"},
		{"旧客户端完整时间戳取 UTC 日期", "2026-10-15T16:00:00Z", "2026-10-15"},
		{"空格分隔的时间戳", "2026-10-15 16:00:00", "2026-10-15"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := vpsDeps(t)
			code, srv := createServerReq(t, deps, map[string]any{
				"name":      "Node",
				"host":      "node.example.com",
				"expire_at": tc.in,
			})
			if code != http.StatusOK {
				t.Fatalf("create status = %d", code)
			}
			if srv.ExpireAt != tc.want {
				t.Errorf("expire_at %q → got %q, want %q", tc.in, srv.ExpireAt, tc.want)
			}
		})
	}
}

func TestServerVPSFields_InvalidDateFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	deps := vpsDeps(t)
	// 非法日期必须被拒绝，而不是被静默存成别的日子
	for _, bad := range []string{"invalid-date-string", "2026-02-30", "2026-13-01", "2026-10", "abc"} {
		code, _ := createServerReq(t, deps, map[string]any{
			"name":      "Bad",
			"host":      "bad.example.com",
			"expire_at": bad,
		})
		if code != http.StatusBadRequest {
			t.Errorf("expire_at %q: expected 400, got %d", bad, code)
		}
	}
}

// TestServerVPSFields_DateIndependentOfLocalTimezone 是这次改造的回归守卫：
// 到期日全程不做时区换算，因此进程本地时区取任何值，同一输入的存储结果都必须一致。
// 实测有效：把解析改回"按本地时区解析再转 UTC 输出"（即当初的 bug 形态），
// 本用例在 UTC+8 / UTC+14 下会报 got "2026-10-14" want 2026-10-15。
func TestServerVPSFields_DateIndependentOfLocalTimezone(t *testing.T) {
	orig := time.Local
	defer func() { time.Local = orig }()

	const in = `"2026-10-15"`
	for _, tz := range []*time.Location{
		time.UTC,
		time.FixedZone("UTC+8", 8*3600),
		time.FixedZone("UTC-5", -5*3600),
		time.FixedZone("UTC+14", 14*3600),
	} {
		time.Local = tz
		got, err := parseExpireDate(json.RawMessage(in))
		if err != nil {
			t.Fatalf("local=%s: parseExpireDate(%s) error: %v", tz, in, err)
		}
		if got != "2026-10-15" {
			t.Errorf("local=%s: got %q, want 2026-10-15（到期日不该随本地时区漂移）", tz, got)
		}
	}
}
