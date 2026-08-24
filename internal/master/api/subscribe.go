package api

import (
	"net"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/master/subscribe"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// Subscribe GET {subscribe_path}/:token | GET {subscribe_path}?token=xxx —— 订阅生成
// （按 UA 区分 Clash YAML / Base64）。2026-08-24 入口统一后仅由独立订阅端口调用。
func (d *Deps) Subscribe(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		token = strings.TrimSpace(c.Query("token"))
	}
	// 统一拒绝码（设置页 sub_deny_code，默认 404 防探测）：无效/缺失 token 与
	// token 查无用户一律按该码返回，不泄露账号存在性。
	if token == "" {
		util.Fail(c, services.SubDenyCode(d.DB), "订阅链接无效")
		return
	}
	var user models.User
	if err := d.DB.Where("subscribe_token = ?", token).First(&user).Error; err != nil {
		util.Fail(c, services.SubDenyCode(d.DB), "订阅链接无效")
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

	// 访问控制单点化（2026-08-23 收口）：订阅只从「用户接入点（AP）」生成——
	// AP 权限组白名单命中用户生效组 → 沿管道解析（直连目标入站，端点可被 CustomHost/Port 覆写）→ 产出节点。
	// 不再为裸入站直接生成订阅节点；无任何可见 AP 即 404。

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

	// 1. 服务器 / 可用用户入站映射表
	var allServers []models.Server
	_ = d.DB.Find(&allServers).Error
	srvMap := make(map[uint64]models.Server)
	for _, s := range allServers {
		srvMap[s.ID] = s
	}

	var allInbs []models.Inbound
	_ = d.DB.Where("enabled = ? AND type = ?", true, models.InboundTypeUser).Find(&allInbs).Error
	// J9：入站级 Total/ExpiryTime 过滤（与生成端同源）
	allInbs = services.FilterAvailableInbounds(allInbs)
	inbMap := make(map[uint64]models.Inbound)
	for _, i := range allInbs {
		inbMap[i.ID] = i
	}

	var allLayers []models.AccessLayer
	_ = d.DB.Find(&allLayers).Error
	layerMap := make(map[uint64]models.AccessLayer)
	for _, l := range allLayers {
		layerMap[l.ID] = l
	}

	// 2. 用户接入点（订阅的唯一入口；严格白名单：未绑定权限组 = 全员不可见）
	dtos := make([]contracts.ProxyNodeDTO, 0)
	var userAPs []models.UserAccessPoint
	_ = d.DB.Where("enabled = ?", true).Order("id ASC").Find(&userAPs).Error
	if len(userAPs) > 0 && permGroupID > 0 {
		apIDs := make([]uint64, 0, len(userAPs))
		for _, ap := range userAPs {
			apIDs = append(apIDs, ap.ID)
		}
		apGroupMap := services.BatchAccessPointPermissionGroupIDs(d.DB, apIDs)
		for _, ap := range userAPs {
			// 严格白名单：AP 开放组未命中用户生效组 → 不可见
			matched := false
			for _, gid := range apGroupMap[ap.ID] {
				if gid == permGroupID {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}

			// 订阅管道（与画布预览同源）：目标入站 DTO（节点自有地址/挂层端点）→ AP 消费（命名/可选覆写）
			dto := subscribe.ResolveAPSubscription(&ap, srvMap, inbMap, layerMap, user.UUID)
			if dto == nil {
				continue
			}
			dtos = append(dtos, *dto)
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
	var up, down int64
	if d.Traffic != nil {
		up, down, _ = d.Traffic.UserUsed(user.ID)
	}
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
