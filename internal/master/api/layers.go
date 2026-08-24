package api

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// accessLayerView 对外接入层视图。
type accessLayerView struct {
	ID           uint64    `json:"id"`
	ServerID     uint64    `json:"server_id"`
	Name         string    `json:"name"`
	Host         string    `json:"host"`
	Port         int       `json:"port"`
	Security     string    `json:"security"`
	Remark       string    `json:"remark"`
	InboundCount int       `json:"inbound_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func toAccessLayerView(l models.AccessLayer, inboundCount int) accessLayerView {
	sec := l.Security
	if sec == "" {
		sec = "tls"
	}
	return accessLayerView{
		ID: l.ID, ServerID: l.ServerID, Name: l.Name, Host: l.Host,
		Port: l.Port, Security: sec, Remark: l.Remark,
		InboundCount: inboundCount,
		CreatedAt:    l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
}

// layerForm 层创建/更新表单（对外端点全量字段；同 L4 规则 PUT 语义）。
type layerForm struct {
	Name     string `json:"name" binding:"required,max=64"`
	Host     string `json:"host" binding:"required,max=255"`
	Port     int    `json:"port" binding:"min=1,max=65535"`
	Security string `json:"security"`
	Remark   string `json:"remark"`
}

func validLayerSecurity(s string) bool {
	return s == "tls" || s == "none"
}

// AdminGetLayers GET /api/v1/admin/servers/:id/layers
func (d *Deps) AdminGetLayers(c *gin.Context) {
	serverID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的服务器 ID")
		return
	}
	var srv models.Server
	if err := d.DB.First(&srv, serverID).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}

	var layers []models.AccessLayer
	if err := d.DB.Where("server_id = ?", serverID).Order("id ASC").Find(&layers).Error; err != nil {
		util.ServerError(c, "查询失败: "+err.Error())
		return
	}

	layerIDs := make([]uint64, 0, len(layers))
	for _, l := range layers {
		layerIDs = append(layerIDs, l.ID)
	}
	countMap := make(map[uint64]int, len(layers))
	if len(layerIDs) > 0 {
		var inbs []models.Inbound
		d.DB.Select("id, layer_id").Where("layer_id IN ?", layerIDs).Find(&inbs)
		for _, inb := range inbs {
			if inb.LayerID != nil {
				countMap[*inb.LayerID]++
			}
		}
	}

	views := make([]accessLayerView, 0, len(layers))
	for _, l := range layers {
		views = append(views, toAccessLayerView(l, countMap[l.ID]))
	}
	util.OK(c, gin.H{"items": views})
}

// AdminCreateLayer POST /api/v1/admin/servers/:id/layers
func (d *Deps) AdminCreateLayer(c *gin.Context) {
	serverID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的服务器 ID")
		return
	}
	var srv models.Server
	if err := d.DB.First(&srv, serverID).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}
	var req layerForm
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Host = strings.TrimSpace(req.Host)
	if req.Name == "" || req.Host == "" {
		util.BadRequest(c, "层名称与对外 Host 均必填")
		return
	}
	if req.Port == 0 {
		req.Port = 443
	}
	if req.Security == "" {
		req.Security = "tls"
	}
	if !validLayerSecurity(req.Security) {
		util.BadRequest(c, "对外安全层仅支持 tls / none")
		return
	}
	layer := models.AccessLayer{
		ServerID: serverID, Name: req.Name, Host: req.Host,
		Port: req.Port, Security: req.Security, Remark: req.Remark,
	}
	if err := d.DB.Create(&layer).Error; err != nil {
		util.ServerError(c, "创建失败")
		return
	}
	util.OK(c, gin.H{"layer": toAccessLayerView(layer, 0)})
}

// AdminUpdateLayer PUT /api/v1/admin/servers/:id/layers/:layer_id
func (d *Deps) AdminUpdateLayer(c *gin.Context) {
	layerID, err := strconv.ParseUint(c.Param("layer_id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的层 ID")
		return
	}
	var layer models.AccessLayer
	if err := d.DB.First(&layer, layerID).Error; err != nil {
		util.Fail(c, 404, "对外层不存在")
		return
	}
	var req layerForm
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Host = strings.TrimSpace(req.Host)
	if req.Name == "" || req.Host == "" {
		util.BadRequest(c, "层名称与对外 Host 均必填")
		return
	}
	if req.Port == 0 {
		req.Port = 443
	}
	if req.Security == "" {
		req.Security = "tls"
	}
	if !validLayerSecurity(req.Security) {
		util.BadRequest(c, "对外安全层仅支持 tls / none")
		return
	}
	updates := map[string]any{
		"name":     req.Name,
		"host":     req.Host,
		"port":     req.Port,
		"security": req.Security,
		"remark":   req.Remark,
	}
	if err := d.DB.Model(&layer).Updates(updates).Error; err != nil {
		util.ServerError(c, "更新失败")
		return
	}
	d.DB.First(&layer, layerID)
	var cnt int64
	d.DB.Model(&models.Inbound{}).Where("layer_id = ?", layerID).Count(&cnt)
	util.OK(c, gin.H{"layer": toAccessLayerView(layer, int(cnt))})
}

// AdminDeleteLayer DELETE /api/v1/admin/servers/:id/layers/:layer_id
// 删除层时挂层入站回退原生（layer_id 置空），订阅消费自动降级为直连端点。
func (d *Deps) AdminDeleteLayer(c *gin.Context) {
	layerID, err := strconv.ParseUint(c.Param("layer_id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的层 ID")
		return
	}
	var layer models.AccessLayer
	if err := d.DB.First(&layer, layerID).Error; err != nil {
		util.Fail(c, 404, "对外层不存在")
		return
	}
	if err := d.DB.Model(&models.Inbound{}).Where("layer_id = ?", layerID).Update("layer_id", gorm.Expr("NULL")).Error; err != nil {
		util.ServerError(c, "解除入站挂层失败")
		return
	}
	if err := d.DB.Delete(&layer).Error; err != nil {
		util.ServerError(c, "删除失败")
		return
	}
	util.OK(c, gin.H{"deleted": true})
}
