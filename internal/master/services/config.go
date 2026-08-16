package services

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/master/xray"
	"github.com/zhx/xray-panel/internal/models"
	"github.com/zhx/xray-panel/internal/pkg/protocol"
)

// ConfigService 服务器 Xray 配置的生成与待推送管理。
type ConfigService struct {
	DB      *gorm.DB
	Traffic *TrafficService
}

// BuildGenerateContext 组装生成器拓扑化上下文（Phase T T3）：
// InboundRef → 目标入站（可跨服务器，含 Server Host）与 CertID → 域名映射。
func BuildGenerateContext(db *gorm.DB, inbounds []models.Inbound, outbounds []models.ServerOutbound) (*xray.GenerateContext, error) {
	ctx := &xray.GenerateContext{
		RefTargets:  map[uint64]xray.RefTarget{},
		CertDomains: map[uint64]string{},
	}
	// 证书映射（CertID → 域名）
	certIDs := make([]uint64, 0, len(inbounds))
	for i := range inbounds {
		if inbounds[i].CertID != nil {
			certIDs = append(certIDs, *inbounds[i].CertID)
		}
	}
	if len(certIDs) > 0 {
		var certs []models.Cert
		if err := db.Where("id IN ?", certIDs).Find(&certs).Error; err != nil {
			return nil, err
		}
		for _, c := range certs {
			ctx.CertDomains[c.ID] = c.Domain
		}
	}
	// 引用映射（出站 InboundRef → 目标入站 + 服务器 Host）
	refIDs := make([]uint64, 0, len(outbounds))
	for i := range outbounds {
		if outbounds[i].InboundRef != nil {
			refIDs = append(refIDs, *outbounds[i].InboundRef)
		}
	}
	if len(refIDs) > 0 {
		var targets []models.Inbound
		if err := db.Where("id IN ?", refIDs).Find(&targets).Error; err != nil {
			return nil, err
		}
		for _, t := range targets {
			var srv models.Server
			if err := db.First(&srv, t.ServerID).Error; err != nil {
				continue // 目标服务器缺失：Generate 预检会报引用不存在
			}
			ctx.RefTargets[t.ID] = xray.RefTarget{Inbound: t, ServerHost: srv.Host}
		}
	}
	return ctx, nil
}

// GetValidUsers 计算服务器各个 Inbound 当前有效的用户列表 (InboundTag -> []protocol.User)。
// 只遍历 type=user 入站（relay 内部账户不参与 SyncUsers，T4）。
// 权限控制按权限组匹配（节点入站定义开放权限组，用户继承/指定权限组）。
// 单一数据源：热更新 SyncUsers 与全量配置生成（Generate）共用本函数（批7 修正访问控制缺口）。
func (s *ConfigService) GetValidUsers(serverID uint64) (map[string][]protocol.User, error) {
	var inbounds []models.Inbound
	if err := s.DB.Where("server_id = ? AND enabled = ? AND type = ?", serverID, true, models.InboundTypeUser).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	res := make(map[string][]protocol.User)
	if len(inbounds) == 0 {
		return res, nil
	}

	validUsers := s.filterValidUsers()

	inboundIDs := make([]uint64, 0, len(inbounds))
	for _, inb := range inbounds {
		inboundIDs = append(inboundIDs, inb.ID)
	}
	inboundGroupMap := BatchInboundPermissionGroupIDs(s.DB, inboundIDs)

	for _, inb := range inbounds {
		res[inb.Tag] = s.protoUsersFor(validUsers, &inb, inboundGroupMap[inb.ID])
	}

	return res, nil
}

// validUser 过滤后的用户候选：用户实体 + 预计算的生效权限组/设备限制/已用流量。
// 预计算目的：避免 protoUsersFor 在用户×入站维度反复查库（ISSUE-10）。
type validUser struct {
	User             models.User
	GroupID          uint64
	DeviceLimit      int
	UsedBytes        int64
	PlanTrafficBytes int64
}

