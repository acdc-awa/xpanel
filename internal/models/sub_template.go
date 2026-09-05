package models

import "time"

// SubTemplate 命名订阅模板库（2026-09-05）：
// 管理员保存多份命名 Clash/Mihomo 订阅模板，在权限组模板编辑器中快速载入。
// 应用 = 把 Content 写入目标 PermissionGroup.ClashTemplate（组级仍单份生效）。
type SubTemplate struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null" json:"name"`                // 模板名（同一显示名可重复，不做唯一约束）
	Content   string    `gorm:"type:text" json:"content"`                    // 模板正文（占位符语法与组模板一致）
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
