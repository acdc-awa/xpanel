package models

import "time"

// Cert 证书表（Phase T：主控统一管理 TLS 证书，push_cert 下发节点落盘）。
// CertPEM/KeyPEM 不回传 API（json:"-"），避免私钥泄露。
// PinSHA256：leaf 证书 DER 的 SHA-256（hex），上传/生成时自动计算；
// 链式代理 TLS 中转出站自动注入 pinnedPeerCertSha256（自签证书防 MITM）。
type Cert struct {
	ID         uint64    `gorm:"primaryKey" json:"id"`
	Domain     string    `gorm:"size:128;not null;uniqueIndex" json:"domain"`
	CertPEM    string    `gorm:"type:text" json:"-"` // 证书链 PEM
	KeyPEM     string    `gorm:"type:text" json:"-"` // 私钥 PEM
	NotAfter   time.Time `json:"not_after"`          // 到期时间（上传时解析自证书）
	PinSHA256  string    `gorm:"size:64" json:"pin_sha256"`   // leaf DER SHA-256（hex）
	SelfSigned bool      `gorm:"default:false" json:"self_signed"` // 面板一键生成的自签证书
	Remark     string    `gorm:"size:255" json:"remark"`
	CreatedAt  time.Time `json:"created_at"`
}
