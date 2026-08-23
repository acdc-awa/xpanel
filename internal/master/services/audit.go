package services

import (
	"time"

	"gorm.io/gorm"

	"github.com/acdc-awa/xpanel/internal/models"
)

// AuditService 审计日志。
type AuditService struct {
	DB *gorm.DB
}

// Log 记录一条审计日志。
func (s *AuditService) Log(opType string, opID uint64, action, detail, ip string) {
	if s == nil || s.DB == nil {
		return
	}
	_ = s.DB.Create(&models.AuditLog{
		OperatorType: opType,
		OperatorID:   opID,
		Action:       action,
		Detail:       detail,
		IP:           ip,
		CreatedAt:    time.Now(),
	}).Error
}
