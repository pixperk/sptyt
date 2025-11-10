package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/pixperk/sptyt/internal/spotify"
	ws "github.com/pixperk/sptyt/internal/websocket"
	"github.com/pixperk/sptyt/internal/youtube"
	"github.com/uptrace/bun"
)

type PlaylistConverterService struct {
	db            *bun.DB
	spotifyClient *spotify.Client
	youtubeClient *youtube.Client
	wsHub         *ws.Hub
	workerCount   int
}

func NewPlaylistConverterService(db *bun.DB, spotifyClient *spotify.Client, youtubeClient *youtube.Client, wsHub *ws.Hub) *PlaylistConverterService {
	return &PlaylistConverterService{
		db:            db,
		spotifyClient: spotifyClient,
		youtubeClient: youtubeClient,
		wsHub:         wsHub,
		workerCount:   5, // 5 concurrent workers
	}
}

type ConversionJob struct {
	ConversionID       string
	UserID             string
	SpotifyPlaylistID  string
	SpotifyPlaylistURL string
	YouTubeAccessToken string
	YouTubePlaylistName string
	UseLyricVideos     bool
}

type TrackMatchResult struct {
	Track       *spotify.PlaylistTrack
	YouTubeURL  string
	VideoID     string
	Error       error
	MatchMethod string
}

// ConvertPlaylist converts a Spotify playlist to YouTube playlist
func (s *PlaylistConverterService) ConvertPlaylist(ctx context.Context, job *ConversionJob) (*models.PlaylistConversion, error) {
	// Parse ConversionID
	conversionID, err := uuid.Parse(job.ConversionID)
	if err != nil {
		return nil, fmt.Errorf("invalid conversion ID: %w", err)
	}

	// Get existing conversion record (created by handler)
	var conversion models.PlaylistConversion
	err = s.db.NewSelect().
		Model(&conversion).
		Where("id = ?", conversionID).
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get conversion record: %w", err)
	}

	log.Printf("ConversionService: Starting conversion %s for user %s", conversion.ID, job.UserID)

	// Send started event
	s.publishProgress(job.UserID, conversion.ID.String(), "started", "Conversion started", nil)

	// Fetch Spotify playlist
	playlist, err := s.spotifyClient.GetPlaylist(ctx, job.SpotifyPlaylistID)
	if err != nil {
		s.updateConversionStatus(ctx, conversion.ID, "failed", fmt.Sprintf("Failed to fetch Spotify playlist: %v", err))
		return nil, fmt.Errorf("failed to fetch playlist: %w", err)
	}

	conversion.PlaylistName = playlist.Name
	conversion.TrackCount = playlist.TrackCount

	// Fetch all tracks
	tracks, err := s.spotifyClient.GetPlaylistTracks(ctx, job.SpotifyPlaylistID)
	if err != nil {
		s.updateConversionStatus(ctx, conversion.ID, "failed", fmt.Sprintf("Failed to fetch playlist tracks: %v", err))
		return nil, fmt.Errorf("failed to fetch tracks: %w", err)
	}

	conversion.TrackCount = len(tracks)
	log.Printf("ConversionService: Fetched %d tracks from Spotify", len(tracks))

	// Send progress: tracks fetched
	s.publishProgress(job.UserID, conversion.ID.String(), "progress", "Fetched tracks from Spotify", ws.ProgressData{
		TotalTracks:     len(tracks),
		ProcessedTracks: 0,
	})

	// Create YouTube playlist
	youtubePlaylistName := job.YouTubePlaylistName
	if youtubePlaylistName == "" {
		youtubePlaylistName = playlist.Name + " (from Spotify)"
	}

	youtubePlaylistID, err := s.youtubeClient.CreatePlaylist(ctx, job.YouTubeAccessToken, youtubePlaylistName, fmt.Sprintf("Converted from Spotify playlist: %s", playlist.Name))
	if err != nil {
		s.updateConversionStatus(ctx, conversion.ID, "failed", fmt.Sprintf("Failed to create YouTube playlist: %v", err))
		s.publishProgress(job.UserID, conversion.ID.String(), "failed", fmt.Sprintf("Failed to create YouTube playlist: %v", err), nil)
		return nil, fmt.Errorf("failed to create YouTube playlist: %w", err)
	}

	conversion.YouTubePlaylistID = youtubePlaylistID
	conversion.YouTubePlaylistURL = fmt.Sprintf("https://www.youtube.com/playlist?list=%s", youtubePlaylistID)

	log.Printf("ConversionService: Created YouTube playlist: %s", youtubePlaylistID)

	// Send progress: YouTube playlist created
	s.publishProgress(job.UserID, conversion.ID.String(), "progress", "YouTube playlist created", ws.ProgressData{
		TotalTracks:        len(tracks),
		ProcessedTracks:    0,
		YouTubePlaylistID:  youtubePlaylistID,
		YouTubePlaylistURL: conversion.YouTubePlaylistURL,
	})

	// Match tracks concurrently using worker pool
	matchResults := s.matchTracksWithWorkerPool(ctx, tracks, job.UseLyricVideos, job.UserID, conversion.ID.String())

	// Add matched videos to YouTube playlist
	var conversionLogs []models.TrackConversionLog
	var videoIDs []string

	for _, result := range matchResults {
		logEntry := models.TrackConversionLog{
			SpotifyTrackID:   result.Track.ID,
			SpotifyTrackName: result.Track.Name,
			SpotifyArtists:   strings.Join(result.Track.Artists, ", "),
		}

		if result.Error != nil {
			logEntry.Status = "error"
			logEntry.Error = result.Error.Error()
			conversion.FailureCount++
		} else if result.VideoID == "" {
			logEntry.Status = "not_found"
			conversion.FailureCount++
		} else {
			logEntry.Status = "success"
			logEntry.YouTubeVideoID = result.VideoID
			logEntry.YouTubeVideoURL = result.YouTubeURL
			logEntry.MatchMethod = result.MatchMethod
			conversion.SuccessCount++
			videoIDs = append(videoIDs, result.VideoID)
		}

		conversionLogs = append(conversionLogs, logEntry)
	}

	conversion.ConversionLog = conversionLogs

	log.Printf("ConversionService: Matched %d/%d tracks successfully", conversion.SuccessCount, conversion.TrackCount)

	// Add videos to YouTube playlist in batches with rate limiting
	if len(videoIDs) > 0 {
		log.Printf("ConversionService: Adding %d videos to YouTube playlist", len(videoIDs))
		addErrors := s.youtubeClient.AddVideosToPlaylistBatch(ctx, job.YouTubeAccessToken, youtubePlaylistID, videoIDs)

		// Log any errors adding videos
		for videoID, err := range addErrors {
			log.Printf("ConversionService: Failed to add video %s: %v", videoID, err)
			// Update the corresponding log entry
			for i := range conversionLogs {
				if conversionLogs[i].YouTubeVideoID == videoID {
					conversionLogs[i].Status = "error"
					conversionLogs[i].Error = fmt.Sprintf("Failed to add to playlist: %v", err)
					conversion.SuccessCount--
					conversion.FailureCount++
				}
			}
		}
	}

	// Update final conversion status
	conversion.Status = "completed"
	conversion.UpdatedAt = time.Now()
	completedAt := time.Now()
	conversion.CompletedAt = &completedAt

	_, err = s.db.NewUpdate().
		Model(conversion).
		Column("playlist_name", "track_count", "success_count", "failure_count", "youtube_playlist_id", "youtube_playlist_url", "conversion_log", "status", "updated_at", "completed_at").
		Where("id = ?", conversion.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("ConversionService: Failed to update conversion record: %v", err)
	}

	log.Printf("ConversionService: Conversion %s completed: %d success, %d failed", conversion.ID, conversion.SuccessCount, conversion.FailureCount)

	// Send completed event
	s.publishProgress(job.UserID, conversion.ID.String(), "completed", "Conversion completed", ws.ProgressData{
		TotalTracks:        conversion.TrackCount,
		ProcessedTracks:    conversion.TrackCount,
		SuccessCount:       conversion.SuccessCount,
		FailureCount:       conversion.FailureCount,
		YouTubePlaylistID:  conversion.YouTubePlaylistID,
		YouTubePlaylistURL: conversion.YouTubePlaylistURL,
	})

	return &conversion, nil
}

