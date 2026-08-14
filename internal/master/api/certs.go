package api

import (
	"log"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/zhx/xray-panel/internal/master/nodegate"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/protocol"
	"github.com/zhx/xray-panel/internal/pkg/tlscert"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// certRefView 证书被引用位置（入站所在服务器 + 入站标签）。
type certRefView struct {
	ServerID   uint64 `json:"server_id"`
	ServerName string `json:"server_name"`
	InboundID  uint64 `json:"inbound_id"`
	InboundTag string `json:"inbound_tag"`
}

// certView 证书对外结构（PEM 不回传，防私钥泄露）。
type certView struct {
	ID        uint64        `json:"id"`
	Domain    string        `json:"domain"`
	NotAfter  string        `json:"not_after"`
	Remark    string        `json:"remark"`
	Refs      []certRefView `json:"refs"` // 引用该证书的入站（服务器/标签）
	CreatedAt string        `json:"created_at"`
}

func toCertView(c *models.Cert) certView {
	return certView{
		ID: c.ID, Domain: c.Domain,
		NotAfter:  c.NotAfter.Format("2006-01-02 15:04"),
		Remark:    c.Remark,
		CreatedAt: c.CreatedAt.Format("2006-01-02 15:04"),
	}
}

// certForm 证书创建/更新表单。
type certForm struct {
	Domain  string `json:"domain" binding:"required,max=128"`
	CertPEM string `json:"cert_pem" binding:"required"`
	KeyPEM  string `json:"key_pem" binding:"required"`
	Remark  string `json:"remark"`
}

// AdminCerts GET /api/v1/admin/certs —— 证书列表（不含 PEM，附带引用位置）。
func (d *Deps) AdminCerts(c *gin.Context) {
	var list []models.Cert
	if err := d.DB.Order("id DESC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	// 引用聚合：cert_id → 引用入站（服务器名）
	refMap := map[uint64][]certRefView{}
	var inbounds []models.Inbound
	if err := d.DB.Where("cert_id IS NOT NULL").Find(&inbounds).Error; err == nil && len(inbounds) > 0 {
		serverIDs := make([]uint64, 0, len(inbounds))
		seenSrv := map[uint64]bool{}
		for _, inb := range inbounds {
			if inb.CertID != nil && !seenSrv[inb.ServerID] {
				seenSrv[inb.ServerID] = true
				serverIDs = append(serverIDs, inb.ServerID)
			}
		}
		srvName := map[uint64]string{}
		if len(serverIDs) > 0 {
			var servers []models.Server
			if err := d.DB.Select("id", "name").Where("id IN ?", serverIDs).Find(&servers).Error; err == nil {
				for _, s := range servers {
					srvName[s.ID] = s.Name
				}
			}
		}
		for _, inb := range inbounds {
			if inb.CertID == nil {
				continue
			}
			refMap[*inb.CertID] = append(refMap[*inb.CertID], certRefView{
				ServerID: inb.ServerID, ServerName: srvName[inb.ServerID],
				InboundID: inb.ID, InboundTag: inb.Tag,
			})
		}
	}
	items := make([]certView, 0, len(list))
	for i := range list {
		v := toCertView(&list[i])
		v.Refs = refMap[list[i].ID]
		items = append(items, v)
	}
	util.OK(c, gin.H{"items": items})
}

// AdminCreateCert POST /api/v1/admin/certs —— 上传证书：校验 PEM 匹配 + 提取 NotAfter，
// 保存后推送到引用该证书的节点（push_cert）。
func (d *Deps) AdminCreateCert(c *gin.Context) {
	var req certForm
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if !tlscert.DomainRe.MatchString(req.Domain) {
		util.BadRequest(c, "非法 domain（仅允许字母数字 . -）")
		return
	}
	notAfter, err := tlscert.NotAfter(req.CertPEM)
	if err != nil {
		util.BadRequest(c, "证书解析失败: "+err.Error())
		return
	}
	if err := tlscert.ValidatePair(req.CertPEM, req.KeyPEM); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	cert := models.Cert{
		Domain: req.Domain, CertPEM: req.CertPEM, KeyPEM: req.KeyPEM,
		NotAfter: notAfter, Remark: req.Remark,
	}
	if err := d.DB.Create(&cert).Error; err != nil {
		util.ServerError(c, "创建失败")
		return
	}
	d.pushCertToUsers(&cert)
	util.OK(c, gin.H{"cert": toCertView(&cert)})
}

// AdminUpdateCert PUT /api/v1/admin/certs/:id —— 更新备注或换证（重传 PEM 时重新校验并推送）。
func (d *Deps) AdminUpdateCert(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var cert models.Cert
	if err := d.DB.First(&cert, id).Error; err != nil {
		util.Fail(c, 404, "证书不存在")
		return
	}
	var req struct {
		CertPEM *string `json:"cert_pem"`
		KeyPEM  *string `json:"key_pem"`
		Remark  *string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}
	updates := map[string]any{}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}
	// 换证：cert/key 必须成对提供，重新校验 + 更新 NotAfter + 推送
	if (req.CertPEM != nil) != (req.KeyPEM != nil) {
		util.BadRequest(c, "换证需同时提供 cert_pem 与 key_pem")
		return
	}
	if req.CertPEM != nil {
		notAfter, err := tlscert.NotAfter(*req.CertPEM)
		if err != nil {
			util.BadRequest(c, "证书解析失败: "+err.Error())
			return
		}
		if err := tlscert.ValidatePair(*req.CertPEM, *req.KeyPEM); err != nil {
			util.BadRequest(c, err.Error())
			return
		}
		updates["cert_pem"] = *req.CertPEM
		updates["key_pem"] = *req.KeyPEM
		updates["not_after"] = notAfter
	}
	if len(updates) > 0 {
		if err := d.DB.Model(&cert).Updates(updates).Error; err != nil {
			util.ServerError(c, "更新失败")
			return
		}
		d.DB.First(&cert, id)
		d.pushCertToUsers(&cert)
	}
	util.OK(c, gin.H{"cert": toCertView(&cert)})
}

// AdminDeleteCert DELETE /api/v1/admin/certs/:id —— 有入站引用时拒绝。
func (d *Deps) AdminDeleteCert(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var cnt int64
	d.DB.Model(&models.Inbound{}).Where("cert_id = ?", id).Count(&cnt)
	if cnt > 0 {
		util.BadRequest(c, "该证书正被入站引用，请先解绑")
		return
	}
	if err := d.DB.Delete(&models.Cert{}, id).Error; err != nil {
		util.ServerError(c, "删除失败")
		return
	}
	util.OK(c, gin.H{"ok": true})
}

// pushCertToUsers 将证书推送到引用它的入站所在节点（按服务器分组，在线 Ask）。
// 节点离线时由管理员稍后重试（证书变更不阻塞 API）。
func (d *Deps) pushCertToUsers(cert *models.Cert) {
	if d.Hub == nil {
		return
	}
	var inbounds []models.Inbound
	if err := d.DB.Where("cert_id = ?", cert.ID).Find(&inbounds).Error; err != nil {
		return
	}
	seen := map[uint64]bool{}
	for _, inb := range inbounds {
		if seen[inb.ServerID] {
			continue
		}
		seen[inb.ServerID] = true
		serverID := inb.ServerID
		_, err := d.Hub.Ask(serverID, protocol.MsgPushCert,
			protocol.PushCertPayload{Domain: cert.Domain, CertPEM: cert.CertPEM, KeyPEM: cert.KeyPEM},
			nodegate.AskTimeout)
		if err != nil {
			// U7：离线节点落待推记录，上线后由 nodegate 回调补推
			log.Printf("certs: 推送证书 %s 到节点 %d 失败: %v（已记待推）", cert.Domain, serverID, err)
			var pc models.PendingCert
			if e := d.DB.Where("server_id = ?", serverID).First(&pc).Error; e == nil {
				_ = d.DB.Model(&pc).Updates(map[string]any{"cert_id": cert.ID, "status": "pending", "updated_at": time.Now()})
			} else {
				_ = d.DB.Create(&models.PendingCert{ServerID: serverID, CertID: cert.ID, Status: "pending"})
			}
		}
	}
}

// PushPendingCerts 补推某节点的待推证书（节点上线回调 / 手动触发）。
func (d *Deps) PushPendingCerts(serverID uint64) {
	if d.Hub == nil {
		return
	}
	var pc models.PendingCert
	if err := d.DB.Where("server_id = ? AND status = ?", serverID, "pending").First(&pc).Error; err != nil {
		return
	}
	var cert models.Cert
	if err := d.DB.First(&cert, pc.CertID).Error; err != nil {
		_ = d.DB.Delete(&pc)
		return
	}
	_, err := d.Hub.Ask(serverID, protocol.MsgPushCert,
		protocol.PushCertPayload{Domain: cert.Domain, CertPEM: cert.CertPEM, KeyPEM: cert.KeyPEM},
		nodegate.AskTimeout)
	if err == nil {
		_ = d.DB.Model(&pc).Update("status", "pushed")
	}
}
