package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// YouTube API quota costs
const (
	YouTubeQuotaCostSearch         = 100
	YouTubeQuotaCostPlaylistInsert = 50
	YouTubeQuotaDailyLimit         = 10000
)

// YouTubeAccountQuota tracks YouTube API quota per Google account
// Quota is tied to the Google account, not the app user
type YouTubeAccountQuota struct {
	bun.BaseModel `bun:"table:youtube_account_quotas,alias:yaq"`

	ID           uuid.UUID `bun:"type:uuid,pk,default:gen_random_uuid()" json:"id"`
	AccountEmail string    `bun:",unique,notnull" json:"account_email"` // Google account email (unique identifier)

	// Daily quota tracking
	// YouTube API costs: Search = 100 units, Playlist operations = 50 units
	// Daily limit is typically 10,000 units per account
	DailySearches        int        `bun:",default:0" json:"daily_searches"`
	DailyPlaylistInserts int        `bun:",default:0" json:"daily_playlist_inserts"`
	LastQuotaResetDate   *time.Time `bun:",nullzero" json:"last_quota_reset_date,omitempty"`

	// Timestamps
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
}

// GetDailyQuotaUsed calculates estimated YouTube quota used today
func (q *YouTubeAccountQuota) GetDailyQuotaUsed() int {
	searchQuota := q.DailySearches * YouTubeQuotaCostSearch
	insertQuota := q.DailyPlaylistInserts * YouTubeQuotaCostPlaylistInsert
	return searchQuota + insertQuota
}

// GetDailyQuotaRemaining calculates remaining YouTube quota for today
func (q *YouTubeAccountQuota) GetDailyQuotaRemaining() int {
	remaining := YouTubeQuotaDailyLimit - q.GetDailyQuotaUsed()
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetDailyQuotaPercentage calculates percentage of daily quota used
func (q *YouTubeAccountQuota) GetDailyQuotaPercentage() float64 {
	return (float64(q.GetDailyQuotaUsed()) / float64(YouTubeQuotaDailyLimit)) * 100
}

// NeedsQuotaReset checks if quota counters need to be reset (new day)
func (q *YouTubeAccountQuota) NeedsQuotaReset() bool {
	if q.LastQuotaResetDate == nil {
		return true
	}
	// YouTube quota resets at midnight Pacific Time
	// For simplicity, we use UTC date comparison
	now := time.Now().UTC()
	lastReset := q.LastQuotaResetDate.UTC()
	return now.Year() != lastReset.Year() ||
		now.YearDay() != lastReset.YearDay()
}

// CanAffordSearch checks if account has enough quota for a search operation
func (q *YouTubeAccountQuota) CanAffordSearch() bool {
	return q.GetDailyQuotaRemaining() >= YouTubeQuotaCostSearch
}

// CanAffordPlaylistInsert checks if account has enough quota for a playlist insert
func (q *YouTubeAccountQuota) CanAffordPlaylistInsert() bool {
	return q.GetDailyQuotaRemaining() >= YouTubeQuotaCostPlaylistInsert
}
