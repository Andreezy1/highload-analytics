package redislimiter

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var rateLimitScript = redis.NewScript(
	`local key = KEYS[1]
	local limit = tonumber(ARGV[1])
	local window_ms = tonumber(ARGV[2])

	local current = redis.call('INCR', key)
	if current == 1 then
    redis.call('PEXPIRE', key, window_ms)
	end

	if current > limit then
    return 0
	else
    return 1
	end`,
)

type Limiter struct {
	rdb *redis.Client
}

func NewRedisLimiter(rdb *redis.Client) *Limiter {
	return &Limiter{
		rdb: rdb,
	}
}

func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	cmd := rateLimitScript.Run(ctx, l.rdb, []string{key}, limit, window.Milliseconds())
	cmdInt, err := cmd.Int64()
	if err != nil {
		return false, fmt.Errorf("redis script execution: %w", err)
	}
	return cmdInt == 1, nil
}
