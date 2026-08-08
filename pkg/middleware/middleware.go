package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/opendelivery/opendelivery/configs"
	"github.com/opendelivery/opendelivery/pkg/response"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

const (
	LocalUserID = "user_id"
	LocalEmail  = "user_email"
	LocalRole   = "user_role"
)

type AuthMiddleware struct {
	jwtConfig *configs.JWTConfig
	logger    *zap.Logger
}

func NewAuthMiddleware(jwtConfig *configs.JWTConfig, logger *zap.Logger) *AuthMiddleware {
	return &AuthMiddleware{jwtConfig: jwtConfig, logger: logger}
}

func (m *AuthMiddleware) RequireAuth() fiber.Handler {
	return func(c fiber.Ctx) error {
		claims, err := parseBearerToken(c, m.jwtConfig.AccessTokenSecret)
		if err != nil {
			m.logger.Warn("invalid token", zap.Error(err))
			return response.Unauthorized(c, "Invalid or expired token")
		}

		c.Locals(LocalUserID, claims.UserID)
		c.Locals(LocalEmail, claims.Email)
		c.Locals(LocalRole, claims.Role)

		return c.Next()
	}
}

// parseBearerToken is shared by RequireAuth and the Idempotency middleware: the
// latter runs as a global app.Use() *before* route-specific RequireAuth has a
// chance to populate Locals, so it needs its own way to identify the caller.
func parseBearerToken(c fiber.Ctx, secret string) (*Claims, error) {
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return nil, fmt.Errorf("missing authorization header")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return nil, fmt.Errorf("invalid authorization header format")
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

func (m *AuthMiddleware) RequireRole(roles ...string) fiber.Handler {
	allowedRoles := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowedRoles[r] = struct{}{}
	}

	return func(c fiber.Ctx) error {
		role, _ := c.Locals(LocalRole).(string)
		if role == "" {
			return response.Unauthorized(c, "Role not found in context")
		}

		if _, ok := allowedRoles[role]; !ok {
			return response.Forbidden(c, fmt.Sprintf("Role '%s' is not allowed", role))
		}

		return c.Next()
	}
}

// UserID extracts the authenticated user id from the request context.
func UserID(c fiber.Ctx) string {
	v, _ := c.Locals(LocalUserID).(string)
	return v
}

// Role extracts the authenticated user role from the request context.
func Role(c fiber.Ctx) string {
	v, _ := c.Locals(LocalRole).(string)
	return v
}

func CORS() fiber.Handler {
	return func(c fiber.Ctx) error {
		c.Set("Access-Control-Allow-Origin", "*")
		c.Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Set("Access-Control-Allow-Headers", "Accept,Authorization,Content-Type,X-CSRF-Token,X-Request-ID,Idempotency-Key")
		c.Set("Access-Control-Allow-Credentials", "true")
		c.Set("Access-Control-Max-Age", "86400")
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "1; mode=block")
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Content-Security-Policy", "default-src 'self'")

		if c.Method() == http.MethodOptions {
			return c.SendStatus(http.StatusNoContent)
		}

		return c.Next()
	}
}

func Logging(logger *zap.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
		}
		c.Set("X-Request-ID", requestID)

		err := c.Next()

		duration := time.Since(start)
		status := c.Response().StatusCode()

		logFunc := logger.Info
		if status >= 500 {
			logFunc = logger.Error
		} else if status >= 400 {
			logFunc = logger.Warn
		}

		logFunc("request completed",
			zap.String("request_id", requestID),
			zap.String("method", c.Method()),
			zap.String("path", c.Path()),
			zap.Int("status", status),
			zap.Duration("duration", duration),
			zap.String("ip", c.IP()),
		)

		return err
	}
}

func Recovery(logger *zap.Logger) fiber.Handler {
	return func(c fiber.Ctx) (err error) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					zap.Any("panic", r),
					zap.String("path", c.Path()),
					zap.String("method", c.Method()),
				)
				err = response.InternalError(c)
			}
		}()
		return c.Next()
	}
}

func RateLimiting(rdb *redis.Client, logger *zap.Logger) fiber.Handler {
	const (
		limit  = 100
		window = time.Minute
	)

	return func(c fiber.Ctx) error {
		ctx := context.Background()
		key := fmt.Sprintf("ratelimit:%s:%s", c.IP(), c.Path())

		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			logger.Warn("redis rate limit error", zap.Error(err))
			return c.Next()
		}

		if count == 1 {
			_ = rdb.Expire(ctx, key, window)
		}

		remaining := limit - int(count)
		if remaining < 0 {
			remaining = 0
		}

		c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
		c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		if count > int64(limit) {
			return response.TooManyRequests(c)
		}

		return c.Next()
	}
}

func RequestID() fiber.Handler {
	return func(c fiber.Ctx) error {
		requestID := c.Get("X-Request-ID")
		if requestID == "" {
			requestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
		}
		c.Set("X-Request-ID", requestID)
		c.Locals("request_id", requestID)
		return c.Next()
	}
}
