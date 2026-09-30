package api

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

type channelView struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	ServerID  uint64 `json:"server_id"`
	ServerName string `json:"server_name"`
	ServerHost string `json:"server_host"`
	InboundID uint64 `json:"inbound_id"`
	Port      int    `json:"port"`
	Listen    string `json:"listen"`
	Protocol  string `json:"protocol"`

	// 业务参数
	TargetAddress string `json:"target_address,omitempty"`
	TargetPort    int    `json:"target_port,omitempty"`
	ProxyProtocol bool   `json:"proxy_protocol"`
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	AllowUDP      bool   `json:"allow_udp"`

	// 计费与状态
	TrafficLimitGB   int64      `json:"traffic_limit_gb"`
	TrafficUsedBytes int64      `json:"traffic_used_bytes"`
	UpBytes          int64      `json:"up_bytes"`
	DownBytes        int64      `json:"down_bytes"`
	TrafficReset     string     `json:"traffic_reset"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	AutoDisable      bool       `json:"auto_disable"`
	Status           string     `json:"status"`
	Enabled          bool       `json:"enabled"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`

	// 快捷连接信息
	ProxyURL string `json:"proxy_url,omitempty"`
}

func parseInboundCredentials(inb *models.Inbound) (username, password string, allowUDP bool, proxyProto bool) {
	allowUDP = true
	if inb.StreamSettings != "" {
		var ss map[string]any
		if err := json.Unmarshal([]byte(inb.StreamSettings), &ss); err == nil {
			if pp, ok := ss["proxy_protocol"].(bool); ok {
				proxyProto = pp
			} else if pp, ok := ss["proxyProtocol"].(bool); ok {
				proxyProto = pp
			}
		}
	}
	if inb.SettingsJSON != "" {
		var set map[string]any
		if err := json.Unmarshal([]byte(inb.SettingsJSON), &set); err == nil {
			if u, ok := set["udp"].(bool); ok {
				allowUDP = u
			}
			if accs, ok := set["accounts"].([]any); ok && len(accs) > 0 {
				if acc, ok := accs[0].(map[string]any); ok {
					if u, ok := acc["user"].(string); ok {
						username = u
					}
					if p, ok := acc["pass"].(string); ok {
						password = p
					}
				}
			}
		}
	}
	return
}

func toChannelViewFromInbound(inb *models.Inbound, srv *models.Server) channelView {
	srvName := ""
	srvHost := ""
	if srv != nil {
		srvName = srv.Name
		srvHost = srv.Host
	}

	proto := inb.Protocol
	switch inb.Protocol {
	case models.ProtocolDokodemo:
		proto = models.ChannelTypeTunnel
	case models.ProtocolSocks:
		proto = models.ChannelTypeSocks5
	case models.ProtocolHTTP:
		proto = models.ChannelTypeHTTP
	}

	username, password, allowUDP, proxyProto := parseInboundCredentials(inb)

	hostOrIP := srvHost
	if hostOrIP == "" {
		hostOrIP = "127.0.0.1"
	}
	proxyURL := ""
	switch proto {
	case models.ChannelTypeSocks5:
		if username != "" {
			proxyURL = fmt.Sprintf("socks5://%s:%s@%s:%d", username, password, hostOrIP, inb.Port)
		} else {
			proxyURL = fmt.Sprintf("socks5://%s:%d", hostOrIP, inb.Port)
		}
	case models.ChannelTypeHTTP:
		if username != "" {
			proxyURL = fmt.Sprintf("http://%s:%s@%s:%d", username, password, hostOrIP, inb.Port)
		} else {
			proxyURL = fmt.Sprintf("http://%s:%d", hostOrIP, inb.Port)
		}
	case models.ChannelTypeTunnel:
		if inb.TargetAddress != "" && inb.TargetPort > 0 {
			proxyURL = fmt.Sprintf("tcp://%s:%d -> %s:%d", hostOrIP, inb.Port, inb.TargetAddress, inb.TargetPort)
		}
	}

	now := time.Now()
	status := models.ChannelStatusActive
	if !inb.Enabled {
		status = models.ChannelStatusDisabled
	} else if inb.Total > 0 && (inb.Up+inb.Down) >= inb.Total {
		status = models.ChannelStatusExceeded
	} else if inb.ExpiryTime != nil && now.After(*inb.ExpiryTime) {
		status = models.ChannelStatusExpired
	}

	name := inb.Remark
	if name == "" {
		name = inb.Tag
	}

	limitGB := inb.Total / (1024 * 1024 * 1024)

	return channelView{
		ID:               inb.ID,
		Name:             name,
		ServerID:         inb.ServerID,
		ServerName:       srvName,
		ServerHost:       srvHost,
		InboundID:        inb.ID,
		Port:             inb.Port,
		Listen:           inb.Listen,
		Protocol:         proto,
		TargetAddress:    inb.TargetAddress,
		TargetPort:       inb.TargetPort,
		ProxyProtocol:    proxyProto,
		Username:         username,
		Password:         password,
		AllowUDP:         allowUDP,
		TrafficLimitGB:   limitGB,
		TrafficUsedBytes: inb.Up + inb.Down,
		UpBytes:          inb.Up,
		DownBytes:        inb.Down,
		TrafficReset:     inb.TrafficReset,
		ExpiresAt:        inb.ExpiryTime,
		AutoDisable:      true,
		Status:           status,
		Enabled:          inb.Enabled,
		CreatedAt:        inb.CreatedAt,
		UpdatedAt:        inb.CreatedAt,
		ProxyURL:         proxyURL,
	}
}

// AdminChannels GET /api/v1/admin/channels
func (d *Deps) AdminChannels(c *gin.Context) {
	q := d.DB.Model(&models.Inbound{}).Where("type IN ?", []string{models.InboundTypeForward, models.InboundTypeChannel, models.InboundTypeTunnel})
	if sid := c.Query("server_id"); sid != "" {
		if id, err := strconv.ParseUint(sid, 10, 64); err == nil {
			q = q.Where("server_id = ?", id)
		}
	}
	if proto := c.Query("protocol"); proto != "" {
		switch proto {
		case models.ChannelTypeTunnel:
			q = q.Where("protocol = ?", models.ProtocolDokodemo)
		case models.ChannelTypeSocks5:
			q = q.Where("protocol = ?", models.ProtocolSocks)
		case models.ChannelTypeHTTP:
			q = q.Where("protocol = ?", models.ProtocolHTTP)
		default:
			q = q.Where("protocol = ?", proto)
		}
	}

	var inbounds []models.Inbound
	if err := q.Order("id DESC").Find(&inbounds).Error; err != nil {
		util.ServerError(c, "查询通道失败")
		return
	}

	// 批量拉取服务器信息
	serverIDs := make([]uint64, 0, len(inbounds))
	for _, inb := range inbounds {
		serverIDs = append(serverIDs, inb.ServerID)
	}
	serverMap := make(map[uint64]*models.Server)
	if len(serverIDs) > 0 {
		var servers []models.Server
		_ = d.DB.Where("id IN ?", serverIDs).Find(&servers).Error
		for i := range servers {
			serverMap[servers[i].ID] = &servers[i]
		}
	}

	wantStatus := c.Query("status")
	views := make([]channelView, 0, len(inbounds))
	for i := range inbounds {
		v := toChannelViewFromInbound(&inbounds[i], serverMap[inbounds[i].ServerID])
		if wantStatus != "" && v.Status != wantStatus {
			continue
		}
		views = append(views, v)
	}

	util.OK(c, gin.H{"channels": views})
}

type channelForm struct {
	Name             string     `json:"name" binding:"required,max=64"`
	ServerID         uint64     `json:"server_id" binding:"required"`
	Port             int        `json:"port" binding:"required,min=1,max=65535"`
	Listen           string     `json:"listen"`
	Protocol         string     `json:"protocol" binding:"required"` // tunnel / socks5 / http
	TargetAddress    string     `json:"target_address"`
	TargetPort       int        `json:"target_port"`
	ProxyProtocol    bool       `json:"proxy_protocol"`
	Username         string     `json:"username"`
	Password         string     `json:"password"`
	AllowUDP         *bool      `json:"allow_udp"`
	TrafficLimitGB   int64      `json:"traffic_limit_gb"`
	TrafficReset     string     `json:"traffic_reset"`
	ExpiresAt        *time.Time `json:"expires_at"`
	AutoDisable      *bool      `json:"auto_disable"`
}

// channelCredentialProblem 独立代理通道的认证判定（禁止免密开放，2026-09-28 拍板）：
// socks5 留空账号会生成 `auth:"noauth"`、http 留空会生成空 `accounts`，两者都是匿名开放代理，
// 返回非空字符串即拒绝；tunnel 无认证概念，不受约束。创建与更新共用本判定（唯一入口）。
func channelCredentialProblem(protocol, username, password string) string {
	if protocol != models.ChannelTypeSocks5 && protocol != models.ChannelTypeHTTP {
		return ""
	}
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return "SOCKS5 / HTTP 通道必须填写认证账号与密码（不允许免密无认证开放）"
	}
	return ""
}

// AdminCreateChannel POST /api/v1/admin/channels
func (d *Deps) AdminCreateChannel(c *gin.Context) {
	var req channelForm
	if !util.BindJSON(c, &req) {
		return
	}

	req.Protocol = strings.ToLower(strings.TrimSpace(req.Protocol))
	if req.Protocol != models.ChannelTypeTunnel && req.Protocol != models.ChannelTypeSocks5 && req.Protocol != models.ChannelTypeHTTP {
		util.BadRequest(c, "协议类型必须为 tunnel、socks5 或 http")
		return
	}
	if req.Protocol == models.ChannelTypeTunnel {
		if strings.TrimSpace(req.TargetAddress) == "" || req.TargetPort <= 0 || req.TargetPort > 65535 {
			util.BadRequest(c, "四层直通必须填写有效的目标地址与目标端口 (1-65535)")
			return
		}
	}
	if msg := channelCredentialProblem(req.Protocol, req.Username, req.Password); msg != "" {
		util.BadRequest(c, msg)
		return
	}

	var srv models.Server
	if err := d.DB.First(&srv, req.ServerID).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}

	var cnt int64
	d.DB.Model(&models.Inbound{}).Where("server_id = ? AND port = ?", req.ServerID, req.Port).Count(&cnt)
	if cnt > 0 {
		util.BadRequest(c, "该服务器上监听端口已被占用")
		return
	}

	resetPolicy := strings.ToLower(strings.TrimSpace(req.TrafficReset))
	if resetPolicy == "" {
		resetPolicy = "never"
	}
	allowUDP := true
	if req.AllowUDP != nil {
		allowUDP = *req.AllowUDP
	}

	inbProto := ""
	settingsJSON := "{}"
	streamSettings := "{}"
	inbType := models.InboundTypeForward

	switch req.Protocol {
	case models.ChannelTypeTunnel:
		inbProto = models.ProtocolDokodemo
		streamSettings = fmt.Sprintf(`{"proxy_protocol":%t}`, req.ProxyProtocol)
	case models.ChannelTypeSocks5:
		inbProto = models.ProtocolSocks
		auth := "noauth"
		accounts := []map[string]string{}
		if strings.TrimSpace(req.Username) != "" {
			auth = "password"
			accounts = append(accounts, map[string]string{
				"user": req.Username,
				"pass": req.Password,
			})
		}
		raw, _ := json.Marshal(map[string]any{
			"auth":     auth,
			"accounts": accounts,
			"udp":      allowUDP,
		})
		settingsJSON = string(raw)
	case models.ChannelTypeHTTP:
		inbProto = models.ProtocolHTTP
		accounts := []map[string]string{}
		if strings.TrimSpace(req.Username) != "" {
			accounts = append(accounts, map[string]string{
				"user": req.Username,
				"pass": req.Password,
			})
		}
		raw, _ := json.Marshal(map[string]any{
			"accounts": accounts,
		})
		settingsJSON = string(raw)
	}

	inb := models.Inbound{
		ServerID:       req.ServerID,
		Tag:            fmt.Sprintf("chan-%s-%d", req.Protocol, req.Port),
		Protocol:       inbProto,
		Port:           req.Port,
		Listen:         req.Listen,
		Type:           inbType,
		Remark:         req.Name,
		SettingsJSON:   settingsJSON,
		StreamSettings: streamSettings,
		TargetAddress:  req.TargetAddress,
		TargetPort:     req.TargetPort,
		Total:          req.TrafficLimitGB * 1024 * 1024 * 1024,
		ExpiryTime:     req.ExpiresAt,
		TrafficReset:   resetPolicy,
		Enabled:        true,
	}

	if err := d.DB.Create(&inb).Error; err != nil {
		log.Printf("[channels] 创建通道入站失败: %v", err)
		util.ServerError(c, "创建独立通道失败")
		return
	}

	if inbProto == models.ProtocolDokodemo {
		if msg := d.tunnelOutboundTagConflict(req.ServerID, inb.Tag, 0); msg != "" {
			_ = d.DB.Delete(&inb).Error
			util.BadRequest(c, msg)
			return
		}
		if msg := d.ensureTunnelDerived(req.ServerID); msg != "" {
			_ = d.DB.Delete(&inb).Error
			_ = services.EnsureTunnelDerivedObjects(d.DB, req.ServerID)
			util.ServerError(c, msg)
			return
		}
	}

	_ = d.enqueueConfig(req.ServerID)
	util.OK(c, gin.H{"channel": toChannelViewFromInbound(&inb, &srv)})
}

// AdminUpdateChannel PUT /api/v1/admin/channels/:id
func (d *Deps) AdminUpdateChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的通道 ID")
		return
	}

	var req channelForm
	if !util.BindJSON(c, &req) {
		return
	}

	req.Protocol = strings.ToLower(strings.TrimSpace(req.Protocol))
	if req.Protocol != models.ChannelTypeTunnel && req.Protocol != models.ChannelTypeSocks5 && req.Protocol != models.ChannelTypeHTTP {
		util.BadRequest(c, "协议类型必须为 tunnel、socks5 或 http")
		return
	}
	if req.Protocol == models.ChannelTypeTunnel {
		if strings.TrimSpace(req.TargetAddress) == "" || req.TargetPort <= 0 || req.TargetPort > 65535 {
			util.BadRequest(c, "四层直通必须填写有效的目标地址与目标端口 (1-65535)")
			return
		}
	}
	if msg := channelCredentialProblem(req.Protocol, req.Username, req.Password); msg != "" {
		util.BadRequest(c, msg)
		return
	}

	var inb models.Inbound
	if err := d.DB.Where("id = ? AND type IN ?", id, []string{models.InboundTypeForward, models.InboundTypeChannel, models.InboundTypeTunnel}).First(&inb).Error; err != nil {
		util.Fail(c, 404, "通道不存在")
		return
	}

	// 端口变更冲突检查
	if req.Port != inb.Port {
		var cnt int64
		d.DB.Model(&models.Inbound{}).Where("server_id = ? AND port = ? AND id != ?", inb.ServerID, req.Port, inb.ID).Count(&cnt)
		if cnt > 0 {
			util.BadRequest(c, "该服务器上监听端口已被占用")
			return
		}
	}

	resetPolicy := strings.ToLower(strings.TrimSpace(req.TrafficReset))
	if resetPolicy == "" {
		resetPolicy = "never"
	}
	allowUDP := true
	if req.AllowUDP != nil {
		allowUDP = *req.AllowUDP
	}

	inbProto := ""
	settingsJSON := "{}"
	streamSettings := inb.StreamSettings

	switch req.Protocol {
	case models.ChannelTypeTunnel:
		inbProto = models.ProtocolDokodemo
		streamSettings = fmt.Sprintf(`{"proxy_protocol":%t}`, req.ProxyProtocol)
	case models.ChannelTypeSocks5:
		inbProto = models.ProtocolSocks
		auth := "noauth"
		accounts := []map[string]string{}
		if strings.TrimSpace(req.Username) != "" {
			auth = "password"
			accounts = append(accounts, map[string]string{
				"user": req.Username,
				"pass": req.Password,
			})
		}
		raw, _ := json.Marshal(map[string]any{
			"auth":     auth,
			"accounts": accounts,
			"udp":      allowUDP,
		})
		settingsJSON = string(raw)
	case models.ChannelTypeHTTP:
		inbProto = models.ProtocolHTTP
		accounts := []map[string]string{}
		if strings.TrimSpace(req.Username) != "" {
			accounts = append(accounts, map[string]string{
				"user": req.Username,
				"pass": req.Password,
			})
		}
		raw, _ := json.Marshal(map[string]any{
			"accounts": accounts,
		})
		settingsJSON = string(raw)
	}

	inb.Port = req.Port
	inb.Listen = req.Listen
	inb.Protocol = inbProto
	inb.Remark = req.Name
	inb.SettingsJSON = settingsJSON
	inb.StreamSettings = streamSettings
	inb.TargetAddress = req.TargetAddress
	inb.TargetPort = req.TargetPort
	inb.Total = req.TrafficLimitGB * 1024 * 1024 * 1024
	inb.ExpiryTime = req.ExpiresAt
	inb.TrafficReset = resetPolicy

	if err := d.DB.Save(&inb).Error; err != nil {
		util.ServerError(c, "更新通道失败")
		return
	}

	if inbProto == models.ProtocolDokodemo {
		_ = d.ensureTunnelDerived(inb.ServerID)
	}

	var srv models.Server
	_ = d.DB.First(&srv, inb.ServerID).Error

	_ = d.enqueueConfig(inb.ServerID)
	util.OK(c, gin.H{"channel": toChannelViewFromInbound(&inb, &srv)})
}

// AdminDeleteChannel DELETE /api/v1/admin/channels/:id
func (d *Deps) AdminDeleteChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的通道 ID")
		return
	}

	var inb models.Inbound
	if err := d.DB.Where("id = ? AND type IN ?", id, []string{models.InboundTypeForward, models.InboundTypeChannel, models.InboundTypeTunnel}).First(&inb).Error; err != nil {
		util.Fail(c, 404, "通道不存在")
		return
	}

	serverID := inb.ServerID
	if err := d.DB.Delete(&inb).Error; err != nil {
		util.ServerError(c, "删除通道失败")
		return
	}

	// 清理派生对象
	_ = services.EnsureTunnelDerivedObjects(d.DB, serverID)

	_ = d.enqueueConfig(serverID)
	util.OK(c, gin.H{"deleted": true})
}

// AdminToggleChannel POST /api/v1/admin/channels/:id/toggle
func (d *Deps) AdminToggleChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的通道 ID")
		return
	}

	var inb models.Inbound
	if err := d.DB.Where("id = ? AND type IN ?", id, []string{models.InboundTypeForward, models.InboundTypeChannel, models.InboundTypeTunnel}).First(&inb).Error; err != nil {
		util.Fail(c, 404, "通道不存在")
		return
	}

	newEnabled := !inb.Enabled
	inb.Enabled = newEnabled
	if err := d.DB.Model(&inb).Update("enabled", newEnabled).Error; err != nil {
		util.ServerError(c, "更新状态失败")
		return
	}

	// 派生对象同步
	_ = services.EnsureTunnelDerivedObjects(d.DB, inb.ServerID)

	_ = d.enqueueConfig(inb.ServerID)

	var srv models.Server
	_ = d.DB.First(&srv, inb.ServerID).Error
	view := toChannelViewFromInbound(&inb, &srv)

	util.OK(c, gin.H{
		"enabled": newEnabled,
		"status":  view.Status,
	})
}

// AdminResetChannelTraffic POST /api/v1/admin/channels/:id/reset-traffic
func (d *Deps) AdminResetChannelTraffic(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的通道 ID")
		return
	}

	var inb models.Inbound
	if err := d.DB.Where("id = ? AND type IN ?", id, []string{models.InboundTypeForward, models.InboundTypeChannel, models.InboundTypeTunnel}).First(&inb).Error; err != nil {
		util.Fail(c, 404, "通道不存在")
		return
	}

	loc := services.BusinessLocation(d.DB)
	key := services.ResetPeriodKey(time.Now(), inb.TrafficReset, loc)

	updates := map[string]any{
		"up":              0,
		"down":            0,
		"last_reset_date": key,
	}
	// 如果超额停用，清零后自动恢复启用
	if inb.Total > 0 && !inb.Enabled && (inb.Up+inb.Down >= inb.Total) {
		updates["enabled"] = true
	}

	if err := d.DB.Model(&inb).Updates(updates).Error; err != nil {
		util.ServerError(c, "重置流量失败")
		return
	}

	_ = d.enqueueConfig(inb.ServerID)
	util.OK(c, gin.H{"reset": true})
}
