package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

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

func toChannelView(ch *models.ProxyChannel, srv *models.Server) channelView {
	srvName := ""
	srvHost := ""
	if srv != nil {
		srvName = srv.Name
		srvHost = srv.Host
	}

	proxyURL := ""
	hostOrIP := srvHost
	if hostOrIP == "" {
		hostOrIP = "127.0.0.1"
	}
	switch ch.Protocol {
	case models.ChannelTypeSocks5:
		if ch.Username != "" {
			proxyURL = fmt.Sprintf("socks5://%s:%s@%s:%d", ch.Username, ch.Password, hostOrIP, ch.Port)
		} else {
			proxyURL = fmt.Sprintf("socks5://%s:%d", hostOrIP, ch.Port)
		}
	case models.ChannelTypeHTTP:
		if ch.Username != "" {
			proxyURL = fmt.Sprintf("http://%s:%s@%s:%d", ch.Username, ch.Password, hostOrIP, ch.Port)
		} else {
			proxyURL = fmt.Sprintf("http://%s:%d", hostOrIP, ch.Port)
		}
	case models.ChannelTypeTunnel:
		if ch.TargetAddress != "" && ch.TargetPort > 0 {
			proxyURL = fmt.Sprintf("tcp://%s:%d -> %s:%d", hostOrIP, ch.Port, ch.TargetAddress, ch.TargetPort)
		}
	}

	return channelView{
		ID:               ch.ID,
		Name:             ch.Name,
		ServerID:         ch.ServerID,
		ServerName:       srvName,
		ServerHost:       srvHost,
		InboundID:        ch.InboundID,
		Port:             ch.Port,
		Listen:           ch.Listen,
		Protocol:         ch.Protocol,
		TargetAddress:    ch.TargetAddress,
		TargetPort:       ch.TargetPort,
		ProxyProtocol:    ch.ProxyProtocol,
		Username:         ch.Username,
		Password:         ch.Password,
		AllowUDP:         ch.AllowUDP,
		TrafficLimitGB:   ch.TrafficLimitGB,
		TrafficUsedBytes: ch.TrafficUsedBytes,
		UpBytes:          ch.UpBytes,
		DownBytes:        ch.DownBytes,
		TrafficReset:     ch.TrafficReset,
		ExpiresAt:        ch.ExpiresAt,
		AutoDisable:      ch.AutoDisable,
		Status:           ch.Status,
		Enabled:          ch.Enabled,
		CreatedAt:        ch.CreatedAt,
		UpdatedAt:        ch.UpdatedAt,
		ProxyURL:         proxyURL,
	}
}

// AdminChannels GET /api/v1/admin/channels
func (d *Deps) AdminChannels(c *gin.Context) {
	q := d.DB.Model(&models.ProxyChannel{})
	if sid := c.Query("server_id"); sid != "" {
		if id, err := strconv.ParseUint(sid, 10, 64); err == nil {
			q = q.Where("server_id = ?", id)
		}
	}
	if proto := c.Query("protocol"); proto != "" {
		q = q.Where("protocol = ?", proto)
	}
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}

	var channels []models.ProxyChannel
	if err := q.Order("id DESC").Find(&channels).Error; err != nil {
		util.ServerError(c, "查询通道失败")
		return
	}

	// 批量拉取服务器信息
	serverIDs := make([]uint64, 0, len(channels))
	for _, ch := range channels {
		serverIDs = append(serverIDs, ch.ServerID)
	}
	serverMap := make(map[uint64]*models.Server)
	if len(serverIDs) > 0 {
		var servers []models.Server
		_ = d.DB.Where("id IN ?", serverIDs).Find(&servers).Error
		for i := range servers {
			serverMap[servers[i].ID] = &servers[i]
		}
	}

	views := make([]channelView, 0, len(channels))
	for i := range channels {
		views = append(views, toChannelView(&channels[i], serverMap[channels[i].ServerID]))
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

// AdminCreateChannel POST /api/v1/admin/channels
func (d *Deps) AdminCreateChannel(c *gin.Context) {
	var req channelForm
	if !util.BindJSON(c, &req) {
		return
	}

	// 协议校验
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

	// 服务器存在性校验
	var srv models.Server
	if err := d.DB.First(&srv, req.ServerID).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}

	// 端口占用冲突校验（与全站入站共用端口池）
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
	autoDisable := true
	if req.AutoDisable != nil {
		autoDisable = *req.AutoDisable
	}

	// 构建底层 Inbound 与 ProxyChannel
	inbProto := ""
	settingsJSON := "{}"
	streamSettings := "{}"

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
		Type:           models.InboundTypeChannel,
		SettingsJSON:   settingsJSON,
		StreamSettings: streamSettings,
		TargetAddress:  req.TargetAddress,
		TargetPort:     req.TargetPort,
		Total:          req.TrafficLimitGB,
		ExpiryTime:     req.ExpiresAt,
		TrafficReset:   resetPolicy,
		Enabled:        true,
	}

	ch := models.ProxyChannel{
		Name:             req.Name,
		ServerID:         req.ServerID,
		Port:             req.Port,
		Listen:           req.Listen,
		Protocol:         req.Protocol,
		TargetAddress:    req.TargetAddress,
		TargetPort:       req.TargetPort,
		ProxyProtocol:    req.ProxyProtocol,
		Username:         req.Username,
		Password:         req.Password,
		AllowUDP:         allowUDP,
		TrafficLimitGB:   req.TrafficLimitGB,
		TrafficReset:     resetPolicy,
		ExpiresAt:        req.ExpiresAt,
		AutoDisable:      autoDisable,
		Status:           models.ChannelStatusActive,
		Enabled:          true,
	}

	err := d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&inb).Error; err != nil {
			return err
		}
		ch.InboundID = inb.ID
		if err := tx.Create(&ch).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		util.ServerError(c, "创建独立通道失败: "+err.Error())
		return
	}

	_ = d.enqueueConfig(req.ServerID)
	util.OK(c, gin.H{"channel": toChannelView(&ch, &srv)})
}

