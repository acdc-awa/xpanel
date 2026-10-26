package models

import "time"

// PermissionGroup 权限组：一个用户属于一个权限组。
// 访问控制单点化：权限组通过「用户接入点（UserAccessPoint）」白名单决定用户可见的订阅入口；
// 套餐绑定权限组后，购买套餐自动获得该组所有接入点的访问权限。
type PermissionGroup struct {
	ID            uint64 `gorm:"primaryKey" json:"id"`
	Name          string `gorm:"size:64;uniqueIndex;not null" json:"name"`
	Remark        string `gorm:"size:255" json:"remark"`
	ClashTemplate string `gorm:"type:text" json:"clash_template,omitempty"` // 自定义 Clash/Mihomo 订阅模板（含绑定模板的快照；留空则走系统内置默认模板）
	// SubTemplateID 绑定的订阅模板库条目（2026-10-26）：
	//   NULL = 自定义（空键），生效内容读 ClashTemplate；
	//   非空 = 绑定模板库条目，生效内容读 SubTemplate.Content（活引用）。
	// 在组编辑器里保存 ClashTemplate 会自动清空本列（转为自定义）；删除被引用的模板时，
	// 服务端先把模板当前内容写进 ClashTemplate 再清空本列，保证订阅输出不跳变。
	SubTemplateID *uint64   `gorm:"index" json:"sub_template_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
