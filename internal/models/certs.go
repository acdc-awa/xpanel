package models

import "time"

// Cert 证书表（Phase T：主控统一管理 TLS 证书，push_cert 下发节点落盘）。
// CertPEM/KeyPEM 不回传 API（json:"-"），避免私钥泄露。
// PinSHA256：leaf 证书 DER 的 SHA-256（hex），上传/生成时自动计算；
// 链式代理 TLS 中转出站自动注入 pinnedPeerCertSha256（自签证书防 MITM）。
// CACert CA 根证书表（统一管理自建或导入的根 CA，用于签发服务端 TLS 证书或下发节点验签）。
type CACert struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	Name         string    `gorm:"size:128;not null;uniqueIndex" json:"name"` // 唯一标识/别名
	CommonName   string    `gorm:"size:128" json:"common_name"`
	Organization string    `gorm:"size:128" json:"organization"`
	Country      string    `gorm:"size:64" json:"country"`
	CACertPEM    string    `gorm:"type:text;not null" json:"ca_cert_pem"` // CA 根证书公钥 PEM
	CAKeyPEM     string    `gorm:"type:text" json:"-"`                   // CA 私钥 PEM（json:"-" 防泄露）
	HasKey       bool      `gorm:"default:false" json:"has_key"`         // 是否持有私钥（可签发）
	NotAfter     time.Time `json:"not_after"`
	KeyType      string    `gorm:"size:32" json:"key_type"`              // ecdsa_p256, rsa_2048, etc.
	Remark       string    `gorm:"size:255" json:"remark"`
	CreatedAt    time.Time `json:"created_at"`
}

type Cert struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	Domain     string    `gorm:"size:128;not null;uniqueIndex" json:"domain"`
	CertPEM    string    `gorm:"type:text" json:"-"` // 证书链 PEM
	KeyPEM     string    `gorm:"type:text" json:"-"` // 私钥 PEM
	NotAfter   time.Time `json:"not_after"`          // 到期时间（上传时解析自证书）
	PinSHA256  string    `gorm:"size:64" json:"pin_sha256"`   // leaf DER SHA-256（hex）
	SelfSigned bool      `gorm:"default:false" json:"self_signed"` // 面板一键生成的自签证书
	CAID       *uint64   `gorm:"index" json:"ca_id,omitempty"`     // 关联的 CA 证书 ID（由 CA 签发时记录）
	KeyType    string    `gorm:"size:32" json:"key_type,omitempty"` // 密钥算法类型
	Remark     string    `gorm:"size:255" json:"remark"`
	CreatedAt  time.Time `json:"created_at"`
}
