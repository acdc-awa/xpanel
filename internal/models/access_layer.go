package models

import "time"

// AccessLayer 对外接入层（订阅端点语义的显式分组）。
// 对外 host/port/security 由管理员自定义，内部实现（直连 TLS / Caddy 反代 / CDN）对上层不可见：
// 入站挂层后其订阅对外端点由层决议（BuildNodeDTO 消费），分享地址/安全层覆写字段仅作可选覆写；
// 未挂层入站沿用 ShareAddrStrategy 直连端点语义。层删除时挂层入站回退原生（layer_id 置空）。
type AccessLayer struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	ServerID  uint64    `gorm:"index;not null" json:"server_id"`     // 所属服务器
	Name      string    `gorm:"size:64;not null" json:"name"`        // 层显示名（如「HK 443 反代层」）
	Host      string    `gorm:"size:255;not null" json:"host"`       // 对外连接端点（域名/IP，订阅与画布消费）
	Port      int       `gorm:"default:443;not null" json:"port"`    // 对外端口（反代/CDN 通常 443）
	Security  string    `gorm:"size:16;default:tls" json:"security"` // 对外安全层 tls / none
	Remark    string    `gorm:"size:255" json:"remark"`              // 备注说明
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