// matchTracksWithWorkerPool uses a worker pool to match tracks concurrently
func (s *PlaylistConverterService) matchTracksWithWorkerPool(ctx context.Context, tracks []*spotify.PlaylistTrack, useLyricVideos bool, userID string, conversionID string) []TrackMatchResult {
	// Create channels
	jobsChan := make(chan *spotify.PlaylistTrack, len(tracks))
	resultsChan := make(chan TrackMatchResult, len(tracks))

	totalTracks := len(tracks)
	processedCount := 0
	var mu sync.Mutex

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < s.workerCount; i++ {
		wg.Add(1)
		go s.trackMatchWorker(ctx, i, jobsChan, resultsChan, useLyricVideos, &wg)
	}

	// Send jobs to workers
	for _, track := range tracks {
		jobsChan <- track
	}
	close(jobsChan)

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results and send progress updates
	var results []TrackMatchResult
	for result := range resultsChan {
		results = append(results, result)

		mu.Lock()
		processedCount++
		currentCount := processedCount
		mu.Unlock()

		// Send progress update every 5 tracks or on completion
		if currentCount%5 == 0 || currentCount == totalTracks {
			var currentTrack string
			if result.Track != nil {
				currentTrack = fmt.Sprintf("%s - %s", result.Track.Name, strings.Join(result.Track.Artists, ", "))
			}

			s.publishProgress(userID, conversionID, "progress", fmt.Sprintf("Matching tracks: %d/%d", currentCount, totalTracks), ws.ProgressData{
				TotalTracks:     totalTracks,
				ProcessedTracks: currentCount,
				CurrentTrack:    currentTrack,
			})
		}
	}

	return results
}

