package api

import (
	"net"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/services"
	"github.com/zhx/xray-panel/internal/master/subscribe"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
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
		item := subscribe.BuildProxyItem(&srv, inb, user.UUID)
		items = append(items, item)
	}
	if len(items) == 0 {
		util.Fail(c, 404, "暂无可用的节点")
		return
	}

	// 按 UA 区分输出；`?format=base64` 强制 Base64（U13：实现原忽略参数）
	ua := strings.ToLower(c.GetHeader("User-Agent"))
	forceBase64 := c.Query("format") == "base64"
	isClash := !forceBase64 && (strings.Contains(ua, "clash") || strings.Contains(ua, "mihomo") ||
		strings.Contains(ua, "stash") || strings.Contains(ua, "verge"))

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
		// Clash 响应头三件套（17 号 P0 ⑨）：更新间隔 / 文件名 / 配置文件网页地址
		c.Header("Profile-Update-Interval", "24")
		c.Header("Content-Disposition", `attachment; filename="xray.yaml"`)
		webPage := "http://" + c.Request.Host
		if scheme := c.GetHeader("X-Forwarded-Proto"); scheme != "" {
			webPage = scheme + "://" + c.Request.Host
		}
		c.Header("Profile-Web-Page-Url", webPage)
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
