package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/pixperk/sptyt/internal/services"
)

// Task type constants
const (
	TypePlaylistConversion  = "playlist:convert"
	TypeAnalyticsUpdate     = "analytics:update"
	TypeRetryFailedTracks   = "playlist:retry"
)

// PlaylistConversionPayload represents the task payload for playlist conversion
type PlaylistConversionPayload struct {
	ConversionID        string `json:"conversion_id"`
	UserID              string `json:"user_id"`       // Database UUID
	ClerkUserID         string `json:"clerk_user_id"` // Clerk user ID for WebSocket
	SpotifyPlaylistID   string `json:"spotify_playlist_id"`
	SpotifyType         string `json:"spotify_type"` // "playlist" or "album"
	SpotifyPlaylistURL  string `json:"spotify_playlist_url"`
	YouTubeAccessToken  string `json:"youtube_access_token"`
	YouTubePlaylistName string `json:"youtube_playlist_name"`
	UseLyricVideos      bool   `json:"use_lyric_videos"`
	GoogleAccountEmail  string `json:"google_account_email"` // Google account email for quota tracking
}

// NewPlaylistConversionTask creates a new Asynq task for playlist conversion
func NewPlaylistConversionTask(payload PlaylistConversionPayload) (*asynq.Task, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	return asynq.NewTask(TypePlaylistConversion, payloadBytes,
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Minute),
	), nil
}

// AnalyticsUpdatePayload represents the task payload for analytics updates
type AnalyticsUpdatePayload struct {
	UserID             string `json:"user_id"`              // Database UUID
	SpotifyType        string `json:"spotify_type"`         // "playlist" or "album"
	IsSuccess          bool   `json:"is_success"`           // Whether conversion succeeded
	TrackCount         int    `json:"track_count"`          // Total tracks
	SuccessCount       int    `json:"success_count"`        // Successfully matched tracks
	FailureCount       int    `json:"failure_count"`        // Failed tracks
	CountsAgainstQuota bool   `json:"counts_against_quota"` // Whether conversion counts against quota
	YouTubeSearches    int    `json:"youtube_searches"`     // Number of YouTube search API calls made
	PlaylistInserts    int    `json:"playlist_inserts"`     // Number of playlist insert API calls made
	GoogleAccountEmail string `json:"google_account_email"` // Google account email for quota tracking
}

// NewAnalyticsUpdateTask creates a new Asynq task for analytics updates
func NewAnalyticsUpdateTask(payload AnalyticsUpdatePayload) (*asynq.Task, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal analytics payload: %w", err)
	}
	return asynq.NewTask(TypeAnalyticsUpdate, payloadBytes,
		asynq.MaxRetry(5),
		asynq.Timeout(1*time.Minute),
	), nil
}

// RetryFailedTracksPayload represents the task payload for retrying failed tracks
type RetryFailedTracksPayload struct {
	ConversionID       string                     `json:"conversion_id"`
	UserID             string                     `json:"user_id"`
	ClerkUserID        string                     `json:"clerk_user_id"`
	YouTubePlaylistID  string                     `json:"youtube_playlist_id"`
	YouTubeAccessToken string                     `json:"youtube_access_token"`
	GoogleAccountEmail string                     `json:"google_account_email"`
	FailedTracks       []models.TrackConversionLog `json:"failed_tracks"`
	UseLyricVideos     bool                       `json:"use_lyric_videos"`
}

// NewRetryFailedTracksTask creates a new Asynq task for retrying failed tracks
func NewRetryFailedTracksTask(payload RetryFailedTracksPayload) (*asynq.Task, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal retry payload: %w", err)
	}
	return asynq.NewTask(TypeRetryFailedTracks, payloadBytes,
		asynq.MaxRetry(3),
		asynq.Timeout(30*time.Minute),
	), nil
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

