package biz

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// StartPeriodicSync 启动时跑一次全量同步，并周期对账（防漏网）。
// onSync 在每次成功同步后调用，适合执行依赖 component_meta 已就绪的补齐任务。
func StartPeriodicSync(
	ctx context.Context,
	s *ComponentSync,
	interval time.Duration,
	logger log.Logger,
	onSync func(context.Context) error,
) {
	helper := log.NewHelper(logger)
	go func() {
		if res, err := s.Run(ctx); err != nil {
			helper.Errorf("component sync failed: %v", err)
		} else {
			helper.Infof("component sync: added=%d updated=%d removed=%d skipped=%d", res.Added, res.Updated, res.Removed, res.Skipped)
			if onSync != nil {
				if err := onSync(ctx); err != nil {
					helper.Errorf("component skill backfill failed: %v", err)
				}
			}
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if res, err := s.Run(ctx); err != nil {
					helper.Errorf("component periodic sync failed: %v", err)
				} else {
					if res.Added+res.Updated+res.Removed > 0 {
						helper.Infof("component resync: added=%d updated=%d removed=%d", res.Added, res.Updated, res.Removed)
					}
					if onSync != nil {
						if err := onSync(ctx); err != nil {
							helper.Errorf("component skill backfill failed: %v", err)
						}
					}
				}
			}
		}
	}()
}
