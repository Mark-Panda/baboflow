package service

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/middleware"
	khttp "github.com/go-kratos/kratos/v2/transport/http"
	"golang.org/x/time/rate"
	"google.golang.org/grpc/peer"
)

const rateLimitReason = "RATE_LIMITED"

type rateLimitEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// RateLimiters owns the per-client login and trigger limiters for one app.
// Stop must be called during application shutdown to stop their cleanup workers.
type RateLimiters struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	login   middleware.Middleware
	trigger middleware.Middleware
}

func NewRateLimiters() *RateLimiters {
	ctx, cancel := context.WithCancel(context.Background())
	limits := &RateLimiters{ctx: ctx, cancel: cancel}
	limits.login = limits.newMiddleware("login:", rate.Every(2*time.Second), 5)
	limits.trigger = limits.newMiddleware("trigger:", rate.Every(time.Second), 10)
	return limits
}

func (r *RateLimiters) LoginMiddleware() middleware.Middleware { return r.login }

func (r *RateLimiters) TriggerMiddleware() middleware.Middleware { return r.trigger }

func (r *RateLimiters) Stop() {
	r.cancel()
	r.wg.Wait()
}

func (r *RateLimiters) newMiddleware(prefix string, limit rate.Limit, capacity int) middleware.Middleware {
	var mu sync.Mutex
	buckets := map[string]*rateLimitEntry{}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		cleanRateLimitBuckets(r.ctx, &mu, buckets)
	}()
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			key := prefix + clientIP(ctx)
			mu.Lock()
			entry, ok := buckets[key]
			if !ok {
				entry = &rateLimitEntry{limiter: rate.NewLimiter(limit, capacity)}
				buckets[key] = entry
			}
			entry.lastSeen = time.Now()
			allowed := entry.limiter.Allow()
			mu.Unlock()
			if !allowed {
				return nil, kerrors.New(http.StatusTooManyRequests, rateLimitReason, "请求过于频繁，请稍后再试")
			}
			return handler(ctx, req)
		}
	}
}

func cleanRateLimitBuckets(ctx context.Context, mu *sync.Mutex, buckets map[string]*rateLimitEntry) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mu.Lock()
			for key, entry := range buckets {
				if time.Since(entry.lastSeen) > 10*time.Minute {
					delete(buckets, key)
				}
			}
			mu.Unlock()
		}
	}
}

func clientIP(ctx context.Context) string {
	if request, ok := khttp.RequestFromServerContext(ctx); ok {
		return hostOnly(request.RemoteAddr)
	}
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		return hostOnly(p.Addr.String())
	}
	return ""
}

func hostOnly(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return address
}
