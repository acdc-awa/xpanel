package api

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel-node/pkg/protocol"
	"github.com/acdc-awa/xpanel/internal/master/nodegate"
	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// agentTagPattern 严格限制版本号格式，防恶意路径穿越。
var agentTagPattern = regexp.MustCompile(`^v?\d+(\.\d+)+(-[0-9A-Za-z.-]+)?$`)

// serverView 服务器对外结构。
type serverView struct {
	ID                    uint64     `json:"id"`
	ServerType            string     `json:"server_type"` // xray（托管 Xray-core 计算节点）
	Name                  string     `json:"name"`
	Host                  string     `json:"host"`
	NodeID                string     `json:"node_id"`
	Location              string     `json:"location"`
	Remark                string     `json:"remark"`
	ExpireAt              string     `json:"expire_at"` // 纯日历日 YYYY-MM-DD，空串=未设置
	BillingCycle          string     `json:"billing_cycle"`
	Price                 string     `json:"price"`
	IDCAddress            string     `json:"idc_address"`
	Status                int        `json:"status"`                  // 0 离线 1 在线
	ConfigStatus          string     `json:"config_status"`           // pushed / pending / ""（无待推送配置）
	PushError             string     `json:"push_error,omitempty"`    // 待推送配置最近一次失败原因（仅 pending 时有值）
	PushAttempts          int        `json:"push_attempts,omitempty"` // 待推送配置累计失败次数
	PushLastTryAt         *time.Time `json:"push_last_try_at,omitempty"`
	DefaultOutboundTag    string     `json:"default_outbound_tag"`      // 路由默认出口
	RoutingDomainStrategy string     `json:"routing_domain_strategy"`   // 路由域名策略（路由匹配阶段）
	AgentVersion          string     `json:"agent_version"`             // 节点心跳上报的 agent 版本（旧 agent 为空）
	XrayRunning           bool       `json:"xray_running"`              // 节点心跳上报的 xray 进程运行状态
	XrayState             string     `json:"xray_state,omitempty"`      // running / restarting / failed / stopped（旧 agent 为空）
	XrayLastError         string     `json:"xray_last_error,omitempty"` // 最近一次启动失败原因（含退出码与 xray 原始报错）
	XrayErrorAt           *time.Time `json:"xray_error_at,omitempty"`   // 该原因的观测时刻
	XrayFailures          int        `json:"xray_failures,omitempty"`   // 连续启动失败次数（成功后归零）
	// 配置对账（2026-09-21）：节点上报的两个内容哈希 + 后端推导的面板状态。
	// XrayDiskHash = 节点磁盘配置的 sha256；XrayRunningHash = 节点当前跑着的那份配置的 sha256
	// （xray 只在启动时读一次配置，热更落盘只改磁盘、不改它）。两者不等是热更落盘后的正常
	// 状态，不报警；报警的是 DiskHash 与主控记录的 AppliedHash 对不上（磁盘偏离）。
	XrayDiskHash    string `json:"xray_disk_hash,omitempty"`
	XrayRunningHash string `json:"xray_running_hash,omitempty"`
	ConfigDrift     bool   `json:"config_drift,omitempty"` // 节点磁盘与主控记录不一致
	// PushState 面板状态（四态 + 磁盘偏离），由后端推导，前端不靠字符串猜：
	// none 未投递 / pending 待推送 / rejected 节点拒绝（附原因）/ synced 已同步 / drift 磁盘偏离
	PushState  string     `json:"push_state"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func toServerView(s *models.Server) serverView {
	st := s.ServerType
	if st == "" {
		st = models.ServerTypeXray
	}
	return serverView{
		ID: s.ID, ServerType: st, Name: s.Name, Host: s.Host, NodeID: s.NodeID,
		Location: s.Location, Remark: s.Remark,
		ExpireAt: s.ExpireAt, BillingCycle: s.BillingCycle, Price: s.Price, IDCAddress: s.IDCAddress,
		Status:                s.Status,
		DefaultOutboundTag:    s.DefaultOutboundTag,
		RoutingDomainStrategy: s.RoutingDomainStrategy,
		AgentVersion:          s.AgentVersion,
		XrayRunning:           s.XrayRunning,
		XrayState:             s.XrayState,
		XrayLastError:         s.XrayLastError,
		XrayErrorAt:           s.XrayErrorAt,
		XrayFailures:          s.XrayFailures,
		XrayDiskHash:          s.XrayDiskHash,
		XrayRunningHash:       s.XrayRunningHash,
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
	// 一次查询所有待推送配置状态（含最近一次失败原因与已应用内容哈希，面板直接展示）
	type pendRow struct {
		ServerID      uint64
		Status        string
		LastError     string
		Attempts      int
		LastAttemptAt *time.Time
		AppliedHash   string
	}
	statusMap := map[uint64]pendRow{}
	var pends []pendRow
	if err := d.DB.Model(&models.PendingConfig{}).
		Select("server_id", "status", "last_error", "attempts", "last_attempt_at", "applied_hash").
		Find(&pends).Error; err == nil {
		for _, p := range pends {
			statusMap[p.ServerID] = p
		}
	}
	items := make([]serverView, 0, len(list))
	for i := range list {
		v := toServerView(&list[i])
		// 在线状态以网关注册表为唯一事实来源双向覆盖：DB status 仅作 Hub 为空时的兜底。
		// 只升不降的话，master 崩溃重启（unregister 未执行）后残留的 status=1 会让节点"永远在线"。
		if d.Hub != nil {
			v.Status = 0
			if d.Hub.IsOnline(v.ID) {
				v.Status = 1
			}
		}
		pend, hasPend := statusMap[list[i].ID]
		if hasPend {
			v.ConfigStatus = pend.Status
			if pend.Status == "pending" {
				v.PushError = pend.LastError
				v.PushAttempts = pend.Attempts
				v.PushLastTryAt = pend.LastAttemptAt
			}
			// 磁盘偏离：节点上报的磁盘内容哈希 ≠ 主控记录的「节点磁盘应有的内容」哈希
			// （第三方改过节点配置 / 热更落盘静默失败 / 节点回退过配置）。两边都有值才判定，
			// 避免旧 agent（不上报哈希）或迁移前数据（无 applied_hash）误报。
			if v.XrayDiskHash != "" && pend.AppliedHash != "" && v.XrayDiskHash != pend.AppliedHash {
				v.ConfigDrift = true
			}
		}
		// 面板状态由后端推导（前端不靠字符串猜）：四态 + 磁盘偏离
		v.PushState = pushStateOf(v.ConfigDrift, hasPend, v.ConfigStatus, v.PushError, v.Status)
		items = append(items, v)
	}
	util.OK(c, gin.H{"items": items})
}

// pushStateOf 推导面板的推送状态（五态）：drift > none > synced > rejected > pending。
// 抽成独立函数供单元测试直接覆盖——面板状态是运维判断"配置到底生效没有"的唯一依据，
// 之前内联在 AdminServers 里只能靠复制一份逻辑来测，等于没测。
func pushStateOf(configDrift bool, hasPend bool, configStatus, pushError string, serverStatus int) string {
	switch {
	case configDrift:
		return "drift"
	case !hasPend:
		return "none"
	case configStatus == "pushed":
		return "synced"
	case pushError != "" && serverStatus == 1:
		// 节点在线却推失败 = 节点拒绝（附原因）；离线导致的重推不算拒绝
		return "rejected"
	default:
		return "pending"
	}
}

// parseExpireDate 归一 VPS 到期日为纯日历日 YYYY-MM-DD（返回空串表示未设置）。
//
// 到期日是"哪一天"而不是某一瞬，故全程不存时刻、不做时区换算——一旦存成时刻，
// 同一日期在不同展示时区会漂成前后一天。接受 YYYY-MM-DD（含未补零写法与 / 分隔），
// 也兼容旧客户端发来的完整时间戳（取其 UTC 日期部分）；非法日期（如 2 月 30 日）报错。
func parseExpireDate(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if i := strings.IndexAny(s, "T "); i > 0 { // 完整时间戳：取日期部分
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "/", "-")
	for _, layout := range []string{"2006-01-02", "2006-1-2"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("无法解析到期日期: %s", s)
}

func (d *Deps) AdminCreateServer(c *gin.Context) {
	var req struct {
		ServerType            string          `json:"server_type"`
		Name                  string          `json:"name" binding:"required,max=64"`
		Host                  string          `json:"host" binding:"required,max=255"`
		Location              string          `json:"location" binding:"max=64"`
		Remark                string          `json:"remark" binding:"max=255"`
		ExpireAt              json.RawMessage `json:"expire_at"`
		BillingCycle          string          `json:"billing_cycle" binding:"max=32"`
		Price                 string          `json:"price" binding:"max=64"`
		IDCAddress            string          `json:"idc_address" binding:"max=255"`
		IDCURL                string          `json:"idc_url" binding:"max=255"` // 兼容别名
		DefaultOutboundTag    string          `json:"default_outbound_tag"`
		RoutingDomainStrategy string          `json:"routing_domain_strategy"`
	}
	if !util.BindJSON(c, &req) {
		return
	}
	expireDate, err := parseExpireDate(req.ExpireAt)
	if err != nil {
		util.BadRequest(c, "到期日期格式错误（需 YYYY-MM-DD）")
		return
	}
	idcAddr := strings.TrimSpace(req.IDCAddress)
	if idcAddr == "" && req.IDCURL != "" {
		idcAddr = strings.TrimSpace(req.IDCURL)
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
		ExpireAt:              expireDate,
		BillingCycle:          strings.TrimSpace(req.BillingCycle),
		Price:                 strings.TrimSpace(req.Price),
		IDCAddress:            idcAddr,
		Status:                0,
		DefaultOutboundTag:    req.DefaultOutboundTag,
		RoutingDomainStrategy: req.RoutingDomainStrategy,
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
		util.BadRequest(c, "无效的 ID")
		return
	}
	var srv models.Server
	if err := d.DB.First(&srv, id).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}
	var req struct {
		ServerType            *string         `json:"server_type"`
		Name                  *string         `json:"name"`
		Host                  *string         `json:"host"`
		Location              *string         `json:"location"`
		Remark                *string         `json:"remark"`
		ExpireAt              json.RawMessage `json:"expire_at"` // 三元：缺省不更新 / null 或空串清空 / 字符串更新
		BillingCycle          *string         `json:"billing_cycle"`
		Price                 *string         `json:"price"`
		IDCAddress            *string         `json:"idc_address"`
		IDCURL                *string         `json:"idc_url"`
		DefaultOutboundTag    *string         `json:"default_outbound_tag"`
		RoutingDomainStrategy *string         `json:"routing_domain_strategy"`
	}
	if !util.BindJSON(c, &req) {
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
			util.BadRequest(c, "服务器地址不能为空")
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
	if req.ExpireAt != nil && len(req.ExpireAt) > 0 {
		// null / 空串都归一为 ""（清空），非空则为 YYYY-MM-DD
		exp, err := parseExpireDate(req.ExpireAt)
		if err != nil {
			util.BadRequest(c, "到期日期格式错误（需 YYYY-MM-DD）")
			return
		}
		updates["expire_at"] = exp
	}
	if req.BillingCycle != nil {
		updates["billing_cycle"] = strings.TrimSpace(*req.BillingCycle)
	}
	if req.Price != nil {
		updates["price"] = strings.TrimSpace(*req.Price)
	}
	if req.IDCAddress != nil {
		updates["idc_address"] = strings.TrimSpace(*req.IDCAddress)
	} else if req.IDCURL != nil {
		updates["idc_address"] = strings.TrimSpace(*req.IDCURL)
	}
	if req.DefaultOutboundTag != nil {
		updates["default_outbound_tag"] = *req.DefaultOutboundTag
	}
	if req.RoutingDomainStrategy != nil {
		updates["routing_domain_strategy"] = *req.RoutingDomainStrategy
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
		util.BadRequest(c, "无效的 ID")
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
	// 旧密钥已被替换，主动断开当前在网长连接促使其重新使用新密钥握手（若使用旧密钥则握手失败）
	if d.Hub != nil {
		d.Hub.Disconnect(id)
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
		util.BadRequest(c, "无效的 ID")
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
	// 服务器已从数据库删除，主动断开现有长连接并熔断指令等待
	if d.Hub != nil {
		d.Hub.Disconnect(id)
	}
	util.OK(c, gin.H{"deleted": id})
}

// AdminServerCommand POST /api/v1/admin/servers/:id/command
// body: {"type":"push_config|restart_xray|get_status|get_logs|upgrade_agent", "config_json": "...", "lines": 100}
func (d *Deps) AdminServerCommand(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 ID")
		return
	}
	var req struct {
		Type       string `json:"type" binding:"required"`
		ConfigJSON string `json:"config_json"`
		Lines      int    `json:"lines"`
		Target     string `json:"target"`
		Force      bool   `json:"force"`
	}
	if !util.BindJSON(c, &req) {
		return
	}

	var payload any
	var actionName, target string
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
	case protocol.MsgUpgradeAgent:
		target = strings.TrimSpace(req.Target)
		if target != "" && !agentTagPattern.MatchString(target) {
			util.BadRequest(c, "目标版本号格式不正确（示例：v0.1.15）")
			return
		}
		if target == "" {
			if latest, _, err := d.GetCachedAgentLatestVersion(c.Request.Context(), false); err == nil && latest != "" {
				target = latest
			}
		}

		// 对比节点当前版本与目标版本：未指定 force 时若当前已是最新或更高，直接返回
		var srv models.Server
		actionName = "自升级"
		if err := d.DB.First(&srv, id).Error; err == nil {
			if srv.AgentVersion != "" && target != "" && CompareAgentVersion(srv.AgentVersion, target) > 0 {
				actionName = "回滚"
			}
			if req.Force {
				actionName = "回滚"
				// <= v0.1.16 的旧 agent 未知 force 字段且硬编码 Compare>=0 拒绝，无法在线执行回滚
				if srv.AgentVersion != "" && srv.AgentVersion != "dev" && CompareAgentVersion(srv.AgentVersion, "v0.1.17") < 0 {
					util.Fail(c, 400, fmt.Sprintf("服务器当前 Agent 版本 %s 不支持在线回滚（需 v0.1.17 及以上版本），请在服务器执行 xray-agent rollback 或重新运行安装脚本", srv.AgentVersion))
					return
				}
				if srv.AgentVersion != "" && target != "" && CompareAgentVersion(srv.AgentVersion, target) == 0 {
					util.Fail(c, 400, fmt.Sprintf("服务器当前已是版本 %s，无需回滚", srv.AgentVersion))
					return
				}
			}
			if !req.Force && srv.AgentVersion != "" && target != "" && CompareAgentVersion(srv.AgentVersion, target) >= 0 {
				util.OK(c, gin.H{
					"ok":   true,
					"data": fmt.Sprintf("服务器当前已是最新版本 %s（目标 %s），无需升级", srv.AgentVersion, target),
				})
				return
			}
		}
		if d.Hub != nil {
			d.Hub.SetUpgradeStatus(id, &protocol.UpgradeProgressPayload{
				Phase:   "starting",
				Target:  target,
				Message: fmt.Sprintf("正在向服务器下发%s指令…", actionName),
				TS:      time.Now().Unix(),
			})
		}
		payload = protocol.UpgradeAgentPayload{Target: target, Force: req.Force}
	default:
		util.BadRequest(c, "不支持的指令类型")
		return
	}

	// 自升级的回执要等节点从 GitHub 拉完二进制才发，用专用的长超时
	askTimeout := nodegate.AskTimeout
	if req.Type == protocol.MsgUpgradeAgent {
		askTimeout = nodegate.UpgradeAskTimeout
	}
	res, err := d.Hub.Ask(id, req.Type, payload, askTimeout)
	if err != nil {
		if req.Type == protocol.MsgUpgradeAgent && d.Hub != nil {
			d.Hub.SetUpgradeStatus(id, &protocol.UpgradeProgressPayload{
				Phase:   "failed",
				Target:  target,
				Message: actionName + "指令超时或失败",
				Error:   err.Error(),
				TS:      time.Now().Unix(),
			})
		}
		util.Fail(c, 502, "指令失败: "+err.Error())
		return
	}
	if req.Type == protocol.MsgUpgradeAgent && d.Hub != nil {
		isRollback := req.Force || actionName == "回滚"
		msg := actionName + "完成"
		if s, ok := res.Data.(string); ok && s != "" {
			msg = s
		}
		// 若为回滚，但节点回复"已是最新版本"或"无需操作"，说明节点未实际执行回滚，不得报告成功
		noOp := isRollback && (strings.Contains(msg, "已是最新版本") || strings.Contains(msg, "无需升级") || strings.Contains(msg, "无需操作"))
		if !res.OK || noOp {
			errMsg := res.Error
			if noOp && errMsg == "" {
				errMsg = "服务器未执行回滚操作（" + msg + "）"
			}
			d.Hub.SetUpgradeStatus(id, &protocol.UpgradeProgressPayload{
				Phase:   "failed",
				Target:  target,
				Message: actionName + "失败",
				Error:   errMsg,
				TS:      time.Now().Unix(),
			})
			util.OK(c, gin.H{"ok": false, "error": errMsg, "data": res.Data})
			return
		}
		d.Hub.SetUpgradeStatus(id, &protocol.UpgradeProgressPayload{
			Phase:   "success",
			Target:  target,
			Message: msg,
			TS:      time.Now().Unix(),
		})
	}
	util.OK(c, gin.H{"ok": res.OK, "error": res.Error, "data": res.Data})
}

// AdminGetServerUpgradeStatus GET /api/v1/admin/servers/:id/upgrade-status
// 查询节点当前的 Agent 升级进度与状态。
func (d *Deps) AdminGetServerUpgradeStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 ID")
		return
	}
	if d.Hub == nil {
		util.OK(c, gin.H{"status": nil})
		return
	}
	st := d.Hub.GetUpgradeStatus(id)
	util.OK(c, gin.H{"status": st})
}

// AdminGetUpgradeStatuses GET /api/v1/admin/servers/upgrade-status?ids=1,2
// 批量查询升级进度快照（ids 省略 = 全部有记录的节点），供批量升级总览轮询。
func (d *Deps) AdminGetUpgradeStatuses(c *gin.Context) {
	var ids []uint64
	if v := c.Query("ids"); v != "" {
		for _, p := range strings.Split(v, ",") {
			if id, err := strconv.ParseUint(strings.TrimSpace(p), 10, 64); err == nil {
				ids = append(ids, id)
			}
		}
	}
	if d.Hub == nil {
		util.OK(c, gin.H{"statuses": map[uint64]*protocol.UpgradeProgressPayload{}})
		return
	}
	util.OK(c, gin.H{"statuses": d.Hub.GetUpgradeStatuses(ids)})
}

// AdminBatchUpgradeServers POST /api/v1/admin/servers/batch-upgrade
// body: {"ids":[...], "target":"v0.1.x", "force":false}
// 预检（不存在/离线/已最新直接跳过并说明原因）后立即返回，逐节点 goroutine 异步执行升级指令；
// 节点自行下载二进制并上报 MsgUpgradeProgress，进度经 GET /servers/upgrade-status 聚合轮询。
func (d *Deps) AdminBatchUpgradeServers(c *gin.Context) {
	var req struct {
		IDs    []uint64 `json:"ids" binding:"required,min=1,max=100"`
		Target string   `json:"target"`
		Force  bool     `json:"force"`
	}
	if !util.BindJSON(c, &req) {
		return
	}
	if d.Hub == nil {
		util.ServerError(c, "节点网关未初始化")
		return
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		if latest, _, err := d.GetCachedAgentLatestVersion(c.Request.Context(), false); err == nil && latest != "" {
			target = latest
		}
	}
	if target == "" {
		util.ServerError(c, "无法确定目标版本（官方最新版本查询失败，可稍后重试）")
		return
	}

	type batchItem struct {
		ID   uint64 `json:"id"`
		Name string `json:"name"`
	}
	type batchSkip struct {
		ID     uint64 `json:"id"`
		Name   string `json:"name"`
		Reason string `json:"reason"`
	}
	var dispatch []batchItem
	var skipped []batchSkip
	seen := make(map[uint64]bool, len(req.IDs))
	for _, id := range req.IDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		var srv models.Server
		if err := d.DB.First(&srv, id).Error; err != nil {
			skipped = append(skipped, batchSkip{ID: id, Reason: "服务器不存在"})
			continue
		}
		if !d.Hub.IsOnline(id) {
			skipped = append(skipped, batchSkip{ID: id, Name: srv.Name, Reason: "服务器离线"})
			continue
		}
		if !req.Force && srv.AgentVersion != "" && CompareAgentVersion(srv.AgentVersion, target) >= 0 {
			skipped = append(skipped, batchSkip{ID: id, Name: srv.Name, Reason: "已是最新版本 " + srv.AgentVersion})
			continue
		}
		d.Hub.SetUpgradeStatus(id, &protocol.UpgradeProgressPayload{
			Phase:   "starting",
			Target:  target,
			Message: "已加入批量升级队列，等待下发自升级指令…",
			TS:      time.Now().Unix(),
		})
		dispatch = append(dispatch, batchItem{ID: id, Name: srv.Name})
	}

	for _, it := range dispatch {
		go func(id uint64) {
			// 回执要等节点从 GitHub 拉完二进制才发（同单台升级的专用长超时）
			res, err := d.Hub.Ask(id, protocol.MsgUpgradeAgent, protocol.UpgradeAgentPayload{Target: target, Force: req.Force}, nodegate.UpgradeAskTimeout)
			if err != nil {
				d.Hub.SetUpgradeStatus(id, &protocol.UpgradeProgressPayload{
					Phase: "failed", Target: target, Message: "升级指令超时或失败", Error: err.Error(), TS: time.Now().Unix(),
				})
				return
			}
			if !res.OK {
				d.Hub.SetUpgradeStatus(id, &protocol.UpgradeProgressPayload{
					Phase: "failed", Target: target, Message: "升级失败", Error: res.Error, TS: time.Now().Unix(),
				})
				return
			}
			msg := "升级完成"
			if s, ok := res.Data.(string); ok && s != "" {
				msg = s
			}
			d.Hub.SetUpgradeStatus(id, &protocol.UpgradeProgressPayload{
				Phase: "success", Target: target, Message: msg, TS: time.Now().Unix(),
			})
		}(it.ID)
	}

	util.OK(c, gin.H{"target": target, "dispatched": dispatch, "skipped": skipped})
}

// AdminGetServerConfigPreview GET /api/v1/admin/servers/:id/config-preview
// 实时渲染该服务器按当前数据库预期应该推送到节点的完整 Xray 配置（只读预览，无网络副作用）。
func (d *Deps) AdminGetServerConfigPreview(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 ID")
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
		util.BadRequest(c, "无效的 ID")
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
			"message": "服务器离线，配置已保存，上线后自动推送",
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
		"message": "配置已保存，正在推送到服务器",
		"config":  cfgStr,
	})
}

// AdminServerMetrics GET /api/v1/admin/servers/:id/metrics —— 查询节点时序监控数据 (1h/6h/24h/7d/30d)。
// 每档的读桶不小于存储分辨率（见 services.CompactNodeReports 的分级降采样档位），
// 否则桶内无真实采样点、只能靠前向填充造曲线。
// 每档同时给出桶内均值与桶内峰值：均值看趋势，峰值看容量（带宽/CPU 的瞬时尖峰是均值抹掉的）。
func (d *Deps) AdminServerMetrics(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 ID")
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
		// 30m 桶（而非 1h）：降采样档位放宽到 10m 后，桶内仍有 3 个真实采样点，
		// 均值不再是「某一个瞬间的读数」，点数 336 也仍在图表可读范围内。
		startTime = now.AddDate(0, 0, -7)
		bucketDuration = 30 * time.Minute
		timeFmt = "01-02 15:00"
	case "30d":
		// 覆盖 node_reports 的完整保留期（默认 30 天）；1h 桶 ≈ 720 点。
		startTime = now.AddDate(0, 0, -30)
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
		timeStr   string
		cpuSum    float64
		memSum    float64
		memTotal  uint64
		diskSum   float64
		diskTotal uint64
		rxRateSum float64
		txRateSum float64
		usersSum  int
		count     int
		// 桶内峰值（原始采样点的最大值）。count==0 时无意义，由首个采样点直接初始化，
		// 不用 0 当哨兵——CPU/速率本就可能为 0，哨兵会让真实 0 值被 max 判成「未初始化」。
		cpuMax    float64
		memMax    float64
		diskMax   float64
		rxRateMax float64
		txRateMax float64
		usersMax  int
	}

	buckets := make([]bucketAgg, numBuckets)
	timestampsISO := make([]string, numBuckets)
	for i := 0; i < numBuckets; i++ {
		bTime := startTime.Add(time.Duration(i) * bucketDuration)
		buckets[i].timeStr = bTime.Format(timeFmt)
		// 原始 UTC 时间戳：供前端按「显示时区」渲染坐标轴，避免坐标轴被服务器时区决定。
		timestampsISO[i] = bTime.UTC().Format(time.RFC3339)
	}

	for _, r := range reports {
		idx := int(r.ReportedAt.Sub(startTime) / bucketDuration)
		if idx >= 0 && idx < numBuckets {
			b := &buckets[idx]
			if b.count == 0 {
				b.cpuMax, b.memMax, b.diskMax = r.CPU, r.Mem, r.Disk
				b.rxRateMax, b.txRateMax = r.RxRate, r.TxRate
				b.usersMax = r.OnlineUsers
			} else {
				b.cpuMax = math.Max(b.cpuMax, r.CPU)
				b.memMax = math.Max(b.memMax, r.Mem)
				b.diskMax = math.Max(b.diskMax, r.Disk)
				b.rxRateMax = math.Max(b.rxRateMax, r.RxRate)
				b.txRateMax = math.Max(b.txRateMax, r.TxRate)
				if r.OnlineUsers > b.usersMax {
					b.usersMax = r.OnlineUsers
				}
			}
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
	cpuMaxList := make([]float64, numBuckets)
	memPercentMaxList := make([]float64, numBuckets)
	diskPercentMaxList := make([]float64, numBuckets)
	rxMbpsMaxList := make([]float64, numBuckets)
	txMbpsMaxList := make([]float64, numBuckets)
	usersMaxList := make([]int, numBuckets)

	// 已用字节 / 总量 → 百分比，1 位小数、封顶 100。均值与峰值共用，保证两条线同口径可比。
	pctOf := func(used float64, total uint64) float64 {
		if total == 0 {
			return 0
		}
		pct := used / float64(total) * 100
		if pct > 100 {
			pct = 100
		}
		return float64(int(pct*10)) / 10
	}
	// 字节/秒 -> Mbps (8 / 1,000,000)，2 位小数。
	mbpsOf := func(bytesPerSec float64) float64 {
		return float64(int(bytesPerSec*8/1_000_000*100)) / 100
	}

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
			memPercentList[i] = pctOf(mVal, lastMemTotal)
			diskPercentList[i] = pctOf(dVal, lastDiskTotal)
			rxMbpsList[i] = mbpsOf(b.rxRateSum / float64(b.count))
			txMbpsList[i] = mbpsOf(b.txRateSum / float64(b.count))
			usersList[i] = b.usersSum / b.count
			// 峰值：桶内原始采样点的最大值，不参与平均。
			cpuMaxList[i] = float64(int(b.cpuMax*10)) / 10
			memPercentMaxList[i] = pctOf(b.memMax, lastMemTotal)
			diskPercentMaxList[i] = pctOf(b.diskMax, lastDiskTotal)
			rxMbpsMaxList[i] = mbpsOf(b.rxRateMax)
			txMbpsMaxList[i] = mbpsOf(b.txRateMax)
			usersMaxList[i] = b.usersMax
		} else if i > 0 {
			// 空桶前向填充：保持曲线连续，避免无上报的时段被画成 0（会误读为「无流量/无负载」）。
			// 峰值同样前向填充——本档位下空桶意味着该区间没有采样点，任何取值都只是占位。
			cpuList[i] = cpuList[i-1]
			memUsedList[i] = memUsedList[i-1]
			memPercentList[i] = memPercentList[i-1]
			diskPercentList[i] = diskPercentList[i-1]
			rxMbpsList[i] = rxMbpsList[i-1]
			txMbpsList[i] = txMbpsList[i-1]
			usersList[i] = usersList[i-1]
			cpuMaxList[i] = cpuMaxList[i-1]
			memPercentMaxList[i] = memPercentMaxList[i-1]
			diskPercentMaxList[i] = diskPercentMaxList[i-1]
			rxMbpsMaxList[i] = rxMbpsMaxList[i-1]
			txMbpsMaxList[i] = txMbpsMaxList[i-1]
			usersMaxList[i] = usersMaxList[i-1]
		}
	}

	util.OK(c, gin.H{
		"server_id":      id,
		"server_name":    srv.Name,
		"host":           srv.Host,
		"location":       srv.Location,
		"range":          timeRange,
		"timestamps":     timestamps,
		"timestamps_iso": timestampsISO,
		"cpu":            cpuList,
		"mem_percent":    memPercentList,
		"mem_used":       memUsedList,
		"mem_total":      lastMemTotal,
		"disk_percent":   diskPercentList,
		"disk_total":     lastDiskTotal,
		"rx_mbps":        rxMbpsList,
		"tx_mbps":        txMbpsList,
		"online_users":   usersList,
		// 峰值（桶内原始采样点最大值，同单位同口径，与上面的均值一一对应）
		"cpu_max":          cpuMaxList,
		"mem_percent_max":  memPercentMaxList,
		"disk_percent_max": diskPercentMaxList,
		"rx_mbps_max":      rxMbpsMaxList,
		"tx_mbps_max":      txMbpsMaxList,
		"online_users_max": usersMaxList,
	})
}

// onlineIPEntry 在线用户条目（email 已归类：user=面板用户（已按用户合并去重、
// 回填真实邮箱）/ relay=中转内部账户 / other=自定义 email）。
type onlineIPEntry struct {
	Email  string   `json:"email"`
	Kind   string   `json:"kind"`
	Name   string   `json:"name,omitempty"` // kind=user 时的面板用户名
	UserID uint64   `json:"user_id,omitempty"`
	IPs    []string `json:"ips"`
}

// mergeIPs 把 add 并入 dst（按 IP 去重，保序）。
func mergeIPs(dst, add []string) []string {
	seen := make(map[string]struct{}, len(dst)+len(add))
	for _, ip := range dst {
		seen[ip] = struct{}{}
	}
	for _, ip := range add {
		if ip == "" {
			continue
		}
		if _, dup := seen[ip]; dup {
			continue
		}
		seen[ip] = struct{}{}
		dst = append(dst, ip)
	}
	return dst
}

// AdminServerOnlineIPs GET /api/v1/admin/servers/:id/online-ips —— 节点当前在线用户
// 与连接源 IP（agent 心跳的最新快照；users 为空 = 无人在线或 agent 版本过旧未上报）。
// 面板用户按统计键反解归类并合并：按入站区分统计键后同一用户每入站一个 email 条目，
// 合并为一行（IP 取并集）；email 回填用户真实邮箱，便于管理员辨认。
func (d *Deps) AdminServerOnlineIPs(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 ID")
		return
	}
	var srv models.Server
	if err := d.DB.First(&srv, id).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}

	var raw []protocol.OnlineUserIPs
	if srv.OnlineIPs != "" && srv.OnlineIPs != "null" {
		_ = json.Unmarshal([]byte(srv.OnlineIPs), &raw)
	}

	entries := make([]onlineIPEntry, 0, len(raw))
	userIdx := make(map[uint64]int, len(raw))
	for _, e := range raw {
		if uid, _, ok := xray.ParseUserEmailAny(e.Email); ok {
			// 同一用户跨入站的多条快照合并为一行（IP 并集去重）
			if idx, dup := userIdx[uid]; dup {
				entries[idx].IPs = mergeIPs(entries[idx].IPs, e.IPs)
				continue
			}
			userIdx[uid] = len(entries)
			entries = append(entries, onlineIPEntry{Email: e.Email, Kind: "user", UserID: uid, IPs: e.IPs})
			continue
		}
		entry := onlineIPEntry{Email: e.Email, Kind: "other", IPs: e.IPs}
		if strings.HasPrefix(e.Email, "relay-") && strings.HasSuffix(e.Email, "@panel.local") {
			entry.Kind = "relay" // 中转内部账户（xray.RelayEmail），非面板用户
		}
		entries = append(entries, entry)
	}

	if len(userIdx) > 0 {
		ids := make([]uint64, 0, len(userIdx))
		for uid := range userIdx {
			ids = append(ids, uid)
		}
		var users []models.User
		d.DB.Select("id, username, email").Where("id IN ?", ids).Find(&users)
		byID := make(map[uint64]models.User, len(users))
		for _, u := range users {
			byID[u.ID] = u
		}
		for i := range entries {
			if entries[i].Kind != "user" {
				continue
			}
			if u, ok := byID[entries[i].UserID]; ok {
				entries[i].Name = u.Username
				if u.Email != "" {
					entries[i].Email = u.Email // 统计键是合成格式，展示回填真实邮箱
				}
			}
		}
	}

	util.OK(c, gin.H{"online_users": len(entries), "users": entries})
}
