package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// metricsResp 是 AdminServerMetrics 响应中本用例关心的字段。
type metricsResp struct {
	Code int `json:"code"`
	Data struct {
		Range         string    `json:"range"`
		CPU           []float64 `json:"cpu"`
		CPUMax        []float64 `json:"cpu_max"`
		MemPercent    []float64 `json:"mem_percent"`
		MemPercentMax []float64 `json:"mem_percent_max"`
		MemTotal      uint64    `json:"mem_total"`
		DiskPercent   []float64 `json:"disk_percent"`
		DiskPctMax    []float64 `json:"disk_percent_max"`
		DiskTotal     uint64    `json:"disk_total"`
		RxMbps        []float64 `json:"rx_mbps"`
		RxMbpsMax     []float64 `json:"rx_mbps_max"`
		TxMbps        []float64 `json:"tx_mbps"`
		TxMbpsMax     []float64 `json:"tx_mbps_max"`
		OnlineUsers   []int     `json:"online_users"`
		OnlineMax     []int     `json:"online_users_max"`
	} `json:"data"`
}

// callMetrics 以给定 range 调用监控接口并解出响应。
func callMetrics(t *testing.T, db *gorm.DB, serverID uint64, rng string) metricsResp {
	t.Helper()
	deps := &Deps{DB: db}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?range="+rng, nil)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(serverID, 10)}}
	deps.AdminServerMetrics(c)

	if w.Code != http.StatusOK {
		t.Fatalf("range=%s status = %d, want 200；body=%s", rng, w.Code, w.Body.String())
	}
	var resp metricsResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("range=%s unmarshal: %v", rng, err)
	}
	return resp
}

