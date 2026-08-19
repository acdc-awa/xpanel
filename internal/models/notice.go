package models

import "time"

// Notice 公告模型（支持置顶、强弹窗、排序与启用状态）。
type Notice struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Title     string    `gorm:"type:varchar(255);not null" json:"title"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	IsPinned  bool      `gorm:"default:false;index" json:"is_pinned"`
	IsPopup   bool      `gorm:"default:false" json:"is_popup"`
	Status    int       `gorm:"default:1;index" json:"status"` // 1: 启用, 0: 隐藏
	SortOrder int       `gorm:"default:0" json:"sort_order"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Notice) TableName() string {
	return "notices"
}
