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
	ConversionID        string
	UserID              string // Database UUID
	ClerkUserID         string // Clerk user ID for WebSocket
	SpotifyPlaylistID   string
	SpotifyType         string // "playlist" or "album"
	SpotifyPlaylistURL  string
	YouTubeAccessToken  string
	YouTubePlaylistName string
	UseLyricVideos      bool
}

type TrackMatchResult struct {
	Index       int // Original position in Spotify playlist
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
	conversion := &models.PlaylistConversion{}
	err = s.db.NewSelect().
		Model(conversion).
		Where("id = ?", conversionID).
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to get conversion record: %w", err)
	}

	log.Printf("ConversionService: Starting conversion %s for user %s", conversion.ID, job.UserID)

	// Send started event
	s.publishProgress(job.ClerkUserID, conversion.ID.String(), "started", "Conversion started", nil)

	// Fetch Spotify content (playlist or album)
	var playlist *spotify.Playlist
	var tracks []*spotify.PlaylistTrack

	if job.SpotifyType == "album" {
		// Fetch album metadata
		playlist, err = s.spotifyClient.GetAlbum(ctx, job.SpotifyPlaylistID)
		if err != nil {
			s.updateConversionStatus(ctx, conversion.ID, "failed", fmt.Sprintf("Failed to fetch Spotify album: %v", err))
			return nil, fmt.Errorf("failed to fetch album: %w", err)
		}

		// Fetch album tracks
		tracks, err = s.spotifyClient.GetAlbumTracks(ctx, job.SpotifyPlaylistID)
		if err != nil {
			s.updateConversionStatus(ctx, conversion.ID, "failed", fmt.Sprintf("Failed to fetch album tracks: %v", err))
			return nil, fmt.Errorf("failed to fetch tracks: %w", err)
		}
	} else {
		// Fetch playlist metadata
		playlist, err = s.spotifyClient.GetPlaylist(ctx, job.SpotifyPlaylistID)
		if err != nil {
			s.updateConversionStatus(ctx, conversion.ID, "failed", fmt.Sprintf("Failed to fetch Spotify playlist: %v", err))
			return nil, fmt.Errorf("failed to fetch playlist: %w", err)
		}

		// Fetch playlist tracks
		tracks, err = s.spotifyClient.GetPlaylistTracks(ctx, job.SpotifyPlaylistID)
		if err != nil {
			s.updateConversionStatus(ctx, conversion.ID, "failed", fmt.Sprintf("Failed to fetch playlist tracks: %v", err))
			return nil, fmt.Errorf("failed to fetch tracks: %w", err)
		}
	}

	conversion.PlaylistName = playlist.Name
	conversion.TrackCount = len(tracks)
	log.Printf("ConversionService: Fetched %d tracks from Spotify %s", len(tracks), job.SpotifyType)

	// Send progress: tracks fetched
	s.publishProgress(job.ClerkUserID, conversion.ID.String(), "progress", "Fetched tracks from Spotify", ws.ProgressData{
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
		s.publishProgress(job.ClerkUserID, conversion.ID.String(), "failed", fmt.Sprintf("Failed to create YouTube playlist: %v", err), nil)
		return nil, fmt.Errorf("failed to create YouTube playlist: %w", err)
	}

	conversion.YouTubePlaylistID = youtubePlaylistID
	conversion.YouTubePlaylistURL = fmt.Sprintf("https://www.youtube.com/playlist?list=%s", youtubePlaylistID)

	log.Printf("ConversionService: Created YouTube playlist: %s", youtubePlaylistID)

	// Send progress: YouTube playlist created
	s.publishProgress(job.ClerkUserID, conversion.ID.String(), "progress", "YouTube playlist created", ws.ProgressData{
		TotalTracks:        len(tracks),
		ProcessedTracks:    0,
		YouTubePlaylistID:  youtubePlaylistID,
		YouTubePlaylistURL: conversion.YouTubePlaylistURL,
	})

	// Match tracks concurrently using worker pool
	matchResults := s.matchTracksWithWorkerPool(ctx, tracks, job.UseLyricVideos, job.ClerkUserID, conversion.ID.String())

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

	result, err := s.db.NewUpdate().
		Model(conversion).
		Column("playlist_name", "track_count", "success_count", "failure_count", "you_tube_playlist_id", "you_tube_playlist_url", "conversion_log", "status", "updated_at", "completed_at").
		Where("id = ?", conversion.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("ConversionService: CRITICAL - Failed to update conversion record: %v", err)
		return nil, fmt.Errorf("failed to save conversion results: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("ConversionService: Conversion %s completed: %d success, %d failed (DB rows affected: %d)", conversion.ID, conversion.SuccessCount, conversion.FailureCount, rowsAffected)

	// Update user analytics
	isSuccess := conversion.Status == "completed"
	if err := s.updateUserAnalytics(ctx, conversion.UserID, job.SpotifyType, isSuccess, conversion.TrackCount, conversion.SuccessCount, conversion.FailureCount); err != nil {
		log.Printf("ConversionService: WARNING - Failed to update user analytics: %v", err)
		// Don't fail the conversion if analytics update fails
	}

	// Send completed event
	s.publishProgress(job.ClerkUserID, conversion.ID.String(), "completed", "Conversion completed", ws.ProgressData{
		TotalTracks:        conversion.TrackCount,
		ProcessedTracks:    conversion.TrackCount,
		SuccessCount:       conversion.SuccessCount,
		FailureCount:       conversion.FailureCount,
		YouTubePlaylistID:  conversion.YouTubePlaylistID,
		YouTubePlaylistURL: conversion.YouTubePlaylistURL,
	})

	return conversion, nil
}

// JobWithIndex wraps a track with its original index
type JobWithIndex struct {
	Index int
	Track *spotify.PlaylistTrack
}

// matchTracksWithWorkerPool uses a worker pool to match tracks concurrently
func (s *PlaylistConverterService) matchTracksWithWorkerPool(ctx context.Context, tracks []*spotify.PlaylistTrack, useLyricVideos bool, userID string, conversionID string) []TrackMatchResult {
	// Create channels
	jobsChan := make(chan JobWithIndex, len(tracks))
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

	// Send jobs to workers with their original indices
	for i, track := range tracks {
		jobsChan <- JobWithIndex{Index: i, Track: track}
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

	// Sort results by original index to maintain Spotify playlist order
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Index > results[j].Index {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	return results
}

// trackMatchWorker is a worker goroutine that matches tracks to YouTube videos
func (s *PlaylistConverterService) trackMatchWorker(ctx context.Context, workerID int, jobs <-chan JobWithIndex, results chan<- TrackMatchResult, useLyricVideos bool, wg *sync.WaitGroup) {
	defer wg.Done()

	for job := range jobs {
		select {
		case <-ctx.Done():
			results <- TrackMatchResult{
				Index: job.Index,
				Track: job.Track,
				Error: ctx.Err(),
			}
			return
		default:
			result := s.matchTrackToYouTube(ctx, job.Track, useLyricVideos)
			result.Index = job.Index // Preserve the original index
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
func (s *PlaylistConverterService) FetchPlaylistInfo(ctx context.Context, spotifyID string, spotifyType string) (*spotify.Playlist, error) {
	if spotifyType == "album" {
		return s.spotifyClient.GetAlbum(ctx, spotifyID)
	}
	return s.spotifyClient.GetPlaylist(ctx, spotifyID)
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

// updateUserAnalytics updates or creates user analytics record
func (s *PlaylistConverterService) updateUserAnalytics(ctx context.Context, userID uuid.UUID, spotifyType string, isSuccess bool, trackCount, successCount, failureCount int) error {
	now := time.Now()

	// Try to get existing analytics record
	var analytics models.UserAnalytics
	err := s.db.NewSelect().
		Model(&analytics).
		Where("user_id = ?", userID).
		Scan(ctx)

	if err != nil {
		// Record doesn't exist, create it
		analytics = models.UserAnalytics{
			UserID:               userID,
			TotalConversions:     1,
			TotalTracksProcessed: trackCount,
			TotalTracksMatched:   successCount,
			TotalTracksFailed:    failureCount,
			FirstConversionAt:    &now,
			LastConversionAt:     &now,
			CreatedAt:            now,
			UpdatedAt:            now,
		}

		if isSuccess {
			analytics.SuccessfulConversions = 1
		} else {
			analytics.FailedConversions = 1
		}

		if spotifyType == "album" {
			analytics.AlbumsConverted = 1
		} else {
			analytics.PlaylistsConverted = 1
		}

		_, err = s.db.NewInsert().Model(&analytics).Exec(ctx)
		return err
	}

	// Record exists, update it
	update := s.db.NewUpdate().
		Model(&analytics).
		Set("total_conversions = total_conversions + 1").
		Set("total_tracks_processed = total_tracks_processed + ?", trackCount).
		Set("total_tracks_matched = total_tracks_matched + ?", successCount).
		Set("total_tracks_failed = total_tracks_failed + ?", failureCount).
		Set("last_conversion_at = ?", now).
		Set("updated_at = ?", now)

	if isSuccess {
		update = update.Set("successful_conversions = successful_conversions + 1")
	} else {
		update = update.Set("failed_conversions = failed_conversions + 1")
	}

	if spotifyType == "album" {
		update = update.Set("albums_converted = albums_converted + 1")
	} else {
		update = update.Set("playlists_converted = playlists_converted + 1")
	}

	_, err = update.Where("user_id = ?", userID).Exec(ctx)
	return err
}
