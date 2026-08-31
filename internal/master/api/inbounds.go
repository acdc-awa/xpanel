package api

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/xray"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
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
	Type               string     `json:"type"`                    // user / relay
	InternalUUID       string     `json:"internal_uuid,omitempty"` // relay 只读（节点上报）
	CertID             *uint64    `json:"cert_id,omitempty"`       // 绑定的证书
	Flow               string     `json:"flow"`                    // 入站级流控（空=自动 / xtls-rprx-vision / none）
	ShareAddrStrategy  string     `json:"share_addr_strategy"`     // node / custom（订阅专用）
	ShareAddr          string     `json:"share_addr"`              // 自定义分享地址（订阅专用，域名/IP）
	SharePort          int        `json:"share_port"`              // 自定义分享端口（0 = 使用入站端口）
	ShareSecurity      string     `json:"share_security"`          // auto / tls / none
	ShareSNI           string     `json:"share_sni"`               // 订阅 SNI 覆写
	ShareHost          string     `json:"share_host"`              // 订阅 HTTP/WS Host 覆写
	SharePath          string     `json:"share_path"`              // 订阅 WS/XHTTP Path 覆写
	ShareAllowInsecure bool       `json:"share_allow_insecure"`    // 订阅跳过证书校验
	LayerID            *uint64    `json:"layer_id,omitempty"`      // 所属对外接入层（空/0 = 直连自持端点）
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
	SettingsJSON       string     `json:"settings_json"`   // 协议 settings（透传）
	StreamSettings     string     `json:"stream_settings"` // 传输 streamSettings（透传）
	Sniffing           string     `json:"sniffing"`        // 嗅探（透传）
	Ratio              float64    `json:"ratio"`
	TotalGB            int64      `json:"total_gb"`              // J9：入站总流量上限（GB，0=不限）
	ExpiryTime         *time.Time `json:"expiry_time,omitempty"` // J9：入站到期时间
	Type               string     `json:"type"`                  // user / relay（空 = user）
	CertID             *uint64    `json:"cert_id"`               // 绑定证书（T5 校验存在性）
	Flow               string     `json:"flow"`                  // 入站级流控（空=自动 / xtls-rprx-vision / none）
	ShareAddrStrategy  string     `json:"share_addr_strategy"`   // node / custom（订阅专用，listen 已退役）
	ShareAddr          string     `json:"share_addr"`            // 自定义分享地址（订阅专用，域名/IP）
	SharePort          int        `json:"share_port"`            // 自定义分享端口（0 = 使用入站端口）
	ShareSecurity      string     `json:"share_security"`        // auto / tls / none
	ShareSNI           string     `json:"share_sni"`
	ShareHost          string     `json:"share_host"`
	SharePath          string     `json:"share_path"`
	ShareAllowInsecure bool       `json:"share_allow_insecure"`
	LayerID            *uint64    `json:"layer_id"` // 所属对外接入层（nil/0 = 直连；仅同一服务器的层有效）
}

