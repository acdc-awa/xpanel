package api

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/nodegate"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// serverView 服务器对外结构。
type serverView struct {
	ID                    uint64     `json:"id"`
	ServerType            string     `json:"server_type"`             // xray（托管 Xray-core 计算节点）
	Name                  string     `json:"name"`
	Host                  string     `json:"host"`
	NodeID                string     `json:"node_id"`
	Location              string     `json:"location"`
	Remark                string     `json:"remark"`
	Status                int        `json:"status"`                  // 0 离线 1 在线
	ConfigStatus          string     `json:"config_status"`           // pushed / pending / ""（无待推送配置）
	DefaultOutboundTag    string     `json:"default_outbound_tag"`    // 路由默认出口
	RoutingDomainStrategy string     `json:"routing_domain_strategy"` // 路由域名策略（路由匹配阶段）
	DefaultOutboundDS     string     `json:"default_outbound_domain_strategy"` // 默认出口出站解析策略（freedom: AsIs/UseIP/UseIPv4/UseIPv6）
	AgentVersion          string     `json:"agent_version"`        // 节点心跳上报的 agent 版本（旧 agent 为空）
	LastSeenAt            *time.Time `json:"last_seen_at"`
	CreatedAt             time.Time  `json:"created_at"`
}

func toServerView(s *models.Server) serverView {
	st := s.ServerType
	if st == "" {
		st = models.ServerTypeXray
	}
	return serverView{
		ID: s.ID, ServerType: st, Name: s.Name, Host: s.Host, NodeID: s.NodeID,
		Location: s.Location, Remark: s.Remark, Status: s.Status,
		DefaultOutboundTag:    s.DefaultOutboundTag,
		RoutingDomainStrategy: s.RoutingDomainStrategy,
		DefaultOutboundDS:     s.DefaultOutboundDS,
		AgentVersion:          s.AgentVersion,
		LastSeenAt:            s.LastSeenAt, CreatedAt: s.CreatedAt,
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
		ServerType            string `json:"server_type"`
		Name                  string `json:"name" binding:"required,max=64"`
		Host                  string `json:"host" binding:"required,max=255"`
		Location              string `json:"location" binding:"max=64"`
		Remark                string `json:"remark" binding:"max=255"`
		DefaultOutboundTag    string `json:"default_outbound_tag"`
		RoutingDomainStrategy string `json:"routing_domain_strategy"`
		DefaultOutboundDS     string `json:"default_outbound_domain_strategy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if req.DefaultOutboundTag == "" {
		req.DefaultOutboundTag = "direct"
	}
	st := req.ServerType
	if st == "" {
		st = models.ServerTypeXray
	}
	nodeID := "node-" + util.RandomID(6)
	secret, err := util.NewNodeSecret()
	if err != nil {
		util.ServerError(c, "生成密钥失败")
		return
	}
	server := models.Server{
		ServerType:            st,
		Name:                  req.Name,
		Host:                  req.Host,
		NodeID:                nodeID,
		Secret:                util.HashSecret(secret),
		Location:              req.Location,
		Remark:                req.Remark,
		Status:                0,
		DefaultOutboundTag:    req.DefaultOutboundTag,
		RoutingDomainStrategy: req.RoutingDomainStrategy,
		DefaultOutboundDS:     req.DefaultOutboundDS,
	}
	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		return tx.Create(&server).Error
	}); err != nil {
		util.ServerError(c, "创建失败")
		return
	}
	EnsureDefaultServerOutbounds(d.DB, server.ID)
	util.OK(c, gin.H{
		"server":      toServerView(&server),
		"node_id":     nodeID,
		"secret":      secret, // 仅此一次返回明文
		"install_cmd": installCmd(d.Cfg.App.PublicURL, d.Cfg.App.WSPublicURL, c.Request.Host, nodeID, secret),
	})
}

// AgentInstallScriptURL 节点一键安装脚本下载地址。2026-08-24 仓库拆分收口：
// 脚本权威源在 XPanel-Node 仓库 GitHub Releases（release.yml 随 tag 发布 deploy/install-agent.sh），
// 面板不再充当脚本下载源，杜绝多源漂移（与 agent 内部 upgrade.DefaultRepo 同仓库）。
const AgentInstallScriptURL = "https://github.com/acdc-awa/XPanel-Node/releases/latest/download/install-agent.sh"

// installCmd 生成节点一键安装命令（脚本从 GitHub Releases 拉取；--master 指向节点
// WebSocket 网关入口——四端口拆分后对外路径固定 /node/ws（Caddy 按该路径分流到 WS 端口），
// 也可用 config.yaml 的 app.ws_public_url（如 wss://ws.example.com/node/ws）整体覆盖为任意路径/独立域名）。
func installCmd(publicURL, wsPublicURL, reqHost, nodeID, secret string) string {
	master := strings.TrimSpace(wsPublicURL)
	if master == "" {
		wsScheme := "ws"
		host := publicURL
		if host == "" {
			host = reqHost
		} else {
			host = strings.TrimRight(host, "/")
			if strings.HasPrefix(host, "https://") {
				wsScheme = "wss"
			}
			host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
		}
		master = fmt.Sprintf("%s://%s/node/ws", wsScheme, host)
	}
	return fmt.Sprintf(
		"bash <(curl -fsSL %s) --master %s --node-id %s --secret %s",
		AgentInstallScriptURL, master, nodeID, secret)
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
		ServerType            *string `json:"server_type"`
		Name                  *string `json:"name"`
		Host                  *string `json:"host"`
		Location              *string `json:"location"`
		Remark                *string `json:"remark"`
		DefaultOutboundTag    *string `json:"default_outbound_tag"`
		RoutingDomainStrategy *string `json:"routing_domain_strategy"`
		DefaultOutboundDS     *string `json:"default_outbound_domain_strategy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	updates := map[string]any{}
	if req.ServerType != nil && *req.ServerType != "" {
		updates["server_type"] = *req.ServerType
	}
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
	if req.DefaultOutboundTag != nil {
		updates["default_outbound_tag"] = *req.DefaultOutboundTag
	}
	if req.RoutingDomainStrategy != nil {
		updates["routing_domain_strategy"] = *req.RoutingDomainStrategy
	}
	if req.DefaultOutboundDS != nil {
		updates["default_outbound_domain_strategy"] = *req.DefaultOutboundDS
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
	util.OK(c, gin.H{
		"node_id":     srv.NodeID,
		"secret":      secret, // 仅此一次返回明文
		"install_cmd": installCmd(d.Cfg.App.PublicURL, d.Cfg.App.WSPublicURL, c.Request.Host, srv.NodeID, secret),
	})
}

// AdminDeleteServer DELETE /api/v1/admin/servers/:id —— 删除服务器并级联清理入站/授权/待推送配置/节点上报。
func (d *Deps) AdminDeleteServer(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	// U4：检查其他服务器出站是否引用本服务器入站（落地链路）——删除会使引用方配置生成死锁
	var refCnt int64
	d.DB.Model(&models.ServerOutbound{}).
		Joins("JOIN inbounds ON inbounds.id = server_outbounds.inbound_ref").
		Where("inbounds.server_id = ?", id).Count(&refCnt)
	if refCnt > 0 {
		util.BadRequest(c, "该服务器有 "+strconv.FormatInt(refCnt, 10)+" 个出站引用其入站（落地），无法删除，请先解除引用")
		return
	}

	// 接入点引用保护（AP 单点授权：删除会使订阅管道断裂）
	var inboundIDs []uint64
	d.DB.Model(&models.Inbound{}).Where("server_id = ?", id).Pluck("id", &inboundIDs)
	if len(inboundIDs) > 0 {
		var apDirect int64
		d.DB.Model(&models.UserAccessPoint{}).Where("target_type = 'inbound' AND target_inbound_id IN ?", inboundIDs).Count(&apDirect)
		if apDirect > 0 {
			util.BadRequest(c, "该服务器入站被 "+strconv.FormatInt(apDirect, 10)+" 个用户接入点直连引用，无法删除，请先解除接入点连线")
			return
		}
	}

	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		if len(inboundIDs) > 0 {
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
		// P2-6：删除服务器时同步清理待推证书，避免悬挂 PendingCert。
		if err := tx.Where("server_id = ?", id).Delete(&models.PendingCert{}).Error; err != nil {
			return err
		}
		if err := tx.Where("server_id = ?", id).Delete(&models.NodeReport{}).Error; err != nil {
			return err
		}
		// 悬空引用收口：对外接入层随服务器级联删除（层无宿主后成孤儿）
		if err := tx.Where("server_id = ?", id).Delete(&models.AccessLayer{}).Error; err != nil {
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

// AdminGetServerConfigPreview GET /api/v1/admin/servers/:id/config-preview
// 实时渲染该服务器按当前数据库预期应该推送到节点的完整 Xray 配置（只读预览，无网络副作用）。
func (d *Deps) AdminGetServerConfigPreview(c *gin.Context) {
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
	util.OK(c, gin.H{
		"config": cfgStr,
	})
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
			"queued":  true,
			"message": "节点离线，配置已保存，节点上线后自动推送",
			"config":  cfgStr,
		})
		return
	}

	// P2-7：generate-config 恢复为真正的非阻塞——API 立即返回 queued，
	// 由后台 PushPending 完成下发与回执处理，不再同步等待最长 30s。
	go d.Hub.PushPending(id)
	util.OK(c, gin.H{
		"ok":      true,
		"pushed":  false,
		"queued":  true,
		"message": "配置已保存，正在后台推送到节点",
		"config":  cfgStr,
	})
}

// AdminServerMetrics GET /api/v1/admin/servers/:id/metrics —— 查询节点时序监控数据 (1h/6h/24h/7d)。
func (d *Deps) AdminServerMetrics(c *gin.Context) {
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

	timeRange := c.DefaultQuery("range", "1h")
	now := time.Now()
	var startTime time.Time
	var bucketDuration time.Duration
	var timeFmt string

	switch timeRange {
	case "6h":
		startTime = now.Add(-6 * time.Hour)
		bucketDuration = 3 * time.Minute
		timeFmt = "15:04"
	case "24h":
		startTime = now.Add(-24 * time.Hour)
		bucketDuration = 10 * time.Minute
		timeFmt = "15:04"
	case "7d":
		startTime = now.AddDate(0, 0, -7)
		bucketDuration = 1 * time.Hour
		timeFmt = "01-02 15:00"
	case "1h":
		fallthrough
	default:
		timeRange = "1h"
		startTime = now.Add(-1 * time.Hour)
		bucketDuration = 1 * time.Minute
		timeFmt = "15:04"
	}

	var reports []models.NodeReport
	d.DB.Where("server_id = ? AND reported_at >= ?", id, startTime).
		Order("reported_at ASC").
		Find(&reports)

	numBuckets := int(now.Sub(startTime) / bucketDuration)
	if numBuckets < 1 {
		numBuckets = 1
	}

	type bucketAgg struct {
		timeStr    string
		cpuSum     float64
		memSum     float64
		memTotal   uint64
		diskSum    float64
		diskTotal  uint64
		rxRateSum  float64
		txRateSum  float64
		usersSum   int
		count      int
	}

	buckets := make([]bucketAgg, numBuckets)
	for i := 0; i < numBuckets; i++ {
		bTime := startTime.Add(time.Duration(i) * bucketDuration)
		buckets[i].timeStr = bTime.Format(timeFmt)
	}

	for _, r := range reports {
		idx := int(r.ReportedAt.Sub(startTime) / bucketDuration)
		if idx >= 0 && idx < numBuckets {
			b := &buckets[idx]
			b.cpuSum += r.CPU
			b.memSum += r.Mem
			if r.MemTotal > 0 {
				b.memTotal = r.MemTotal
			}
			b.diskSum += r.Disk
			if r.DiskTotal > 0 {
				b.diskTotal = r.DiskTotal
			}
			b.rxRateSum += r.RxRate
			b.txRateSum += r.TxRate
			b.usersSum += r.OnlineUsers
			b.count++
		}
	}

	timestamps := make([]string, numBuckets)
	cpuList := make([]float64, numBuckets)
	memPercentList := make([]float64, numBuckets)
	memUsedList := make([]float64, numBuckets)
	diskPercentList := make([]float64, numBuckets)
	rxMbpsList := make([]float64, numBuckets)
	txMbpsList := make([]float64, numBuckets)
	usersList := make([]int, numBuckets)

	var lastMemTotal uint64
	var lastDiskTotal uint64

	for i := 0; i < numBuckets; i++ {
		b := buckets[i]
		timestamps[i] = b.timeStr
		if b.memTotal > 0 {
			lastMemTotal = b.memTotal
		}
		if b.diskTotal > 0 {
			lastDiskTotal = b.diskTotal
		}

		if b.count > 0 {
			cVal := b.cpuSum / float64(b.count)
			mVal := b.memSum / float64(b.count)
			dVal := b.diskSum / float64(b.count)
			cpuList[i] = float64(int(cVal*10)) / 10
			memUsedList[i] = mVal
			if lastMemTotal > 0 {
				memPercentList[i] = float64(int((mVal/float64(lastMemTotal)*100)*10)) / 10
			}
			diskPercentList[i] = float64(int(dVal*10)) / 10
			// 字节/秒 -> Mbps (8 / 1,000,000)
			rxMbps := (b.rxRateSum / float64(b.count)) * 8 / 1_000_000
			txMbps := (b.txRateSum / float64(b.count)) * 8 / 1_000_000
			rxMbpsList[i] = float64(int(rxMbps*100)) / 100
			txMbpsList[i] = float64(int(txMbps*100)) / 100
			usersList[i] = b.usersSum / b.count
		} else {
			if i > 0 {
				cpuList[i] = cpuList[i-1]
				memUsedList[i] = memUsedList[i-1]
				memPercentList[i] = memPercentList[i-1]
				diskPercentList[i] = diskPercentList[i-1]
				rxMbpsList[i] = rxMbpsList[i-1]
				txMbpsList[i] = txMbpsList[i-1]
				usersList[i] = usersList[i-1]
			}
		}
	}

	util.OK(c, gin.H{
		"server_id":    id,
		"server_name":  srv.Name,
		"host":         srv.Host,
		"location":     srv.Location,
		"range":        timeRange,
		"timestamps":   timestamps,
		"cpu":          cpuList,
		"mem_percent":  memPercentList,
		"mem_used":     memUsedList,
		"mem_total":    lastMemTotal,
		"disk_percent": diskPercentList,
		"disk_total":   lastDiskTotal,
		"rx_mbps":      rxMbpsList,
		"tx_mbps":      txMbpsList,
		"online_users": usersList,
	})
}
