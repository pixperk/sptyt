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
