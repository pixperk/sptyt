package tasks

import "strings"

// parseRedisAddr strips the redis:// protocol prefix if present
// Asynq expects just "host:port" format, not "redis://host:port"
func parseRedisAddr(redisURL string) string {
	// Remove redis:// prefix if present
	addr := strings.TrimPrefix(redisURL, "redis://")
	// Remove rediss:// prefix if present (TLS)
	addr = strings.TrimPrefix(addr, "rediss://")
	return addr
}
