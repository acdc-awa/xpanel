package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/master/services"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// 管理端提醒（派生模型）：提醒本身不落库，每次按当前状态现算；
// 只有「忽略」落库（notification_dismissals），并带条件指纹，条件升级时重新出现。
const (
	notifCategoryServer  = "server"
	notifCategoryVersion = "version"
	notifCategoryCert    = "cert"

	notifSeverityCritical = "critical"
	notifSeverityHigh     = "high"
	notifSeverityMedium   = "medium"
	notifSeverityLow      = "low"

	// serverExpiringWindowDays 服务器到期预警窗口（与前端 utils/vps.ts 的紧急阈值同口径）。
	serverExpiringWindowDays = 7
	// certExpiringWindowDays 证书到期预警窗口：证书续期通常按月安排，30 天足够走完重签/换证。
	certExpiringWindowDays = 30
)

// notificationItem 单条提醒。Key 是跨刷新稳定的标识（同一条件就同一条），
// Fingerprint 描述条件的具体状态，用于判断「忽略之后条件是否已经变化」。
type notificationItem struct {
	Key         string `json:"key"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Detail      string `json:"detail"`
	ActionPath  string `json:"action_path"`
	ActionLabel string `json:"action_label"`
	DueAt       string `json:"due_at,omitempty"`
	Fingerprint string `json:"fingerprint"`
}

// AdminListNotifications GET /api/v1/admin/notifications
// 返回当前管理员的待办提醒（不落库，按需派生；已忽略且条件未变化的条目不返回）。
func (d *Deps) AdminListNotifications(c *gin.Context) {
	items := d.visibleNotifications(middleware.CurrentUser(c))
	util.OK(c, gin.H{
		"items":        items,
		"total":        len(items),
		"generated_at": time.Now().Format(time.RFC3339),
	})
}

// AdminDismissNotification DELETE /api/v1/admin/notifications/:key
// 「已读 = 删掉」：记录忽略指纹；若该条件已经解除，则只清理旧忽略记录（幂等）。
func (d *Deps) AdminDismissNotification(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" || len(key) > 128 {
		util.BadRequest(c, "无效的提醒 ID")
		return
	}
	adminID := middleware.CurrentUser(c)

	fingerprint := ""
	for _, item := range d.buildNotifications() {
		if item.Key == key {
			fingerprint = item.Fingerprint
			break
		}
	}
	if fingerprint == "" {
		// 条件已解除：清掉可能残留的忽略记录，保持表干净。
		if err := d.DB.Where("admin_id = ? AND key = ?", adminID, key).Delete(&models.NotificationDismissal{}).Error; err != nil {
			log.Printf("[notifications] 清理忽略记录失败: %v", err)
		}
		util.OK(c, gin.H{"dismissed": false})
		return
	}

	var row models.NotificationDismissal
	err := d.DB.Where("admin_id = ? AND key = ?", adminID, key).First(&row).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := d.DB.Create(&models.NotificationDismissal{
			AdminID:     adminID,
			Key:         key,
			Fingerprint: fingerprint,
		}).Error; err != nil {
			log.Printf("[notifications] 写入忽略记录失败: %v", err)
			util.Fail(c, http.StatusInternalServerError, "忽略提醒失败")
			return
		}
	case err != nil:
		log.Printf("[notifications] 读取忽略记录失败: %v", err)
		util.Fail(c, http.StatusInternalServerError, "忽略提醒失败")
		return
	case row.Fingerprint != fingerprint:
		row.Fingerprint = fingerprint
		if err := d.DB.Save(&row).Error; err != nil {
			log.Printf("[notifications] 更新忽略记录失败: %v", err)
			util.Fail(c, http.StatusInternalServerError, "忽略提醒失败")
			return
		}
	}

	util.OK(c, gin.H{"dismissed": true})
}

// visibleNotifications 派生后过滤掉「已忽略且条件未变化」的条目。
func (d *Deps) visibleNotifications(adminID uint64) []notificationItem {
	items := d.buildNotifications()
	if adminID == 0 || len(items) == 0 {
		return items
	}

	var rows []models.NotificationDismissal
	if err := d.DB.Where("admin_id = ?", adminID).Find(&rows).Error; err != nil {
		log.Printf("[notifications] 读取忽略记录失败: %v", err)
		return items
	}
	if len(rows) == 0 {
		return items
	}
	ignored := make(map[string]string, len(rows))
	for _, r := range rows {
		ignored[r.Key] = r.Fingerprint
	}

	out := items[:0]
	for _, item := range items {
		if fp, ok := ignored[item.Key]; ok && fp == item.Fingerprint {
			continue
		}
		out = append(out, item)
	}
	return out
}

// buildNotifications 按当前状态派生全部提醒（未过滤忽略），并按严重度/紧迫度排序。
func (d *Deps) buildNotifications() []notificationItem {
	if d.DB == nil {
		return nil
	}

	var servers []models.Server
	if err := d.DB.Find(&servers).Error; err != nil {
		log.Printf("[notifications] 读取服务器列表失败: %v", err)
		return nil
	}

	items := make([]notificationItem, 0, len(servers))
	items = append(items, d.serverExpiryItems(servers)...)
	items = append(items, d.certExpiryItems()...)
	items = append(items, d.agentVersionItems(servers)...)
	if item, ok := d.xrayVersionItem(servers); ok {
		items = append(items, item)
	}
	if item, ok := d.panelVersionItem(); ok {
		items = append(items, item)
	}

	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := severityRank(items[i].Severity), severityRank(items[j].Severity)
		if ri != rj {
			return ri < rj
		}
		if items[i].DueAt != items[j].DueAt {
			if items[i].DueAt == "" {
				return false
			}
			if items[j].DueAt == "" {
				return true
			}
			return items[i].DueAt < items[j].DueAt
		}
		return items[i].Key < items[j].Key
	})
	return items
}

// serverExpiryItems 服务器（VPS）到期提醒：已过期 → critical；≤7 天 → high（≤3 天）/ medium。
func (d *Deps) serverExpiryItems(servers []models.Server) []notificationItem {
	now := time.Now().In(services.BusinessLocation(d.DB))
	items := make([]notificationItem, 0, len(servers))
	for _, srv := range servers {
		days, ok := daysUntilCalendarDay(srv.ExpireAt, now)
		if !ok {
			continue
		}
		expireDate := strings.TrimSpace(srv.ExpireAt)[:10]

		switch {
		case days < 0:
			items = append(items, notificationItem{
				Key:         fmt.Sprintf("server-expired:%d", srv.ID),
				Category:    notifCategoryServer,
				Severity:    notifSeverityCritical,
				Title:       fmt.Sprintf("服务器「%s」已过期", srv.Name),
				Detail:      fmt.Sprintf("到期日 %s（已过期 %d 天），请续费或确认是否保留", expireDate, -days),
				ActionPath:  "/admin/servers",
				ActionLabel: "查看服务器",
				DueAt:       expireDate,
				Fingerprint: "expired:" + expireDate,
			})
		case days <= serverExpiringWindowDays:
			severity := notifSeverityMedium
			if days <= 3 {
				severity = notifSeverityHigh
			}
			title := fmt.Sprintf("服务器「%s」还有 %d 天到期", srv.Name, days)
			if days == 0 {
				title = fmt.Sprintf("服务器「%s」今日到期", srv.Name)
			}
			items = append(items, notificationItem{
				Key:         fmt.Sprintf("server-expiring:%d", srv.ID),
				Category:    notifCategoryServer,
				Severity:    severity,
				Title:       title,
				Detail:      fmt.Sprintf("到期日 %s，请及时续费", expireDate),
				ActionPath:  "/admin/servers",
				ActionLabel: "查看服务器",
				DueAt:       expireDate,
				// 指纹带天数和严重度：7 天 → 3 天 → 今日到期，忽略过的提醒会重新出现。
				Fingerprint: fmt.Sprintf("expiring:%s:%d:%s", expireDate, days, severity),
			})
		}
	}
	return items
}

// certExpiryItems 证书到期提醒：只提醒仍在链路里的证书——被入站引用的 leaf
// 与已签发过证书的 CA。未被引用的证书到期不影响服务，不制造噪音。
func (d *Deps) certExpiryItems() []notificationItem {
	now := time.Now()
	items := make([]notificationItem, 0)

	var certs []models.Cert
	if err := d.DB.Find(&certs).Error; err != nil {
		log.Printf("[notifications] 读取证书列表失败: %v", err)
		return nil
	}
	if len(certs) > 0 {
		refs := make(map[uint64]int64)
		var rows []struct {
			CertID uint64 `gorm:"column:cert_id"`
			Cnt    int64  `gorm:"column:cnt"`
		}
		if err := d.DB.Model(&models.Inbound{}).
			Select("cert_id, count(*) as cnt").
			Where("cert_id IS NOT NULL").
			Group("cert_id").Scan(&rows).Error; err != nil {
			log.Printf("[notifications] 统计证书引用失败: %v", err)
		} else {
			for _, row := range rows {
				refs[row.CertID] = row.Cnt
			}
			for _, cert := range certs {
				refCnt := refs[cert.ID]
				if refCnt == 0 || cert.NotAfter.IsZero() {
					continue
				}
				if item, ok := certExpiryItem("cert", "证书", cert.ID, cert.Domain, cert.NotAfter, now,
					fmt.Sprintf("被 %d 个入站引用", refCnt), "请及时重签或换证"); ok {
					items = append(items, item)
				}
			}
		}
	}

	var cas []models.CACert
	if err := d.DB.Find(&cas).Error; err != nil {
		log.Printf("[notifications] 读取 CA 列表失败: %v", err)
		return items
	}
	if len(cas) > 0 {
		issued := make(map[uint64]int64)
		var rows []struct {
			CAID uint64 `gorm:"column:ca_id"`
			Cnt  int64  `gorm:"column:cnt"`
		}
		if err := d.DB.Model(&models.Cert{}).
			Select("ca_id, count(*) as cnt").
			Where("ca_id IS NOT NULL").
			Group("ca_id").Scan(&rows).Error; err != nil {
			log.Printf("[notifications] 统计 CA 签发数失败: %v", err)
		} else {
			for _, row := range rows {
				issued[row.CAID] = row.Cnt
			}
			for _, ca := range cas {
				n := issued[ca.ID]
				if n == 0 || ca.NotAfter.IsZero() {
					continue
				}
				ref := fmt.Sprintf("已签发 %d 张证书", n)
				task := "请及时轮换 CA 并重签相关证书"
				if !ca.HasKey {
					ref = fmt.Sprintf("作为 %d 张证书的校验根", n)
					task = "请及时更换校验证书"
				}
				if item, ok := certExpiryItem("ca", "CA 证书", ca.ID, ca.Name, ca.NotAfter, now, ref, task); ok {
					items = append(items, item)
				}
			}
		}
	}
	return items
}

// certExpiryItem 把一张证书的到期状态转成提醒；窗口外返回 ok=false。
// kind 决定 Key 前缀（cert / ca），label 只影响标题措辞。
func certExpiryItem(kind, label string, id uint64, name string, notAfter, now time.Time, ref, task string) (notificationItem, bool) {
	expired, days, far := certExpiryState(notAfter, now)
	if far {
		return notificationItem{}, false
	}

	severity := notifSeverityMedium
	if expired {
		severity = notifSeverityCritical
	} else if days <= 7 {
		severity = notifSeverityHigh
	}

	due := notAfter.Format("2006-01-02")
	ts := notAfter.Format("2006-01-02 15:04")
	var title, detail string
	if expired {
		ago := "已过期不足 1 天"
		if days > 0 {
			ago = fmt.Sprintf("已过期 %d 天", days)
		}
		title = fmt.Sprintf("%s「%s」已过期", label, name)
		detail = fmt.Sprintf("到期于 %s（%s），%s，%s", ts, ago, ref, task)
		return notificationItem{
			Key:         fmt.Sprintf("%s-expired:%d", kind, id),
			Category:    notifCategoryCert,
			Severity:    severity,
			Title:       title,
			Detail:      detail,
			ActionPath:  "/admin/certs",
			ActionLabel: "查看证书",
			DueAt:       due,
			Fingerprint: "expired:" + notAfter.Format(time.RFC3339),
		}, true
	}

	if days == 0 {
		title = fmt.Sprintf("%s「%s」今日到期", label, name)
	} else {
		title = fmt.Sprintf("%s「%s」还有 %d 天到期", label, name, days)
	}
	detail = fmt.Sprintf("到期于 %s，%s，%s", ts, ref, task)
	return notificationItem{
		Key:         fmt.Sprintf("%s-expiring:%d", kind, id),
		Category:    notifCategoryCert,
		Severity:    severity,
		Title:       title,
		Detail:      detail,
		ActionPath:  "/admin/certs",
		ActionLabel: "查看证书",
		DueAt:       due,
		// 指纹带剩余天数与严重度：30 天 → 7 天 → 今日到期会重新提醒。
		Fingerprint: fmt.Sprintf("expiring:%s:%d:%s", notAfter.Format(time.RFC3339), days, severity),
	}, true
}

// certExpiryState 计算到期状态：expired / 剩余天数（0=不足 1 天）/ 是否在预警窗口外。
func certExpiryState(notAfter, now time.Time) (expired bool, days int, far bool) {
	remain := notAfter.Sub(now)
	if remain <= 0 {
		return true, int(-remain.Hours() / 24), false
	}
	days = int(remain.Hours() / 24)
	return false, days, days > certExpiringWindowDays
}

// agentVersionItems Agent 版本落后提醒。基准版本只认「已校验资产齐备」的探测结果，
// 否则会在 release 资产还没上传完的窗口里报一个升不上去的版本。
func (d *Deps) agentVersionItems(servers []models.Server) []notificationItem {
	probe := agentProbeSnapshot()
	if !probe.Verified || probe.Version == "" {
		return nil
	}

	items := make([]notificationItem, 0, len(servers))
	for _, srv := range servers {
		// 空版本 = 旧 agent 从未上报，不能判定为落后。
		if srv.AgentVersion == "" {
			continue
		}
		if CompareAgentVersion(srv.AgentVersion, probe.Version) >= 0 {
			continue
		}

		severity := notifSeverityMedium
		detail := fmt.Sprintf("当前 %s，最新 %s", srv.AgentVersion, probe.Version)
		if srv.Status != 1 {
			// 离线服务器本来就升不了级：降一档，避免和在线节点混在一起报。
			severity = notifSeverityLow
			detail += "；服务器当前离线，恢复后再升级"
		}
		items = append(items, notificationItem{
			Key:         fmt.Sprintf("agent-outdated:%d", srv.ID),
			Category:    notifCategoryVersion,
			Severity:    severity,
			Title:       fmt.Sprintf("服务器「%s」的 Agent 有新版本", srv.Name),
			Detail:      detail,
			ActionPath:  "/admin/servers",
			ActionLabel: "查看服务器",
			Fingerprint: srv.AgentVersion + "->" + probe.Version,
		})
	}
	return items
}

// xrayVersionItem Xray 内核落后提醒，聚合成一条。
//
// 内核版本是团队刻意钉死的，逐台报会在多机部署里刷屏；这里只给一个「有新版、还有几台没升」的信号，
// 严重的版本决策仍然在服务器页逐台做。severity 固定 low：不是缺陷，是可选的升级评估。
func (d *Deps) xrayVersionItem(servers []models.Server) (notificationItem, bool) {
	probe := xrayProbeSnapshot()
	if !probe.Verified || probe.Version == "" {
		return notificationItem{}, false
	}

	outdated := 0
	versions := make(map[string]int, 4)
	for _, srv := range servers {
		// 空版本 = 旧 agent 从未上报内核版本，不能判定为落后。
		if srv.XrayVersion == "" {
			continue
		}
		if CompareAgentVersion(srv.XrayVersion, probe.Version) >= 0 {
			continue
		}
		outdated++
		versions[srv.XrayVersion]++
	}
	if outdated == 0 {
		return notificationItem{}, false
	}

	distinct := make([]string, 0, len(versions))
	for v := range versions {
		distinct = append(distinct, v)
	}
	sort.Strings(distinct)

	detail := fmt.Sprintf("%d 台服务器的内核低于最新 %s", outdated, probe.Version)
	if len(distinct) == 1 {
		detail = fmt.Sprintf("%d 台服务器当前 %s，最新 %s", outdated, distinct[0], probe.Version)
	}

	return notificationItem{
		Key:         "xray-outdated",
		Category:    notifCategoryVersion,
		Severity:    notifSeverityLow,
		Title:       "Xray 内核有新版本",
		Detail:      detail,
		ActionPath:  "/admin/servers",
		ActionLabel: "查看服务器",
		// 指纹只看「最新版 + 现存旧版本集合」：升掉一部分机器不会让已忽略的提醒复活，
		// 但旧版本集合变化（某个版本已全部升完）或官方又发新版时会重新提醒。
		Fingerprint: probe.Version + "|" + strings.Join(distinct, ","),
	}, true
}

// panelVersionItem 面板自身版本落后提醒。
func (d *Deps) panelVersionItem() (notificationItem, bool) {
	probe := panelProbeSnapshot()
	if !probe.Verified || probe.Version == "" {
		return notificationItem{}, false
	}
	// dev 是本地构建的占位版本，无从比较。
	if PanelVersion == "" || PanelVersion == "dev" {
		return notificationItem{}, false
	}
	if CompareAgentVersion(PanelVersion, probe.Version) >= 0 {
		return notificationItem{}, false
	}
	return notificationItem{
		Key:         "panel-outdated",
		Category:    notifCategoryVersion,
		Severity:    notifSeverityLow,
		Title:       "面板有新版本",
		Detail:      fmt.Sprintf("当前 %s，最新 %s", PanelVersion, probe.Version),
		ActionPath:  "/admin/settings",
		ActionLabel: "前往更新",
		Fingerprint: PanelVersion + "->" + probe.Version,
	}, true
}

// daysUntilCalendarDay 计算到期日距今天的天数（负数=已过期）。
//
// 到期日是纯日历日，唯一需要时区的是「今天算哪一天」，由调用方用业务时区取 now；
// 这里的减法借 UTC 做中性日历减法，不掺杂时区/夏令时语义。
func daysUntilCalendarDay(expireAt string, now time.Time) (int, bool) {
	raw := strings.TrimSpace(expireAt)
	if len(raw) < 10 {
		return 0, false
	}
	target, err := time.Parse("2006-01-02", raw[:10])
	if err != nil {
		return 0, false
	}
	today, err := time.Parse("2006-01-02", now.Format("2006-01-02"))
	if err != nil {
		return 0, false
	}
	return int(target.Sub(today) / (24 * time.Hour)), true
}

func severityRank(severity string) int {
	switch severity {
	case notifSeverityCritical:
		return 0
	case notifSeverityHigh:
		return 1
	case notifSeverityMedium:
		return 2
	default:
		return 3
	}
}
