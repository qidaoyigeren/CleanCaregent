package middleware

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"CleanCaregent/internal/config"
	"CleanCaregent/pkg/response"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

type RateLimiter struct {
	enabled     bool
	limit       rate.Limit
	burst       int
	mu          sync.Mutex
	clients     map[string]clientLimiter
	lastCleanup time.Time
	clientTTL   time.Duration
	redis       goredis.UniversalClient
	redisLimit  int64
}

type clientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func NewRateLimiter(cfg config.RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		enabled:   cfg.Enabled,
		limit:     rate.Limit(cfg.RequestsPerSecond),
		burst:     cfg.Burst,
		clients:   make(map[string]clientLimiter),
		clientTTL: 10 * time.Minute,
	}
}

func NewRedisRateLimiter(cfg config.RateLimitConfig, client goredis.UniversalClient) *RateLimiter {
	limiter := NewRateLimiter(cfg)
	limiter.redis = client
	limiter.redisLimit = int64(max(cfg.Burst, int(math.Ceil(cfg.RequestsPerSecond))))
	return limiter
}

func (l *RateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.enabled {
			c.Next()
			return
		}

		key := c.ClientIP()
		if l.redis != nil {
			allowed, err := l.allowRedis(c.Request.Context(), key)
			if err == nil {
				if !allowed {
					response.Error(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
					return
				}
				c.Next()
				return
			}
		}
		limiter := l.client(key)
		if !limiter.Allow() {
			response.Error(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
			return
		}
		c.Next()
	}
}

func (l *RateLimiter) allowRedis(ctx context.Context, key string) (bool, error) {
	window := time.Now().UTC().Unix()
	redisKey := fmt.Sprintf("ratelimit:%d:%s", window, key)
	const script = `
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('EXPIRE', KEYS[1], 2)
end
return current
`
	value, err := l.redis.Eval(ctx, script, []string{redisKey}).Int64()
	if err != nil {
		return false, err
	}
	return value <= l.redisLimit, nil
}

func (l *RateLimiter) client(key string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	l.cleanup(now)

	entry, ok := l.clients[key]
	if !ok {
		entry = clientLimiter{limiter: rate.NewLimiter(l.limit, l.burst)}
	}
	entry.lastSeen = now
	l.clients[key] = entry
	return entry.limiter
}

func (l *RateLimiter) cleanup(now time.Time) {
	if !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < time.Minute {
		return
	}
	for key, entry := range l.clients {
		if now.Sub(entry.lastSeen) > l.clientTTL {
			delete(l.clients, key)
		}
	}
	l.lastCleanup = now
}
