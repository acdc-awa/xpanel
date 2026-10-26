package models

import "time"

// SubTemplate 命名订阅模板库（2026-09-05；2026-10-26 起支持权限组绑定）：
// 管理员保存多份命名 Clash/Mihomo 订阅模板。权限组有两种使用方式：
//   - 绑定（PermissionGroup.SubTemplateID 指向本表）：活引用，改 Content 即更新所有绑定组；
//   - 组内自定义（PermissionGroup.ClashTemplate）：编辑器里保存的副本，不随本表变化。
type SubTemplate struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null" json:"name"`                // 模板名（同一显示名可重复，不做唯一约束）
	Content   string    `gorm:"type:text" json:"content"`                    // 模板正文（占位符语法与组模板一致）
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
