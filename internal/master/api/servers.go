package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/master/nodegate"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/protocol"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// serverView 服务器对外结构。
type serverView struct {
	ID           uint64     `json:"id"`
	Name         string     `json:"name"`
	Host         string     `json:"host"`
	NodeID       string     `json:"node_id"`
	Location     string     `json:"location"`
	Remark       string     `json:"remark"`
	Status       int        `json:"status"`        // 0 离线 1 在线
	ConfigStatus string     `json:"config_status"` // pushed / pending / ""（无待推送配置）
	LastSeenAt   *time.Time `json:"last_seen_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

func toServerView(s *models.Server) serverView {
	return serverView{
		ID: s.ID, Name: s.Name, Host: s.Host, NodeID: s.NodeID,
		Location: s.Location, Remark: s.Remark, Status: s.Status,
		LastSeenAt: s.LastSeenAt, CreatedAt: s.CreatedAt,
	}
}

// AdminServers GET /api/v1/admin/servers —— 服务器列表（在线状态以网关实时为准，附带配置同步状态）。
func (d *Deps) AdminServers(c *gin.Context) {
	var list []models.Server
	if err := d.DB.Order("id DESC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	// 一次查询所有待推送配置状态
	statusMap := map[uint64]string{}
	var pends []models.PendingConfig
	if err := d.DB.Select("server_id", "status").Find(&pends).Error; err == nil {
		for _, p := range pends {
			statusMap[p.ServerID] = p.Status
		}
	}
	items := make([]serverView, 0, len(list))
	for i := range list {
		v := toServerView(&list[i])
		if d.Hub != nil && d.Hub.IsOnline(v.ID) {
			v.Status = 1
		}
		v.ConfigStatus = statusMap[list[i].ID]
		items = append(items, v)
	}
	util.OK(c, gin.H{"items": items})
}

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
	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&server).Error
	}); err != nil {
		util.ServerError(c, "创建失败")
		return
	}
	webBase := ""
	if d.Site != nil {
		webBase = d.Site.WebBase()
	}
	util.OK(c, gin.H{
		"server":      toServerView(&server),
		"node_id":     nodeID,
		"secret":      secret, // 仅此一次返回明文
		"install_cmd": installCmd(webBase, d.Cfg.App.PublicURL, c.Request.Host, nodeID, secret),
	})
}

// installCmd 生成节点一键安装命令（主控作为安装脚本与 agent 二进制的下载源）。
func installCmd(webBase, publicURL, reqHost, nodeID, secret string) string {
	httpScheme := "http"
	wsScheme := "ws"
	host := publicURL
	if host == "" {
		host = reqHost
	} else {
		host = strings.TrimRight(host, "/")
		if strings.HasPrefix(host, "https://") {
			httpScheme = "https"
			wsScheme = "wss"
		}
		host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	}
	return fmt.Sprintf(
		"bash <(curl -fsSL %s://%s%s/api/v1/download/install-agent.sh) --master %s://%s%s/api/v1/node/ws --node-id %s --secret %s",
		httpScheme, host, webBase, wsScheme, host, webBase, nodeID, secret)
}

// AdminUpdateServer PUT /api/v1/admin/servers/:id —— 编辑服务器信息。
func (d *Deps) AdminUpdateServer(c *gin.Context) {
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
	var req struct {
		Name     *string `json:"name"`
		Host     *string `json:"host"`
		Location *string `json:"location"`
		Remark   *string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		if *req.Name == "" {
			util.BadRequest(c, "名称不能为空")
			return
		}
		updates["name"] = *req.Name
	}
	if req.Host != nil {
		if *req.Host == "" {
			util.BadRequest(c, "地址不能为空")
			return
		}
		updates["host"] = *req.Host
	}
	if req.Location != nil {
		updates["location"] = *req.Location
	}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}
	if len(updates) > 0 {
		if err := d.DB.Model(&srv).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新失败")
			return
		}
	}
	d.DB.First(&srv, id)
	util.OK(c, gin.H{"server": toServerView(&srv)})
}

// AdminResetSecret POST /api/v1/admin/servers/:id/reset-secret —— 重置节点密钥（旧密钥立即失效，返回新 secret 一次）。
func (d *Deps) AdminResetSecret(c *gin.Context) {
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
	secret, err := util.NewNodeSecret()
	if err != nil {
		util.ServerError(c, "生成密钥失败")
		return
	}
	if err := d.DB.Model(&srv).Update("secret", util.HashSecret(secret)).Error; err != nil {
		util.ServerError(c, "重置失败")
		return
	}
	util.OK(c, gin.H{"node_id": srv.NodeID, "secret": secret})
}

// AdminDeleteServer DELETE /api/v1/admin/servers/:id —— 删除服务器并级联清理入站/授权/待推送配置/节点上报。
func (d *Deps) AdminDeleteServer(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		var inboundIDs []uint64
		if err := tx.Model(&models.Inbound{}).Where("server_id = ?", id).Pluck("id", &inboundIDs).Error; err != nil {
			return err
		}
		if len(inboundIDs) > 0 {
			if err := tx.Where("inbound_id IN ?", inboundIDs).Delete(&models.UserInbound{}).Error; err != nil {
				return err
			}
			if err := tx.Where("server_id = ?", id).Delete(&models.Inbound{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("server_id = ?", id).Delete(&models.ServerOutbound{}).Error; err != nil {
			return err
		}
		if err := tx.Where("server_id = ?", id).Delete(&models.ServerRoutingRule{}).Error; err != nil {
			return err
		}
		if err := tx.Where("server_id = ?", id).Delete(&models.PendingConfig{}).Error; err != nil {
			return err
		}
		if err := tx.Where("server_id = ?", id).Delete(&models.NodeReport{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Server{}, id).Error
	}); err != nil {
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
// 由主控根据「服务器启用入站 + 节点出站 + 节点路由 + 全部启用用户」生成 Xray 配置：
// 保存为待推送 → 节点在线则立即下发；离线则保留，节点上线自动补推（非阻塞）。
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
	if d.Config == nil {
		util.ServerError(c, "配置服务未初始化")
		return
	}
	cfgStr, err := d.Config.Generate(id)
	if err != nil {
		util.BadRequest(c, "配置生成失败: "+err.Error())
		return
	}

	// 保存待推送（无论节点是否在线）
	if d.Config != nil {
		if serr := d.Config.SavePending(id, cfgStr); serr != nil {
			util.ServerError(c, "保存待推送配置失败")
			return
		}
	}

	if d.Hub == nil || !d.Hub.IsOnline(id) {
		util.OK(c, gin.H{
			"ok":      true,
			"pushed":  false,
			"message": "节点离线，配置已保存，节点上线后自动推送",
			"config":  cfgStr,
		})
		return
	}

	res, err := d.Hub.Ask(id, protocol.MsgPushConfig, protocol.PushConfigPayload{ConfigJSON: cfgStr}, nodegate.AskTimeout)
	if err != nil || res == nil || !res.OK {
		msg := "节点在线但下发失败"
		if err != nil {
			msg += ": " + err.Error()
		} else if res != nil && res.Error != "" {
			msg += ": " + res.Error
		}
		msg += "（配置已保存，节点重连后将自动补推）"
		util.OK(c, gin.H{"ok": false, "pushed": false, "message": msg, "config": cfgStr})
		return
	}
	if d.Config != nil {
		// 条件标记：若 Ask 期间 pending 被并发覆盖（内容不一致），保持 pending 待后续推送
		if _, serr := d.Config.MarkPushedByServerIfSame(id, cfgStr); serr != nil {
			log.Printf("api: 标记配置已推送失败 (server=%d): %v", id, serr)
		}
	}
	util.OK(c, gin.H{
		"ok":      true,
		"pushed":  true,
		"message": "配置已生成并下发到节点",
		"config":  cfgStr,
	})
}

// DownloadInstallScript GET /api/v1/download/install-agent.sh —— 节点一键安装脚本（部署用）。
func (d *Deps) DownloadInstallScript(c *gin.Context) {
	p := os.Getenv("INSTALL_SCRIPT_PATH")
	if p == "" {
		for _, cand := range []string{"/app/install-agent.sh", "deploy/agent/install-agent.sh"} {
			if _, err := os.Stat(cand); err == nil {
				p = cand
				break
			}
		}
	}
	if p == "" {
		util.Fail(c, http.StatusNotFound, "安装脚本未内置")
		return
	}
	if _, err := os.Stat(p); err != nil {
		util.Fail(c, http.StatusNotFound, "安装脚本不存在: "+p)
		return
	}
	c.File(p)
}
