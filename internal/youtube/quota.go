package youtube

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// QuotaTracker tracks YouTube API quota usage
type QuotaTracker struct {
	redisClient *redis.Client
	mu          sync.RWMutex
	localQuota  int // Fallback for when Redis is unavailable
	dailyLimit  int
}

const (
	// YouTube API quota costs (units per operation)
	QuotaCostSearch         = 100
	QuotaCostPlaylistInsert = 50
	QuotaCostVideoInsert    = 50
	QuotaCostPlaylistList   = 1
	QuotaCostVideoList      = 1

	// Default daily quota limit
	DefaultDailyQuota = 10000

	// Redis keys
	redisQuotaKey      = "youtube:quota:daily"
	redisQuotaResetKey = "youtube:quota:reset_time"
)

// NewQuotaTracker creates a new YouTube quota tracker
func NewQuotaTracker(redisClient *redis.Client, dailyLimit int) *QuotaTracker {
	if dailyLimit <= 0 {
		dailyLimit = DefaultDailyQuota
	}

	qt := &QuotaTracker{
		redisClient: redisClient,
		localQuota:  0,
		dailyLimit:  dailyLimit,
	}

	// Initialize or restore quota tracking
	if redisClient != nil {
		qt.initializeQuota()
	}

	return qt
}

// initializeQuota sets up quota tracking in Redis
func (qt *QuotaTracker) initializeQuota() {
	ctx := context.Background()

	// Check if we need to reset (new day)
	resetTimeStr, err := qt.redisClient.Get(ctx, redisQuotaResetKey).Result()
	switch err {
	case redis.Nil:
		// First time setup
		qt.resetQuota(ctx)
	case nil:
		// Check if reset time has passed
		resetTime, err := time.Parse(time.RFC3339, resetTimeStr)
		if err == nil && time.Now().After(resetTime) {
			qt.resetQuota(ctx)
		}
	}
}

// resetQuota resets the daily quota counter
func (qt *QuotaTracker) resetQuota(ctx context.Context) {
	// Set quota to 0
	qt.redisClient.Set(ctx, redisQuotaKey, 0, 24*time.Hour)

	// Set next reset time (midnight UTC)
	now := time.Now().UTC()
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	qt.redisClient.Set(ctx, redisQuotaResetKey, tomorrow.Format(time.RFC3339), 24*time.Hour)

	log.Printf("YouTube quota reset. Next reset: %s", tomorrow.Format(time.RFC3339))
}

// ConsumeQuota records quota usage and returns error if limit exceeded
func (qt *QuotaTracker) ConsumeQuota(ctx context.Context, cost int) error {
	if qt.redisClient != nil {
		// Use Redis for distributed quota tracking
		return qt.consumeQuotaRedis(ctx, cost)
	}

	// Fallback to local tracking
	return qt.consumeQuotaLocal(cost)
}

// consumeQuotaRedis uses Redis for quota tracking (multi-server support)
func (qt *QuotaTracker) consumeQuotaRedis(ctx context.Context, cost int) error {
	// Increment quota usage
	newQuota, err := qt.redisClient.IncrBy(ctx, redisQuotaKey, int64(cost)).Result()
	if err != nil {
		log.Printf("YouTube quota tracking error (falling back to local): %v", err)
		return qt.consumeQuotaLocal(cost)
	}

	// Check if exceeded
	if newQuota > int64(qt.dailyLimit) {
		remaining := qt.dailyLimit - int(newQuota-int64(cost))
		return fmt.Errorf("YouTube API quota exceeded: used %d/%d units (needed %d more)", newQuota-int64(cost), qt.dailyLimit, cost-remaining)
	}

	log.Printf("YouTube quota consumed: %d units (total: %d/%d)", cost, newQuota, qt.dailyLimit)
	return nil
}

// consumeQuotaLocal uses in-memory quota tracking (single server fallback)
func (qt *QuotaTracker) consumeQuotaLocal(cost int) error {
	qt.mu.Lock()
	defer qt.mu.Unlock()

	if qt.localQuota+cost > qt.dailyLimit {
		return fmt.Errorf("YouTube API quota exceeded (local): used %d/%d units (needed %d more)", qt.localQuota, qt.dailyLimit, cost-(qt.dailyLimit-qt.localQuota))
	}

	qt.localQuota += cost
	log.Printf("YouTube quota consumed (local): %d units (total: %d/%d)", cost, qt.localQuota, qt.dailyLimit)
	return nil
}

// GetRemainingQuota returns remaining quota units
func (qt *QuotaTracker) GetRemainingQuota(ctx context.Context) (int, error) {
	if qt.redisClient != nil {
		used, err := qt.redisClient.Get(ctx, redisQuotaKey).Int()
		if err != nil && err != redis.Nil {
			return 0, err
		}
		return qt.dailyLimit - used, nil
	}

	// Local fallback
	qt.mu.RLock()
	defer qt.mu.RUnlock()
	return qt.dailyLimit - qt.localQuota, nil
}

// GetQuotaInfo returns detailed quota information
func (qt *QuotaTracker) GetQuotaInfo(ctx context.Context) map[string]interface{} {
	remaining, _ := qt.GetRemainingQuota(ctx)
	used := qt.dailyLimit - remaining

	var resetTime string
	if qt.redisClient != nil {
		resetTime, _ = qt.redisClient.Get(ctx, redisQuotaResetKey).Result()
	}

	return map[string]interface{}{
		"daily_limit":     qt.dailyLimit,
		"used":            used,
		"remaining":       remaining,
		"percentage_used": float64(used) / float64(qt.dailyLimit) * 100,
		"reset_time":      resetTime,
		"costs": map[string]int{
			"search":          QuotaCostSearch,
			"playlist_insert": QuotaCostPlaylistInsert,
			"video_insert":    QuotaCostVideoInsert,
			"playlist_list":   QuotaCostPlaylistList,
			"video_list":      QuotaCostVideoList,
		},
	}
}

// CanAfford checks if we have enough quota for an operation without consuming it
func (qt *QuotaTracker) CanAfford(ctx context.Context, cost int) (bool, error) {
	remaining, err := qt.GetRemainingQuota(ctx)
	if err != nil {
		return false, err
	}
	return remaining >= cost, nil
}