// protoUsersFor 按权限组规则从 validUsers 计算单个入站的用户列表（GetValidUsers 与预览共用）。
// Xboard 规范：用户必须拥有生效权限组（uGroup > 0）；入站声明开放组时，用户组须在开放组内。
func (s *ConfigService) protoUsersFor(validUsers []validUser, inb *models.Inbound, allowedGroups []uint64) []protocol.User {
	hasGroupLimit := len(allowedGroups) > 0
	allowedGroupSet := make(map[uint64]bool, len(allowedGroups))
	for _, g := range allowedGroups {
		allowedGroupSet[g] = true
	}

	var protoUsers []protocol.User
	for _, vu := range validUsers {
		u := vu.User
		// 未分配权限组的用户（uGroup == 0）不具备任何节点的访问权限，不注入 UUID
		if vu.GroupID == 0 {
			continue
		}
		if hasGroupLimit && !allowedGroupSet[vu.GroupID] {
			continue
		}
		flow := inb.Flow
		if flow == "" && xray.StreamHasReality(inb.StreamSettings) && xray.StreamNetwork(inb.StreamSettings) == "tcp" {
			flow = "xtls-rprx-vision"
		} else if flow == "none" {
			flow = ""
		}
		protoUsers = append(protoUsers, protocol.User{
			UUID:  u.UUID,
			Email: xray.UserEmail(&u),
			Flow:  flow,
			Level: 0,
			Limit: vu.DeviceLimit,
		})
	}
	return protoUsers
}

// PreviewUsers 预览用：按表单（可能未入库）入站的开放权限组计算其用户列表（与 GetValidUsers 同规则）。
func (s *ConfigService) PreviewUsers(inb *models.Inbound, groupIDs []uint64) []protocol.User {
	if inb.Type == models.InboundTypeRelay || inb.Type == models.InboundTypeIdle {
		return nil
	}
	return s.protoUsersFor(s.filterValidUsers(), inb, groupIDs)
}

// filterValidUsers 返回全部有效的用户（状态正常、有 UUID、未过期、未超流量）。
// ISSUE-10：套餐、权限组与设备限制批量预取；已用流量单条 SQL 聚合，不再逐用户查询。
func (s *ConfigService) filterValidUsers() []validUser {
	var users []models.User
	if err := s.DB.Where("status = ?", models.StatusActive).Find(&users).Error; err != nil {
		return nil
	}
	if len(users) == 0 {
		return nil
	}

	var plans []models.Plan
	_ = s.DB.Find(&plans)
	planMap := make(map[uint64]models.Plan, len(plans))
	for _, p := range plans {
		planMap[p.ID] = p
	}

	// 单条 SQL 计算每个有效用户在其计费周期内的已用流量。
	type usedRow struct {
		UserID    uint64
		UsedBytes int64
	}
	var usedRows []usedRow
	s.DB.Raw(`
		SELECT u.id AS user_id,
		       COALESCE(SUM(CASE WHEN l.period_start >= u.traffic_cycle_start THEN l.up_bytes + l.down_bytes ELSE 0 END), 0) AS used_bytes
		FROM users u
		LEFT JOIN traffic_logs l ON l.user_id = u.id
		WHERE u.status = ?
		GROUP BY u.id, u.traffic_cycle_start`, models.StatusActive).Scan(&usedRows)
	usedMap := make(map[uint64]int64, len(usedRows))
	for _, r := range usedRows {
		usedMap[r.UserID] = r.UsedBytes
	}

	now := time.Now()
	valid := make([]validUser, 0, len(users))
	for _, u := range users {
		if u.UUID == "" {
			continue
		}
		if u.ExpireAt != nil && now.After(*u.ExpireAt) {
			continue
		}

		vu := validUser{User: u}
		if plan, ok := planMap[u.PlanID]; ok && plan.Enabled {
			if u.DeviceLimit > 0 {
				vu.DeviceLimit = u.DeviceLimit
			} else {
				vu.DeviceLimit = plan.DeviceLimit
			}
			if u.PermissionGroupID > 0 {
				vu.GroupID = u.PermissionGroupID
			} else {
				vu.GroupID = plan.PermissionGroupID
			}
			vu.PlanTrafficBytes = plan.TrafficGB * 1024 * 1024 * 1024
		} else {
			// 无有效套餐：设备限制仅看用户自定义；权限组仅看用户显式分组。
			vu.DeviceLimit = u.DeviceLimit
			vu.GroupID = u.PermissionGroupID
		}

		if vu.PlanTrafficBytes > 0 {
			vu.UsedBytes = usedMap[u.ID]
			if vu.UsedBytes >= vu.PlanTrafficBytes {
				continue
			}
		}
		valid = append(valid, vu)
	}
	return valid
}

