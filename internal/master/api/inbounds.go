package api

import (
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/xray"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// inboundView 入站对外结构。
type inboundView struct {
	ID             uint64    `json:"id"`
	ServerID       uint64    `json:"server_id"`
	ServerName     string    `json:"server_name"`
	Tag            string    `json:"tag"`
	Protocol       string    `json:"protocol"`
	Port           int       `json:"port"`
	Listen         string    `json:"listen"`
	SettingsJSON   string    `json:"settings_json"`
	StreamSettings string    `json:"stream_settings"`
	Sniffing       string    `json:"sniffing"`
	Ratio          float64   `json:"ratio"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
}

// inboundForm 入站创建/更新表单（透传 JSON）。
type inboundForm struct {
	ServerID       uint64  `json:"server_id" binding:"required"`
	Tag            string  `json:"tag" binding:"required,max=64"`
	Protocol       string  `json:"protocol" binding:"required"`
	Port           int     `json:"port" binding:"required,min=1,max=65535"`
	Listen         string  `json:"listen"`
	SettingsJSON   string  `json:"settings_json"`   // 协议 settings（透传）
	StreamSettings string  `json:"stream_settings"` // 传输 streamSettings（透传）
	Sniffing       string  `json:"sniffing"`        // 嗅探（透传）
	Ratio          float64 `json:"ratio"`
}

func toInboundView(i *models.Inbound, serverName string) inboundView {
	return inboundView{
		ID: i.ID, ServerID: i.ServerID, ServerName: serverName,
		Tag: i.Tag, Protocol: i.Protocol, Port: i.Port,
		Listen: i.Listen, SettingsJSON: i.SettingsJSON,
		StreamSettings: i.StreamSettings, Sniffing: i.Sniffing,
		Ratio: i.Ratio, Enabled: i.Enabled, CreatedAt: i.CreatedAt,
	}
}

// AdminInbounds GET /api/v1/admin/inbounds?server_id=
func (d *Deps) AdminInbounds(c *gin.Context) {
	q := d.DB.Model(&models.Inbound{})
	if sid := c.Query("server_id"); sid != "" {
		if id, err := strconv.ParseUint(sid, 10, 64); err == nil {
			q = q.Where("server_id = ?", id)
		}
	}
	var list []models.Inbound
	if err := q.Order("id DESC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	items := make([]inboundView, 0, len(list))
	for i := range list {
		serverName := ""
		var srv models.Server
		if err := d.DB.First(&srv, list[i].ServerID).Error; err == nil {
			serverName = srv.Name
		}
		items = append(items, toInboundView(&list[i], serverName))
	}
	util.OK(c, gin.H{"items": items})
}

// AdminCreateInbound POST /api/v1/admin/inbounds
func (d *Deps) AdminCreateInbound(c *gin.Context) {
	var req inboundForm
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	var srv models.Server
	if err := d.DB.First(&srv, req.ServerID).Error; err != nil {
		util.BadRequest(c, "服务器不存在")
		return
	}
	if err := xray.ValidateInbound(req.SettingsJSON, req.StreamSettings, req.Sniffing); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	// 同服务器端口冲突
	var cnt int64
	d.DB.Model(&models.Inbound{}).Where("server_id = ? AND port = ?", req.ServerID, req.Port).Count(&cnt)
	if cnt > 0 {
		util.BadRequest(c, "该服务器上端口已被占用")
		return
	}
	inb := models.Inbound{
		ServerID: req.ServerID, Tag: req.Tag, Protocol: req.Protocol,
		Port: req.Port, Listen: req.Listen,
		SettingsJSON: req.SettingsJSON, StreamSettings: req.StreamSettings,
		Sniffing: req.Sniffing, Ratio: req.Ratio, Enabled: true,
	}
	if err := d.DB.Create(&inb).Error; err != nil {
		util.ServerError(c, "创建失败")
		return
	}
	if err := d.enqueueConfig(req.ServerID); err != nil {
		log.Printf("inbounds: 自动推送配置失败 (server=%d): %v", req.ServerID, err)
	}
	util.OK(c, gin.H{"inbound": toInboundView(&inb, srv.Name)})
}

// AdminUpdateInbound PUT /api/v1/admin/inbounds/:id
func (d *Deps) AdminUpdateInbound(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var inb models.Inbound
	if err := d.DB.First(&inb, id).Error; err != nil {
		util.Fail(c, 404, "入站不存在")
		return
	}
	var req struct {
		Tag            *string  `json:"tag"`
		Protocol       *string  `json:"protocol"`
		Port           *int     `json:"port"`
		Listen         *string  `json:"listen"`
		SettingsJSON   *string  `json:"settings_json"`
		StreamSettings *string  `json:"stream_settings"`
		Sniffing       *string  `json:"sniffing"`
		Ratio          *float64 `json:"ratio"`
		Enabled        *bool    `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	// 校验 JSON 有效性
	sj := inb.SettingsJSON
	if req.SettingsJSON != nil {
		sj = *req.SettingsJSON
	}
	ss := inb.StreamSettings
	if req.StreamSettings != nil {
		ss = *req.StreamSettings
	}
	sn := inb.Sniffing
	if req.Sniffing != nil {
		sn = *req.Sniffing
	}
	if err := xray.ValidateInbound(sj, ss, sn); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	// 端口冲突（排除自己）
	port := inb.Port
	if req.Port != nil {
		port = *req.Port
	}
	var cnt int64
	d.DB.Model(&models.Inbound{}).Where("server_id = ? AND port = ? AND id != ?", inb.ServerID, port, id).Count(&cnt)
	if cnt > 0 {
		util.BadRequest(c, "该服务器上端口已被占用")
		return
	}

	updates := map[string]any{}
	if req.Tag != nil {
		updates["tag"] = *req.Tag
	}
	if req.Protocol != nil {
		updates["protocol"] = *req.Protocol
	}
	if req.Port != nil {
		updates["port"] = *req.Port
	}
	if req.Listen != nil {
		updates["listen"] = *req.Listen
	}
	if req.SettingsJSON != nil {
		updates["settings_json"] = *req.SettingsJSON
	}
	if req.StreamSettings != nil {
		updates["stream_settings"] = *req.StreamSettings
	}
	if req.Sniffing != nil {
		updates["sniffing"] = *req.Sniffing
	}
	if req.Ratio != nil {
		updates["ratio"] = *req.Ratio
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(updates) > 0 {
		if err := d.DB.Model(&inb).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新失败")
			return
		}
	}
	if err := d.enqueueConfig(inb.ServerID); err != nil {
		log.Printf("inbounds: 自动推送配置失败 (server=%d): %v", inb.ServerID, err)
	}
	var srv models.Server
	serverName := ""
	if err := d.DB.First(&srv, inb.ServerID).Error; err == nil {
		serverName = srv.Name
	}
	d.DB.First(&inb, id)
	util.OK(c, gin.H{"inbound": toInboundView(&inb, serverName)})
}

// AdminDeleteInbound DELETE /api/v1/admin/inbounds/:id
func (d *Deps) AdminDeleteInbound(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var inb models.Inbound
	d.DB.First(&inb, id)
	if err := d.DB.Delete(&models.Inbound{}, id).Error; err != nil {
		util.ServerError(c, "删除失败")
		return
	}
	if inb.ID > 0 {
		if err := d.enqueueConfig(inb.ServerID); err != nil {
			log.Printf("inbounds: 自动推送配置失败 (server=%d): %v", inb.ServerID, err)
		}
	}
	util.OK(c, gin.H{"deleted": id})
}

// AdminToggleInbound POST /api/v1/admin/inbounds/:id/toggle
func (d *Deps) AdminToggleInbound(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var inb models.Inbound
	if err := d.DB.First(&inb, id).Error; err != nil {
		util.Fail(c, 404, "入站不存在")
		return
	}
	inb.Enabled = !inb.Enabled
	if err := d.DB.Model(&inb).Update("enabled", inb.Enabled).Error; err != nil {
		util.ServerError(c, "更新失败")
		return
	}
	if err := d.enqueueConfig(inb.ServerID); err != nil {
		log.Printf("inbounds: 自动推送配置失败 (server=%d): %v", inb.ServerID, err)
	}
	util.OK(c, gin.H{"id": id, "enabled": inb.Enabled})
}

// AdminXrayKeys GET /api/v1/admin/xray/keys —— REALITY x25519 + shortId 一键生成。
func (d *Deps) AdminXrayKeys(c *gin.Context) {
	priv, pub, err := xray.GenerateKeys()
	if err != nil {
		util.ServerError(c, err.Error())
		return
	}
	util.OK(c, gin.H{
		"private_key": priv,
		"public_key":  pub,
		"short_id":    xray.GenerateShortID(),
	})
}

// enqueueConfig 入站变更后自动生成配置并待推送。
func (d *Deps) enqueueConfig(serverID uint64) error {
	if d.Config == nil || d.Hub == nil {
		return nil
	}
	cfg, err := d.Config.Generate(serverID)
	if err != nil {
		return fmt.Errorf("生成配置失败: %w", err)
	}
	if err := d.Config.SavePending(serverID, cfg); err != nil {
		return fmt.Errorf("保存待推送配置失败: %w", err)
	}
	go d.Hub.PushPending(serverID)
	return nil
}

// formToInbound 把入站表单转为 models.Inbound（配置预览用）。
func formToInbound(f *inboundForm) models.Inbound {
	return models.Inbound{
		ServerID: f.ServerID, Tag: f.Tag, Protocol: f.Protocol,
		Port: f.Port, Listen: f.Listen,
		SettingsJSON: f.SettingsJSON, StreamSettings: f.StreamSettings,
		Sniffing: f.Sniffing, Ratio: f.Ratio, Enabled: true,
	}
}

// AdminPreviewConfig POST /api/v1/admin/xray/preview-config
func (d *Deps) AdminPreviewConfig(c *gin.Context) {
	var req struct {
		ServerID uint64       `json:"server_id" binding:"required"`
		Form     *inboundForm `json:"form"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	var srv models.Server
	if err := d.DB.First(&srv, req.ServerID).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}
	q := d.DB.Where("server_id = ? AND enabled = ?", req.ServerID, true)
	if req.Form != nil && req.Form.Tag != "" {
		q = q.Where("tag != ?", req.Form.Tag)
	}
	var inbounds []models.Inbound
	if err := q.Find(&inbounds).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	if req.Form != nil {
		inbounds = append(inbounds, formToInbound(req.Form))
	}
	var users []models.User
	if err := d.DB.Where("status = ?", models.StatusActive).Find(&users).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	var outbounds []models.ServerOutbound
	if err := d.DB.Where("server_id = ? AND enabled = ?", req.ServerID, true).Order("priority asc, id asc").Find(&outbounds).Error; err != nil {
		util.ServerError(c, "查询出站失败")
		return
	}
	var routingRules []models.ServerRoutingRule
	if err := d.DB.Where("server_id = ? AND enabled = ?", req.ServerID, true).Order("priority asc, id asc").Find(&routingRules).Error; err != nil {
		util.ServerError(c, "查询路由失败")
		return
	}
	cfg, err := xray.Generate(inbounds, outbounds, routingRules, users, nil)
	if err != nil {
		util.BadRequest(c, "配置生成失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{"config": string(cfg)})
}
