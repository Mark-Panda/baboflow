package service

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"

	"baboflow/internal/server/httputil"
)

// RateLimitMiddleware 基于 x/time/rate 的每键令牌桶限流。
// keyFunc 决定限流维度（如 IP、IP+账号）。b 为桶容量（突发），r 为每秒补充速率。
// 超出后返回 429 风格错误。后台定期清理空闲键，防止 map 无限增长。
func RateLimitMiddleware(keyFunc func(*gin.Context) string, r rate.Limit, b int) gin.HandlerFunc {
	type entry struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}
	var mu sync.Mutex
	buckets := map[string]*entry{}

	// 后台清理 10 分钟未命中的键
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			for k, e := range buckets {
				if time.Since(e.lastSeen) > 10*time.Minute {
					delete(buckets, k)
				}
			}
			mu.Unlock()
		}
	}()

	return func(c *gin.Context) {
		key := keyFunc(c)
		mu.Lock()
		e, ok := buckets[key]
		if !ok {
			e = &entry{limiter: rate.NewLimiter(r, b)}
			buckets[key] = e
		}
		e.lastSeen = time.Now()
		lim := e.limiter
		mu.Unlock()

		if !lim.Allow() {
			httputil.Fail(c, 429, "请求过于频繁，请稍后再试")
			c.Abort()
			return
		}
		c.Next()
	}
}
