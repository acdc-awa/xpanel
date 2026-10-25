package api

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/acdc-awa/xpanel/internal/master/protocols"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// inboundView 入站对外结构。
type inboundView struct {
	ID                 uint64     `json:"id"`
	ServerID           uint64     `json:"server_id"`
	ServerName         string     `json:"server_name"`
	Tag                string     `json:"tag"`
	Protocol           string     `json:"protocol"`
	Port               int        `json:"port"`
	Listen             string     `json:"listen"`
	SettingsJSON       string     `json:"settings_json"`
	StreamSettings     string     `json:"stream_settings"`
	Sniffing           string     `json:"sniffing"`
	Ratio              float64    `json:"ratio"`
	TotalGB            int64      `json:"total_gb"`
	ExpiryTime         *time.Time `json:"expiry_time,omitempty"`
	Enabled            bool       `json:"enabled"`
	Type               string     `json:"type"`                        // user / relay / tunnel / forward
	InternalUUID       string     `json:"internal_uuid,omitempty"`     // relay 只读（节点上报）
	TargetInboundID    *uint64    `json:"target_inbound_id,omitempty"` // 四层直通目标落地入站 ID
	TargetAddress      string     `json:"target_address,omitempty"`    // 手动指定的外部目标地址
	TargetPort         int        `json:"target_port,omitempty"`       // 手动指定的外部目标端口
	CertID             *uint64    `json:"cert_id,omitempty"`           // 绑定的证书
	Flow               string     `json:"flow"`                        // 入站级流控（空=自动 / xtls-rprx-vision / none）
	ShareAddrStrategy  string     `json:"share_addr_strategy"`         // node / custom（订阅专用）
	ShareAddr          string     `json:"share_addr"`                  // 自定义分享地址（订阅专用，域名/IP）
	SharePort          int        `json:"share_port"`                  // 自定义分享端口（0 = 使用入站端口）
	ShareSNI           string     `json:"share_sni"`                   // 订阅 SNI 覆写
	ShareAllowInsecure bool       `json:"share_allow_insecure"`        // 订阅跳过证书校验
	LayerID            *uint64    `json:"layer_id,omitempty"`          // 所属对外接入层（空/0 = 直连自持端点）
	Remark             string     `json:"remark,omitempty"`            // 入站备注/通道名称
	TrafficReset       string     `json:"traffic_reset,omitempty"`     // 重置周期
	CreatedAt          time.Time  `json:"created_at"`
}

// inboundForm 入站创建/更新表单（透传 JSON）。
type inboundForm struct {
	ID                 uint64     `json:"id"` // 预览编辑已存在入站时透传（AP 授权解析用；创建时忽略）
	ServerID           uint64     `json:"server_id" binding:"required"`
	Tag                string     `json:"tag" binding:"required,max=64"`
	Protocol           string     `json:"protocol" binding:"required"`
	Port               int        `json:"port" binding:"required,min=1,max=65535"`
	Listen             string     `json:"listen"`
	Remark             string     `json:"remark"`          // 入站备注/通道名称
	TrafficReset       string     `json:"traffic_reset"`   // 重置周期 (never / daily / weekly / monthly)
	SettingsJSON       string     `json:"settings_json"`   // 协议 settings（透传）
	StreamSettings     string     `json:"stream_settings"` // 传输 streamSettings（透传）
	Sniffing           string     `json:"sniffing"`        // 嗅探（透传）
	Ratio              float64    `json:"ratio"`
	TotalGB            int64      `json:"total_gb"`              // J9：入站总流量上限（GB，0=不限）
	ExpiryTime         *time.Time `json:"expiry_time,omitempty"` // J9：入站到期时间
	Type               string     `json:"type"`                  // user / relay / tunnel / forward
	TargetInboundID    *uint64    `json:"target_inbound_id"`     // 四层直通目标落地入站 ID
	TargetAddress      string     `json:"target_address"`        // 手动指定的外部目标地址
	TargetPort         int        `json:"target_port"`           // 手动指定的外部目标端口
	CertID             *uint64    `json:"cert_id"`               // 绑定证书（T5 校验存在性）
	Flow               string     `json:"flow"`                  // 入站级流控（空=自动 / xtls-rprx-vision / none）
	ShareAddrStrategy  string     `json:"share_addr_strategy"`   // node / custom（订阅专用，listen 已退役）
	ShareAddr          string     `json:"share_addr"`            // 自定义分享地址（订阅专用，域名/IP）
	SharePort          int        `json:"share_port"`            // 自定义分享端口（0 = 使用入站端口）
	ShareSNI           string     `json:"share_sni"`
	ShareAllowInsecure bool       `json:"share_allow_insecure"`
	LayerID            *uint64    `json:"layer_id"` // 所属对外接入层（nil/0 = 直连；仅同一服务器的层有效）
}

func normalizeInboundType(protocol, currentType string) string {
	switch protocol {
	case models.ProtocolTunnel:
		return models.InboundTypeTunnel
	default:
		return models.InboundTypeUser
	}
}

