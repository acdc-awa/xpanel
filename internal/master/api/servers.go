package api

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/nodegate"
	"github.com/zhx/xray-panel/internal/master/xray"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/protocol"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// serverView 服务器对外结构。
type serverView struct {
	ID         uint64     `json:"id"`
	Name       string     `json:"name"`
	Host       string     `json:"host"`
	NodeID     string     `json:"node_id"`
	Location   string     `json:"location"`
	Remark     string     `json:"remark"`
	Status     int        `json:"status"` // 0 离线 1 在线
	LastSeenAt *time.Time `json:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func toServerView(s *models.Server) serverView {
	return serverView{
		ID: s.ID, Name: s.Name, Host: s.Host, NodeID: s.NodeID,
		Location: s.Location, Remark: s.Remark, Status: s.Status,
		LastSeenAt: s.LastSeenAt, CreatedAt: s.CreatedAt,
	}
}

// AdminServers GET /api/v1/admin/servers —— 服务器列表（在线状态以网关实时为准）。
func (d *Deps) AdminServers(c *gin.Context) {
	var list []models.Server
	if err := d.DB.Order("id DESC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	items := make([]serverView, 0, len(list))
	for i := range list {
		v := toServerView(&list[i])
		if d.Hub != nil && d.Hub.IsOnline(v.ID) {
			v.Status = 1
		}
		items = append(items, v)
	}
	util.OK(c, gin.H{"items": items})
}

// AdminCreateServer POST /api/v1/admin/servers —— 新增服务器，返回一次性 secret。
func (d *Deps) AdminCreateServer(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required,max=64"`
		Host     string `json:"host" binding:"required,max=255"`
		Location string `json:"location" binding:"max=64"`
		Remark   string `json:"remark" binding:"max=255"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	nodeID := "node-" + util.RandomID(6)
	secret, err := util.NewNodeSecret()
	if err != nil {
		util.ServerError(c, "生成密钥失败")
		return
	}
	server := models.Server{
		Name:     req.Name,
		Host:     req.Host,
		NodeID:   nodeID,
		Secret:   util.HashSecret(secret),
		Location: req.Location,
		Remark:   req.Remark,
		Status:   0,
	}
	if err := d.DB.Create(&server).Error; err != nil {
		util.ServerError(c, "创建失败")
		return
	}
	util.OK(c, gin.H{
		"server": toServerView(&server),
		"node_id": nodeID,
		"secret": secret, // 仅此一次返回明文
	})
}

// AdminDeleteServer DELETE /api/v1/admin/servers/:id
func (d *Deps) AdminDeleteServer(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	if err := d.DB.Delete(&models.Server{}, id).Error; err != nil {
		util.ServerError(c, "删除失败")
		return
	}
	util.OK(c, gin.H{"deleted": id})
}

// AdminServerCommand POST /api/v1/admin/servers/:id/command
// body: {"type":"push_config|restart_xray|get_status|get_logs", "config_json": "...", "lines": 100}
func (d *Deps) AdminServerCommand(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var req struct {
		Type       string `json:"type" binding:"required"`
		ConfigJSON string `json:"config_json"`
		Lines      int    `json:"lines"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	var payload any
	switch req.Type {
	case protocol.MsgPushConfig:
		if req.ConfigJSON == "" {
			util.BadRequest(c, "push_config 需要 config_json")
			return
		}
		payload = protocol.PushConfigPayload{ConfigJSON: req.ConfigJSON}
	case protocol.MsgRestartXray:
		payload = nil
	case protocol.MsgGetStatus:
		payload = nil
	case protocol.MsgGetLogs:
		payload = protocol.GetLogsPayload{Lines: req.Lines}
	default:
		util.BadRequest(c, "不支持的指令类型")
		return
	}

	res, err := d.Hub.Ask(id, req.Type, payload, nodegate.AskTimeout)
	if err != nil {
		util.Fail(c, 502, "指令失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{"ok": res.OK, "error": res.Error, "data": res.Data})
}
// AdminGenerateConfig POST /api/v1/admin/servers/:id/generate-config
// 由主控根据「服务器启用入站 + 全部启用用户」生成 Xray 配置并下发（返回回执）。
func (d *Deps) AdminGenerateConfig(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var srv models.Server
	if err := d.DB.First(&srv, id).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}
	var inbounds []models.Inbound
	if err := d.DB.Where("server_id = ? AND enabled = ?", id, true).Find(&inbounds).Error; err != nil {
		util.ServerError(c, "查询入站失败")
		return
	}
	var users []models.User
	if err := d.DB.Where("status = ?", models.StatusActive).Find(&users).Error; err != nil {
		util.ServerError(c, "查询用户失败")
		return
	}
	cfg, err := xray.Generate(&srv, inbounds, users)
	if err != nil {
		util.BadRequest(c, "配置生成失败: "+err.Error())
		return
	}

	res, err := d.Hub.Ask(id, protocol.MsgPushConfig, protocol.PushConfigPayload{ConfigJSON: string(cfg)}, nodegate.AskTimeout)
	if err != nil {
		util.Fail(c, 502, "下发失败: "+err.Error())
		return
	}
	util.OK(c, gin.H{
		"ok":     res.OK,
		"error":  res.Error,
		"config": string(cfg), // 返回生成结果供查看
	})
}