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

// GetValidUsers 计算服务器各个 Inbound 当前有效的用户列表 (InboundTag -> []protocol.User)。
func (s *ConfigService) GetValidUsers(serverID uint64) (map[string][]protocol.User, error) {
	var inbounds []models.Inbound
	if err := s.DB.Where("server_id = ? AND enabled = ?", serverID, true).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	res := make(map[string][]protocol.User)
	if len(inbounds) == 0 {
		return res, nil
	}

	validUsers := s.filterValidUsers()

	var allGrants []models.UserInbound
	_ = s.DB.Where("enabled = ?", true).Find(&allGrants)
	userHasGrants := make(map[uint64]bool)
	userGrantedInbounds := make(map[uint64]map[uint64]bool)
	for _, g := range allGrants {
		userHasGrants[g.UserID] = true
		if userGrantedInbounds[g.UserID] == nil {
			userGrantedInbounds[g.UserID] = make(map[uint64]bool)
		}
		userGrantedInbounds[g.UserID][g.InboundID] = true
	}

	for _, inb := range inbounds {
		var protoUsers []protocol.User
		for _, u := range validUsers {
			if userHasGrants[u.ID] {
				if !userGrantedInbounds[u.ID][inb.ID] {
					continue
				}
			}
			flow := ""
			if xray.StreamHasReality(inb.StreamSettings) && xray.StreamNetwork(inb.StreamSettings) == "tcp" {
				flow = "xtls-rprx-vision"
			}
			protoUsers = append(protoUsers, protocol.User{
				UUID:  u.UUID,
				Email: xray.UserEmail(u.ID),
				Flow:  flow,
				Level: 0,
			})
		}
		res[inb.Tag] = protoUsers
	}

	return res, nil
}

// filterValidUsers 返回全部有效的用户（状态正常、有 UUID、未过期、未超流量）。
// 与 GetValidUsers 共享同一过滤逻辑，避免两处重复。
func (s *ConfigService) filterValidUsers() []models.User {
	var users []models.User
	if err := s.DB.Where("status = ?", models.StatusActive).Find(&users).Error; err != nil {
		return nil
	}
	var plans []models.Plan
	_ = s.DB.Find(&plans)
	planMap := make(map[uint64]models.Plan, len(plans))
	for _, p := range plans {
		planMap[p.ID] = p
	}
	now := time.Now()
	valid := make([]models.User, 0, len(users))
	for _, u := range users {
		if u.UUID == "" {
			continue
		}
		if u.ExpireAt != nil && now.After(*u.ExpireAt) {
			continue
		}
		if u.PlanID > 0 {
			if plan, ok := planMap[u.PlanID]; ok && plan.Enabled && plan.TrafficGB > 0 {
				if s.Traffic != nil {
					up, down, err := s.Traffic.UserUsed(u.ID)
					if err == nil {
						if up+down >= plan.TrafficGB*1024*1024*1024 {
							continue
						}
					}
				}
			}
		}
		valid = append(valid, u)
	}
	return valid
}

// Generate 为服务器生成完整 Xray 配置（启用入站 + 节点出站 + 节点路由 + 全部有效启用用户）。
// 无启用入站时返回仅含 api 入站的配置（用于全停用后清理节点入站）；
// 有启用入站但无可用用户时返回错误（此时不需要推送）。
func (s *ConfigService) Generate(serverID uint64) (string, error) {
	var srv models.Server
	if err := s.DB.First(&srv, serverID).Error; err != nil {
		return "", err
	}
	var inbounds []models.Inbound
	if err := s.DB.Where("server_id = ? AND enabled = ?", serverID, true).Find(&inbounds).Error; err != nil {
		return "", err
	}
	var outbounds []models.ServerOutbound
	if err := s.DB.Where("server_id = ? AND enabled = ?", serverID, true).Order("priority asc, id asc").Find(&outbounds).Error; err != nil {
		return "", err
	}
	var routingRules []models.ServerRoutingRule
	if err := s.DB.Where("server_id = ? AND enabled = ?", serverID, true).Order("priority asc, id asc").Find(&routingRules).Error; err != nil {
		return "", err
	}

	validUsers := s.filterValidUsers()

	cfg, err := xray.Generate(inbounds, outbounds, routingRules, validUsers)
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
