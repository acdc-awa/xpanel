package backup

import (
	"context"
	"log"

	"github.com/robfig/cron/v3"
)

// Start 启动定时备份（cfg.Enabled=false 或 schedule 非法时静默降级为仅手动）。
// 返回的 func 可调用 Stop() 停止 cron。
func (s *Service) Start(ctx context.Context) {
	if !s.enabled {
		return
	}
	c := cron.New(cron.WithSeconds())
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
