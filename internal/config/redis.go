package config

import (
	"crypto/tls"
	"os"
	"strconv"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// RedisConfig holds unified Redis configuration
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	TLS      bool
}

// LoadRedisConfig loads Redis configuration from environment variables
// Environment variables:
//   - REDIS_ADDR: Redis server address (default: localhost:6379)
//   - REDIS_PASSWORD: Redis password (optional)
//   - REDIS_DB: Redis database number (default: 0)
//   - REDIS_TLS: Enable TLS (default: false, set to "true" or "1" to enable)
func LoadRedisConfig() *RedisConfig {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	password := os.Getenv("REDIS_PASSWORD")

	db := 0
	if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
		if parsed, err := strconv.Atoi(dbStr); err == nil {
			db = parsed
		}
	}

	tlsEnabled := false
	if tlsStr := os.Getenv("REDIS_TLS"); tlsStr != "" {
		tlsEnabled = tlsStr == "true" || tlsStr == "1" || tlsStr == "yes"
	}

	return &RedisConfig{
		Addr:     addr,
		Password: password,
		DB:       db,
		TLS:      tlsEnabled,
	}
}

// NewRedisClient creates a new go-redis client from config
func (c *RedisConfig) NewRedisClient() *redis.Options {
	opts := &redis.Options{
		Addr:         c.Addr,
		Password:     c.Password,
		DB:           c.DB,
		PoolSize:     20,
		MinIdleConns: 2,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}

	if c.TLS {
		opts.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	return opts
}

// NewAsynqRedisOpt creates Asynq Redis options from config
func (c *RedisConfig) NewAsynqRedisOpt() asynq.RedisClientOpt {
	opt := asynq.RedisClientOpt{
		Addr:     c.Addr,
		Password: c.Password,
		DB:       c.DB,
	}

	if c.TLS {
		opt.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	// Upstash and some cloud Redis providers require username
	if c.Password != "" {
		opt.Username = "default"
	}

	return opt
}

// IsConfigured returns true if Redis is configured (non-default address or has password)
func (c *RedisConfig) IsConfigured() bool {
	return c.Addr != "localhost:6379" || c.Password != ""
}

// String returns a safe string representation (no password)
func (c *RedisConfig) String() string {
	tlsStr := "disabled"
	if c.TLS {
		tlsStr = "enabled"
	}
	return "redis://" + c.Addr + "/" + strconv.Itoa(c.DB) + " (TLS: " + tlsStr + ")"
}
