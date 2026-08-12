package api

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/services"
	"github.com/zhx/xray-panel/internal/master/subscribe"
	"github.com/zhx/xray-panel/internal/master/xray"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

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
	// 动态授权：UserInbound 授权 ∪ Plan→权限组→入站集合（T4，不写授权表避免膨胀）
	granted := services.AuthorizedInboundSet(d.DB, &user)
	if len(granted) > 0 {
		filtered := inbounds[:0]
		for _, inb := range inbounds {
			if granted[inb.ID] {
				filtered = append(filtered, inb)
			}
		}
		inbounds = filtered
	}
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
		item := subscribe.ProxyItem{
			Name:    subscribe.NodeName(&srv, inb),
			Host:    srv.Host,
			Port:    inb.Port,
			UUID:    user.UUID,
			Network: xray.StreamNetwork(inb.StreamSettings),
			TLSType: xray.StreamSecurity(inb.StreamSettings),
			Flow:    flowByInbound[inb.ID], // TCP+TLS+Vision 场景订阅必须带 flow（生成侧同源）
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
		content = subscribe.BuildClash(&user, items)
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
