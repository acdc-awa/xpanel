package api

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/certutil"
	"github.com/acdc-awa/xpanel/internal/pkg/db"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// caCertView CA 证书对外展示结构（私钥不回传）。
type caCertView struct {
	ID           uint64 `json:"id"`
	Name         string `json:"name"`
	CommonName   string `json:"common_name"`
	Organization string `json:"organization"`
	Country      string `json:"country"`
	CACertPEM    string `json:"ca_cert_pem"`
	HasKey       bool   `json:"has_key"` // 是否可签发证书
	NotAfter     string `json:"not_after"`
	KeyType      string `json:"key_type"`
	Remark       string `json:"remark"`
	IssuedCount  int64  `json:"issued_count"` // 已签发证书数
	CreatedAt    string `json:"created_at"`
}

func toCACertView(ca *models.CACert, issuedCount int64) caCertView {
	return caCertView{
		ID:           ca.ID,
		Name:         ca.Name,
		CommonName:   ca.CommonName,
		Organization: ca.Organization,
		Country:      ca.Country,
		CACertPEM:    ca.CACertPEM,
		HasKey:       ca.HasKey,
		NotAfter:     ca.NotAfter.Format("2006-01-02 15:04"),
		KeyType:      ca.KeyType,
		Remark:       ca.Remark,
		IssuedCount:  issuedCount,
		CreatedAt:    ca.CreatedAt.Format("2006-01-02 15:04"),
	}
}

// createCACertForm 导入外部 CA 表单
type createCACertForm struct {
	Name      string `json:"name" binding:"required,max=128"`
	CACertPEM string `json:"ca_cert_pem" binding:"required"`
	CAKeyPEM  string `json:"ca_key_pem"` // 可选；有私钥即可签发，无私钥仅验签
	Remark    string `json:"remark"`
}

// generateCACertForm 面板生成 CA 表单
type generateCACertForm struct {
	Name         string `json:"name" binding:"required,max=128"`
	CommonName   string `json:"common_name"`
	Organization string `json:"organization"`
	Country      string `json:"country"`
	KeyType      string `json:"key_type"`   // 默认 ecdsa_p256
	ValidDays    int    `json:"valid_days"` // 默认 3650
	Remark       string `json:"remark"`
}

// issueCertFromCAForm 基于 CA 签发服务端证书表单
type issueCertFromCAForm struct {
	Domain       string   `json:"domain" binding:"required,max=128"`
	SANDNS       []string `json:"san_dns"`
	SANIP        []string `json:"san_ip"`
	Organization string   `json:"organization"`
	Country      string   `json:"country"`
	KeyType      string   `json:"key_type"`   // 默认 ecdsa_p256
	ValidDays    int      `json:"valid_days"` // 默认 365
	Remark       string   `json:"remark"`
}

// AdminCACerts GET /api/v1/admin/ca-certs —— 获取 CA 根证书列表
func (d *Deps) AdminCACerts(c *gin.Context) {
	var list []models.CACert
	if err := d.DB.Order("id DESC").Find(&list).Error; err != nil {
		util.ServerError(c, "查询 CA 证书失败")
		return
	}

	// 统计每个 CA 已签发的证书数
	counts := map[uint64]int64{}
	type countResult struct {
		CAID  uint64 `gorm:"column:ca_id"`
		Count int64  `gorm:"column:cnt"`
	}
	var res []countResult
	d.DB.Model(&models.Cert{}).
		Select("ca_id, count(*) as cnt").
		Where("ca_id IS NOT NULL").
		Group("ca_id").
		Scan(&res)
	for _, r := range res {
		counts[r.CAID] = r.Count
	}

	items := make([]caCertView, 0, len(list))
	for i := range list {
		items = append(items, toCACertView(&list[i], counts[list[i].ID]))
	}
	util.OK(c, gin.H{"items": items})
}

// AdminCreateCACert POST /api/v1/admin/ca-certs —— 导入外部 CA 证书
func (d *Deps) AdminCreateCACert(c *gin.Context) {
	var req createCACertForm
	if !util.BindJSON(c, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		util.BadRequest(c, "CA 标识名称不能为空")
		return
	}

	// 解析与校验 CA 证书
	info, err := certutil.ParseCACertInfo(req.CACertPEM)
	if err != nil {
		util.BadRequest(c, "CA 证书解析失败: "+err.Error())
		return
	}
	if !info.IsCA {
		util.BadRequest(c, "该证书缺少 CA 标记（BasicConstraints.IsCA=true），无法作为根 CA")
		return
	}

	// 若提供了私钥，校验是否匹配
	hasKey := strings.TrimSpace(req.CAKeyPEM) != ""
	if hasKey {
		if err := certutil.ValidateCAPair(req.CACertPEM, req.CAKeyPEM); err != nil {
			util.BadRequest(c, "CA 私钥与证书不匹配: "+err.Error())
			return
		}
	}

	ca := models.CACert{
		Name:         name,
		CommonName:   info.CommonName,
		Organization: info.Organization,
		Country:      info.Country,
		CACertPEM:    strings.TrimSpace(req.CACertPEM),
		CAKeyPEM:     strings.TrimSpace(req.CAKeyPEM),
		HasKey:       hasKey,
		NotAfter:     info.NotAfter,
		KeyType:      info.KeyType,
		Remark:       req.Remark,
	}

	if err := d.DB.Create(&ca).Error; err != nil {
		if db.IsUniqueViolation(err, "ca_certs.name") {
			util.BadRequest(c, "该 CA 名称已存在")
			return
		}
		util.ServerError(c, "保存 CA 证书失败: "+err.Error())
		return
	}

	util.OK(c, gin.H{"ca_cert": toCACertView(&ca, 0)})
}

