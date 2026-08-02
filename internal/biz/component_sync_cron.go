package biz

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// StartPeriodicSync 启动时跑一次全量同步，并周期对账（防漏网）。
func StartPeriodicSync(ctx context.Context, s *ComponentSync, interval time.Duration, logger log.Logger) {
	helper := log.NewHelper(logger)
	go func() {
		if res, err := s.Run(ctx); err != nil {
			helper.Errorf("component sync failed: %v", err)
		} else {
			helper.Infof("component sync: added=%d updated=%d removed=%d skipped=%d", res.Added, res.Updated, res.Removed, res.Skipped)
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
				} else if res.Added+res.Updated+res.Removed > 0 {
					helper.Infof("component resync: added=%d updated=%d removed=%d", res.Added, res.Updated, res.Removed)
				}
			}
		}
	}()
}
