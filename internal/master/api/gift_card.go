package api

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/acdc-awa/xpanel/internal/master/middleware"
	"github.com/acdc-awa/xpanel/internal/models"
	"github.com/acdc-awa/xpanel/internal/pkg/util"
)

// UserRedeemGiftCard POST /api/v1/user/gift-cards/redeem —— 用户卡密兑换充值。
func (d *Deps) UserRedeemGiftCard(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "请输入卡密")
		return
	}

	card, newBalance, err := d.GiftCard.Redeem(uid, req.Code)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}

	d.Audit.Log("user", uid, "gift_card.redeem", "兑换礼品卡 "+card.Code+" (+¥"+strconv.FormatFloat(float64(card.FaceValueCents)/100, 'f', 2, 64)+")", util.ClientIPFromContext(c))
	util.OK(c, gin.H{
		"face_value_cents": card.FaceValueCents,
		"balance_cents":    newBalance,
		"card_name":        card.Name,
	})
}

// UserPayOrderByBalance POST /api/v1/user/orders/pay-balance —— 余额直付购买/续费套餐。
func (d *Deps) UserPayOrderByBalance(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req struct {
		PlanID uint64 `json:"plan_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误")
		return
	}

	order, err := d.Order.PayWithBalance(uid, req.PlanID)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}

	// 用户热更新已由 Stage 5 事件驱动（billing 发布 OrderPaidEvent → 订阅方 SyncUsersToAll），
	// 此处不再直调 Hub——任何新增支付路径自动获得同步能力。

	ov := d.toOrderView(order)
	detail := "余额直付购买套餐"
	if ov.PlanName != "" {
		detail += "「" + ov.PlanName + "」"
	}
	d.Audit.Log("user", uid, "order.pay_balance", detail+" #"+strconv.FormatUint(req.PlanID, 10)+" (订单 #"+order.OrderNo+")", util.ClientIPFromContext(c))
	util.OK(c, gin.H{
		"order": ov,
	})
}

// UserBalanceLogs GET /api/v1/user/balance-logs —— 个人余额变动明细。
func (d *Deps) UserBalanceLogs(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	page := atoiDefault(c.Query("page"), 1)
	size := atoiDefault(c.Query("size"), 20)

	list, total, err := d.GiftCard.ListBalanceLogs(uid, page, size)
	if err != nil {
		util.ServerError(c, "查询明细失败")
		return
	}
	util.OK(c, gin.H{
		"items": list,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// AdminGiftCards GET /api/v1/admin/gift-cards —— 管理端礼品卡列表。
func (d *Deps) AdminGiftCards(c *gin.Context) {
	page := atoiDefault(c.Query("page"), 1)
	size := atoiDefault(c.Query("size"), 20)
	status := c.Query("status")
	search := c.Query("search")

	list, total, err := d.GiftCard.ListCards(page, size, status, search)
	if err != nil {
		util.ServerError(c, "查询礼品卡失败")
		return
	}

	// 批量补兑换人用户名，前端不再显示裸编号；查询失败静默降级
	usedIDs := make([]uint64, 0, len(list))
	seenUsed := make(map[uint64]bool, len(list))
	for _, card := range list {
		if card.UsedBy != 0 && !seenUsed[card.UsedBy] {
			seenUsed[card.UsedBy] = true
			usedIDs = append(usedIDs, card.UsedBy)
		}
	}
	nameByUsed := make(map[uint64]string, len(usedIDs))
	if len(usedIDs) > 0 {
		var users []models.User
		if err := d.DB.Select("id", "username").Where("id IN ?", usedIDs).Find(&users).Error; err == nil {
			for _, u := range users {
				nameByUsed[u.ID] = u.Username
			}
		}
	}
	type cardView struct {
		models.GiftCard
		UsedByUsername string `json:"used_by_username"`
	}
	items := make([]cardView, 0, len(list))
	for i := range list {
		items = append(items, cardView{GiftCard: list[i], UsedByUsername: nameByUsed[list[i].UsedBy]})
	}
	util.OK(c, gin.H{
		"items": items,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// AdminBatchCreateGiftCards POST /api/v1/admin/gift-cards —— 批量生成礼品卡。
func (d *Deps) AdminBatchCreateGiftCards(c *gin.Context) {
	var req struct {
		Count          int    `json:"count" binding:"required,min=1,max=500"`
		Name           string `json:"name"`
		FaceValueCents int64  `json:"face_value_cents" binding:"required,min=1"`
		ExpiresAt      string `json:"expires_at"` // RFC3339 可选
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	var exp *time.Time
	if req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err == nil {
			exp = &t
		}
	}

	adminID := middleware.CurrentUser(c)
	cards, err := d.GiftCard.BatchGenerate(adminID, req.Count, req.Name, req.FaceValueCents, exp)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}

	util.OK(c, gin.H{
		"items": cards,
		"count": len(cards),
	})
}

// AdminDeleteGiftCard DELETE /api/v1/admin/gift-cards/:id —— 删除/作废礼品卡。
func (d *Deps) AdminDeleteGiftCard(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法 ID")
		return
	}
	if err := d.GiftCard.DisableOrDelete(id); err != nil {
		util.BadRequest(c, err.Error())
		return
	}
	util.OK(c, gin.H{"deleted": id})
}

// AdminAdjustUserBalance POST /api/v1/admin/users/:id/balance —— 管理员调整用户余额。
func (d *Deps) AdminAdjustUserBalance(c *gin.Context) {
	targetUID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.BadRequest(c, "非法用户 ID")
		return
	}
	var req struct {
		AmountCents int64  `json:"amount_cents" binding:"required"`
		Remark      string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		util.BadRequest(c, "参数错误: "+err.Error())
		return
	}

	adminID := middleware.CurrentUser(c)
	newBalance, err := d.GiftCard.AdminAdjustBalance(adminID, targetUID, req.AmountCents, req.Remark)
	if err != nil {
		util.BadRequest(c, err.Error())
		return
	}

	util.OK(c, gin.H{
		"user_id":       targetUID,
		"new_balance":   newBalance,
		"balance_cents": newBalance,
	})
}
