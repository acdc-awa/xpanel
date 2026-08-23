package api

import (
	"net"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc/xray-panel/internal/contracts"
	"github.com/acdc/xray-panel/internal/master/services"
	"github.com/acdc/xray-panel/internal/master/subscribe"
	"github.com/acdc/xray-panel/internal/models"
	"github.com/acdc/xray-panel/internal/pkg/util"
)

// Subscribe GET /sub/:token | GET /sub?token=xxx —— 订阅生成（按 UA 区分 Clash YAML / Base64）。
func (d *Deps) Subscribe(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		token = strings.TrimSpace(c.Query("token"))
	}
	if token == "" || token == "healthz" || token == "favicon.ico" {
		util.Fail(c, 404, "订阅链接无效")
		return
	}
	var user models.User
	if err := d.DB.Where("subscribe_token = ?", token).First(&user).Error; err != nil {
		util.Fail(c, 404, "订阅链接无效")
		return
	}
	if user.Status != models.StatusActive {
		util.Fail(c, 403, "账号已被禁用")
		return
	}
	// U13：订阅端到期/超流量过滤（与生成端 filterValidUsers 同源——
	// 过期/超量用户不注入任何节点配置，订阅也应拒绝拉取，避免客户端持有失效配置）
	if user.ExpireAt != nil && time.Now().After(*user.ExpireAt) {
		util.Fail(c, 403, "订阅已过期，请续费后使用")
		return
	}
	if user.PlanID > 0 {
		var plan models.Plan
		if err := d.DB.First(&plan, user.PlanID).Error; err == nil && plan.Enabled && plan.TrafficGB > 0 {
			up, down, _ := d.Traffic.UserUsed(user.ID)
			if up+down >= plan.TrafficGB*1024*1024*1024 {
				util.Fail(c, 403, "流量已用尽，请购买新套餐")
				return
			}
		}
	}

	// 收集用户可用节点（无授权记录则全部 type=user 入站；再按用户生效权限组过滤）
	var inbounds []models.Inbound
	if err := d.DB.Where("enabled = ? AND type = ?", true, models.InboundTypeUser).Order("id ASC").Find(&inbounds).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
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
	// J9：入站级 Total/ExpiryTime 过滤（与生成端同源）
	inbounds = services.FilterAvailableInbounds(filtered)
	// 确定用户生效的权限组 ID
	var permGroupID uint64
	if user.PermissionGroupID > 0 {
		permGroupID = user.PermissionGroupID
	} else if user.PlanID > 0 {
		var plan models.Plan
		if err := d.DB.First(&plan, user.PlanID).Error; err == nil {
			permGroupID = plan.PermissionGroupID
		}
	}

	// 收集入站的附加接入点
	inboundIDs := make([]uint64, 0, len(inbounds))
	for _, inb := range inbounds {
		inboundIDs = append(inboundIDs, inb.ID)
	}
	var allEndpoints []models.InboundEndpoint
	if len(inboundIDs) > 0 {
		_ = d.DB.Where("inbound_id IN ? AND enabled = ?", inboundIDs, true).
			Order("priority ASC, id ASC").Find(&allEndpoints).Error
	}
	epIDs := make([]uint64, 0, len(allEndpoints))
	for _, ep := range allEndpoints {
		epIDs = append(epIDs, ep.ID)
	}
	epGroupMap := services.BatchEndpointPermissionGroupIDs(d.DB, epIDs)
	endpointsByInbound := make(map[uint64][]models.InboundEndpoint)
	for _, ep := range allEndpoints {
		endpointsByInbound[ep.InboundID] = append(endpointsByInbound[ep.InboundID], ep)
	}

	dtos := make([]contracts.ProxyNodeDTO, 0, len(inbounds))
	for i := range inbounds {
		inb := &inbounds[i]
		var srv models.Server
		if err := d.DB.First(&srv, inb.ServerID).Error; err != nil {
			continue
		}
		// 1. 协议插件分发：构建主接入点
		dto := subscribe.BuildNodeDTO(&srv, inb, user.UUID)
		if dto != nil {
			dtos = append(dtos, *dto)
		}

		// 2. 附加接入点派生（显式白名单权限控制：PermissionGroupIDs 为空默认全部不可见）
		for _, ep := range endpointsByInbound[inb.ID] {
			gids := epGroupMap[ep.ID]
			if len(gids) == 0 || permGroupID == 0 {
				continue
			}
			matched := false
			for _, gid := range gids {
				if gid == permGroupID {
					matched = true
					break
				}
			}
			if !matched || dto == nil {
				continue
			}
			// 派生节点：克隆主节点 DTO 并覆写 Name、ServerHost、ServerPort
			epDTO := *dto
			epDTO.Name = subscribe.NodeName(&srv, inb) + " | " + ep.Name
			epDTO.ServerHost = ep.Host
			epDTO.ServerPort = ep.Port
			dtos = append(dtos, epDTO)
		}
	}
	if len(dtos) == 0 {
		util.Fail(c, 404, "暂无可用的节点")
		return
	}

	// 按 UA 区分输出；`?format=base64` 强制 Base64（U13：实现原忽略参数）
	ua := strings.ToLower(c.GetHeader("User-Agent"))
	forceBase64 := c.Query("format") == "base64"

	// 权限组自定义 Clash 模板
	var clashTemplate string
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

	// 通过 exporter 注册表选择并生成订阅内容
	registry := subscribe.DefaultRegistry()
	var exporter contracts.SubscriptionExporter
	if forceBase64 {
		exporter = registry.Find("base64")
	} else {
		exporter = registry.Match(ua)
	}
	if exporter == nil {
		exporter = registry.Find("base64")
	}

	content, contentType, err := exporter.Export(c.Request.Context(),
		subscribe.UserToSummaryDTO(&user),
		dtos,
		contracts.ExportOptions{
			Template:  clashTemplate,
			PanelHost: panelHost,
		})
	if err != nil {
		util.ServerError(c, "生成订阅失败")
		return
	}
	c.Header("Content-Type", contentType)
	if strings.Contains(contentType, "yaml") {
		// Clash 响应头三件套（17 号 P0 ⑨）：更新间隔 / 文件名 / 配置文件网页地址
		c.Header("Profile-Update-Interval", "24")
		c.Header("Content-Disposition", `attachment; filename="xray.yaml"`)
		webPage := "http://" + c.Request.Host
		if scheme := c.GetHeader("X-Forwarded-Proto"); scheme != "" {
			webPage = scheme + "://" + c.Request.Host
		}
		c.Header("Profile-Web-Page-Url", webPage)
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