// TestAdminServerMetricsBucketAvgAndPeak 桶内均值与桶内峰值同口径并存：
// 同一分钟桶内三个采样点（CPU 10/90/20、Rx 1/4/2 Mbps、在线 3/7/5）应产出
// 均值（CPU 40、Rx 2.33Mbps、在线 5）与峰值（CPU 90、Rx 4Mbps、在线 7），
// 且各档峰值恒 ≥ 同档均值。
func TestAdminServerMetricsBucketAvgAndPeak(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Server{}, &models.NodeReport{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	srv := models.Server{Name: "node1", Host: "1.2.3.4", NodeID: "n1"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}

	// 三个采样点取同一时刻，保证落在同一个桶里（桶边界 = startTime + k*60s，而 startTime 是
	// handler 自己的 now - 1h，秒数由请求时刻决定，测试侧不可预知）。原实现取 +5/+15/+25 秒，
	// 边界秒数落在 5~25 区间内时第三个点会被分到下一个桶，用例按钟表秒数时红时绿
	// （2026-09-21 实测：连续 8 次跑出 2 次失败）。聚合语义（均值/峰值/计数）不受影响。
	base := time.Now().Add(-10 * time.Minute).Truncate(time.Minute)
	rows := []models.NodeReport{
		{
			ServerID: srv.ID, ReportedAt: base,
			CPU: 10, Mem: 200_000_000, MemTotal: 1_000_000_000,
			Disk: 500_000_000, DiskTotal: 2_000_000_000,
			RxRate: 125_000, TxRate: 250_000, OnlineUsers: 3,
		},
		{
			ServerID: srv.ID, ReportedAt: base,
			CPU: 90, Mem: 400_000_000, MemTotal: 1_000_000_000,
			Disk: 600_000_000, DiskTotal: 2_000_000_000,
			RxRate: 500_000, TxRate: 250_000, OnlineUsers: 7,
		},
		{
			ServerID: srv.ID, ReportedAt: base,
			CPU: 20, Mem: 300_000_000, MemTotal: 1_000_000_000,
			Disk: 550_000_000, DiskTotal: 2_000_000_000,
			RxRate: 250_000, TxRate: 250_000, OnlineUsers: 5,
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	resp := callMetrics(t, db, srv.ID, "1h")
	d := resp.Data

	if len(d.CPU) != len(d.CPUMax) {
		t.Fatalf("cpu 与 cpu_max 长度不一致: %d vs %d", len(d.CPU), len(d.CPUMax))
	}
	// 定位承载这三个采样点的桶：Rx 峰值非零即该桶。
	idx := -1
	for i, v := range d.RxMbpsMax {
		if v > 0 {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("未在响应中找到承载采样点的桶（rx_mbps_max 全 0）")
	}

	if got := d.CPU[idx]; got != 40 {
		t.Errorf("cpu[%d] = %v, want 40（10/90/20 的均值）", idx, got)
	}
	if got := d.CPUMax[idx]; got != 90 {
		t.Errorf("cpu_max[%d] = %v, want 90", idx, got)
	}
	if got := d.RxMbps[idx]; got != 2.33 {
		t.Errorf("rx_mbps[%d] = %v, want 2.33（1/4/2 Mbps 的均值）", idx, got)
	}
	if got := d.RxMbpsMax[idx]; got != 4 {
		t.Errorf("rx_mbps_max[%d] = %v, want 4", idx, got)
	}
	if got := d.TxMbps[idx]; got != 2 {
		t.Errorf("tx_mbps[%d] = %v, want 2", idx, got)
	}
	if got := d.TxMbpsMax[idx]; got != 2 {
		t.Errorf("tx_mbps_max[%d] = %v, want 2", idx, got)
	}
	if got := d.MemPercent[idx]; got != 30 {
		t.Errorf("mem_percent[%d] = %v, want 30（200/400/300MB 对 1GB 的均值）", idx, got)
	}
	if got := d.MemPercentMax[idx]; got != 40 {
		t.Errorf("mem_percent_max[%d] = %v, want 40", idx, got)
	}
	if got := d.DiskPercent[idx]; got != 27.5 {
		t.Errorf("disk_percent[%d] = %v, want 27.5", idx, got)
	}
	if got := d.DiskPctMax[idx]; got != 30 {
		t.Errorf("disk_percent_max[%d] = %v, want 30", idx, got)
	}
	if got := d.OnlineUsers[idx]; got != 5 {
		t.Errorf("online_users[%d] = %v, want 5（3/7/5 的均值）", idx, got)
	}
	if got := d.OnlineMax[idx]; got != 7 {
		t.Errorf("online_users_max[%d] = %v, want 7", idx, got)
	}
	if d.MemTotal != 1_000_000_000 || d.DiskTotal != 2_000_000_000 {
		t.Errorf("mem_total/disk_total = %d/%d, want 1e9/2e9", d.MemTotal, d.DiskTotal)
	}

	// 全局不变量：任一下标峰值 ≥ 均值（空桶前向填充后仍应成立）。
	for i := range d.CPU {
		if d.CPUMax[i] < d.CPU[i] {
			t.Fatalf("cpu_max[%d]=%v < cpu[%d]=%v", i, d.CPUMax[i], i, d.CPU[i])
		}
		if d.RxMbpsMax[i] < d.RxMbps[i] {
			t.Fatalf("rx_mbps_max[%d]=%v < rx_mbps[%d]=%v", i, d.RxMbpsMax[i], i, d.RxMbps[i])
		}
		if d.TxMbpsMax[i] < d.TxMbps[i] {
			t.Fatalf("tx_mbps_max[%d]=%v < tx_mbps[%d]=%v", i, d.TxMbpsMax[i], i, d.TxMbps[i])
		}
		if d.OnlineMax[i] < d.OnlineUsers[i] {
			t.Fatalf("online_users_max[%d]=%d < online_users[%d]=%d", i, d.OnlineMax[i], i, d.OnlineUsers[i])
		}
		if d.MemPercentMax[i] < d.MemPercent[i] {
			t.Fatalf("mem_percent_max[%d]=%v < mem_percent[%d]=%v", i, d.MemPercentMax[i], i, d.MemPercent[i])
		}
		if d.DiskPctMax[i] < d.DiskPercent[i] {
			t.Fatalf("disk_percent_max[%d]=%v < disk_percent[%d]=%v", i, d.DiskPctMax[i], i, d.DiskPercent[i])
		}
	}
}

// TestAdminServerMetricsRanges 各档 range 均被接受并回显，桶数与读桶档位一致：
// 1h@1m、6h@3m、24h@10m、7d@30m、30d@1h；未知档位回退 1h。
// 读桶不得细于降采样分辨率（>6h 为 10 分钟），否则桶内无真实采样点。
func TestAdminServerMetricsRanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.Server{}, &models.NodeReport{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	srv := models.Server{Name: "node1", Host: "1.2.3.4", NodeID: "n1"}
	if err := db.Create(&srv).Error; err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		rng        string
		wantRange  string
		wantBucket time.Duration
	}{
		{"1h", "1h", time.Minute},
		{"6h", "6h", 3 * time.Minute},
		{"24h", "24h", 10 * time.Minute},
		{"7d", "7d", 30 * time.Minute},
		{"30d", "30d", time.Hour},
		{"nonsense", "1h", time.Minute}, // 未知档位回退 1h，不报错（轮询接口不应因参数漂移 500）
	}
	for _, tc := range cases {
		resp := callMetrics(t, db, srv.ID, tc.rng)
		if resp.Data.Range != tc.wantRange {
			t.Errorf("range=%s 回显 = %q, want %q", tc.rng, resp.Data.Range, tc.wantRange)
		}
		// 桶数按 (now - startTime) / bucketDuration 取整，允许 ±1 的边界抖动。
		want := int(spanOf(tc.wantRange) / tc.wantBucket)
		if got := len(resp.Data.CPU); got < want-1 || got > want+1 {
			t.Errorf("range=%s 桶数 = %d, want ≈%d（读桶 %s）", tc.rng, got, want, tc.wantBucket)
		}
	}
}

// spanOf 返回某档 range 的时间跨度（与 handler 的 startTime 取法一致）。
func spanOf(rng string) time.Duration {
	switch rng {
	case "6h":
		return 6 * time.Hour
	case "24h":
		return 24 * time.Hour
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	default:
		return time.Hour
	}
}
