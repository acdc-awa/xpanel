package api

import (
	"time"

	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 服务器网卡周期流量的读侧组装（2026-11-15）。
//
// 口径与数据源：
//   - 数据来自 agent 上报的累计物理网卡 Rx/Tx 字节，经 nodegate 差分后写入
//     server_nic_traffic_daily（业务时区日聚合）；
//   - 周期由 services.ResolveNICCycle 解析（计费周期+到期日 / 自定义起点+天数 / 自然月）；
//   - 额度按 NICQuotaDirection 选择 total（Rx+Tx）/ rx / tx / max 参与进度判定。

// nicTrafficView 一台服务器的网卡周期流量 + 额度进度。
type nicTrafficView struct {
	RxBytes    int64 `json:"rx_bytes"`    // 本周期网卡接收（下行）
	TxBytes    int64 `json:"tx_bytes"`    // 本周期网卡发送（上行）
	TotalBytes int64 `json:"total_bytes"` // Rx+Tx

	CycleStart string `json:"cycle_start"` // YYYY-MM-DD（含）
	CycleEnd   string `json:"cycle_end"`   // YYYY-MM-DD（不含）
	CycleLabel string `json:"cycle_label"` // 自然月 / 计费周期（月付，按到期日）/ 自定义 30 天

	// TrackedSince 日表里这台服务器最早的一天（YYYY-MM-DD）；空 = 还没有任何数据。
	// Partial = 最早一天晚于本周期起点：本周期数字只覆盖从 TrackedSince 起的一段，
	// 面板要显式标注，不能把半周期的数当成整周期用量。
	TrackedSince string `json:"tracked_since,omitempty"`
	Partial      bool   `json:"partial"`

	// 额度（0 = 不限）。QuotaUsedBytes 已按 QuotaDirection 取口径；
	// QuotaPercent 可以 >100（超额），QuotaState: ""不限 / ok / warning(≥80%) / exceeded(≥100%)。
	QuotaGB        int64   `json:"quota_gb"`
	QuotaBytes     int64   `json:"quota_bytes"`
	QuotaDirection string  `json:"quota_direction"`
	QuotaUsedBytes int64   `json:"quota_used_bytes"`
	QuotaPercent   float64 `json:"quota_percent"`
	QuotaState     string  `json:"quota_state,omitempty"`
}

// fillNICTraffic 给服务器列表批量补齐网卡周期流量与额度。
//
// 每台服务器一条带索引的日表聚合（不是扫 node_reports）：日表每台每天一行，
// 即使 3 年历史也只有 ~1000 行，列表接口的开销可忽略。
func (d *Deps) fillNICTraffic(views []serverView, servers []models.Server) {
	if d.DB == nil || len(views) == 0 {
		return
	}
	loc := services.BusinessLocation(d.DB)
	now := time.Now()
	for i := range views {
		if i >= len(servers) {
			break
		}
		s := &servers[i]
		cycle := services.ResolveNICCycle(s, now, loc)
		start, end := cycle.Start.Format("2006-01-02"), cycle.End.Format("2006-01-02")

		var row struct {
			RxBytes   int64  `gorm:"column:rx_bytes"`
			TxBytes   int64  `gorm:"column:tx_bytes"`
			FirstDate string `gorm:"column:first_date"`
		}
		// 周期内 SUM 走 CASE，MIN(date) 取全表：一个查询同时拿到「本周期用量」与「数据起点」。
		_ = d.DB.Model(&models.ServerNICTrafficDaily{}).
			Select(`COALESCE(SUM(CASE WHEN date >= ? AND date < ? THEN rx_bytes ELSE 0 END), 0) AS rx_bytes,
				COALESCE(SUM(CASE WHEN date >= ? AND date < ? THEN tx_bytes ELSE 0 END), 0) AS tx_bytes,
				MIN(date) AS first_date`, start, end, start, end).
			Where("server_id = ?", s.ID).
			Scan(&row).Error

		v := &nicTrafficView{
			RxBytes:        row.RxBytes,
			TxBytes:        row.TxBytes,
			TotalBytes:     row.RxBytes + row.TxBytes,
			CycleStart:     start,
			CycleEnd:       end,
			CycleLabel:     cycle.Label,
			TrackedSince:   row.FirstDate,
			Partial:        row.FirstDate != "" && row.FirstDate > start,
			QuotaGB:        s.NICQuotaGB,
			QuotaDirection: services.NICQuotaDirection(s.NICQuotaDirection),
		}
		if v.QuotaGB > 0 {
			v.QuotaBytes = services.NICGBToBytes(v.QuotaGB)
			v.QuotaUsedBytes = services.NICQuotaUsed(v.QuotaDirection, v.RxBytes, v.TxBytes)
			v.QuotaPercent = services.NICQuotaPercent(v.QuotaBytes, v.QuotaUsedBytes)
			v.QuotaState = services.NICQuotaState(v.QuotaBytes, v.QuotaUsedBytes)
		}
		views[i].NICTraffic = v
	}
}
