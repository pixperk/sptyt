package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// UserAnalytics tracks user conversion statistics
type UserAnalytics struct {
	bun.BaseModel `bun:"table:user_analytics,alias:ua"`

	ID     uuid.UUID `bun:"type:uuid,pk,default:gen_random_uuid()" json:"id"`
	UserID uuid.UUID `bun:"type:uuid,notnull,unique" json:"user_id"`

	// Conversion stats
	TotalConversions       int `bun:",default:0" json:"total_conversions"`
	SuccessfulConversions  int `bun:",default:0" json:"successful_conversions"`
	FailedConversions      int `bun:",default:0" json:"failed_conversions"`

	// Content type breakdown
	PlaylistsConverted int `bun:",default:0" json:"playlists_converted"`
	AlbumsConverted    int `bun:",default:0" json:"albums_converted"`

	// Track stats
	TotalTracksProcessed int `bun:",default:0" json:"total_tracks_processed"`
	TotalTracksMatched   int `bun:",default:0" json:"total_tracks_matched"`
	TotalTracksFailed    int `bun:",default:0" json:"total_tracks_failed"`

	// Custom links (for future feature)
	TotalCustomLinks int `bun:",default:0" json:"total_custom_links"`

	// Monthly usage tracking (for subscription limits)
	MonthlyConversions int `bun:",default:0" json:"monthly_conversions"`
	CurrentMonth       int `bun:",default:0" json:"current_month"` // 1-12
	CurrentYear        int `bun:",default:0" json:"current_year"`  // e.g., 2025

	// YouTube quota tracking (uses user's OAuth token quota)
	// YouTube API costs: Search = 100 units, Playlist operations = 50 units
	// Daily limit is typically 10,000 units per user
	DailyYouTubeSearches int        `bun:"daily_youtube_searches,default:0" json:"daily_youtube_searches"`
	DailyPlaylistInserts int        `bun:"daily_playlist_inserts,default:0" json:"daily_playlist_inserts"`
	LastQuotaResetDate   *time.Time `bun:"last_quota_reset_date,nullzero" json:"last_quota_reset_date,omitempty"`

	// Time tracking
	FirstConversionAt *time.Time `bun:",nullzero" json:"first_conversion_at,omitempty"`
	LastConversionAt  *time.Time `bun:",nullzero" json:"last_conversion_at,omitempty"`

	// Timestamps
	CreatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`

	// Relations
	User *User `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty"`
}

// GetSuccessRate calculates the success rate percentage
func (ua *UserAnalytics) GetSuccessRate() float64 {
	if ua.TotalConversions == 0 {
		return 0
	}
	return (float64(ua.SuccessfulConversions) / float64(ua.TotalConversions)) * 100
}

// GetTrackMatchRate calculates the track matching success rate
func (ua *UserAnalytics) GetTrackMatchRate() float64 {
	if ua.TotalTracksProcessed == 0 {
		return 0
	}
	return (float64(ua.TotalTracksMatched) / float64(ua.TotalTracksProcessed)) * 100
}

// YouTube API quota costs
const (
	YouTubeQuotaCostSearch         = 100
	YouTubeQuotaCostPlaylistInsert = 50
	YouTubeQuotaDailyLimit         = 10000
)

// GetDailyQuotaUsed calculates estimated YouTube quota used today
func (ua *UserAnalytics) GetDailyQuotaUsed() int {
	searchQuota := ua.DailyYouTubeSearches * YouTubeQuotaCostSearch
	insertQuota := ua.DailyPlaylistInserts * YouTubeQuotaCostPlaylistInsert
	return searchQuota + insertQuota
}

// GetDailyQuotaRemaining calculates remaining YouTube quota for today
func (ua *UserAnalytics) GetDailyQuotaRemaining() int {
	remaining := YouTubeQuotaDailyLimit - ua.GetDailyQuotaUsed()
	if remaining < 0 {
		return 0
	}
	return remaining
}

// GetDailyQuotaPercentage calculates percentage of daily quota used
func (ua *UserAnalytics) GetDailyQuotaPercentage() float64 {
	return (float64(ua.GetDailyQuotaUsed()) / float64(YouTubeQuotaDailyLimit)) * 100
}

// NeedsQuotaReset checks if quota counters need to be reset (new day)
func (ua *UserAnalytics) NeedsQuotaReset() bool {
	if ua.LastQuotaResetDate == nil {
		return true
	}
	// YouTube quota resets at midnight Pacific Time
	// For simplicity, we use UTC date comparison
	now := time.Now().UTC()
	lastReset := ua.LastQuotaResetDate.UTC()
	return now.Year() != lastReset.Year() ||
		now.YearDay() != lastReset.YearDay()
}