// trackMatchWorker is a worker goroutine that matches tracks to YouTube videos
func (s *PlaylistConverterService) trackMatchWorker(ctx context.Context, workerID int, jobs <-chan *spotify.PlaylistTrack, results chan<- TrackMatchResult, useLyricVideos bool, wg *sync.WaitGroup) {
	defer wg.Done()

	for track := range jobs {
		select {
		case <-ctx.Done():
			results <- TrackMatchResult{
				Track: track,
				Error: ctx.Err(),
			}
			return
		default:
			result := s.matchTrackToYouTube(ctx, track, useLyricVideos)
			results <- result
		}
	}
}

// matchTrackToYouTube matches a single Spotify track to a YouTube video
func (s *PlaylistConverterService) matchTrackToYouTube(ctx context.Context, track *spotify.PlaylistTrack, useLyricVideos bool) TrackMatchResult {
	result := TrackMatchResult{
		Track: track,
	}

	artistsStr := strings.Join(track.Artists, " ")

	// Try different matching strategies
	var videoURL string
	var err error

	// Strategy 1: Official Music Video
	if !useLyricVideos {
		videoURL, err = s.youtubeClient.SearchOfficialMV(ctx, track.Name, artistsStr)
		if err == nil && videoURL != "" {
			result.YouTubeURL = videoURL
			result.VideoID = extractVideoID(videoURL)
			result.MatchMethod = "official_mv"
			return result
		}
	}

	// Strategy 2: Lyric Video
	videoURL, err = s.youtubeClient.SearchLyricVideo(ctx, track.Name, artistsStr)
	if err == nil && videoURL != "" {
		result.YouTubeURL = videoURL
		result.VideoID = extractVideoID(videoURL)
		result.MatchMethod = "lyric_video"
		return result
	}

	// No match found
	result.Error = fmt.Errorf("no YouTube video found")
	return result
}

// extractVideoID extracts video ID from YouTube URL
func extractVideoID(url string) string {
	// URL format: https://www.youtube.com/watch?v=VIDEO_ID
	if strings.Contains(url, "v=") {
		parts := strings.Split(url, "v=")
		if len(parts) > 1 {
			videoID := parts[1]
			// Remove any additional query parameters
			if idx := strings.Index(videoID, "&"); idx != -1 {
				videoID = videoID[:idx]
			}
			return videoID
		}
	}
	return ""
}

// updateConversionStatus updates the status of a conversion
func (s *PlaylistConverterService) updateConversionStatus(ctx context.Context, id uuid.UUID, status string, errorMsg string) {
	update := s.db.NewUpdate().
		Model((*models.PlaylistConversion)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id)

	if errorMsg != "" {
		update = update.Set("error_message = ?", errorMsg)
	}

	if status == "completed" || status == "failed" {
		completedAt := time.Now()
		update = update.Set("completed_at = ?", completedAt)
	}

	_, err := update.Exec(ctx)
	if err != nil {
		log.Printf("Failed to update conversion status: %v", err)
	}
}

// GetConversion fetches a conversion by ID
func (s *PlaylistConverterService) GetConversion(ctx context.Context, id uuid.UUID) (*models.PlaylistConversion, error) {
	var conversion models.PlaylistConversion
	err := s.db.NewSelect().
		Model(&conversion).
		Where("id = ?", id).
		Scan(ctx)

	if err != nil {
		return nil, err
	}

	return &conversion, nil
}

// GetUserConversions fetches all conversions for a user
func (s *PlaylistConverterService) GetUserConversions(ctx context.Context, userID uuid.UUID, limit int) ([]*models.PlaylistConversion, error) {
	var conversions []*models.PlaylistConversion

	query := s.db.NewSelect().
		Model(&conversions).
		Where("user_id = ?", userID).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Scan(ctx)
	if err != nil {
		return nil, err
	}

	return conversions, nil
}

// FetchPlaylistInfo fetches basic playlist info from Spotify
func (s *PlaylistConverterService) FetchPlaylistInfo(ctx context.Context, playlistID string) (*spotify.Playlist, error) {
	return s.spotifyClient.GetPlaylist(ctx, playlistID)
}

// publishProgress sends progress updates via WebSocket
func (s *PlaylistConverterService) publishProgress(userID string, conversionID string, eventType string, message string, data interface{}) {
	if s.wsHub == nil {
		return
	}

	event := ws.ProgressEvent{
		Type:         eventType,
		ConversionID: conversionID,
		Message:      message,
		Data:         data,
	}

	s.wsHub.BroadcastToUser(userID, event)
}
