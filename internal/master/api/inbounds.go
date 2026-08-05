package api

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/xray"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// inboundView 入站对外结构。
type inboundView struct {
	ID           uint64    `json:"id"`
	ServerID     uint64    `json:"server_id"`
	ServerName   string    `json:"server_name"`
	Tag          string    `json:"tag"`
	Protocol     string    `json:"protocol"`
	Port         int       `json:"port"`
	Network      string    `json:"network"`
	TLSType      string    `json:"tls_type"`
	SettingsJSON string    `json:"settings_json"`
	Ratio        float64   `json:"ratio"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
}

// inboundForm 入站创建/更新表单（结构化连接参数，替代手填 JSON）。
type inboundForm struct {
	ID       uint64                `json:"id"`
	ServerID uint64                `json:"server_id" binding:"required"`
	Tag      string                `json:"tag" binding:"required,max=64"`
	Protocol string                `json:"protocol" binding:"required"` // vless
	Port     int                   `json:"port" binding:"required,min=1,max=65535"`
	Network  string                `json:"network" binding:"required"` // tcp / ws / xhttp
	TLSType  string                `json:"tls_type"`                   // none / tls / reality
	Settings *xray.InboundSettings `json:"settings"`                   // reality/ws/xhttp/tls 结构化参数
	Ratio    float64               `json:"ratio"`
}

func toInboundView(i *models.Inbound, serverName string) inboundView {
	return inboundView{
		ID: i.ID, ServerID: i.ServerID, ServerName: serverName,
		Tag: i.Tag, Protocol: i.Protocol, Port: i.Port,
		Network: i.Network, TLSType: i.TLSType, SettingsJSON: i.SettingsJSON,
		Ratio: i.Ratio, Enabled: i.Enabled, CreatedAt: i.CreatedAt,
	}
}

// marshalSettings 序列化入站连接参数为 settings_json。
func marshalSettings(s *xray.InboundSettings) (string, error) {
	if s == nil {
		return "", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
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
	if err := xray.ValidateSettings(req.Settings, req.Network, req.TLSType); err != nil {
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
	settingsJSON, err := marshalSettings(req.Settings)
	if err != nil {
		util.ServerError(c, "参数序列化失败")
		return
	}
	inb := models.Inbound{
		ServerID: req.ServerID, Tag: req.Tag, Protocol: req.Protocol,
		Port: req.Port, Network: req.Network, TLSType: req.TLSType,
		SettingsJSON: settingsJSON, Ratio: req.Ratio, Enabled: true,
	}
	if err := d.DB.Create(&inb).Error; err != nil {
		util.ServerError(c, "创建失败")
		return
	}
	d.enqueueConfig(req.ServerID)
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
		Tag      *string               `json:"tag"`
		Protocol *string               `json:"protocol"`
		Port     *int                  `json:"port"`
		Network  *string               `json:"network"`
		TLSType  *string               `json:"tls_type"`
		Settings *xray.InboundSettings `json:"settings"`
		Ratio    *float64              `json:"ratio"`
		Enabled  *bool                 `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	// 组装最终值用于校验
	network := inb.Network
	if req.Network != nil {
		network = *req.Network
	}
	tlsType := inb.TLSType
	if req.TLSType != nil {
		tlsType = *req.TLSType
	}
	port := inb.Port
	if req.Port != nil {
		port = *req.Port
	}
	settings := req.Settings
	if settings == nil {
		if parsed, perr := xray.ParseSettings(&inb); perr == nil {
			settings = parsed
		}
	}
	if err := xray.ValidateSettings(settings, network, tlsType); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	// 端口冲突（排除自己）
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
	if req.Network != nil {
		updates["network"] = *req.Network
	}
	if req.TLSType != nil {
		updates["tls_type"] = *req.TLSType
	}
	if req.Settings != nil {
		if sj, merr := marshalSettings(req.Settings); merr == nil {
			updates["settings_json"] = sj
		}
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
	d.enqueueConfig(inb.ServerID)
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
		d.enqueueConfig(inb.ServerID)
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
	d.enqueueConfig(inb.ServerID)
	util.OK(c, gin.H{"id": id, "enabled": inb.Enabled})
}

// AdminXrayKeys GET /api/v1/admin/xray/keys —— 生成 REALITY X25519 密钥对。
func (d *Deps) AdminXrayKeys(c *gin.Context) {
	priv, pub, err := xray.GenerateKeys()
	if err != nil {
		util.ServerError(c, err.Error())
		return
	}
	util.OK(c, gin.H{"private_key": priv, "public_key": pub})
}

// enqueueConfig 入站变更后自动生成配置并待推送（非阻塞；节点离线保留，上线补推）。
func (d *Deps) enqueueConfig(serverID uint64) {
	if d.Config == nil || d.Hub == nil {
		return
	}
	cfg, err := d.Config.Generate(serverID)
	if err != nil {
		return // 无启用入站/无可用用户等，无需推送
	}
	if err := d.Config.SavePending(serverID, cfg); err != nil {
		return
	}
	go d.Hub.PushPending(serverID)
}

// formToInbound 把入站表单转为 models.Inbound（用于配置预览）。
func formToInbound(f *inboundForm) (models.Inbound, error) {
	settingsJSON, err := marshalSettings(f.Settings)
	if err != nil {
		return models.Inbound{}, err
	}
	return models.Inbound{
		ID: f.ID, ServerID: f.ServerID, Tag: f.Tag, Protocol: f.Protocol,
		Port: f.Port, Network: f.Network, TLSType: f.TLSType,
		SettingsJSON: settingsJSON, Ratio: f.Ratio, Enabled: true,
	}, nil
}

// AdminPreviewConfig POST /api/v1/admin/xray/preview-config
// body: {"server_id": x, "form": {入站表单字段}} —— 用当前表单（临时替换同名 ID）预览生成的完整 xray 配置。
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
	if req.Form != nil && req.Form.ID > 0 {
		q = q.Where("id != ?", req.Form.ID)
	}
	var inbounds []models.Inbound
	if err := q.Find(&inbounds).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	// 追加当前表单入站
	if req.Form != nil {
		inb, err := formToInbound(req.Form)
		if err != nil {
			util.BadRequest(c, "参数序列化失败")
			return
		}
		inbounds = append(inbounds, inb)
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
	cfg, err := xray.Generate(&srv, inbounds, outbounds, routingRules, users)
	if err != nil {
		util.BadRequest(c, "配置生成失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{"config": string(cfg)})
}
