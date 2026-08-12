package models

import "time"

// Cert 证书表（Phase T：主控统一管理 TLS 证书，push_cert 下发节点落盘）。
// CertPEM/KeyPEM 不回传 API（json:"-"），避免私钥泄露。
type Cert struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Domain    string    `gorm:"size:128;not null;uniqueIndex" json:"domain"`
	CertPEM   string    `gorm:"type:text" json:"-"` // 证书链 PEM
	KeyPEM    string    `gorm:"type:text" json:"-"` // 私钥 PEM
	NotAfter  time.Time `json:"not_after"`          // 到期时间（上传时解析自证书）
	Remark    string    `gorm:"size:255" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
}
