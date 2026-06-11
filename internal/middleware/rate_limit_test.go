package middleware

import (
	"context"
	"testing"

	"CleanCaregent/internal/config"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func TestRedisRateLimiter(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	limiter := NewRedisRateLimiter(config.RateLimitConfig{
		Enabled:           true,
		Backend:           "redis",
		RequestsPerSecond: 2,
		Burst:             2,
	}, client)
	for index := 0; index < 2; index++ {
		allowed, err := limiter.allowRedis(context.Background(), "127.0.0.1")
		if err != nil || !allowed {
			t.Fatalf("request %d: allowed=%v err=%v", index, allowed, err)
		}
	}
	allowed, err := limiter.allowRedis(context.Background(), "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("third request should be rate limited")
	}
}
