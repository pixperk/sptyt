package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/hibiken/asynq"
	"github.com/pixperk/sptyt/internal/services"
)

// Task type constants
const (
	TypePlaylistConversion = "playlist:convert"
)

// PlaylistConversionPayload represents the task payload for playlist conversion
type PlaylistConversionPayload struct {
	ConversionID        string `json:"conversion_id"`
	UserID              string `json:"user_id"`
	SpotifyPlaylistID   string `json:"spotify_playlist_id"`
	SpotifyType         string `json:"spotify_type"` // "playlist" or "album"
	SpotifyPlaylistURL  string `json:"spotify_playlist_url"`
	YouTubeAccessToken  string `json:"youtube_access_token"`
	YouTubePlaylistName string `json:"youtube_playlist_name"`
	UseLyricVideos      bool   `json:"use_lyric_videos"`
}

// NewPlaylistConversionTask creates a new Asynq task for playlist conversion
func NewPlaylistConversionTask(payload PlaylistConversionPayload) (*asynq.Task, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	return asynq.NewTask(TypePlaylistConversion, payloadBytes), nil
}

// PlaylistConversionProcessor handles playlist conversion tasks
type PlaylistConversionProcessor struct {
	converterService *services.PlaylistConverterService
}

func NewPlaylistConversionProcessor(converterService *services.PlaylistConverterService) *PlaylistConversionProcessor {
	return &PlaylistConversionProcessor{
		converterService: converterService,
	}
}

// ProcessPlaylistConversion processes a playlist conversion task
func (p *PlaylistConversionProcessor) ProcessPlaylistConversion(ctx context.Context, t *asynq.Task) error {
	var payload PlaylistConversionPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	log.Printf("Processing playlist conversion task: %s for user: %s", payload.ConversionID, payload.UserID)

	// Create conversion job
	job := &services.ConversionJob{
		ConversionID:        payload.ConversionID,
		UserID:              payload.UserID,
		SpotifyPlaylistID:   payload.SpotifyPlaylistID,
		SpotifyType:         payload.SpotifyType,
		SpotifyPlaylistURL:  payload.SpotifyPlaylistURL,
		YouTubeAccessToken:  payload.YouTubeAccessToken,
		YouTubePlaylistName: payload.YouTubePlaylistName,
		UseLyricVideos:      payload.UseLyricVideos,
	}

	// Process the conversion
	_, err := p.converterService.ConvertPlaylist(ctx, job)
	if err != nil {
		log.Printf("Playlist conversion failed: %v", err)
		return fmt.Errorf("conversion failed: %w", err)
	}

	log.Printf("Playlist conversion completed successfully: %s", payload.ConversionID)
	return nil
}