func toInboundView(i *models.Inbound, serverName string) inboundView {
	return inboundView{
		ID: i.ID, ServerID: i.ServerID, ServerName: serverName,
		Tag: i.Tag, Protocol: i.Protocol, Port: i.Port,
		Listen: i.Listen, SettingsJSON: i.SettingsJSON,
		StreamSettings: i.StreamSettings, Sniffing: i.Sniffing,
		Ratio: i.Ratio, TotalGB: i.Total, ExpiryTime: i.ExpiryTime,
		Enabled: i.Enabled, CreatedAt: i.CreatedAt,
		Type: i.Type, InternalUUID: i.InternalUUID,
		TargetInboundID: i.TargetInboundID, TargetAddress: i.TargetAddress, TargetPort: i.TargetPort,
		CertID: i.CertID, Flow: i.Flow, ShareAddrStrategy: i.ShareAddrStrategy, ShareAddr: i.ShareAddr,
		SharePort: i.SharePort,
		ShareSNI:  i.ShareSNI, ShareAllowInsecure: i.ShareAllowInsecure,
		LayerID: i.LayerID,
		Remark:  i.Remark, TrafficReset: i.TrafficReset,
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
	if typ := c.Query("type"); typ != "" {
		q = q.Where("type = ?", typ)
	}
	var list []models.Inbound
	if err := q.Order("id DESC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}

	serverMap := make(map[uint64]string)
	if len(list) > 0 {
		var sids []uint64
		for _, item := range list {
			sids = append(sids, item.ServerID)
		}
		var servers []models.Server
		d.DB.Where("id IN ?", sids).Find(&servers)
		for _, s := range servers {
			serverMap[s.ID] = s.Name
		}
	}

	items := make([]inboundView, 0, len(list))
	for i := range list {
		v := toInboundView(&list[i], serverMap[list[i].ServerID])
		items = append(items, v)
	}
	util.OK(c, gin.H{"items": items})
}

// AdminCreateInbound POST /api/v1/admin/inbounds
func (d *Deps) AdminCreateInbound(c *gin.Context) {
	var req inboundForm
	if !util.BindJSON(c, &req) {
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
	if req.Type != "" && !validInboundType(req.Type) {
		util.BadRequest(c, "入站类型仅支持 user / relay / tunnel / forward")
		return
	}

	// 协议与类型权威归一化
	req.Type = normalizeInboundType(req.Protocol, req.Type)
	effType := req.Type
	if effType == models.InboundTypeTunnel {
		req.Protocol = models.ProtocolTunnel
	}
	if effType != models.InboundTypeTunnel {
		if err := checkVlessDecryption(req.Protocol, effType, req.SettingsJSON); err != nil {
			util.BadRequest(c, err.Error())
			return
		}
	}
	// U15：创建校验闭环——协议白名单 / tag 非空且同服务器唯一 / CertID 存在性
	if !validInboundProtocol(req.Protocol) {
		util.BadRequest(c, "不支持的协议: "+req.Protocol+"（支持 vless / hysteria / tunnel）")
		return
	}
	// TLS 入站必须有服务端证书来源（托管证书或成对路径），否则节点加载配置直接失败。
	if err := xray.ValidateTLSCertSource(req.StreamSettings, req.CertID, req.Protocol); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	if strings.TrimSpace(req.Tag) == "" || len(req.Tag) > 64 {
		util.BadRequest(c, "入站标签需为 1-64 字符")
		return
	}
	var tagCnt int64
	d.DB.Model(&models.Inbound{}).Where("server_id = ? AND tag = ?", req.ServerID, req.Tag).Count(&tagCnt)
	if tagCnt > 0 {
		util.BadRequest(c, "该服务器上已存在同名入站标签")
		return
	}
	// 目标存在性校验；acceptProxyProtocol 配对挪到主记录写库成功之后执行，
	// 避免后续校验失败（400）时落地入站已被改写并重推配置。
	var ensureTarget *models.Inbound
	if req.TargetInboundID != nil && *req.TargetInboundID > 0 {
		var targetInb models.Inbound
		if err := d.DB.First(&targetInb, *req.TargetInboundID).Error; err != nil {
			util.BadRequest(c, "目标落地入站不存在")
			return
		}
		ensureTarget = &targetInb
	}
	if req.TargetPort < 0 || req.TargetPort > 65535 {
		util.BadRequest(c, "目标端口需在 0-65535 之间")
		return
	}
	if req.Flow != "" && !validInboundFlow(req.Flow) {
		util.BadRequest(c, "入站流控仅支持空（自动）/ xtls-rprx-vision / none")
		return
	}
	if req.ShareAddrStrategy != "" && !validShareAddrStrategy(req.ShareAddrStrategy) {
		util.BadRequest(c, "分享地址策略仅支持 node / custom")
		return
	}
	if req.SharePort < 0 || req.SharePort > 65535 {
		util.BadRequest(c, "分享端口需在 0-65535 之间（0=使用入站端口）")
		return
	}
	if req.Ratio < 0 {
		util.BadRequest(c, "流量倍率不能为负数")
		return
	}
	if req.TotalGB < 0 {
		util.BadRequest(c, "流量上限不能为负数（0=不限）")
		return
	}
	if req.CertID != nil && *req.CertID != 0 {
		var cert models.Cert
		if err := d.DB.First(&cert, *req.CertID).Error; err != nil {
			util.BadRequest(c, "绑定证书不存在")
			return
		}
	} else if req.CertID != nil {
		req.CertID = nil // 显式 0 = 未绑定，落库保持 NULL（与 layer_id 一致）
	}
	if req.LayerID != nil && *req.LayerID != 0 {
		var layer models.AccessLayer
		if err := d.DB.First(&layer, *req.LayerID).Error; err != nil {
			util.BadRequest(c, "所属对外层不存在")
			return
		}
		if layer.ServerID != req.ServerID {
			util.BadRequest(c, "所属对外层必须与入站同属一台服务器")
			return
		}
	} else if req.LayerID != nil {
		req.LayerID = nil // 显式 0 = 直连，与空一致
	}
	// 同服务器端口冲突
	var cnt int64
	d.DB.Model(&models.Inbound{}).Where("server_id = ? AND port = ?", req.ServerID, req.Port).Count(&cnt)
	if cnt > 0 {
		util.BadRequest(c, "该服务器上端口已被占用")
		return
	}
	resetPolicy := strings.ToLower(strings.TrimSpace(req.TrafficReset))
	if resetPolicy == "" {
		resetPolicy = "never"
	}
	inb := models.Inbound{
		ServerID: req.ServerID, Tag: req.Tag, Protocol: req.Protocol,
		Port: req.Port, Listen: req.Listen,
		Remark:       strings.TrimSpace(req.Remark),
		TrafficReset: resetPolicy,
		SettingsJSON: req.SettingsJSON, StreamSettings: req.StreamSettings,
		Sniffing: req.Sniffing, Ratio: req.Ratio,
		Total: req.TotalGB, ExpiryTime: utcPtr(req.ExpiryTime), Enabled: true,
		Type:            req.Type,
		TargetInboundID: req.TargetInboundID, TargetAddress: req.TargetAddress, TargetPort: req.TargetPort,
		CertID: req.CertID,
		Flow:   req.Flow, ShareAddrStrategy: req.ShareAddrStrategy, ShareAddr: req.ShareAddr,
		SharePort:          req.SharePort,
		ShareSNI:           req.ShareSNI,
		ShareAllowInsecure: req.ShareAllowInsecure, LayerID: req.LayerID,
	}
	// Type 空 = user（与模型默认一致）
	if inb.Type == "" {
		inb.Type = models.InboundTypeUser
	}
	// ShareAddrStrategy 空 = node（与模型默认一致）
	if inb.ShareAddrStrategy == "" {
		inb.ShareAddrStrategy = "node"
	}
	// 字段声明交叉规则（S4）：协议强约束与安全层互斥，返回结构化问题便于前端定位。
	if issues := protocols.ValidateInboundFields(&inb); len(issues) > 0 {
		util.BadRequestIssues(c, "入站字段校验未通过", issues)
		return
	}
	if err := d.DB.Create(&inb).Error; err != nil {
		util.ServerError(c, "创建失败")
		return
	}
	// C2：直通的派生出口 tag 早查晚截——落库前先拒绝撞名（避免建出一条永远连不上的直通），
	// 落库后立刻对账把派生出口与转发规则建出来。对账失败即回滚刚建的入站（fail-closed）。
	if services.IsTunnelInbound(&inb) {
		if msg := d.tunnelOutboundTagConflict(inb.ServerID, inb.Tag, 0); msg != "" {
			_ = d.DB.Delete(&models.Inbound{}, inb.ID).Error
			util.BadRequest(c, msg)
			return
		}
		if msg := d.ensureTunnelDerived(inb.ServerID); msg != "" {
			_ = d.DB.Delete(&models.Inbound{}, inb.ID).Error
			_ = services.EnsureTunnelDerivedObjects(d.DB, inb.ServerID)
			util.BadRequest(c, msg)
			return
		}
	}
	// P2-8 配对：全部校验通过且主记录落库后再给落地入站开 acceptProxyProtocol
	// 仅当直通出站确实发送 PROXY Protocol（TunnelSendProxyProtocol 为 true）时才确保落地入站开启
	if ensureTarget != nil && xray.TunnelSendProxyProtocol(&inb) {
		d.ensureTargetAcceptProxyProtocol(ensureTarget)
	}
	// GORM 零值陷阱：Ratio 带 default:1，Create 时显式 0（免费入站）会被当零值
	// 跳过而落成列默认 1，需补一次显式写（更新路径走 map 赋值无此问题）。
	if req.Ratio == 0 {
		if err := d.DB.Model(&models.Inbound{}).Where("id = ?", inb.ID).UpdateColumn("ratio", 0).Error; err != nil {
			util.ServerError(c, "创建失败")
			return
		}
		inb.Ratio = 0
	}
	// 绑定证书即推送：证书端点只在「证书记录变化」时推送，覆盖不到
	// 「证书先建、入站后绑」与「新节点首次引用既有证书」这两种时序。
	// 推送失败不阻断创建（配置推送会失败并提示；节点重连时 ReconcileServerCerts 兜底）。
	if inb.CertID != nil && *inb.CertID != 0 {
		if err := d.pushCertToServer(inb.ServerID, *inb.CertID); err != nil {
			log.Printf("inbounds: 创建入站 %s 时推送证书 %d 失败: %v", inb.Tag, *inb.CertID, err)
		}
	}
	if err := d.enqueueConfig(req.ServerID); err != nil {
		pushFail(c, req.ServerID, err)
		return
	}
	util.OK(c, gin.H{"inbound": toInboundView(&inb, srv.Name)})
}

// AdminUpdateInbound PUT /api/v1/admin/inbounds/:id
func (d *Deps) AdminUpdateInbound(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 ID")
		return
	}
	var inb models.Inbound
	if err := d.DB.First(&inb, id).Error; err != nil {
		util.Fail(c, 404, "入站不存在")
		return
	}
	var req struct {
		Tag                *string         `json:"tag"`
		Protocol           *string         `json:"protocol"`
		Port               *int            `json:"port"`
		Listen             *string         `json:"listen"`
		Remark             *string         `json:"remark"`
		TrafficReset       *string         `json:"traffic_reset"`
		SettingsJSON       *string         `json:"settings_json"`
		StreamSettings     *string         `json:"stream_settings"`
		Sniffing           *string         `json:"sniffing"`
		Ratio              *float64        `json:"ratio"`
		TotalGB            *int64          `json:"total_gb"`
		ExpiryTime         json.RawMessage `json:"expiry_time"` // 三元：字段缺省=不动 / null=清空 / 字符串=设置
		Enabled            *bool           `json:"enabled"`
		Type               *string         `json:"type"`
		InternalUUID       *string         `json:"internal_uuid"`     // 仅节点回执写入（管理员只读展示）
		TargetInboundID    *uint64         `json:"target_inbound_id"` // nil 不更新；显式传 0 解绑
		TargetAddress      *string         `json:"target_address"`
		TargetPort         *int            `json:"target_port"`
		CertID             *uint64         `json:"cert_id"`             // nil 不更新；显式传 0 解绑
		Flow               *string         `json:"flow"`                // 入站级流控（nil 不更新；空串=自动）
		ShareAddrStrategy  *string         `json:"share_addr_strategy"` //
		ShareAddr          *string         `json:"share_addr"`          //
		SharePort          *int            `json:"share_port"`          // 0 = 使用入站端口
		ShareSNI           *string         `json:"share_sni"`
		ShareAllowInsecure *bool           `json:"share_allow_insecure"`
		LayerID            *uint64         `json:"layer_id"` // nil 不更新；显式传 0 解绑回原生
	}
	if !util.BindJSON(c, &req) {
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
	// VLESS Encryption（vlessenc 安全层）保存期校验：按本次请求生效的 protocol/type 判定
	//（一期仅转发入站；user→relay 切换放行、relay→user 携带 encryption 则拒绝）
	effProto := inb.Protocol
	if req.Protocol != nil {
		effProto = *req.Protocol
	}
	effType := inb.Type
	if req.Type != nil {
		if !validInboundType(*req.Type) {
			util.BadRequest(c, "入站类型仅支持 user / relay / tunnel / forward")
			return
		}
		effType = *req.Type
	}
	effType = normalizeInboundType(effProto, effType)

	if effType != models.InboundTypeTunnel {
		if err := checkVlessDecryption(effProto, effType, sj); err != nil {
			util.BadRequest(c, err.Error())
			return
		}
	}

	if effProto == models.ProtocolSocks || effProto == models.ProtocolHTTP || effProto == models.ProtocolMixed {
		util.BadRequest(c, "SOCKS5 / HTTP 入站已退役，不支持的协议: "+effProto)
		return
	}
	// TLS 入站必须有服务端证书来源：本次未传 cert_id 时沿用库内既有绑定。
	effCertID := inb.CertID
	if req.CertID != nil {
		if *req.CertID == 0 {
			effCertID = nil
		} else {
			effCertID = req.CertID
		}
	}
	effCertProtocol := effProto
	if effType == models.InboundTypeTunnel {
		effCertProtocol = models.ProtocolTunnel // 直通无安全层，按终态协议判定
	}
	if err := xray.ValidateTLSCertSource(ss, effCertID, effCertProtocol); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	// U15：更新校验闭环——tag 非空/同服务器唯一 / 协议白名单 / CertID 存在性
	if req.Tag != nil {
		t := strings.TrimSpace(*req.Tag)
		if t == "" || len(t) > 64 {
			util.BadRequest(c, "入站标签需为 1-64 字符")
			return
		}
		var tagCnt int64
		d.DB.Model(&models.Inbound{}).Where("server_id = ? AND tag = ? AND id != ?", inb.ServerID, t, id).Count(&tagCnt)
		if tagCnt > 0 {
			util.BadRequest(c, "该服务器上已存在同名入站标签")
			return
		}
	}
	if req.Protocol != nil && !validInboundProtocol(*req.Protocol) {
		util.BadRequest(c, "不支持的协议: "+*req.Protocol+"（支持 vless / hysteria / tunnel）")
		return
	}
	// ISSUE-13：更新接口补充端口范围 / type / flow / 分享端口等语义校验。
	if req.Port != nil && (*req.Port < 1 || *req.Port > 65535) {
		util.BadRequest(c, "端口需在 1-65535 之间")
		return
	}
	if req.Type != nil && !validInboundType(*req.Type) {
		util.BadRequest(c, "入站类型仅支持 user / relay / tunnel / forward")
		return
	}
	if req.Flow != nil && *req.Flow != "" && !validInboundFlow(*req.Flow) {
		util.BadRequest(c, "入站流控仅支持空（自动）/ xtls-rprx-vision / none")
		return
	}
	if req.ShareAddrStrategy != nil && *req.ShareAddrStrategy != "" && !validShareAddrStrategy(*req.ShareAddrStrategy) {
		util.BadRequest(c, "分享地址策略仅支持 node / custom")
		return
	}
	if req.SharePort != nil && (*req.SharePort < 0 || *req.SharePort > 65535) {
		util.BadRequest(c, "分享端口需在 0-65535 之间（0=使用入站端口）")
		return
	}
	if req.Ratio != nil && *req.Ratio < 0 {
		util.BadRequest(c, "流量倍率不能为负数")
		return
	}
	if req.TotalGB != nil && *req.TotalGB < 0 {
		util.BadRequest(c, "流量上限不能为负数（0=不限）")
		return
	}
	if req.CertID != nil && *req.CertID != 0 {
		var cert models.Cert
		if err := d.DB.First(&cert, *req.CertID).Error; err != nil {
			util.BadRequest(c, "绑定证书不存在")
			return
		}
	}
	if req.LayerID != nil && *req.LayerID != 0 {
		var layer models.AccessLayer
		if err := d.DB.First(&layer, *req.LayerID).Error; err != nil {
			util.BadRequest(c, "所属对外层不存在")
			return
		}
		if layer.ServerID != inb.ServerID {
			util.BadRequest(c, "所属对外层必须与入站同属一台服务器")
			return
		}
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
		updates["settings_json"] = sj
		inb.SettingsJSON = sj
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
	if req.TotalGB != nil {
		updates["total"] = *req.TotalGB
	}
	if req.ExpiryTime != nil && len(req.ExpiryTime) > 0 && string(req.ExpiryTime) != "null" {
		var s string
		if err := json.Unmarshal(req.ExpiryTime, &s); err == nil && strings.TrimSpace(s) == "" {
			updates["expiry_time"] = nil
		} else {
			var t time.Time
			if err := json.Unmarshal(req.ExpiryTime, &t); err != nil {
				util.BadRequest(c, "到期时间格式错误（需 RFC3339，如 2026-12-31T23:59:59Z）")
				return
			}
			updates["expiry_time"] = t.UTC()
		}
	} else if req.ExpiryTime != nil && len(req.ExpiryTime) > 0 {
		updates["expiry_time"] = nil // 显式 null = 清空到期时间
	}
	if req.Enabled != nil {
		if !*req.Enabled && inb.Enabled {
			if d.refInboundProtected(c, id) || d.tunnelLandingProtected(c, id) {
				return
			}
		}
		updates["enabled"] = *req.Enabled
	}
	// 先应用显式 internal_uuid（节点回执路径），再判空补齐——保证 relay + 空 UUID 的终态不存在
	if req.InternalUUID != nil {
		updates["internal_uuid"] = *req.InternalUUID
		inb.InternalUUID = *req.InternalUUID
	}

	if req.Type != nil || req.Protocol != nil {
		updates["type"] = effType
		updates["previous_type"] = ""
		if effType == models.InboundTypeTunnel && inb.Protocol != models.ProtocolTunnel && req.Protocol == nil {
			updates["protocol"] = models.ProtocolTunnel
			inb.Protocol = models.ProtocolTunnel
		}
		inb.Type = effType
	}
	// 目标存在性校验；acceptProxyProtocol 配对挪到主记录写库成功之后执行，
	// 避免后续校验失败（400）时落地入站已被改写并重推配置。
	var ensureTarget *models.Inbound
	if req.TargetInboundID != nil {
		if *req.TargetInboundID == 0 {
			updates["target_inbound_id"] = nil
			inb.TargetInboundID = nil
		} else {
			if *req.TargetInboundID == inb.ID {
				util.BadRequest(c, "目标入站不能为自身")
				return
			}
			var targetInb models.Inbound
			if err := d.DB.First(&targetInb, *req.TargetInboundID).Error; err != nil {
				util.BadRequest(c, "目标落地入站不存在")
				return
			}
			ensureTarget = &targetInb
			updates["target_inbound_id"] = *req.TargetInboundID
			inb.TargetInboundID = req.TargetInboundID
		}
	}
	if req.TargetAddress != nil {
		updates["target_address"] = *req.TargetAddress
		inb.TargetAddress = *req.TargetAddress
	}
	if req.TargetPort != nil {
		if *req.TargetPort < 0 || *req.TargetPort > 65535 {
			util.BadRequest(c, "目标端口需在 0-65535 之间")
			return
		}
		updates["target_port"] = *req.TargetPort
		inb.TargetPort = *req.TargetPort
	}
	if req.CertID != nil {
		if *req.CertID == 0 {
			updates["cert_id"] = nil
		} else {
			updates["cert_id"] = *req.CertID
		}
	}
	if req.Flow != nil {
		updates["flow"] = *req.Flow
	}
	if req.ShareAddrStrategy != nil {
		updates["share_addr_strategy"] = *req.ShareAddrStrategy
	}
	if req.ShareAddr != nil {
		updates["share_addr"] = *req.ShareAddr
	}
	if req.SharePort != nil {
		updates["share_port"] = *req.SharePort
	}
	if req.ShareSNI != nil {
		updates["share_sni"] = *req.ShareSNI
	}
	if req.ShareAllowInsecure != nil {
		updates["share_allow_insecure"] = *req.ShareAllowInsecure
	}
	if req.LayerID != nil {
		if *req.LayerID == 0 {
			updates["layer_id"] = nil
		} else {
			updates["layer_id"] = *req.LayerID
		}
	}
	if req.Remark != nil {
		updates["remark"] = strings.TrimSpace(*req.Remark)
		inb.Remark = strings.TrimSpace(*req.Remark)
	}
	if req.TrafficReset != nil {
		resetPolicy := strings.ToLower(strings.TrimSpace(*req.TrafficReset))
		if resetPolicy == "" {
			resetPolicy = "never"
		}
		updates["traffic_reset"] = resetPolicy
		inb.TrafficReset = resetPolicy
	}
	oldTag := inb.Tag
	// 字段声明交叉规则（S4）：用「更新后的终态」校验，避免只校验本次提交的字段。
	candidate := inb
	if req.Tag != nil {
		candidate.Tag = *req.Tag
	}
	if req.Protocol != nil {
		candidate.Protocol = *req.Protocol
	}
	if req.Port != nil {
		candidate.Port = *req.Port
	}
	if req.Listen != nil {
		candidate.Listen = *req.Listen
	}
	if req.SettingsJSON != nil {
		candidate.SettingsJSON = sj
	}
	if req.StreamSettings != nil {
		candidate.StreamSettings = *req.StreamSettings
	}
	if req.Sniffing != nil {
		candidate.Sniffing = *req.Sniffing
	}
	if req.Flow != nil {
		candidate.Flow = *req.Flow
	}
	candidate.Type = effType
	if effType == models.InboundTypeTunnel {
		candidate.Protocol = models.ProtocolTunnel
	}
	if issues := protocols.ValidateInboundFields(&candidate); len(issues) > 0 {
		util.BadRequestIssues(c, "入站字段校验未通过", issues)
		return
	}
	if len(updates) > 0 {
		if err := d.DB.Model(&inb).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新失败")
			return
		}
		// 级联改名：若入站 Tag 发生改名，同步更新本服务器上引用该 Tag 的路由规则，杜绝拓扑隐藏或规则悬空
		if req.Tag != nil {
			newTag := *req.Tag
			if oldTag != "" && oldTag != newTag {
				d.DB.Model(&models.ServerRoutingRule{}).
					Where("server_id = ? AND inbound_tag = ?", inb.ServerID, oldTag).
					Update("inbound_tag", newTag)
			}
		}
	}
	// C2：直通的目标变更 / tag 改名 / 启停都会改变它的派生出口与转发规则，这里统一对账
	// （改名级联的唯一落点：对账按 owner_inbound_id 认出旧行，把 tag 与规则的 inbound_tag 一起改写）。
	// 对账失败只记日志：入站已经改完了，此时拒绝只会让状态更不一致，下次推送/打开路由页还会再对账。
	// 先重读一次：上面的 Updates 走 map，内存里的 inb 可能还是旧值（例如本次刚把 type 改成 tunnel）。
	_ = d.DB.First(&inb, id)
	if services.IsTunnelInbound(&inb) {
		d.ensureTunnelDerived(inb.ServerID)
	}
	// P2-8 配对：全部校验通过且主记录落库后再给落地入站开 acceptProxyProtocol
	// 仅当直通出站确实发送 PROXY Protocol（TunnelSendProxyProtocol 为 true）时才确保落地入站开启
	if ensureTarget != nil && xray.TunnelSendProxyProtocol(&inb) {
		d.ensureTargetAcceptProxyProtocol(ensureTarget)
	}
	// 分级推送：仅当存在影响 Xray 运行期配置的核心字段改动时，才向节点重推配置与重载；
	// 纯计费倍率、备注、周期、配额或订阅分享门面修改仅写库，跳过下发推送。
	hasXrayConfigChanges := false
	for k := range updates {
		switch k {
		case "tag", "protocol", "port", "listen", "settings_json", "stream_settings",
			"sniffing", "enabled", "type", "internal_uuid", "target_inbound_id",
			"target_address", "target_port", "cert_id", "flow", "layer_id":
			hasXrayConfigChanges = true
		}
	}
	if ensureTarget != nil {
		hasXrayConfigChanges = true
	}
	// 绑定/换绑证书、或本次更新会影响运行配置时，先把证书推到节点（幂等）。
	// 覆盖「入站更新后才绑定既有证书」「节点从未收到过该证书」等时序。
	if hasXrayConfigChanges && inb.CertID != nil && *inb.CertID != 0 {
		if err := d.pushCertToServer(inb.ServerID, *inb.CertID); err != nil {
			log.Printf("inbounds: 更新入站 %s 时推送证书 %d 失败: %v", inb.Tag, *inb.CertID, err)
		}
	}
	if hasXrayConfigChanges {
		if err := d.enqueueConfig(inb.ServerID); err != nil {
			pushFail(c, inb.ServerID, err)
			return
		}
		// 解绑/换绑后清理节点上不再被引用的托管证书目录（幂等；旧 agent 自动跳过）。
		d.SyncServerCerts(inb.ServerID)
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
		util.BadRequest(c, "无效的 ID")
		return
	}
	var inb models.Inbound
	if err := d.DB.First(&inb, id).Error; err != nil {
		util.Fail(c, 404, "入站不存在")
		return
	}
	// 四层直通落地保护：被直通管道（tunnel）引用为落地目标的入站禁止删除
	var tunnelCnt int64
	d.DB.Model(&models.Inbound{}).Where("(type IN ? OR protocol = ?) AND target_inbound_id = ?", []string{models.InboundTypeTunnel, models.InboundTypeForward}, models.ProtocolTunnel, id).Count(&tunnelCnt)
	if tunnelCnt > 0 {
		util.BadRequest(c, "该入站被 "+strconv.FormatInt(tunnelCnt, 10)+" 个四层直通管道引用为落地目标，无法删除，请先解除直通管道绑定")
		return
	}
	// U4：被出站引用（落地）的入站禁止删除——删除会导致引用方配置生成死锁
	if d.refInboundProtected(c, id) {
		return
	}
	// 接入点引用保护：被用户接入点直连引用的入站禁止删除（订阅管道断裂）
	var apCnt int64
	d.DB.Model(&models.UserAccessPoint{}).Where("target_type = 'inbound' AND target_inbound_id = ?", id).Count(&apCnt)
	if apCnt > 0 {
		util.BadRequest(c, "该入站被 "+strconv.FormatInt(apCnt, 10)+" 个用户接入点直连引用，无法删除，请先解除接入点连线")
		return
	}
	// 盒内路由规则引用保护：规则 InboundTag 为 JSON 数组或逗号/换行/分号分隔，与生成器同源解析精确匹配。
	// C2：该入站自己名下的派生直通规则不算"管理员引用"——它是直通的附属物，随直通一起删
	// （否则每个直通都会被自己派生出来的规则挡住，永远删不掉）。
	var rules []models.ServerRoutingRule
	if err := d.DB.Where("server_id = ?", inb.ServerID).Find(&rules).Error; err == nil {
		ruleCnt := int64(0)
		for _, r := range rules {
			// 派生规则（owner > 0）不算"管理员引用"：它是某个直通的附属物，随直通一起清理。
			// 不排除的话，每个直通都会被自己派生出来的规则挡住，永远删不掉。
			if r.OwnerInboundID > 0 {
				continue
			}
			for _, tg := range xray.ParseStringList(r.InboundTag) {
				if tg == inb.Tag {
					ruleCnt++
					break
				}
			}
		}
		if ruleCnt > 0 {
			util.BadRequest(c, "该入站被 "+strconv.FormatInt(ruleCnt, 10)+" 条路由规则引用，无法删除，请先删除对应规则")
			return
		}
	}
	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		// C2：连带删除该直通名下的派生出口与派生规则（出站删除还会级联规则，这里显式两清）
		if err := tx.Where("owner_inbound_id = ?", id).Delete(&models.ServerOutbound{}).Error; err != nil {
			return err
		}
		if err := tx.Where("owner_inbound_id = ?", id).Delete(&models.ServerRoutingRule{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Inbound{}, id).Error
	}); err != nil {
		util.ServerError(c, "删除失败")
		return
	}
	if inb.ID > 0 {
		if err := d.enqueueConfig(inb.ServerID); err != nil {
			pushFail(c, inb.ServerID, err)
			return
		}
		// 入站删除后其引用证书可能已无人使用：清理节点上的遗留证书目录。
		d.SyncServerCerts(inb.ServerID)
	}
	util.OK(c, gin.H{"deleted": id})
}

// AdminToggleInbound POST /api/v1/admin/inbounds/:id/toggle
func (d *Deps) AdminToggleInbound(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 ID")
		return
	}
	var inb models.Inbound
	if err := d.DB.First(&inb, id).Error; err != nil {
		util.Fail(c, 404, "入站不存在")
		return
	}
	// U4：停用被引用的落地入站同样禁止（生成器会产出指向不存在入站的 vnext）
	// P2-7：停用被已启用的四层直通管道引用的入站禁止
	if inb.Enabled && (d.refInboundProtected(c, id) || d.tunnelLandingProtected(c, id)) {
		return
	}
	inb.Enabled = !inb.Enabled
	if err := d.DB.Model(&inb).Update("enabled", inb.Enabled).Error; err != nil {
		util.ServerError(c, "更新失败")
		return
	}
	// C2：直通停用/启用要连带派生出口与转发规则的 enabled（历史上它们只随启用入站生成）
	if services.IsTunnelInbound(&inb) {
		d.ensureTunnelDerived(inb.ServerID)
	}
	if err := d.enqueueConfig(inb.ServerID); err != nil {
		pushFail(c, inb.ServerID, err)
		return
	}
	util.OK(c, gin.H{"id": id, "enabled": inb.Enabled})
}

// refInboundProtected 检查入站是否被其他**已启用**出站 inbound_ref 引用（U4：删除/停用保护）。
// 只统计启用出站：C2 的派生出站会随直通入站停用而 enabled=false，此时它不再生成，
// 若仍计数会导致落地入站被永久锁死（停用直通后仍无法停用/删除落地）。
func (d *Deps) refInboundProtected(c *gin.Context, inbID uint64) bool {
	var cnt int64
	d.DB.Model(&models.ServerOutbound{}).Where("inbound_ref = ? AND enabled = ?", inbID, true).Count(&cnt)
	if cnt > 0 {
		util.BadRequest(c, "该入站被 "+strconv.FormatInt(cnt, 10)+" 个出站引用（落地），无法删除/停用，请先解除引用")
		return true
	}
	return false
}

// tunnelLandingProtected 检查入站是否被已启用的四层直通管道引用为落地目标（P2-7：停用保护）。
func (d *Deps) tunnelLandingProtected(c *gin.Context, inbID uint64) bool {
	var cnt int64
	d.DB.Model(&models.Inbound{}).Where("(type IN ? OR protocol = ?) AND target_inbound_id = ? AND enabled = ?", []string{models.InboundTypeTunnel, models.InboundTypeForward}, models.ProtocolTunnel, inbID, true).Count(&cnt)
	if cnt > 0 {
		util.BadRequest(c, "该入站被 "+strconv.FormatInt(cnt, 10)+" 个已启用的四层直通管道引用为落地目标，无法停用，请先停用或解除管道连线")
		return true
	}
	return false
}

// ensureTargetAcceptProxyProtocol 确保直通管道的目标落地入站开启 acceptProxyProtocol（P2-8）。
// 失败只记日志不回滚主操作：落地侧没开 PROXY Protocol 时链路会拒绝连接，日志是唯一排查线索。
func (d *Deps) ensureTargetAcceptProxyProtocol(targetInb *models.Inbound) {
	if targetInb == nil {
		return
	}
	var ssMap map[string]any
	if targetInb.StreamSettings != "" {
		if err := json.Unmarshal([]byte(targetInb.StreamSettings), &ssMap); err != nil {
			// 存量数据非法时不能整体覆写（会丢掉原 TLS/REALITY 配置），跳过并留痕
			log.Printf("ensureTargetAcceptProxyProtocol: 入站 %d(%s) streamSettings 非法 JSON，跳过 PROXY Protocol 配对: %v", targetInb.ID, targetInb.Tag, err)
			return
		}
	}
	if ssMap == nil {
		ssMap = make(map[string]any)
	}
	var tcpS map[string]any
	if t, ok := ssMap["tcpSettings"].(map[string]any); ok {
		tcpS = t
	}
	tcpApp, _ := tcpS["acceptProxyProtocol"].(bool)

	// 接收侧语义的正式位置是 tcpSettings.acceptProxyProtocol；顶层键是旧版面板的
	// 私有写法（xray 不识别，生成前会被规范化），这里只保留存量数据、不再新写。
	if !tcpApp {
		if tcpS == nil {
			tcpS = make(map[string]any)
			ssMap["tcpSettings"] = tcpS
		}
		tcpS["acceptProxyProtocol"] = true
		newSS, err := json.Marshal(ssMap)
		if err != nil {
			log.Printf("ensureTargetAcceptProxyProtocol: 入站 %d(%s) streamSettings 序列化失败: %v", targetInb.ID, targetInb.Tag, err)
			return
		}
		targetInb.StreamSettings = string(newSS)
		if err := d.DB.Model(targetInb).Update("stream_settings", targetInb.StreamSettings).Error; err != nil {
			log.Printf("ensureTargetAcceptProxyProtocol: 落地入站 %d(%s) 写库失败，该落地将拒绝 PROXY Protocol 连接: %v", targetInb.ID, targetInb.Tag, err)
			return
		}
		if err := d.enqueueConfig(targetInb.ServerID); err != nil {
			log.Printf("ensureTargetAcceptProxyProtocol: 落地入站 %d 所在服务器 %d 配置推送失败，落地侧改动待下次全量推送生效: %v", targetInb.ID, targetInb.ServerID, err)
		}
	}
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

// AdminXrayVlessEnc GET /api/v1/admin/xray/vlessenc —— VLESS Encryption（ML-KEM-768）
// decryption/encryption 配对一键生成。
func (d *Deps) AdminXrayVlessEnc(c *gin.Context) {
	decryption, encryption, err := xray.GenerateVlessEnc()
	if err != nil {
		util.ServerError(c, err.Error())
		return
	}
	util.OK(c, gin.H{"decryption": decryption, "encryption": encryption})
}

// checkVlessDecryption VLESS Encryption（vlessenc 安全层）保存期校验：
// settings_json.decryption 启用时预检完整点格式，避免配置推送后 agent 端 xray -test 才暴露。
func checkVlessDecryption(protocol, inboundType, settingsJSON string) error {
	if protocol != "vless" {
		return nil
	}
	dec, err := xray.VlessDecryptionFromSettings(settingsJSON)
	if err != nil {
		return err
	}
	if !xray.VlessEncEnabled(dec) {
		return nil
	}
	return xray.ValidateVlessEncDecryption(dec)
}

// validInboundProtocol 入站协议白名单（支持已注册协议插件、四层直通 tunnel；Mixed / SOCKS5 / HTTP 已退役）。
func validInboundProtocol(p string) bool {
	if protocols.Find(p) != nil {
		return true
	}
	return p == models.ProtocolTunnel
}

func validInboundType(t string) bool {
	switch t {
	case models.InboundTypeUser, models.InboundTypeRelay, models.InboundTypeTunnel, models.InboundTypeForward:
		return true
	}
	return false
}

func validInboundFlow(f string) bool {
	return f == "xtls-rprx-vision" || f == "none"
}

func validShareAddrStrategy(s string) bool {
	switch s {
	case "node", "custom":
		return true
	}
	return false
}

// pushFail 配置生成/待推送失败时向客户端上抛（U19：不再静默打日志返回成功——
// 变更已入库但节点侧未生效，必须让管理员知道并处理）。
func pushFail(c *gin.Context, serverID uint64, err error) {
	log.Printf("config push failed (server=%d): %v", serverID, err)
	util.ServerError(c, "变更已保存，但配置生成失败: "+err.Error())
}

// ensureInternalUUID 确保入站拥有内部 UUID。
func (d *Deps) ensureInternalUUID(inb *models.Inbound) error {
	if inb.InternalUUID != "" {
		return nil
	}
	u, err := util.NewUUID()
	if err != nil {
		return fmt.Errorf("生成内部 UUID 失败: %w", err)
	}
	inb.InternalUUID = u
	return nil
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
	inb := models.Inbound{
		ID: f.ID, ServerID: f.ServerID, Tag: f.Tag, Protocol: f.Protocol,
		Port: f.Port, Listen: f.Listen,
		SettingsJSON: f.SettingsJSON, StreamSettings: f.StreamSettings,
		Sniffing: f.Sniffing, Ratio: f.Ratio, Enabled: true,
		Total: f.TotalGB, ExpiryTime: f.ExpiryTime,
		Remark: f.Remark, TrafficReset: f.TrafficReset,
		TargetInboundID:    f.TargetInboundID,
		TargetAddress:      f.TargetAddress,
		TargetPort:         f.TargetPort,
		ShareAddrStrategy:  f.ShareAddrStrategy,
		ShareAddr:          f.ShareAddr,
		SharePort:          f.SharePort,
		ShareSNI:           f.ShareSNI,
		ShareAllowInsecure: f.ShareAllowInsecure,
		LayerID:            f.LayerID,
	}
	if f.Type != "" {
		inb.Type = f.Type
	}
	inb.Flow = f.Flow
	inb.CertID = f.CertID
	return inb
}

// AdminPreviewConfig POST /api/v1/admin/xray/preview-config
func (d *Deps) AdminPreviewConfig(c *gin.Context) {
	var req struct {
		ServerID uint64       `json:"server_id" binding:"required"`
		Form     *inboundForm `json:"form"`
	}
	if !util.BindJSON(c, &req) {
		return
	}
	var srv models.Server
	if err := d.DB.First(&srv, req.ServerID).Error; err != nil {
		util.Fail(c, 404, "服务器不存在")
		return
	}
	var formInb *models.Inbound
	if req.Form != nil {
		f := formToInbound(req.Form)
		formInb = &f
	}
	cfg, err := d.Config.Preview(req.ServerID, formInb)
	if err != nil {
		util.BadRequest(c, "配置生成失败: "+err.Error())
		return
	}
	resp := gin.H{"config": cfg}
	// S4：只读预览附带字段来源与交叉规则问题。
	// 有表单 → 解释本次编辑的终态；无表单 → 逐入站解释服务器现有配置。
	if formInb != nil {
		resp["provenance"] = protocols.ExplainInbound(formInb)
		resp["issues"] = protocols.ValidateInboundFields(formInb)
	} else {
		var list []models.Inbound
		if err := d.DB.Where("server_id = ?", req.ServerID).Order("id").Find(&list).Error; err == nil {
			items := make([]gin.H, 0, len(list))
			for i := range list {
				items = append(items, gin.H{
					"inbound_id": list[i].ID,
					"tag":        list[i].Tag,
					"provenance": protocols.ExplainInbound(&list[i]),
					"issues":     protocols.ValidateInboundFields(&list[i]),
				})
			}
			resp["inbounds"] = items
		}
	}
	util.OK(c, resp)
}
