package services

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/zhx/xray-panel/internal/master/xray"
	"github.com/zhx/xray-panel/internal/models"
)

// ConfigService 服务器 Xray 配置的生成与待推送管理。
type ConfigService struct {
	DB *gorm.DB
}

// Generate 为服务器生成完整 Xray 配置（启用入站 + 全部启用用户）。
// 无启用入站或无可用用户时返回错误（此时不需要推送）。
func (s *ConfigService) Generate(serverID uint64) (string, error) {
	var srv models.Server
	if err := s.DB.First(&srv, serverID).Error; err != nil {
		return "", err
	}
	var inbounds []models.Inbound
	if err := s.DB.Where("server_id = ? AND enabled = ?", serverID, true).Find(&inbounds).Error; err != nil {
		return "", err
	}
	if len(inbounds) == 0 {
		return "", errors.New("服务器无启用入站")
	}
	var users []models.User
	if err := s.DB.Where("status = ?", models.StatusActive).Find(&users).Error; err != nil {
		return "", err
	}
	cfg, err := xray.Generate(&srv, inbounds, users)
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

// MarkPushed 标记配置已推送。
func (s *ConfigService) MarkPushed(id uint64) error {
	now := time.Now()
	return s.DB.Model(&models.PendingConfig{}).Where("id = ?", id).Updates(map[string]any{
		"status":    "pushed",
		"pushed_at": now,
		"updated_at": now,
	}).Error
}