package backup

import (
	"context"
	"log"

	"github.com/robfig/cron/v3"
)

// Start 启动定时备份（cfg.Enabled=false 或 schedule 非法时静默降级为仅手动）。
// 停止调度：调用方 cancel ctx，Start 内部通过 ctx.Done() 停止 cron。
func (s *Service) Start(ctx context.Context) {
	if !s.enabled {
		return
	}
	if s.driver != "sqlite" {
		log.Printf("backup: 驱动 %q 暂不支持在线快照，定时备份已禁用（可接入外部 mysqldump）", s.driver)
		return
	}
	c := cron.New(cron.WithParser(cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)))
	spec := s.schedule
	if spec == "" {
		spec = "0 3 * * *"
	}
	if _, err := c.AddFunc(spec, func() {
		if _, err := s.Snapshot(); err != nil {
			log.Printf("backup: 定时备份失败: %v", err)
		}
	}); err != nil {
		log.Printf("backup: schedule %q 非法，定时备份禁用（手动触发仍可用）: %v", spec, err)
		return
	}
	c.Start()
	go func() {
		<-ctx.Done()
		c.Stop()
	}()
}
