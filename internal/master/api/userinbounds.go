package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/util"
)

// AdminUserInbounds GET /api/v1/admin/users/:id/inbounds
// 返回全部启用入站 + 该用户已授权列表（用于勾选授权）。
func (d *Deps) AdminUserInbounds(c *gin.Context) {
	uid, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var inbounds []models.Inbound
	if err := d.DB.Where("enabled = ?", true).Order("id ASC").Find(&inbounds).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	var grants []models.UserInbound
	if err := d.DB.Where("user_id = ?", uid).Find(&grants).Error; err != nil {
		util.ServerError(c, "查询失败")
		return
	}
	granted := make([]uint64, 0, len(grants))
	for _, g := range grants {
		if g.Enabled {
			granted = append(granted, g.InboundID)
		}
	}
	util.OK(c, gin.H{"inbounds": inbounds, "granted_ids": granted})
}

// AdminSetUserInbounds POST /api/v1/admin/users/:id/inbounds
// body: {"inbound_ids": [1,2,3]} —— 全量替换授权（空数组 = 清空 → 订阅回退全部入站）。
func (d *Deps) AdminSetUserInbounds(c *gin.Context) {
	uid, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	var req struct {
		InboundIDs []uint64 `json:"inbound_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
		return
	}
	if err := d.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", uid).Delete(&models.UserInbound{}).Error; err != nil {
			return err
		}
		if len(req.InboundIDs) == 0 {
			return nil
		}
		rows := make([]models.UserInbound, 0, len(req.InboundIDs))
		for _, iid := range req.InboundIDs {
			rows = append(rows, models.UserInbound{UserID: uid, InboundID: iid, Enabled: true})
		}
		return tx.Create(&rows).Error
	}); err != nil {
		util.ServerError(c, "保存授权失败")
		return
	}
	if d.Hub != nil {
		d.Hub.SyncUsersToAll()
	}
	util.OK(c, gin.H{"user_id": uid, "granted": len(req.InboundIDs)})
}