func (p *PlaylistConversionProcessor) ProcessPlaylistConversion(ctx context.Context, t *asynq.Task) error {
	var payload PlaylistConversionPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	log.Printf("Processing playlist conversion task: %s for user: %s", payload.ConversionID, payload.UserID)

	// Parse user ID for token manager
	userUUID, err := uuid.Parse(payload.UserID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	// Create token manager for auto-refreshing YouTube tokens during long conversions
	tokenManager := p.converterService.CreateTokenManager(userUUID)

	job := &services.ConversionJob{
		ConversionID:        payload.ConversionID,
		UserID:              payload.UserID,
		ClerkUserID:         payload.ClerkUserID,
		SpotifyPlaylistID:   payload.SpotifyPlaylistID,
		SpotifyType:         payload.SpotifyType,
		SpotifyPlaylistURL:  payload.SpotifyPlaylistURL,
		YouTubeAccessToken:  payload.YouTubeAccessToken,
		TokenManager:        tokenManager,
		YouTubePlaylistName: payload.YouTubePlaylistName,
		UseLyricVideos:      payload.UseLyricVideos,
		GoogleAccountEmail:  payload.GoogleAccountEmail,
	}

	// Process the conversion
	_, err = p.converterService.ConvertPlaylist(ctx, job)
	if err != nil {
		log.Printf("Playlist conversion failed: %v", err)
		return fmt.Errorf("conversion failed: %w", err)
	}

	log.Printf("Playlist conversion completed successfully: %s", payload.ConversionID)
	return nil
}

// ProcessAnalyticsUpdate processes an analytics update task
func (p *PlaylistConversionProcessor) ProcessAnalyticsUpdate(ctx context.Context, t *asynq.Task) error {
	var payload AnalyticsUpdatePayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal analytics payload: %w", err)
	}

	log.Printf("Processing analytics update task for user: %s, google_account: %s (counts_against_quota: %v, searches: %d, inserts: %d)",
		payload.UserID, payload.GoogleAccountEmail, payload.CountsAgainstQuota, payload.YouTubeSearches, payload.PlaylistInserts)

	// Call the analytics update service method
	err := p.converterService.UpdateUserAnalytics(ctx, payload.UserID, payload.SpotifyType, payload.IsSuccess,
		payload.TrackCount, payload.SuccessCount, payload.FailureCount, payload.CountsAgainstQuota,
		payload.YouTubeSearches, payload.PlaylistInserts, payload.GoogleAccountEmail)
	if err != nil {
		log.Printf("Analytics update failed: %v", err)
		return fmt.Errorf("analytics update failed: %w", err)
	}

	log.Printf("Analytics update completed successfully for user: %s", payload.UserID)
	return nil
}

// ProcessRetryFailedTracks processes a retry failed tracks task
func (p *PlaylistConversionProcessor) ProcessRetryFailedTracks(ctx context.Context, t *asynq.Task) error {
	var payload RetryFailedTracksPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal retry payload: %w", err)
	}

	log.Printf("Processing retry failed tracks task: conversion=%s, tracks=%d", payload.ConversionID, len(payload.FailedTracks))

	// Parse user ID for token manager
	userUUID, err := uuid.Parse(payload.UserID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	// Create token manager for auto-refreshing YouTube tokens during retries
	tokenManager := p.converterService.CreateTokenManager(userUUID)

	// Convert payload to service RetryJob
	job := &services.RetryJob{
		ConversionID:       payload.ConversionID,
		UserID:             payload.UserID,
		ClerkUserID:        payload.ClerkUserID,
		YouTubePlaylistID:  payload.YouTubePlaylistID,
		YouTubeAccessToken: payload.YouTubeAccessToken,
		TokenManager:       tokenManager,
		GoogleAccountEmail: payload.GoogleAccountEmail,
		FailedTracks:       payload.FailedTracks,
		UseLyricVideos:     payload.UseLyricVideos,
	}

	err = p.converterService.RetryFailedTracks(ctx, job)
	if err != nil {
		log.Printf("Retry failed tracks failed: %v", err)
		return fmt.Errorf("retry failed: %w", err)
	}

	log.Printf("Retry failed tracks completed: conversion=%s", payload.ConversionID)
	return nil
}