// AdminUpdateChannel PUT /api/v1/admin/channels/:id
func (d *Deps) AdminUpdateChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 ID")
		return
	}

	var ch models.ProxyChannel
	if err := d.DB.First(&ch, id).Error; err != nil {
		util.Fail(c, 404, "通道不存在")
		return
	}

	var inb models.Inbound
	if err := d.DB.First(&inb, ch.InboundID).Error; err != nil {
		util.Fail(c, 404, "关联入站不存在")
		return
	}

	var req channelForm
	if !util.BindJSON(c, &req) {
		return
	}

	// 端口若被更改，校验占用
	if req.Port != ch.Port {
		var cnt int64
		d.DB.Model(&models.Inbound{}).Where("server_id = ? AND port = ? AND id != ?", ch.ServerID, req.Port, inb.ID).Count(&cnt)
		if cnt > 0 {
			util.BadRequest(c, "该服务器上监听端口已被占用")
			return
		}
		ch.Port = req.Port
		inb.Port = req.Port
		inb.Tag = fmt.Sprintf("chan-%s-%d", ch.Protocol, req.Port)
	}

	ch.Name = req.Name
	ch.Listen = req.Listen
	inb.Listen = req.Listen

	if ch.Protocol == models.ChannelTypeTunnel {
		if strings.TrimSpace(req.TargetAddress) == "" || req.TargetPort <= 0 || req.TargetPort > 65535 {
			util.BadRequest(c, "四层直通必须填写有效的目标地址与目标端口")
			return
		}
		ch.TargetAddress = req.TargetAddress
		ch.TargetPort = req.TargetPort
		ch.ProxyProtocol = req.ProxyProtocol
		inb.TargetAddress = req.TargetAddress
		inb.TargetPort = req.TargetPort
		inb.StreamSettings = fmt.Sprintf(`{"proxy_protocol":%t}`, req.ProxyProtocol)
	} else if ch.Protocol == models.ChannelTypeSocks5 {
		ch.Username = req.Username
		ch.Password = req.Password
		if req.AllowUDP != nil {
			ch.AllowUDP = *req.AllowUDP
		}
		auth := "noauth"
		accounts := []map[string]string{}
		if strings.TrimSpace(ch.Username) != "" {
			auth = "password"
			accounts = append(accounts, map[string]string{
				"user": ch.Username,
				"pass": ch.Password,
			})
		}
		raw, _ := json.Marshal(map[string]any{
			"auth":     auth,
			"accounts": accounts,
			"udp":      ch.AllowUDP,
		})
		inb.SettingsJSON = string(raw)
	} else if ch.Protocol == models.ChannelTypeHTTP {
		ch.Username = req.Username
		ch.Password = req.Password
		accounts := []map[string]string{}
		if strings.TrimSpace(ch.Username) != "" {
			accounts = append(accounts, map[string]string{
				"user": ch.Username,
				"pass": ch.Password,
			})
		}
		raw, _ := json.Marshal(map[string]any{
			"accounts": accounts,
		})
		inb.SettingsJSON = string(raw)
	}

	ch.TrafficLimitGB = req.TrafficLimitGB
	inb.Total = req.TrafficLimitGB
	if req.TrafficReset != "" {
		ch.TrafficReset = req.TrafficReset
		inb.TrafficReset = req.TrafficReset
	}
	ch.ExpiresAt = req.ExpiresAt
	inb.ExpiryTime = req.ExpiresAt
	if req.AutoDisable != nil {
		ch.AutoDisable = *req.AutoDisable
	}

	// 若之前因超额关停，而新配额提升大于已用，自动恢复为 active
	limitBytes := ch.TrafficLimitGB * 1024 * 1024 * 1024
	if ch.Status == models.ChannelStatusExceeded && (ch.TrafficLimitGB == 0 || ch.TrafficUsedBytes < limitBytes) {
		ch.Status = models.ChannelStatusActive
		if ch.Enabled {
			inb.Enabled = true
		}
	}

	err = d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&inb).Error; err != nil {
			return err
		}
		if err := tx.Save(&ch).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		util.ServerError(c, "更新通道失败")
		return
	}

	_ = d.enqueueConfig(ch.ServerID)
	var srv models.Server
	_ = d.DB.First(&srv, ch.ServerID).Error
	util.OK(c, gin.H{"channel": toChannelView(&ch, &srv)})
}

