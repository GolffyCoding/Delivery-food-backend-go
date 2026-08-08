package database

import (
	"context"
	"fmt"

	"github.com/opendelivery/opendelivery/configs"
	"github.com/redis/go-redis/v9"
)

func NewRedisClient(ctx context.Context, cfg configs.RedisConfig) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     50,
		MinIdleConns: 10,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return rdb, nil
}

func CloseRedisClient(rdb *redis.Client) error {
	return rdb.Close()
}
