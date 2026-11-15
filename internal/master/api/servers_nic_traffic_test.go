package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 服务器列表的网卡周期流量 + 额度进度（2026-11-15）：
// 周期内 SUM、数据起点/partial、额度口径（total/rx/max）与状态阈值。
func TestAdminServersNICTraffic(t *testing.T) {
	deps := vpsDeps(t)
	if err := services.SetSetting(deps.DB, services.SettingBusinessTimezone, "UTC"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	day := func(d time.Time) string { return d.Format("2006-01-02") }
	const gb = int64(1024 * 1024 * 1024)

	start1 := today.AddDate(0, 0, -10)
	s1 := models.Server{Name: "s1", Host: "h1", NodeID: "n1", Secret: "x",
		NICCycleStart: day(start1), NICCycleDays: 30, NICQuotaGB: 100, NICQuotaDirection: "total"}
	s2 := models.Server{Name: "s2", Host: "h2", NodeID: "n2", Secret: "x",
		NICCycleStart: day(today.AddDate(0, 0, -40)), NICCycleDays: 30}
	s3 := models.Server{Name: "s3", Host: "h3", NodeID: "n3", Secret: "x",
		NICCycleStart: day(today.AddDate(0, 0, -10)), NICCycleDays: 30, NICQuotaGB: 100, NICQuotaDirection: "rx"}
	s4 := models.Server{Name: "s4", Host: "h4", NodeID: "n4", Secret: "x",
		NICCycleStart: day(today.AddDate(0, 0, -10)), NICCycleDays: 30, NICQuotaGB: 100, NICQuotaDirection: "max"}
	for _, s := range []*models.Server{&s1, &s2, &s3, &s4} {
		if err := deps.DB.Create(s).Error; err != nil {
			t.Fatal(err)
		}
	}

	rows := []models.ServerNICTrafficDaily{
		// s1：周期外一行（999GB）+ 周期内 3GB 下行 + 3GB 上行
		{ServerID: s1.ID, Date: day(start1.AddDate(0, 0, -1)), RxBytes: 999 * gb},
		{ServerID: s1.ID, Date: day(start1), RxBytes: 1 * gb},
		{ServerID: s1.ID, Date: day(today.AddDate(0, 0, -1)), TxBytes: 3 * gb},
		{ServerID: s1.ID, Date: day(today), RxBytes: 2 * gb},
		// s2：只有今天的数据，周期起点更早 → partial
		{ServerID: s2.ID, Date: day(today), RxBytes: 1 * gb},
		// s3：额度只看 rx
		{ServerID: s3.ID, Date: day(today), RxBytes: 80 * gb, TxBytes: 1 * gb},
		// s4：额度看双向较大值，tx 超额
		{ServerID: s4.ID, Date: day(today), RxBytes: 20 * gb, TxBytes: 120 * gb},
	}
	for i := range rows {
		if err := deps.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/servers", nil)
	deps.AdminServers(c)

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Items []serverView `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	byID := map[uint64]serverView{}
	for _, v := range resp.Data.Items {
		byID[v.ID] = v
	}

	t1 := byID[s1.ID].NICTraffic
	if t1 == nil {
		t.Fatal("s1 缺少 nic_traffic")
	}
	if t1.RxBytes != 3*gb || t1.TxBytes != 3*gb || t1.TotalBytes != 6*gb {
		t.Fatalf("s1 用量 = %+v, want rx=3GB tx=3GB total=6GB", t1)
	}
	if t1.CycleStart != day(start1) || t1.CycleEnd != day(start1.AddDate(0, 0, 30)) {
		t.Fatalf("s1 周期 = %s → %s", t1.CycleStart, t1.CycleEnd)
	}
	if t1.Partial || t1.TrackedSince != day(start1.AddDate(0, 0, -1)) {
		t.Fatalf("s1 partial/tracked = %v/%s", t1.Partial, t1.TrackedSince)
	}
	if t1.QuotaGB != 100 || t1.QuotaBytes != 100*gb || t1.QuotaUsedBytes != 6*gb ||
		math.Abs(t1.QuotaPercent-6) > 0.05 || t1.QuotaState != "ok" {
		t.Fatalf("s1 额度 = %+v", t1)
	}

	t2 := byID[s2.ID].NICTraffic
	if t2 == nil || !t2.Partial || t2.TrackedSince != day(today) {
		t.Fatalf("s2 partial/tracked = %+v", t2)
	}

	t3 := byID[s3.ID].NICTraffic
	if t3 == nil || t3.QuotaDirection != "rx" || t3.QuotaUsedBytes != 80*gb ||
		math.Abs(t3.QuotaPercent-80) > 0.05 || t3.QuotaState != "warning" {
		t.Fatalf("s3 额度 = %+v", t3)
	}

	t4 := byID[s4.ID].NICTraffic
	if t4 == nil || t4.QuotaDirection != "max" || t4.QuotaUsedBytes != 120*gb ||
		math.Abs(t4.QuotaPercent-120) > 0.05 || t4.QuotaState != "exceeded" {
		t.Fatalf("s4 额度 = %+v", t4)
	}
}

// 创建/更新接口：固定天数周期必须有起点；跨字段校验用库内现值 + 本次传参。
func TestServerNICCycleValidation(t *testing.T) {
	deps := vpsDeps(t)

	code, _ := createServerReq(t, deps, map[string]any{
		"name": "bad", "host": "h", "nic_cycle_days": 30,
	})
	if code != http.StatusBadRequest {
		t.Fatalf("days 无起点应 400，实际 %d", code)
	}

	code, srv := createServerReq(t, deps, map[string]any{
		"name": "good", "host": "h", "nic_cycle_start": "2026-11-01", "nic_cycle_days": 30,
		"nic_quota_gb": 2048, "nic_quota_direction": "tx",
	})
	if code != http.StatusOK {
		t.Fatalf("create 失败: %d", code)
	}
	if srv.NICCycleStart != "2026-11-01" || srv.NICCycleDays != 30 || srv.NICQuotaGB != 2048 || srv.NICQuotaDirection != "tx" {
		t.Fatalf("create 回显 = %+v", srv)
	}

	// 只传天数、沿用库内起点 → 合法
	code, _ = updateServerReq(t, deps, srv.ID, map[string]any{"nic_cycle_days": 60})
	if code != http.StatusOK {
		t.Fatalf("沿用起点改天数应成功，实际 %d", code)
	}
	// 清空起点但库内天数仍 >0 → 400
	code, _ = updateServerReq(t, deps, srv.ID, map[string]any{"nic_cycle_start": ""})
	if code != http.StatusBadRequest {
		t.Fatalf("清空起点但保留天数应 400，实际 %d", code)
	}
	// 同时清空天数+起点 → 合法（回到自动口径）
	code, _ = updateServerReq(t, deps, srv.ID, map[string]any{"nic_cycle_start": "", "nic_cycle_days": 0})
	if code != http.StatusOK {
		t.Fatalf("清空周期应成功，实际 %d", code)
	}
}