func toInboundView(i *models.Inbound, serverName string) inboundView {
	sec := i.ShareSecurity
	if sec == "" {
		sec = "auto"
	}
	return inboundView{
		ID: i.ID, ServerID: i.ServerID, ServerName: serverName,
		Tag: i.Tag, Protocol: i.Protocol, Port: i.Port,
		Listen: i.Listen, SettingsJSON: i.SettingsJSON,
		StreamSettings: i.StreamSettings, Sniffing: i.Sniffing,
		Ratio: i.Ratio, TotalGB: i.Total, ExpiryTime: i.ExpiryTime,
		Enabled: i.Enabled, CreatedAt: i.CreatedAt,
		Type: i.Type, InternalUUID: i.InternalUUID, CertID: i.CertID,
		Flow: i.Flow, ShareAddrStrategy: i.ShareAddrStrategy, ShareAddr: i.ShareAddr,
		SharePort:     i.SharePort,
		ShareSecurity: sec, ShareSNI: i.ShareSNI, ShareHost: i.ShareHost,
		SharePath: i.SharePath, ShareAllowInsecure: i.ShareAllowInsecure,
		LayerID: i.LayerID,
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
	// U15：创建校验闭环——协议白名单 / tag 非空且同服务器唯一 / CertID 存在性
	if !validInboundProtocol(req.Protocol) {
		util.BadRequest(c, "不支持的协议: "+req.Protocol+"（暂仅支持 vless，vmess/trojan/ss 订阅导出未实现）")
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
	if req.Type != "" && !validInboundType(req.Type) {
		util.BadRequest(c, "入站类型仅支持 user / relay")
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
	inb := models.Inbound{
		ServerID: req.ServerID, Tag: req.Tag, Protocol: req.Protocol,
		Port: req.Port, Listen: req.Listen,
		SettingsJSON: req.SettingsJSON, StreamSettings: req.StreamSettings,
		Sniffing: req.Sniffing, Ratio: req.Ratio,
		Total: req.TotalGB, ExpiryTime: req.ExpiryTime, Enabled: true,
		Type:   req.Type,
		CertID: req.CertID,
		Flow:   req.Flow, ShareAddrStrategy: req.ShareAddrStrategy, ShareAddr: req.ShareAddr,
		SharePort: req.SharePort, ShareSecurity: req.ShareSecurity, ShareSNI: req.ShareSNI,
		ShareHost: req.ShareHost, SharePath: req.SharePath,
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
	if inb.ShareSecurity == "" {
		inb.ShareSecurity = "auto"
	}
	if inb.ShareSecurity != "" && !validShareSecurity(inb.ShareSecurity) {
		util.BadRequest(c, "订阅安全层覆写仅支持 auto / tls / none")
		return
	}
	if err := d.DB.Create(&inb).Error; err != nil {
		util.ServerError(c, "创建失败")
		return
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
		util.BadRequest(c, "非法 ID")
		return
	}
	var inb models.Inbound
	if err := d.DB.First(&inb, id).Error; err != nil {
		util.Fail(c, 404, "入站不存在")
		return
	}
	var req struct {
		Tag                *string    `json:"tag"`
		Protocol           *string    `json:"protocol"`
		Port               *int       `json:"port"`
		Listen             *string    `json:"listen"`
		SettingsJSON       *string    `json:"settings_json"`
		StreamSettings     *string    `json:"stream_settings"`
		Sniffing           *string    `json:"sniffing"`
		Ratio              *float64   `json:"ratio"`
		TotalGB            *int64          `json:"total_gb"`
		ExpiryTime         json.RawMessage `json:"expiry_time"` // 三元：字段缺省=不动 / null=清空 / 字符串=设置
		Enabled            *bool      `json:"enabled"`
		Type               *string    `json:"type"`
		InternalUUID       *string    `json:"internal_uuid"`       // 仅节点回执写入（管理员只读展示）
		CertID             *uint64    `json:"cert_id"`             // nil 不更新；显式传 0 解绑
		Flow               *string    `json:"flow"`                // 入站级流控（nil 不更新；空串=自动）
		ShareAddrStrategy  *string    `json:"share_addr_strategy"` //
		ShareAddr          *string    `json:"share_addr"`          //
		SharePort          *int       `json:"share_port"`          // 0 = 使用入站端口
		ShareSecurity      *string    `json:"share_security"`      // auto / tls / none
		ShareSNI           *string    `json:"share_sni"`
		ShareHost          *string    `json:"share_host"`
		SharePath          *string    `json:"share_path"`
		ShareAllowInsecure *bool      `json:"share_allow_insecure"`
		LayerID            *uint64    `json:"layer_id"` // nil 不更新；显式传 0 解绑回原生
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
		util.BadRequest(c, "不支持的协议: "+*req.Protocol+"（暂仅支持 vless，vmess/trojan/ss 订阅导出未实现）")
		return
	}
	// ISSUE-13：更新接口补充端口范围 / type / flow / 分享端口等语义校验。
	if req.Port != nil && (*req.Port < 1 || *req.Port > 65535) {
		util.BadRequest(c, "端口需在 1-65535 之间")
		return
	}
	if req.Type != nil && !validInboundType(*req.Type) {
		util.BadRequest(c, "入站类型仅支持 user / relay")
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
	if req.ShareSecurity != nil && *req.ShareSecurity != "" && !validShareSecurity(*req.ShareSecurity) {
		util.BadRequest(c, "订阅安全层覆写仅支持 auto / tls / none")
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
	if req.TotalGB != nil {
		updates["total"] = *req.TotalGB
	}
	if req.ExpiryTime != nil && len(req.ExpiryTime) > 0 && string(req.ExpiryTime) != "null" {
		var t time.Time
		if err := json.Unmarshal(req.ExpiryTime, &t); err != nil {
			util.BadRequest(c, "到期时间格式错误（需 RFC3339，如 2026-12-31T23:59:59Z）")
			return
		}
		updates["expiry_time"] = t
	} else if req.ExpiryTime != nil && len(req.ExpiryTime) > 0 {
		updates["expiry_time"] = nil // 显式 null = 清空到期时间
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Type != nil {
		updates["type"] = *req.Type
		// 手动修改 type：清除自动标 relay 的历史（PreviousType），避免后续解绑引用错误回退
		updates["previous_type"] = ""
	}
	if req.InternalUUID != nil {
		updates["internal_uuid"] = *req.InternalUUID
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
	if req.ShareSecurity != nil {
		updates["share_security"] = *req.ShareSecurity
	}
	if req.ShareSNI != nil {
		updates["share_sni"] = *req.ShareSNI
	}
	if req.ShareHost != nil {
		updates["share_host"] = *req.ShareHost
	}
	if req.SharePath != nil {
		updates["share_path"] = *req.SharePath
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
	if len(updates) > 0 {
		if err := d.DB.Model(&inb).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新失败")
			return
		}
	}
	if err := d.enqueueConfig(inb.ServerID); err != nil {
		pushFail(c, inb.ServerID, err)
		return
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
	if err := d.DB.First(&inb, id).Error; err != nil {
		util.Fail(c, 404, "入站不存在")
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
	// 盒内路由规则引用保护：规则 InboundTag 为 JSON 数组或逗号/换行/分号分隔，与生成器同源解析精确匹配
	var rules []models.ServerRoutingRule
	if err := d.DB.Where("server_id = ?", inb.ServerID).Find(&rules).Error; err == nil {
		ruleCnt := int64(0)
		for _, r := range rules {
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
	// U4：停用被引用的落地入站同样禁止（生成器会产出指向不存在入站的 vnext）
	if inb.Enabled && d.refInboundProtected(c, id) {
		return
	}
	inb.Enabled = !inb.Enabled
	if err := d.DB.Model(&inb).Update("enabled", inb.Enabled).Error; err != nil {
		util.ServerError(c, "更新失败")
		return
	}
	if err := d.enqueueConfig(inb.ServerID); err != nil {
		pushFail(c, inb.ServerID, err)
		return
	}
	util.OK(c, gin.H{"id": id, "enabled": inb.Enabled})
}

// refInboundProtected 检查入站是否被其他出站 inbound_ref 引用（U4：删除/停用保护）。
func (d *Deps) refInboundProtected(c *gin.Context, inbID uint64) bool {
	var cnt int64
	d.DB.Model(&models.ServerOutbound{}).Where("inbound_ref = ?", inbID).Count(&cnt)
	if cnt > 0 {
		util.BadRequest(c, "该入站被 "+strconv.FormatInt(cnt, 10)+" 个出站引用（落地），无法删除/停用，请先解除引用")
		return true
	}
	return false
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

// validInboundProtocol 入站协议白名单（仅 VLESS 全功能可用；vmess/trojan/ss 订阅导出未实现，
// 创建时拒绝避免"能建但订阅静默丢弃"，同时防拼写错误/未知协议卡出不可能存在的配置）。
func validInboundProtocol(p string) bool {
	return p == "vless"
}

func validInboundType(t string) bool {
	switch t {
	case models.InboundTypeUser, models.InboundTypeRelay:
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

func validShareSecurity(s string) bool {
	switch s {
	case "auto", "tls", "none":
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
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
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
	util.OK(c, gin.H{"config": cfg})
}
