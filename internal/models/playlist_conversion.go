package models

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type PlaylistConversion struct {
	bun.BaseModel `bun:"table:playlist_conversions,alias:pc"`

	ID                 uuid.UUID `bun:"type:uuid,pk,default:gen_random_uuid()" json:"id"`
	UserID             uuid.UUID `bun:"type:uuid,notnull" json:"user_id"`
	SpotifyPlaylistID  string    `bun:",notnull" json:"spotify_playlist_id"`
	SpotifyPlaylistURL string    `bun:",notnull" json:"spotify_playlist_url"`
	YouTubePlaylistID  string    `bun:"column:you_tube_playlist_id" json:"youtube_playlist_id,omitempty"`
	YouTubePlaylistURL string    `bun:"column:you_tube_playlist_url" json:"youtube_playlist_url,omitempty"`

	PlaylistName      string `bun:",notnull" json:"playlist_name"`
	SpotifyCoverImage string `bun:"" json:"spotify_cover_image,omitempty"`
	TrackCount        int    `bun:",notnull,default:0" json:"track_count"`
	SuccessCount      int    `bun:",notnull,default:0" json:"success_count"`
	FailureCount      int    `bun:",notnull,default:0" json:"failure_count"`

	Status             string               `bun:",notnull,default:'pending'" json:"status"` // pending, processing, completed, failed
	ConversionLog      []TrackConversionLog `bun:"type:jsonb" json:"conversion_log,omitempty"`
	ErrorMessage       string               `bun:"" json:"error_message,omitempty"`
	CountsAgainstQuota bool                 `bun:",default:true" json:"counts_against_quota"`     // false if failed due to API errors
	GoogleAccountEmail string               `bun:"" json:"google_account_email,omitempty"` // Google account used to create YouTube playlist

	CreatedAt   time.Time  `bun:",nullzero,notnull,default:current_timestamp" json:"created_at"`
	UpdatedAt   time.Time  `bun:",nullzero,notnull,default:current_timestamp" json:"updated_at"`
	CompletedAt *time.Time `bun:",nullzero" json:"completed_at,omitempty"`
	DeletedAt   *time.Time `bun:",nullzero,soft_delete" json:"deleted_at,omitempty"` // Soft delete - keeps record for analytics

	// Relations
	User *User `bun:"rel:belongs-to,join:user_id=id" json:"user,omitempty"`
}

// IsDeleted checks if the conversion has been soft deleted
func (pc *PlaylistConversion) IsDeleted() bool {
	return pc.DeletedAt != nil
}

// TrackConversionLog represents the conversion result for a single track
type TrackConversionLog struct {
	SpotifyTrackID   string `json:"spotify_track_id"`
	SpotifyTrackName string `json:"spotify_track_name"`
	SpotifyArtists   string `json:"spotify_artists"`
	YouTubeVideoID   string `json:"youtube_video_id,omitempty"`
	YouTubeVideoURL  string `json:"youtube_video_url,omitempty"`
	Status           string `json:"status"` // success, not_found, error
	Error            string `json:"error,omitempty"`
	MatchMethod      string `json:"match_method,omitempty"` // isrc, official_mv, lyric_video, title_parse
}

// GetProgress returns the conversion progress as a percentage
func (pc *PlaylistConversion) GetProgress() float64 {
	if pc.TrackCount == 0 {
		return 0
	}
	processed := pc.SuccessCount + pc.FailureCount
	return float64(processed) / float64(pc.TrackCount) * 100
}

// IsComplete checks if the conversion is finished
func (pc *PlaylistConversion) IsComplete() bool {
	return pc.Status == "completed" || pc.Status == "failed"
}