// AdminGenerateCACert POST /api/v1/admin/ca-certs/generate —— 面板生成新根 CA
func (d *Deps) AdminGenerateCACert(c *gin.Context) {
	var req generateCACertForm
	if !util.BindJSON(c, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		util.BadRequest(c, "CA 标识名称不能为空")
		return
	}

	cn := strings.TrimSpace(req.CommonName)
	if cn == "" {
		cn = name
	}

	genRes, err := certutil.GenerateCA(certutil.CAGenRequest{
		CommonName:   cn,
		Organization: req.Organization,
		Country:      req.Country,
		KeyType:      req.KeyType,
		ValidDays:    req.ValidDays,
	})
	if err != nil {
		util.BadRequest(c, "生成 CA 失败: "+err.Error())
		return
	}

	ca := models.CACert{
		Name:         name,
		CommonName:   cn,
		Organization: req.Organization,
		Country:      req.Country,
		CACertPEM:    genRes.CertPEM,
		CAKeyPEM:     genRes.KeyPEM,
		HasKey:       true,
		NotAfter:     genRes.NotAfter,
		KeyType:      genRes.KeyType,
		Remark:       req.Remark,
	}

	if err := d.DB.Create(&ca).Error; err != nil {
		if db.IsUniqueViolation(err, "ca_certs.name") {
			util.BadRequest(c, "该 CA 名称已存在")
			return
		}
		util.ServerError(c, "保存 CA 证书失败")
		return
	}

	util.OK(c, gin.H{"ca_cert": toCACertView(&ca, 0)})
}

// AdminIssueCertFromCA POST /api/v1/admin/ca-certs/:id/issue —— 基于 CA 签发服务端 TLS 证书
func (d *Deps) AdminIssueCertFromCA(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 CA ID")
		return
	}

	var ca models.CACert
	if err := d.DB.First(&ca, id).Error; err != nil {
		util.Fail(c, 404, "CA 证书不存在")
		return
	}

	if !ca.HasKey || ca.CAKeyPEM == "" {
		util.BadRequest(c, "该 CA 为纯公钥验签证书（无私钥），无法签发新证书")
		return
	}

	var req issueCertFromCAForm
	if !util.BindJSON(c, &req) {
		return
	}
	domain := strings.TrimSpace(req.Domain)
	if domain == "" {
		util.BadRequest(c, "域名不能为空")
		return
	}

	// 签发证书
	signRes, err := certutil.SignServerCert(ca.CACertPEM, ca.CAKeyPEM, certutil.CertSignRequest{
		CommonName:   domain,
		SANDNS:       req.SANDNS,
		SANIP:        req.SANIP,
		Organization: req.Organization,
		Country:      req.Country,
		KeyType:      req.KeyType,
		ValidDays:    req.ValidDays,
	})
	if err != nil {
		util.BadRequest(c, "签发证书失败: "+err.Error())
		return
	}

	cert := models.Cert{
		Domain:     domain,
		CertPEM:    signRes.FullchainPEM,
		KeyPEM:     signRes.KeyPEM,
		NotAfter:   signRes.NotAfter,
		PinSHA256:  signRes.PinSHA256,
		SelfSigned: false,
		CAID:       &ca.ID,
		KeyType:    signRes.KeyType,
		Remark:     req.Remark,
	}

	if err := d.DB.Create(&cert).Error; err != nil {
		if db.IsUniqueViolation(err, "certs.domain") {
			util.BadRequest(c, "该域名证书已存在")
			return
		}
		util.ServerError(c, "保存证书记录失败")
		return
	}

	// 自动推送证书到引用节点
	d.pushCertToUsers(&cert)

	view := toCertView(&cert)
	view.CAName = ca.Name
	util.OK(c, gin.H{"cert": view})
}

// AdminDeleteCACert DELETE /api/v1/admin/ca-certs/:id —— 删除 CA 证书（有签发记录时拒绝）
func (d *Deps) AdminDeleteCACert(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "无效的 ID")
		return
	}

	var count int64
	d.DB.Model(&models.Cert{}).Where("ca_id = ?", id).Count(&count)
	if count > 0 {
		util.BadRequest(c, "该 CA 已签发 "+strconv.FormatInt(count, 10)+" 张证书，请先删除关联证书")
		return
	}

	if err := d.DB.Delete(&models.CACert{}, id).Error; err != nil {
		util.ServerError(c, "删除 CA 证书失败")
		return
	}
	util.OK(c, gin.H{"ok": true})
}
