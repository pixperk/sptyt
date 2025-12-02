package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

// rateLimitEntry tracks request counts and expiration for in-memory fallback
type rateLimitEntry struct {
	count     int
	expiresAt time.Time
}

type RateLimiter struct {
	client        *redis.Client
	limit         int
	fallbackLimit int // Stricter limit when Redis is unavailable (default: 50/min)
	window        time.Duration
	keyPrefix     string

	// In-memory fallback (used when Redis is down)
	fallbackCache map[string]*rateLimitEntry
	fallbackMu    sync.RWMutex
	stopCleanup   chan struct{}
	inFallback    bool // Track if we're in fallback mode
	fallbackMu2   sync.RWMutex
}

func NewRateLimiter(client *redis.Client, requestsPerMinute int) *RateLimiter {
	rl := &RateLimiter{
		client:        client,
		limit:         requestsPerMinute,
		fallbackLimit: 50, // Stricter limit in fallback mode
		window:        time.Minute,
		keyPrefix:     "ratelimit:",
		fallbackCache: make(map[string]*rateLimitEntry),
		stopCleanup:   make(chan struct{}),
		inFallback:    false,
	}

	// Start cleanup goroutine for in-memory cache
	go rl.cleanupExpiredEntries()

	return rl
}

// cleanupExpiredEntries removes expired entries from in-memory cache
func (rl *RateLimiter) cleanupExpiredEntries() {
	ticker := time.NewTicker(30 * time.Second) // Cleanup every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.fallbackMu.Lock()
			now := time.Now()
			for key, entry := range rl.fallbackCache {
				if now.After(entry.expiresAt) {
					delete(rl.fallbackCache, key)
				}
			}
			rl.fallbackMu.Unlock()

		case <-rl.stopCleanup:
			return
		}
	}
}

// Close stops the cleanup goroutine
func (rl *RateLimiter) Close() {
	close(rl.stopCleanup)
}

// checkFallbackRateLimit uses in-memory rate limiting when Redis is unavailable
func (rl *RateLimiter) checkFallbackRateLimit(ip string) (int, int, bool) {
	rl.fallbackMu.Lock()
	defer rl.fallbackMu.Unlock()

	now := time.Now()
	key := rl.keyPrefix + ip

	entry, exists := rl.fallbackCache[key]
	if !exists || now.After(entry.expiresAt) {
		// Create new entry
		entry = &rateLimitEntry{
			count:     1,
			expiresAt: now.Add(rl.window),
		}
		rl.fallbackCache[key] = entry
		return entry.count, rl.fallbackLimit - entry.count, false
	}

	// Increment existing entry
	entry.count++
	remaining := rl.fallbackLimit - entry.count
	if remaining < 0 {
		remaining = 0
	}
	exceeded := entry.count > rl.fallbackLimit

	return entry.count, remaining, exceeded
}

func (rl *RateLimiter) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip rate limiting for OPTIONS requests (CORS preflight)
			if c.Request().Method == "OPTIONS" {
				return next(c)
			}

			// Skip rate limiting for certain endpoints
			path := c.Path()

			// Health check endpoints
			if path == "/health" || path == "/ping" {
				return next(c)
			}

			// WebSocket endpoint (has its own connection management)
			if path == "/api/ws/playlist-progress" {
				return next(c)
			}

			// Webhook endpoints (verified by signature, not IP)
			if path == "/webhooks/dodopay" {
				return next(c)
			}

			ip := c.RealIP()
			key := rl.keyPrefix + ip

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			current, err := rl.client.Incr(ctx, key).Result()
			if err != nil {
				// Redis is down - fall back to in-memory rate limiting
				rl.fallbackMu2.Lock()
				rl.inFallback = true
				rl.fallbackMu2.Unlock()

				// Use in-memory rate limiting with stricter limits
				_, remaining, exceeded := rl.checkFallbackRateLimit(ip)

				// Set headers with fallback limit
				c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.fallbackLimit))
				c.Response().Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
				c.Response().Header().Set("X-RateLimit-Mode", "fallback") // Indicate fallback mode

				if exceeded {
					c.Response().Header().Set("Retry-After", strconv.Itoa(int(rl.window.Seconds())))
					return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
						"error": fmt.Sprintf("rate limit exceeded: %d requests per minute allowed (fallback mode)", rl.fallbackLimit),
						"mode":  "fallback",
					})
				}

				return next(c)
			}

			// Redis is working - reset fallback flag if needed
			rl.fallbackMu2.Lock()
			rl.inFallback = false
			rl.fallbackMu2.Unlock()

			if current == 1 {
				rl.client.Expire(ctx, key, rl.window)
			}

			remaining := rl.limit - int(current)
			if remaining < 0 {
				remaining = 0
			}

			c.Response().Header().Set("X-RateLimit-Limit", strconv.Itoa(rl.limit))
			c.Response().Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			c.Response().Header().Set("X-RateLimit-Mode", "redis") // Indicate Redis mode

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