// AdminDeleteChannel DELETE /api/v1/admin/channels/:id
func (d *Deps) AdminDeleteChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 ID")
		return
	}

	var ch models.ProxyChannel
	if err := d.DB.First(&ch, id).Error; err != nil {
		util.Fail(c, 404, "通道不存在")
		return
	}

	serverID := ch.ServerID
	inboundID := ch.InboundID

	err = d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&models.ProxyChannel{}, id).Error; err != nil {
			return err
		}
		if inboundID > 0 {
			if err := tx.Delete(&models.Inbound{}, inboundID).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		util.ServerError(c, "删除通道失败")
		return
	}

	_ = d.enqueueConfig(serverID)
	util.OK(c, gin.H{"id": id})
}

// AdminToggleChannel POST /api/v1/admin/channels/:id/toggle
func (d *Deps) AdminToggleChannel(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 ID")
		return
	}

	var ch models.ProxyChannel
	if err := d.DB.First(&ch, id).Error; err != nil {
		util.Fail(c, 404, "通道不存在")
		return
	}

	ch.Enabled = !ch.Enabled
	now := time.Now()
	limitBytes := ch.TrafficLimitGB * 1024 * 1024 * 1024

	inboundEnabled := ch.Enabled
	if !ch.Enabled {
		ch.Status = models.ChannelStatusDisabled
		inboundEnabled = false
	} else {
		if ch.TrafficLimitGB > 0 && ch.TrafficUsedBytes >= limitBytes {
			ch.Status = models.ChannelStatusExceeded
			if ch.AutoDisable {
				inboundEnabled = false
			}
		} else if ch.ExpiresAt != nil && now.After(*ch.ExpiresAt) {
			ch.Status = models.ChannelStatusExpired
			if ch.AutoDisable {
				inboundEnabled = false
			}
		} else {
			ch.Status = models.ChannelStatusActive
			inboundEnabled = true
		}
	}

	err = d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.ProxyChannel{}).Where("id = ?", ch.ID).Updates(map[string]any{
			"enabled": ch.Enabled,
			"status":  ch.Status,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Inbound{}).Where("id = ?", ch.InboundID).Update("enabled", inboundEnabled).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		util.ServerError(c, "切换状态失败")
		return
	}

	_ = d.enqueueConfig(ch.ServerID)
	util.OK(c, gin.H{"id": ch.ID, "enabled": ch.Enabled, "status": ch.Status})
}

// AdminResetChannelTraffic POST /api/v1/admin/channels/:id/reset-traffic
func (d *Deps) AdminResetChannelTraffic(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 ID")
		return
	}

	var ch models.ProxyChannel
	if err := d.DB.First(&ch, id).Error; err != nil {
		util.Fail(c, 404, "通道不存在")
		return
	}

	updates := map[string]any{
		"traffic_used_bytes": 0,
		"up_bytes":          0,
		"down_bytes":        0,
	}
	shouldReopen := false
	if ch.Status == models.ChannelStatusExceeded {
		updates["status"] = models.ChannelStatusActive
		ch.Status = models.ChannelStatusActive
		if ch.Enabled {
			shouldReopen = true
		}
	}

	err = d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.ProxyChannel{}).Where("id = ?", ch.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Inbound{}).Where("id = ?", ch.InboundID).Updates(map[string]any{
			"up":   0,
			"down": 0,
		}).Error; err != nil {
			return err
		}
		if shouldReopen {
			if err := tx.Model(&models.Inbound{}).Where("id = ?", ch.InboundID).Update("enabled", true).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		util.ServerError(c, "重置用量失败")
		return
	}

	if shouldReopen {
		_ = d.enqueueConfig(ch.ServerID)
	}

	util.OK(c, gin.H{"id": ch.ID, "traffic_used_bytes": 0, "status": ch.Status})
}
