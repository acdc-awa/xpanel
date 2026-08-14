package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
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
		PendingOrders       int64   `json:"pending_orders"`
		TotalOrders         int64   `json:"total_orders"`
		TodayOrders         int64   `json:"today_orders"`
		RealtimeRxRate      float64 `json:"realtime_rx_rate"`
		RealtimeTxRate      float64 `json:"realtime_tx_rate"`
	} `json:"summary"`

	TrafficTrend []TrafficTrendPoint `json:"traffic_trend"`

	ServerBreakdown []ServerTrafficItem `json:"server_breakdown"`

	UserRank []UserTrafficRankItem `json:"user_rank"`

	ServerMatrix []ServerMatrixItem `json:"server_matrix"`

	RecentGiftCards []RecentGiftCardItem `json:"recent_gift_cards"`

	RecentOrders []RecentOrderItem `json:"recent_orders"`
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
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	todayStr := now.Format("2006-01-02")
	monthPrefix := now.Format("2006-01")

	var data DashboardData

	// 1. 财务数据（Mini Financial System）
	// 今日卡密激活金额与张数
	var todayRev struct {
		Total int64
		Count int64
	}
	d.DB.Model(&models.GiftCard{}).
		Select("COALESCE(SUM(face_value_cents), 0) as total, COUNT(id) as count").
		Where("status = ? AND used_at >= ?", models.GiftCardUsed, todayStart).
		Scan(&todayRev)
	data.Summary.TodayRevenueCents = todayRev.Total
	data.Summary.TodayUsedCardsCount = todayRev.Count

	// 本月卡密激活金额
	var monthRev int64
	d.DB.Model(&models.GiftCard{}).
		Select("COALESCE(SUM(face_value_cents), 0)").
		Where("status = ? AND used_at >= ?", models.GiftCardUsed, monthStart).
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

	d.DB.Model(&models.Order{}).Where("status = 'pending'").Count(&data.Summary.PendingOrders)
	d.DB.Model(&models.Order{}).Count(&data.Summary.TotalOrders)
	d.DB.Model(&models.Order{}).Where("created_at >= ?", todayStart).Count(&data.Summary.TodayOrders)

	// 3. 流量汇总与趋势（近 30 天）
	trendDays := 30
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
		if strings.HasPrefix(td.Date, monthPrefix) {
			data.Summary.MonthTrafficTotal += (td.UpBytes + td.DownBytes)
		}
	}

	// 叠加今日未落 Daily 的最新 TrafficLog
	var todayLogs []models.TrafficLog
	d.DB.Where("period_start >= ?", todayStart).Find(&todayLogs)
	for _, log := range todayLogs {
		if pt, ok := dailyMap[todayStr]; ok {
			pt.UpBytes += log.UpBytes
			pt.DownBytes += log.DownBytes
			pt.TotalBytes += (log.UpBytes + log.DownBytes)
		}
		data.Summary.TodayTrafficUp += log.UpBytes
		data.Summary.TodayTrafficDown += log.DownBytes
		data.Summary.MonthTrafficTotal += (log.UpBytes + log.DownBytes)
	}
	data.Summary.TodayTrafficTotal = data.Summary.TodayTrafficUp + data.Summary.TodayTrafficDown

	data.TrafficTrend = make([]TrafficTrendPoint, 0, trendDays)
	for i := 0; i < trendDays; i++ {
		dStr := startTime.AddDate(0, 0, i).Format("2006-01-02")
		if pt, ok := dailyMap[dStr]; ok {
			data.TrafficTrend = append(data.TrafficTrend, *pt)
		}
	}

	// 4. 服务器健康度矩阵 & 实时速率
	var servers []models.Server
	d.DB.Find(&servers)

	for _, s := range servers {
		var latestReport models.NodeReport
		d.DB.Where("server_id = ?", s.ID).Order("reported_at DESC").First(&latestReport)

		isActiveFlow := (latestReport.RxRate > 1024 || latestReport.TxRate > 1024)
		if s.Status == 1 {
			data.Summary.RealtimeRxRate += latestReport.RxRate
			data.Summary.RealtimeTxRate += latestReport.TxRate
		}

		data.ServerMatrix = append(data.ServerMatrix, ServerMatrixItem{
			ID:           s.ID,
			Name:         s.Name,
			NodeID:       s.NodeID,
			Host:         s.Host,
			Location:     s.Location,
			Status:       s.Status,
			LastSeenAt:   s.LastSeenAt,
			CPU:          latestReport.CPU,
			Mem:          latestReport.Mem,
			MemTotal:     latestReport.MemTotal,
			Disk:         latestReport.Disk,
			DiskTotal:    latestReport.DiskTotal,
			RxRate:       latestReport.RxRate,
			TxRate:       latestReport.TxRate,
			OnlineUsers:  latestReport.OnlineUsers,
			IsActiveFlow: isActiveFlow,
		})
	}

	// 5. 节点流量占比分布（按入站累加）
	var inbounds []models.Inbound
	d.DB.Find(&inbounds)
	serverTrafficMap := make(map[uint64]*ServerTrafficItem)
	var grandTotalBytes int64

	for _, s := range servers {
		serverTrafficMap[s.ID] = &ServerTrafficItem{
			ServerID: s.ID,
			Name:     s.Name,
			Location: s.Location,
		}
	}
	for _, inb := range inbounds {
		if item, ok := serverTrafficMap[inb.ServerID]; ok {
			item.UpBytes += inb.Up
			item.DownBytes += inb.Down
			item.TotalBytes += (inb.Up + inb.Down)
			grandTotalBytes += (inb.Up + inb.Down)
		}
	}
	for _, item := range serverTrafficMap {
		if grandTotalBytes > 0 {
			item.Percent = float64(item.TotalBytes) / float64(grandTotalBytes) * 100
		}
		data.ServerBreakdown = append(data.ServerBreakdown, *item)
	}

	// 6. 用户流量消耗排行榜 Top 10
	var users []models.User
	d.DB.Find(&users)
	var plans []models.Plan
	d.DB.Find(&plans)
	planMap := make(map[uint64]string, len(plans))
	for _, p := range plans {
		planMap[p.ID] = p.Name
	}

	userRankList := make([]UserTrafficRankItem, 0, len(users))
	for _, u := range users {
		up, down, _ := d.Traffic.UserUsed(u.ID)
		tot := up + down
		pName := planMap[u.PlanID]
		if pName == "" {
			pName = "无套餐"
		}
		userRankList = append(userRankList, UserTrafficRankItem{
			UserID:     u.ID,
			Username:   u.Username,
			Email:      u.Email,
			PlanName:   pName,
			UpBytes:    up,
			DownBytes:  down,
			TotalBytes: tot,
		})
	}
	// 排序取前 10
	for i := 0; i < len(userRankList); i++ {
		for j := i + 1; j < len(userRankList); j++ {
			if userRankList[j].TotalBytes > userRankList[i].TotalBytes {
				userRankList[i], userRankList[j] = userRankList[j], userRankList[i]
			}
		}
	}
	if len(userRankList) > 10 {
		data.UserRank = userRankList[:10]
	} else {
		data.UserRank = userRankList
	}

	// 7. 最近卡密激活流水 Top 5
	var usedCards []models.GiftCard
	d.DB.Where("status = ?", models.GiftCardUsed).Order("used_at DESC").Limit(5).Find(&usedCards)
	userMap := make(map[uint64]string)
	for _, u := range users {
		userMap[u.ID] = u.Username
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
			pName = fmt.Sprintf("套餐#%d", ord.PlanID)
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
