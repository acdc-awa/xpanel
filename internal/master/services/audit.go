package services

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
)

// AuditService 审计日志。
type AuditService struct {
	DB *gorm.DB
}

// AuditEvent 见 contracts.AuditEvent：身份与目标都写成「列 + 写入时快照」，
// 改邮箱、删账号都不会改写历史行，检索也直接打在列上。
type AuditEvent = contracts.AuditEvent

// LogEvent 记录一条审计事件（detail 为 v2 信封：{"v":2,"message":中文句子,...}）。
func (s *AuditService) LogEvent(ev AuditEvent) {
	if s == nil || s.DB == nil || strings.TrimSpace(ev.Action) == "" {
		return
	}
	if ev.OperatorName == "" && ev.OperatorID > 0 {
		ev.OperatorName = auditEmailByID(s.DB, ev.OperatorID)
	}
	if ev.TargetType == "" {
		ev.TargetType = models.AuditTargetType(ev.Action)
	}
	_ = s.DB.Create(&models.AuditLog{
		OperatorType: ev.OperatorType,
		OperatorID:   ev.OperatorID,
		OperatorName: ev.OperatorName,
		Action:       ev.Action,
		Category:     models.AuditCategory(ev.Action),
		TargetType:   ev.TargetType,
		TargetID:     ev.TargetID,
		TargetName:   ev.TargetName,
		Status:       ev.Status,
		Detail:       buildAuditEventDetail(ev),
		IP:           ev.IP,
		CreatedAt:    time.Now(),
	}).Error
}

// Log 兼容旧调用形态（句子直接作为 detail）：转成 v2 信封的 message 字段。
func (s *AuditService) Log(opType string, opID uint64, action, detail, ip string) {
	s.LogEvent(AuditEvent{
		OperatorType: opType,
		OperatorID:   opID,
		Action:       action,
		Status:       200,
		Message:      detail,
		IP:           ip,
	})
}

// buildAuditEventDetail 组装 v2 信封；message 是给人看的中文句子，Extra 放结构化补充。
func buildAuditEventDetail(ev AuditEvent) string {
	env := map[string]any{"v": 2}
	if ev.Message != "" {
		env["message"] = ev.Message
	}
	if ev.Status != 0 {
		env["status"] = ev.Status
	}
	for k, v := range ev.Extra {
		if _, exists := env[k]; !exists {
			env[k] = v
		}
	}
	out, err := json.Marshal(env)
	if err != nil {
		return "{}"
	}
	return string(out)
}

func auditEmailByID(db *gorm.DB, id uint64) string {
	var row struct{ Email string }
	err := db.Session(&gorm.Session{Logger: logger.Discard}).
		Model(&models.User{}).Select("email").Where("id = ?", id).Take(&row).Error
	if err != nil {
		return ""
	}
	return strings.TrimSpace(row.Email)
}
