package api

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/services"
	"github.com/zhx/xray-panel/internal/master/subscribe"
	"github.com/zhx/xray-panel/internal/master/xray"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// shareAddrOf 计算订阅对外地址与端口（订阅专用，与 xray 监听解耦——四层转发场景
// 监听为内网，订阅给用户的是转发端点）。
// custom 且 ShareAddr 非空 → ShareAddr +（SharePort>0 ? SharePort : 入站端口）；
// listen 且 Listen 非空非 0.0.0.0 → Listen + 入站端口；默认 node → 服务器 Host + 入站端口。
func shareAddrOf(srv *models.Server, inb *models.Inbound) (string, int) {
	switch inb.ShareAddrStrategy {
	case "custom":
		if inb.ShareAddr != "" {
			port := inb.Port
			if inb.SharePort > 0 {
				port = inb.SharePort
			}
			return inb.ShareAddr, port
		}
	case "listen":
		if inb.Listen != "" && inb.Listen != "0.0.0.0" {
			return inb.Listen, inb.Port
		}
	}
	return srv.Host, inb.Port
}

// subscribeFlow 计算订阅中的 flow（与生成侧 buildClients 同源）：
// UserInbound.Flow（最高）→ 入站级 Flow（none 视为空并禁用自动注入）→ TCP+REALITY 自动 vision。
func subscribeFlow(userInboundFlow, inboundFlow string, tcpReality bool) (flow string, noAutoFlow bool) {
	flow = userInboundFlow
	if flow == "" {
		flow = inboundFlow
	}
	if flow == "none" {
		return "", true
	}
	if flow == "" && tcpReality {
		flow = "xtls-rprx-vision"
	}
	return flow, false
}

// Subscribe GET /api/v1/sub/:token —— 订阅生成（按 UA 区分 Clash YAML / Base64）。
func (d *Deps) Subscribe(c *gin.Context) {
	token := c.Param("token")
	var user models.User
	if err := d.DB.Where("subscribe_token = ?", token).First(&user).Error; err != nil {
		util.Fail(c, 404, "订阅链接无效")
		return
	}
	if user.Status != models.StatusActive {
		util.Fail(c, 403, "账号已被禁用")
		return
	}

	// 收集用户可用节点（有授权记录则过滤；无记录回退全部 type=user 入站）
	var inbounds []models.Inbound
	if err := d.DB.Where("enabled = ? AND type = ?", true, models.InboundTypeUser).Order("id ASC").Find(&inbounds).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	var grants []models.UserInbound
	d.DB.Where("user_id = ? AND enabled = ?", user.ID, true).Find(&grants)
	flowByInbound := make(map[uint64]string, len(grants))
	for _, g := range grants {
		flowByInbound[g.InboundID] = g.Flow
	}
	// 动态授权：根据用户生效权限组过滤入站（纯净 Xboard 权限组架构）。
	// 若用户未分配权限组（无套餐且未指定权限组），granted 为空集，inbounds 过滤后为 0，不返回任何节点。
	granted := services.AuthorizedInboundSet(d.DB, &user)
	filtered := make([]models.Inbound, 0, len(granted))
	for _, inb := range inbounds {
		if granted[inb.ID] {
			filtered = append(filtered, inb)
		}
	}
	inbounds = filtered
	items := make([]subscribe.ProxyItem, 0, len(inbounds))
	for i := range inbounds {
		inb := &inbounds[i]
		switch inb.Protocol {
		case "vless":
			// 支持
		case "vmess":
			fallthrough
		case "trojan":
			fallthrough
		default:
			continue
		}
		var srv models.Server
		if err := d.DB.First(&srv, inb.ServerID).Error; err != nil {
			continue
		}
		host, port := shareAddrOf(&srv, inb)
		flow, noAutoFlow := subscribeFlow(flowByInbound[inb.ID], inb.Flow,
			xray.StreamNetwork(inb.StreamSettings) == "tcp" && xray.StreamHasReality(inb.StreamSettings))
		item := subscribe.ProxyItem{
			Name:    subscribe.NodeName(&srv, inb),
			Host:    host,
			Port:    port,
			UUID:    user.UUID,
			Network: xray.StreamNetwork(inb.StreamSettings),
			TLSType: xray.StreamSecurity(inb.StreamSettings),
			Flow:    flow,
			NoAutoFlow: noAutoFlow,
			Reality: xray.StreamReality(inb.StreamSettings),
			TLS:     xray.StreamTLS(inb.StreamSettings),
			WS:      xray.StreamWS(inb.StreamSettings),
			XHTTP:   xray.StreamXHTTP(inb.StreamSettings),
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		util.Fail(c, 404, "暂无可用的节点")
		return
	}

	// 按 UA 区分输出
	ua := strings.ToLower(c.GetHeader("User-Agent"))
	isClash := strings.Contains(ua, "clash") || strings.Contains(ua, "mihomo") ||
		strings.Contains(ua, "stash") || strings.Contains(ua, "verge")

	var content string
	if isClash {
		var clashTemplate string
		var permGroupID uint64
		if user.PermissionGroupID > 0 {
			permGroupID = user.PermissionGroupID
		} else if user.PlanID > 0 {
			var plan models.Plan
			if err := d.DB.First(&plan, user.PlanID).Error; err == nil {
				permGroupID = plan.PermissionGroupID
			}
		}
		if permGroupID > 0 {
			var pg models.PermissionGroup
			if err := d.DB.First(&pg, permGroupID).Error; err == nil {
				clashTemplate = pg.ClashTemplate
			}
		}
		panelHost := c.Request.Host
		if h, _, err := net.SplitHostPort(panelHost); err == nil {
			panelHost = h
		}
		content = subscribe.BuildClashWithTemplate(&user, items, clashTemplate, panelHost)
		c.Header("Content-Type", "application/yaml; charset=utf-8")
	} else {
		content = subscribe.BuildBase64(&user, items)
		c.Header("Content-Type", "text/plain; charset=utf-8")
	}

	// subscription-userinfo
	up, down, _ := d.Traffic.UserUsed(user.ID)
	totalBytes := int64(0)
	expire := int64(0)
	if user.PlanID > 0 {
		var plan models.Plan
		if err := d.DB.First(&plan, user.PlanID).Error; err == nil && plan.Enabled {
			totalBytes = plan.TrafficGB * 1024 * 1024 * 1024
		}
	}
	if user.ExpireAt != nil {
		expire = user.ExpireAt.Unix()
	}
	c.Header("Subscription-Userinfo", subscribe.UserInfoHeader(up, down, totalBytes, expire))

	// ETag / 304 增量
	etag := subscribe.Hash(content)
	if inm := c.GetHeader("If-None-Match"); inm != "" && inm == etag {
		c.Status(304)
		return
	}
	c.Header("ETag", etag)
	c.String(200, content)
}
