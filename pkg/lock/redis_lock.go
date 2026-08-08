package lock

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

var ErrNotAcquired = errors.New("lock not acquired")

var unlockScript = redis.NewScript(`
	if redis.call("get", KEYS[1]) == ARGV[1] then
		return redis.call("del", KEYS[1])
	else
		return 0
	end
`)

// RedisLocker implements order.Locker using a Redis SETNX-based mutex with a
// Lua-guarded release so only the holder can unlock.
type RedisLocker struct {
	rdb *redis.Client
}

func NewRedisLocker(rdb *redis.Client) *RedisLocker {
	return &RedisLocker{rdb: rdb}
}

func (l *RedisLocker) Acquire(ctx context.Context, resource string, ttl time.Duration) (func(context.Context) error, error) {
	key := fmt.Sprintf("lock:%s", resource)
	token := uuid.New().String()

	ok, err := l.rdb.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotAcquired
	}

	release := func(ctx context.Context) error {
		_, err := unlockScript.Run(ctx, l.rdb, []string{key}, token).Result()
		return err
	}
	return release, nil
}
