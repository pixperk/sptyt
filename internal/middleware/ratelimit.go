package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	client       *redis.Client
	limit        int
	window       time.Duration
	keyPrefix    string
}

func NewRateLimiter(client *redis.Client, requestsPerMinute int) *RateLimiter {
	return &RateLimiter{
		client:    client,
		limit:     requestsPerMinute,
		window:    time.Minute,
		keyPrefix: "ratelimit:",
	}
}

func (rl *RateLimiter) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip rate limiting for OPTIONS requests (CORS preflight)
			if c.Request().Method == "OPTIONS" {
				return next(c)
			}

			// Skip rate limiting for health check endpoints
			path := c.Path()
			if path == "/health" || path == "/ping" {
				return next(c)
			}

			ip := c.RealIP()
			key := rl.keyPrefix + ip

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			current, err := rl.client.Incr(ctx, key).Result()
			if err != nil {
				// CRITICAL: Redis is down - rate limiting is bypassed!
				// Log the error and alert, but allow request to proceed
				log.Printf("CRITICAL: Rate limiter Redis error (bypassing rate limit): %v", err)
				// In production, you should also:
				// - Send alert to monitoring system
				// - Increment a metric counter
				// - Consider returning error instead if you want fail-closed behavior
				return next(c)
			}

			if current == 1 {
				rl.client.Expire(ctx, key, rl.window)
			}

			remaining := rl.limit - int(current)
			if remaining < 0 {
				remaining = 0
			}

			c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.limit))
			c.Response().Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

			if current > int64(rl.limit) {
				c.Response().Header().Set("Retry-After", strconv.Itoa(int(rl.window.Seconds())))
				return c.JSON(http.StatusTooManyRequests, map[string]string{
					"error": fmt.Sprintf("rate limit exceeded: %d requests per minute allowed", rl.limit),
				})
			}

			return next(c)
		}
	}
}
