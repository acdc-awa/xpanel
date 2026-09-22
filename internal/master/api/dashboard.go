package api

import (
	"fmt"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/nodegate"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// DashboardData 响应数据结构。
type DashboardData struct {
	Summary struct {
		TodayRevenueCents   int64   `json:"today_revenue_cents"`
		TodayUsedCardsCount int64   `json:"today_used_cards_count"`
		MonthRevenueCents   int64   `json:"month_revenue_cents"`
		TotalRevenueCents   int64   `json:"total_revenue_cents"`
		TodayTrafficUp      int64   `json:"today_traffic_up"`
		TodayTrafficDown    int64   `json:"today_traffic_down"`
		TodayTrafficTotal   int64   `json:"today_traffic_total"`
		MonthTrafficTotal   int64   `json:"month_traffic_total"`
		OnlineServers       int64   `json:"online_servers"`
		TotalServers        int64   `json:"total_servers"`
		ActiveUsers         int64   `json:"active_users"`
		TotalUsers          int64   `json:"total_users"`
		TotalOrders         int64   `json:"total_orders"`
		TodayOrders         int64   `json:"today_orders"`
		RealtimeRxRate      float64 `json:"realtime_rx_rate"`
		RealtimeTxRate      float64 `json:"realtime_tx_rate"`
	} `json:"summary"`

	TrafficTrend []TrafficTrendPoint `json:"traffic_trend"`

	// 时间口径（2026-09-17）：server_breakdown / user_rank / server_rank 三者同源同期，
	// 均由 rank_period 查询参数决定（today / 7d / month），避免「累计/占比」语义不明。
	RankPeriod string `json:"rank_period"`
	RankLabel  string `json:"rank_label"` // 中文口径标签，前端直接用于图表副标题
	RankSince  string `json:"rank_since"` // 口径起点（业务日期，含）

	ServerBreakdown []ServerTrafficItem `json:"server_breakdown"`

	UserRank []UserTrafficRankItem `json:"user_rank"`

	ServerRank []ServerTrafficItem `json:"server_rank"`

	ServerMatrix []ServerMatrixItem `json:"server_matrix"`

	RecentGiftCards []RecentGiftCardItem `json:"recent_gift_cards"`

	RecentOrders []RecentOrderItem `json:"recent_orders"`
}

// DashboardRealtimeData 轻量实时网速与服务器监控矩阵响应（纯内存组装，0 复杂 SQL 聚合）。
type DashboardRealtimeData struct {
	RealtimeRxRate float64            `json:"realtime_rx_rate"`
	RealtimeTxRate float64            `json:"realtime_tx_rate"`
	OnlineServers  int64              `json:"online_servers"`
	TotalServers   int64              `json:"total_servers"`
	ServerMatrix   []ServerMatrixItem `json:"server_matrix"`
}

type TrafficTrendPoint struct {
	Date       string `json:"date"`
	UpBytes    int64  `json:"up_bytes"`
	DownBytes  int64  `json:"down_bytes"`
	TotalBytes int64  `json:"total_bytes"`
}

type ServerTrafficItem struct {
	ServerID   uint64  `json:"server_id"`
	Name       string  `json:"name"`
	Location   string  `json:"location"`
	UpBytes    int64   `json:"up_bytes"`
	DownBytes  int64   `json:"down_bytes"`
	TotalBytes int64   `json:"total_bytes"`
	Percent    float64 `json:"percent"`
}

type UserTrafficRankItem struct {
	UserID     uint64 `json:"user_id"`
	Username   string `json:"username"`
	Email      string `json:"email"`
	PlanName   string `json:"plan_name"`
	UpBytes    int64  `json:"up_bytes"`
	DownBytes  int64  `json:"down_bytes"`
	TotalBytes int64  `json:"total_bytes"`
}

// rankWindow 排行榜/流量分布的时间口径窗口（2026-09-17）。
// 三个档位均以业务时区切天，与流量趋势、traffic_dailies 同源：
//   - today：今日 0 点起（默认）
//   - 7d   ：近 7 个业务日（含今日）
//   - month：本月 1 日起至今日（月初至今）
type rankWindow struct {
	Key       string    // 回显前端：today / 7d / month
	Label     string    // 中文口径标签（前端副标题直用，避免各处复述走样）
	StartDate string    // 业务日期下界（含），用于 traffic_dailies.date
	EndDate   string    // 业务日期上界（含），恒为今日
	StartUTC  time.Time // traffic_logs.period_start 下界（含）
	EndUTC    time.Time // traffic_logs.period_start 上界（不含，= 明日 0 点）
}

// parseRankWindow 解析 rank_period 查询参数；未知/缺省一律回退今日。
// 不可识别的取值不做报错：仪表盘是轮询接口，参数漂移不应让整个看板 500。
func parseRankWindow(raw string, now time.Time, loc *time.Location) rankWindow {
	bizNow := now.In(loc)
	todayStart := services.DayStart(now, loc)
	var start time.Time
	var key, label string
	switch raw {
	case "7d":
		key, label = "7d", "近 7 天"
		start = todayStart.AddDate(0, 0, -6)
	case "month":
		key, label = "month", "本月"
		start = time.Date(bizNow.Year(), bizNow.Month(), 1, 0, 0, 0, 0, loc)
	default:
		key, label = "today", "今日"
		start = todayStart
	}
	return rankWindow{
		Key:       key,
		Label:     label,
		StartDate: start.Format("2006-01-02"),
		EndDate:   todayStart.Format("2006-01-02"),
		StartUTC:  start.UTC(),
		EndUTC:    todayStart.AddDate(0, 0, 1).UTC(),
	}
}

type ServerMatrixItem struct {
	ID           uint64     `json:"id"`
	Name         string     `json:"name"`
	NodeID       string     `json:"node_id"`
	Host         string     `json:"host"`
	Location     string     `json:"location"`
	Status       int        `json:"status"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
	CPU          float64    `json:"cpu"`
	Mem          float64    `json:"mem"`
	MemTotal     uint64     `json:"mem_total"`
	Disk         float64    `json:"disk"`
	DiskTotal    uint64     `json:"disk_total"`
	RxRate       float64    `json:"rx_rate"`
	TxRate       float64    `json:"tx_rate"`
	OnlineUsers  int        `json:"online_users"`
	IsActiveFlow bool       `json:"is_active_flow"`
}

type RecentGiftCardItem struct {
	ID             uint64    `json:"id"`
	CodeMasked     string    `json:"code_masked"`
	Name           string    `json:"name"`
	FaceValueCents int64     `json:"face_value_cents"`
	UsedByUsername string    `json:"used_by_username"`
	UsedAt         time.Time `json:"used_at"`
}

type RecentOrderItem struct {
	ID            uint64     `json:"id"`
	OrderNo       string     `json:"order_no"`
	Username      string     `json:"username"`
	PlanName      string     `json:"plan_name"`
	AmountCents   int64      `json:"amount_cents"`
	PaymentMethod string     `json:"payment_method"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	PaidAt        *time.Time `json:"paid_at"`
}

// AdminDashboard GET /api/v1/admin/dashboard
func (d *Deps) AdminDashboard(c *gin.Context) {
	now := time.Now()
	// 按天口径以业务时区切分（跨地域部署的「今日/本月」一致）；绑定查询统一转 UTC——
	// SQLite 以带偏移文本存储并按字面量比较，混入非 UTC 偏移会得到错误的区间结果。
	loc := services.BusinessLocation(d.DB)
	bizNow := now.In(loc)
	todayStart := services.DayStart(now, loc)
	monthStart := time.Date(bizNow.Year(), bizNow.Month(), 1, 0, 0, 0, 0, loc)
	todayStartUTC := todayStart.UTC()
	monthStartUTC := monthStart.UTC()
	todayStr := bizNow.Format("2006-01-02")
	monthPrefix := bizNow.Format("2006-01")

	// 排行榜与流量分布的时间口径（2026-09-17）：三者同源，前端切换一次全部联动。
	rankWin := parseRankWindow(c.Query("rank_period"), now, loc)

	var data DashboardData
	data.RankPeriod = rankWin.Key
	data.RankLabel = rankWin.Label
	data.RankSince = rankWin.StartDate

	// 1. 财务数据（Mini Financial System）
	// 今日卡密激活金额与张数
	var todayRev struct {
		Total int64
		Count int64
	}
	d.DB.Model(&models.GiftCard{}).
		Select("COALESCE(SUM(face_value_cents), 0) as total, COUNT(id) as count").
		Where("status = ? AND used_at >= ?", models.GiftCardUsed, todayStartUTC).
		Scan(&todayRev)
	data.Summary.TodayRevenueCents = todayRev.Total
	data.Summary.TodayUsedCardsCount = todayRev.Count

	// 本月卡密激活金额
	var monthRev int64
	d.DB.Model(&models.GiftCard{}).
		Select("COALESCE(SUM(face_value_cents), 0)").
		Where("status = ? AND used_at >= ?", models.GiftCardUsed, monthStartUTC).
		Scan(&monthRev)
	data.Summary.MonthRevenueCents = monthRev

	// 累计卡密激活金额
	var totalRev int64
	d.DB.Model(&models.GiftCard{}).
		Select("COALESCE(SUM(face_value_cents), 0)").
		Where("status = ?", models.GiftCardUsed).
		Scan(&totalRev)
	data.Summary.TotalRevenueCents = totalRev

	// 2. 基础计数
	d.DB.Model(&models.Server{}).Where("status = 1").Count(&data.Summary.OnlineServers)
	d.DB.Model(&models.Server{}).Count(&data.Summary.TotalServers)

	d.DB.Model(&models.User{}).Count(&data.Summary.TotalUsers)
	d.DB.Model(&models.User{}).Where("status = 1").Count(&data.Summary.ActiveUsers)

	d.DB.Model(&models.Order{}).Count(&data.Summary.TotalOrders)
	d.DB.Model(&models.Order{}).Where("created_at >= ?", todayStartUTC).Count(&data.Summary.TodayOrders)

	// 3. 流量汇总与趋势（天粒度，范围可调 3/7/30 天，默认 30）
	trendDays := 30
	switch c.Query("days") {
	case "3":
		trendDays = 3
	case "7":
		trendDays = 7
	}
	startTime := todayStart.AddDate(0, 0, -(trendDays - 1))
	var dailies []models.TrafficDaily
	d.DB.Where("date >= ?", startTime.Format("2006-01-02")).Find(&dailies)

	dailyMap := make(map[string]*TrafficTrendPoint)
	for i := 0; i < trendDays; i++ {
		dStr := startTime.AddDate(0, 0, i).Format("2006-01-02")
		dailyMap[dStr] = &TrafficTrendPoint{Date: dStr}
	}
	for _, td := range dailies {
		if pt, ok := dailyMap[td.Date]; ok {
			pt.UpBytes += td.UpBytes
			pt.DownBytes += td.DownBytes
			pt.TotalBytes += (td.UpBytes + td.DownBytes)
		}
	}

	// 本月流量合计独立统计（与趋势范围解耦：days=3/7 时不能借趋势切片计算）
	var monthAgg struct {
		Total int64
	}
	d.DB.Model(&models.TrafficDaily{}).
		Select("COALESCE(SUM(up_bytes + down_bytes), 0) AS total").
		Where("date LIKE ?", monthPrefix+"%").
		Scan(&monthAgg)
	data.Summary.MonthTrafficTotal = monthAgg.Total

	// ISSUE-06：traffic_dailies 已包含今日（每 5 分钟聚合），不能再与 traffic_logs 相加。
	// 只取 max(daily, 实时 logs) 修正最近 5 分钟未聚合窗口，保持单一数据源、无重复计数。
	// 仅计真实用户流量（user_id > 0），内部转发流量（user_id = 0）由服务器承载分布统计，不双计全站业务吞吐。
	var todayLog struct {
		Up   int64
		Down int64
	}
	d.DB.Model(&models.TrafficLog{}).
		Where("period_start >= ? AND user_id > 0", todayStartUTC).
		Select("COALESCE(SUM(up_bytes),0) AS up, COALESCE(SUM(down_bytes),0) AS down").
		Scan(&todayLog)
	if pt, ok := dailyMap[todayStr]; ok {
		deltaUp := todayLog.Up - pt.UpBytes
		deltaDown := todayLog.Down - pt.DownBytes
		if deltaUp > 0 {
			data.Summary.MonthTrafficTotal += deltaUp
			pt.UpBytes = todayLog.Up
		}
		if deltaDown > 0 {
			data.Summary.MonthTrafficTotal += deltaDown
			pt.DownBytes = todayLog.Down
		}
		pt.TotalBytes = pt.UpBytes + pt.DownBytes
		data.Summary.TodayTrafficUp = pt.UpBytes
		data.Summary.TodayTrafficDown = pt.DownBytes
		data.Summary.TodayTrafficTotal = pt.TotalBytes
	}

	data.TrafficTrend = make([]TrafficTrendPoint, 0, trendDays)
	for i := 0; i < trendDays; i++ {
		dStr := startTime.AddDate(0, 0, i).Format("2006-01-02")
		if pt, ok := dailyMap[dStr]; ok {
			data.TrafficTrend = append(data.TrafficTrend, *pt)
		}
	}

	// 4. 服务器健康度矩阵 & 实时速率（优先读取 Hub 纯内存快照，0 磁盘 I/O）
	var servers []models.Server
	d.DB.Order("id ASC").Find(&servers)

	var liveMetrics map[uint64]*nodegate.NodeMetricsSnapshot
	if d.Hub != nil {
		liveMetrics = d.Hub.GetAllLatestMetrics()
	}

	// 离线或尚无内存快照的节点，回退至 node_reports 查静态底数
	var fallbackReports []models.NodeReport
	needFallback := false
	for _, s := range servers {
		if liveMetrics == nil || liveMetrics[s.ID] == nil {
			needFallback = true
			break
		}
	}
	latestByServer := make(map[uint64]models.NodeReport)
	if needFallback {
		d.DB.Raw(`SELECT nr.* FROM node_reports nr
			JOIN (SELECT server_id, MAX(reported_at) AS max_at FROM node_reports GROUP BY server_id) x
			ON nr.server_id = x.server_id AND nr.reported_at = x.max_at`).Scan(&fallbackReports)
		for _, nr := range fallbackReports {
			latestByServer[nr.ServerID] = nr
		}
	}

	var onlineCount int64
	for _, s := range servers {
		isOnline := (s.Status == 1)
		if d.Hub != nil {
			isOnline = d.Hub.IsOnline(s.ID)
		}
		if isOnline {
			onlineCount++
		}

		var cpu, mem, disk, rxRate, txRate float64
		var memTotal, diskTotal uint64
		var onlineUsers int
		lastSeen := s.LastSeenAt

		if m, ok := liveMetrics[s.ID]; ok && m != nil {
			cpu = m.CPU
			mem = m.Mem
			memTotal = m.MemTotal
			disk = m.Disk
			diskTotal = m.DiskTotal
			if isOnline {
				rxRate = m.RxRate
				txRate = m.TxRate
				onlineUsers = m.OnlineUsers
				t := m.ReportedAt
				lastSeen = &t
			}
		} else if fb, ok := latestByServer[s.ID]; ok {
			cpu = fb.CPU
			mem = fb.Mem
			memTotal = fb.MemTotal
			disk = fb.Disk
			diskTotal = fb.DiskTotal
			onlineUsers = fb.OnlineUsers
			// 离线节点速率归零
			rxRate = 0
			txRate = 0
		}

		isActiveFlow := (rxRate > 1024 || txRate > 1024)
		if isOnline {
			data.Summary.RealtimeRxRate += rxRate
			data.Summary.RealtimeTxRate += txRate
		}

		statusVal := 0
		if isOnline {
			statusVal = 1
		}

		data.ServerMatrix = append(data.ServerMatrix, ServerMatrixItem{
			ID:           s.ID,
			Name:         s.Name,
			NodeID:       s.NodeID,
			Host:         s.Host,
			Location:     s.Location,
			Status:       statusVal,
			LastSeenAt:   lastSeen,
			CPU:          cpu,
			Mem:          mem,
			MemTotal:     memTotal,
			Disk:         disk,
			DiskTotal:    diskTotal,
			RxRate:       rxRate,
			TxRate:       txRate,
			OnlineUsers:  onlineUsers,
			IsActiveFlow: isActiveFlow,
		})
	}
	if d.Hub != nil {
		data.Summary.OnlineServers = onlineCount
	}

	// 5. 节点/服务器流量分布（时间口径，原始用量）
	// 2026-09-17 起口径由「入站冗余计数器累加」改为「traffic_logs 按时间窗聚合」：
	// 入站 up/down 是自上次流量重置（traffic_reset: never/daily/weekly/monthly）以来的
	// 累计值，被周期性清零、且无时间维度，无法回答「哪个时间段用了多少」——原先文案
	// 只写「累计承载占比」，与重置语义自相矛盾。现与排行榜共用 rank_period 时间窗。
	//
	// 归集口径：traffic_logs.inbound_id → inbounds.server_id（原始字节不乘倍率）。
	// 包含前台用户流量（user_id > 0）与内部链式转发流量（relay 入站，user_id = 0），
	// 准确反映该服务器承载的中转/落地流量总量。
	// 已知边界：inbound_id=0 的明细无法归属服务器（agent 未带 tag 且统计键未注入入站维度），由 JOIN 排除。
	type serverAggRow struct {
		ServerID  uint64 `gorm:"column:server_id"`
		UpBytes   int64  `gorm:"column:up_bytes"`
		DownBytes int64  `gorm:"column:down_bytes"`
	}
	var serverRows []serverAggRow
	d.DB.Raw(`
		SELECT i.server_id AS server_id,
		       COALESCE(SUM(l.up_bytes), 0)   AS up_bytes,
		       COALESCE(SUM(l.down_bytes), 0) AS down_bytes
		FROM traffic_logs l
		JOIN inbounds i ON i.id = l.inbound_id
		WHERE l.period_start >= ? AND l.period_start < ?
		GROUP BY i.server_id`, rankWin.StartUTC, rankWin.EndUTC).Scan(&serverRows)

	aggByServer := make(map[uint64]serverAggRow, len(serverRows))
	var grandTotalBytes int64
	for _, r := range serverRows {
		aggByServer[r.ServerID] = r
		grandTotalBytes += r.UpBytes + r.DownBytes
	}

	serverTrafficMap := make(map[uint64]*ServerTrafficItem, len(servers))
	for _, s := range servers {
		r := aggByServer[s.ID]
		item := &ServerTrafficItem{
			ServerID:   s.ID,
			Name:       s.Name,
			Location:   s.Location,
			UpBytes:    r.UpBytes,
			DownBytes:  r.DownBytes,
			TotalBytes: r.UpBytes + r.DownBytes,
		}
		if grandTotalBytes > 0 {
			item.Percent = float64(item.TotalBytes) / float64(grandTotalBytes) * 100
		}
		serverTrafficMap[s.ID] = item
		// 排行只收有流量的服务器（分布饼图仍保留全量，避免图例/占比数学被改写）
		if item.TotalBytes > 0 {
			data.ServerRank = append(data.ServerRank, *item)
		}
	}
	// 分布按 ServerID 排序固定顺序：Go map 遍历随机，否则前端饼图按索引着色会每次刷新变颜色
	for _, s := range servers {
		if item, ok := serverTrafficMap[s.ID]; ok {
			data.ServerBreakdown = append(data.ServerBreakdown, *item)
		}
	}
	// 排行按用量倒序（并列时按 ServerID 稳定）
	sort.Slice(data.ServerRank, func(i, j int) bool {
		if data.ServerRank[i].TotalBytes != data.ServerRank[j].TotalBytes {
			return data.ServerRank[i].TotalBytes > data.ServerRank[j].TotalBytes
		}
		return data.ServerRank[i].ServerID < data.ServerRank[j].ServerID
	})
	if len(data.ServerRank) > 10 {
		data.ServerRank = data.ServerRank[:10]
	}

	// 6. 用户流量消耗排行榜 Top 10（时间口径，原始用量）
	// 数据源用 traffic_dailies（按业务日期聚合、有索引，且不受 traffic_logs 保留期清理影响），
	// 今日那一天叠加 max(daily, logs) 修正——每日汇总每 5 分钟才落盘，最近窗口尚未聚合；
	// 取 max 而非相加，保持单一数据源不双计（同 ISSUE-06 口径）。
	var plans []models.Plan
	d.DB.Find(&plans)
	planMap := make(map[uint64]string, len(plans))
	for _, p := range plans {
		planMap[p.ID] = p.Name
	}

	type userAggRow struct {
		UserID    uint64 `gorm:"column:user_id"`
		UpBytes   int64  `gorm:"column:up_bytes"`
		DownBytes int64  `gorm:"column:down_bytes"`
	}
	scanByUser := func(q *gorm.DB) []userAggRow {
		var rows []userAggRow
		q.Select("user_id, COALESCE(SUM(up_bytes),0) AS up_bytes, COALESCE(SUM(down_bytes),0) AS down_bytes").
			Group("user_id").Scan(&rows)
		return rows
	}

	// 6.1 区间每日汇总（口径窗口内的全部业务日）
	periodRows := scanByUser(d.DB.Model(&models.TrafficDaily{}).
		Where("date >= ? AND date <= ?", rankWin.StartDate, rankWin.EndDate))
	// 6.2 今日与实时明细（修正用；三档窗口都含今日，故恒需修正）
	dailyTodayRows := scanByUser(d.DB.Model(&models.TrafficDaily{}).Where("date = ? AND user_id > 0", rankWin.EndDate))
	logsTodayRows := scanByUser(d.DB.Model(&models.TrafficLog{}).
		Where("period_start >= ? AND period_start < ? AND user_id > 0", todayStartUTC, rankWin.EndUTC))

	// 上下行分别聚合：修正按方向取 max，保持两个方向互不串味
	periodUp := make(map[uint64]int64, len(periodRows))
	periodDown := make(map[uint64]int64, len(periodRows))
	for _, r := range periodRows {
		periodUp[r.UserID] = r.UpBytes
		periodDown[r.UserID] = r.DownBytes
	}
	dailyTodayUp := make(map[uint64]int64, len(dailyTodayRows))
	dailyTodayDown := make(map[uint64]int64, len(dailyTodayRows))
	for _, r := range dailyTodayRows {
		dailyTodayUp[r.UserID] = r.UpBytes
		dailyTodayDown[r.UserID] = r.DownBytes
	}
	logsTodayUp := make(map[uint64]int64, len(logsTodayRows))
	logsTodayDown := make(map[uint64]int64, len(logsTodayRows))
	for _, r := range logsTodayRows {
		logsTodayUp[r.UserID] = r.UpBytes
		logsTodayDown[r.UserID] = r.DownBytes
	}

	// 今日有明细但区间内尚无汇总行的用户（新用户/尚未聚合）也要进榜
	for uid := range logsTodayUp {
		if _, ok := periodUp[uid]; !ok {
			periodUp[uid] = 0
			periodDown[uid] = 0
		}
	}
	// 合并：区间汇总 - 今日汇总 + max(今日汇总, 今日明细)
	maxI64 := func(a, b int64) int64 {
		if a > b {
			return a
		}
		return b
	}
	userUp := make(map[uint64]int64, len(periodUp))
	userDown := make(map[uint64]int64, len(periodUp))
	for uid, up := range periodUp {
		down := periodDown[uid]
		dUp, dDown := dailyTodayUp[uid], dailyTodayDown[uid]
		userUp[uid] = up - dUp + maxI64(dUp, logsTodayUp[uid])
		userDown[uid] = down - dDown + maxI64(dDown, logsTodayDown[uid])
	}

	rankIDs := make([]uint64, 0, len(userUp))
	for uid := range userUp {
		if userUp[uid]+userDown[uid] > 0 { // 零用量不进排行
			rankIDs = append(rankIDs, uid)
		}
	}
	userRankList := make([]UserTrafficRankItem, 0, len(rankIDs))
	if len(rankIDs) > 0 {
		var rankUsers []models.User
		d.DB.Select("id, username, email, plan_id").Where("id IN ?", rankIDs).Find(&rankUsers)
		for _, u := range rankUsers {
			pName := planMap[u.PlanID]
			if pName == "" {
				pName = "无套餐"
			}
			userRankList = append(userRankList, UserTrafficRankItem{
				UserID:     u.ID,
				Username:   u.Username,
				Email:      u.Email,
				PlanName:   pName,
				UpBytes:    userUp[u.ID],
				DownBytes:  userDown[u.ID],
				TotalBytes: userUp[u.ID] + userDown[u.ID],
			})
		}
	}
	sort.Slice(userRankList, func(i, j int) bool {
		if userRankList[i].TotalBytes != userRankList[j].TotalBytes {
			return userRankList[i].TotalBytes > userRankList[j].TotalBytes
		}
		return userRankList[i].UserID < userRankList[j].UserID
	})
	if len(userRankList) > 10 {
		data.UserRank = userRankList[:10]
	} else {
		data.UserRank = userRankList
	}

	// 7. 最近卡密激活流水 Top 5
	var usedCards []models.GiftCard
	d.DB.Where("status = ?", models.GiftCardUsed).Order("used_at DESC").Limit(5).Find(&usedCards)
	userMap := make(map[uint64]string)
	// 只为卡密使用者的用户名取名（原实现全表载入用户仅为此一处，改按需查询）
	if len(usedCards) > 0 {
		ids := make([]uint64, 0, len(usedCards))
		for _, card := range usedCards {
			if card.UsedBy > 0 {
				ids = append(ids, card.UsedBy)
			}
		}
		if len(ids) > 0 {
			var usedBy []models.User
			d.DB.Select("id, username").Where("id IN ?", ids).Find(&usedBy)
			for _, u := range usedBy {
				userMap[u.ID] = u.Username
			}
		}
	}
	for _, card := range usedCards {
		masked := card.Code
		if len(masked) > 8 {
			masked = masked[:4] + "-****-" + masked[len(masked)-4:]
		}
		usedTime := time.Now()
		if card.UsedAt != nil {
			usedTime = *card.UsedAt
		}
		data.RecentGiftCards = append(data.RecentGiftCards, RecentGiftCardItem{
			ID:             card.ID,
			CodeMasked:     masked,
			Name:           card.Name,
			FaceValueCents: card.FaceValueCents,
			UsedByUsername: userMap[card.UsedBy],
			UsedAt:         usedTime,
		})
	}

	// 8. 最近套餐订单流水 Top 5
	var recentOrders []models.Order
	d.DB.Order("id DESC").Limit(5).Find(&recentOrders)
	for _, ord := range recentOrders {
		pName := planMap[ord.PlanID]
		if pName == "" {
			pName = fmt.Sprintf("套餐 #%d", ord.PlanID)
		}
		data.RecentOrders = append(data.RecentOrders, RecentOrderItem{
			ID:            ord.ID,
			OrderNo:       ord.OrderNo,
			Username:      userMap[ord.UserID],
			PlanName:      pName,
			AmountCents:   ord.AmountCents,
			PaymentMethod: ord.PaymentMethod,
			Status:        ord.Status,
			CreatedAt:     ord.CreatedAt,
			PaidAt:        ord.PaidAt,
		})
	}

	util.OK(c, data)
}

// AdminDashboardRealtime GET /api/v1/admin/dashboard/realtime —— 高频轻量实时网速接口（纯内存组装，耗时 < 0.1ms）。
func (d *Deps) AdminDashboardRealtime(c *gin.Context) {
	var servers []models.Server
	if err := d.DB.Select("id, name, host, node_id, location, status, last_seen_at").Order("id ASC").Find(&servers).Error; err != nil {
		util.ServerError(c, "查询服务器失败")
		return
	}

	var liveMetrics map[uint64]*nodegate.NodeMetricsSnapshot
	if d.Hub != nil {
		liveMetrics = d.Hub.GetAllLatestMetrics()
	}

	var data DashboardRealtimeData
	data.TotalServers = int64(len(servers))

	for _, s := range servers {
		isOnline := (s.Status == 1)
		if d.Hub != nil {
			isOnline = d.Hub.IsOnline(s.ID)
		}
		if isOnline {
			data.OnlineServers++
		}

		var cpu, mem, disk, rxRate, txRate float64
		var memTotal, diskTotal uint64
		var onlineUsers int
		lastSeen := s.LastSeenAt

		if m, ok := liveMetrics[s.ID]; ok && m != nil {
			cpu = m.CPU
			mem = m.Mem
			memTotal = m.MemTotal
			disk = m.Disk
			diskTotal = m.DiskTotal
			if isOnline {
				rxRate = m.RxRate
				txRate = m.TxRate
				onlineUsers = m.OnlineUsers
				t := m.ReportedAt
				lastSeen = &t
			}
		}

		isActiveFlow := (rxRate > 1024 || txRate > 1024)
		if isOnline {
			data.RealtimeRxRate += rxRate
			data.RealtimeTxRate += txRate
		}

		statusVal := 0
		if isOnline {
			statusVal = 1
		}

		data.ServerMatrix = append(data.ServerMatrix, ServerMatrixItem{
			ID:           s.ID,
			Name:         s.Name,
			NodeID:       s.NodeID,
			Host:         s.Host,
			Location:     s.Location,
			Status:       statusVal,
			LastSeenAt:   lastSeen,
			CPU:          cpu,
			Mem:          mem,
			MemTotal:     memTotal,
			Disk:         disk,
			DiskTotal:    diskTotal,
			RxRate:       rxRate,
			TxRate:       txRate,
			OnlineUsers:  onlineUsers,
			IsActiveFlow: isActiveFlow,
		})
	}

	util.OK(c, data)
}
