package middleware

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/opendelivery/opendelivery/configs"
	"github.com/redis/go-redis/v9"
)

type idempotencyResponse struct {
	Status int    `json:"status"`
	Body   []byte `json:"body"`
}

// Idempotency replays the cached response for a POST/PATCH request carrying the
// same Idempotency-Key + user, so retried requests (double taps, client timeouts
// with automatic retry) don't double-charge or double-create resources.
func Idempotency(rdb *redis.Client, jwtConfig *configs.JWTConfig) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Method() != fiber.MethodPost && c.Method() != fiber.MethodPatch {
			return c.Next()
		}

		key := c.Get("Idempotency-Key")
		if key == "" {
			return c.Next()
		}

		// This middleware runs before route-specific RequireAuth, so Locals
		// aren't populated yet: parse the bearer token directly instead.
		claims, err := parseBearerToken(c, jwtConfig.AccessTokenSecret)
		if err != nil {
			return c.Next()
		}
		userID := claims.UserID

		ctx := c.Context()
		redisKey := fmt.Sprintf("idempotency:%s:%s", userID, key)

		if cached, err := rdb.Get(ctx, redisKey).Bytes(); err == nil {
			var resp idempotencyResponse
			if json.Unmarshal(cached, &resp) == nil {
				c.Set("X-Idempotency-Replayed", "true")
				return c.Status(resp.Status).Send(resp.Body)
			}
		}

		lockKey := redisKey + ":lock"
		locked, err := rdb.SetNX(ctx, lockKey, "1", 10*time.Second).Result()
		if err == nil && !locked {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"success": false,
				"error":   fiber.Map{"code": "REQUEST_IN_PROGRESS", "message": "This request is already being processed"},
			})
		}
		defer rdb.Del(ctx, lockKey)

		if err := c.Next(); err != nil {
			return err
		}

		status := c.Response().StatusCode()
		if status >= 200 && status < 300 {
			body := append([]byte(nil), c.Response().Body()...)
			if data, err := json.Marshal(idempotencyResponse{Status: status, Body: body}); err == nil {
				_ = rdb.Set(ctx, redisKey, data, 24*time.Hour).Err()
			}
		}

		return nil
	}
}
