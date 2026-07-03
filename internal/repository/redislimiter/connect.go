package redislimiter

import (
	"context"
	"highload-analytics/config"

	"github.com/redis/go-redis/v9"
)

func NewRedisConnect(ctx context.Context, cfg *config.Config) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.RedisAddress,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return rdb, nil
}
