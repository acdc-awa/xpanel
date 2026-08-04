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

	var users []models.User
	if err := s.DB.Where("status = ?", models.StatusActive).Find(&users).Error; err != nil {
		return nil, err
	}

	var plans []models.Plan
	_ = s.DB.Find(&plans)
	planMap := make(map[uint64]models.Plan, len(plans))
	for _, p := range plans {
		planMap[p.ID] = p
	}

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

	now := time.Now()
	validUsers := make([]models.User, 0, len(users))
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
						limitBytes := plan.TrafficGB * 1024 * 1024 * 1024
						if up+down >= limitBytes {
							continue
						}
					}
				}
			}
		}
		validUsers = append(validUsers, u)
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
			if inb.Network == "tcp" && inb.TLSType == "reality" {
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
	var users []models.User
	if err := s.DB.Where("status = ?", models.StatusActive).Find(&users).Error; err != nil {
		return "", err
	}

	now := time.Now()
	var plans []models.Plan
	_ = s.DB.Find(&plans)
	planMap := make(map[uint64]models.Plan, len(plans))
	for _, p := range plans {
		planMap[p.ID] = p
	}
	var validUsers []models.User
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
		validUsers = append(validUsers, u)
	}

	cfg, err := xray.Generate(&srv, inbounds, outbounds, routingRules, validUsers)
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

// MarkPushedByServer 按服务器标记配置已推送。
func (s *ConfigService) MarkPushedByServer(serverID uint64) error {
	now := time.Now()
	return s.DB.Model(&models.PendingConfig{}).Where("server_id = ?", serverID).Updates(map[string]any{
		"status":     "pushed",
		"pushed_at":  now,
		"updated_at": now,
	}).Error
}

// MarkPushed 标记配置已推送。
func (s *ConfigService) MarkPushed(id uint64) error {
	now := time.Now()
	return s.DB.Model(&models.PendingConfig{}).Where("id = ?", id).Updates(map[string]any{
		"status":    "pushed",
		"pushed_at": now,
		"updated_at": now,
	}).Error
}