// Generate 为服务器生成完整 Xray 配置（启用入站 + 节点出站 + 节点路由 + 按权限组过滤的用户）。
// 用户列表与热更新 SyncUsers 同源（GetValidUsers：有效期/流量/权限组过滤统一在此处完成），
// 保证全量推送与增量同步的访问控制一致（12 号文档：订阅/热更新/配置生成消费同一组计算结果）。
// 无启用入站时返回仅含 api 入站的配置（用于全停用后清理节点入站）；
// 入站无可用用户时输出空 clients（U25：清空配置推得动，节点立即移除失效用户）。
func (s *ConfigService) Generate(serverID uint64) (string, error) {
	var srv models.Server
	if err := s.DB.First(&srv, serverID).Error; err != nil {
		return "", err
	}
	var inbounds []models.Inbound
	if err := s.DB.Where("server_id = ? AND enabled = ?", serverID, true).Find(&inbounds).Error; err != nil {
		return "", err
	}
	inbounds = FilterAvailableInbounds(inbounds)
	var outbounds []models.ServerOutbound
	if err := s.DB.Where("server_id = ? AND enabled = ?", serverID, true).Order("priority asc, id asc").Find(&outbounds).Error; err != nil {
		return "", err
	}
	var routingRules []models.ServerRoutingRule
	if err := s.DB.Where("server_id = ? AND enabled = ?", serverID, true).Order("priority asc, id asc").Find(&routingRules).Error; err != nil {
		return "", err
	}

	// 用户按入站 tag 分组（与热更新 SyncUsers 同一函数、同一过滤规则）
	usersByTag, err := s.GetValidUsers(serverID)
	if err != nil {
		return "", err
	}

	ctx, err := BuildGenerateContext(s.DB, inbounds, outbounds)
	if err != nil {
		return "", err
	}
	cfg, err := xray.Generate(inbounds, outbounds, routingRules, usersByTag, ctx, srv.DefaultOutboundTag, srv.RoutingDomainStrategy, srv.DefaultOutboundDS)
	if err != nil {
		return "", err
	}
	return string(cfg), nil
}

// SavePending 保存（覆盖）服务器最新待推送配置。
func (s *ConfigService) SavePending(serverID uint64, configJSON string) error {
	var p models.PendingConfig
	err := s.DB.Where("server_id = ?", serverID).First(&p).Error
	now := time.Now()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.DB.Create(&models.PendingConfig{
			ServerID:   serverID,
			ConfigJSON: configJSON,
			Status:     "pending",
			CreatedAt:  now,
			UpdatedAt:  now,
		}).Error
	}
	if err != nil {
		return err
	}
	return s.DB.Model(&p).Updates(map[string]any{
		"config_json": configJSON,
		"status":      "pending",
		"pushed_at":   nil,
		"updated_at":  now,
	}).Error
}

// GetPending 读取服务器待推送配置（不存在返回 nil）。
func (s *ConfigService) GetPending(serverID uint64) (*models.PendingConfig, error) {
	var p models.PendingConfig
	err := s.DB.Where("server_id = ?", serverID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// MarkPushedIfSame 仅当待推送配置内容仍为本次已下发内容（未被并发 SavePending 覆盖）
// 且状态为 pending 时标记已推送，返回是否实际标记。
// 背景：SavePending 覆盖的是同一行（ID 不变），若旧内容的推送回执到达时行内容已被
// 更新，盲目按 ID 标记会把"从未下发的新内容"误标为 pushed，导致节点长期运行旧配置。
// 内容不匹配时保持 pending，等待下一轮推送（用户编辑/重连补推/每小时校准）。
func (s *ConfigService) MarkPushedIfSame(id uint64, configJSON string) (bool, error) {
	now := time.Now()
	res := s.DB.Model(&models.PendingConfig{}).
		Where("id = ? AND config_json = ? AND status = ?", id, configJSON, "pending").
		Updates(map[string]any{
			"status":     "pushed",
			"pushed_at":  now,
			"updated_at": now,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// MarkPushedByServerIfSame 同 MarkPushedIfSame，按服务器 + 内容匹配标记
// （供 API 生成下发后的确认路径使用）。
func (s *ConfigService) MarkPushedByServerIfSame(serverID uint64, configJSON string) (bool, error) {
	now := time.Now()
	res := s.DB.Model(&models.PendingConfig{}).
		Where("server_id = ? AND config_json = ? AND status = ?", serverID, configJSON, "pending").
		Updates(map[string]any{
			"status":     "pushed",
			"pushed_at":  now,
			"updated_at": now,
		})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// FilterAvailableInbounds 过滤不可用入站（J9 激活，订阅与生成双端同源）：
// Total>0 且 up+down >= Total（入站总流量跑满）→ 移除；ExpiryTime 已过 → 移除。
func FilterAvailableInbounds(inbounds []models.Inbound) []models.Inbound {
	now := time.Now()
	out := inbounds[:0]
	for _, inb := range inbounds {
		if inb.Total > 0 && inb.Up+inb.Down >= inb.Total {
			continue
		}
		if inb.ExpiryTime != nil && now.After(*inb.ExpiryTime) {
			continue
		}
		out = append(out, inb)
	}
	return out
}
