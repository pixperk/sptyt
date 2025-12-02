package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pixperk/sptyt/internal/cache"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/pixperk/sptyt/internal/spotify"
	ws "github.com/pixperk/sptyt/internal/websocket"
	"github.com/pixperk/sptyt/internal/youtube"
	"github.com/pixperk/sptyt/pkg/errors"
	"github.com/uptrace/bun"
)

// AnalyticsTaskClient interface for enqueuing analytics tasks (avoids import cycle)
type AnalyticsTaskClient interface {
	EnqueueAnalyticsUpdate(userID, spotifyType string, isSuccess bool, trackCount, successCount, failureCount int, countsAgainstQuota bool, youtubeSearches, playlistInserts int, googleAccountEmail string) error
}

type PlaylistConverterService struct {
	db            *bun.DB
	spotifyClient *spotify.Client
	youtubeClient *youtube.Client
	wsHub         *ws.Hub
	taskClient    AnalyticsTaskClient
	cache         *cache.RedisCache
	workerCount   int
}

// YouTubeSearchCacheTTL is the TTL for cached YouTube search results (48 hours)
// YouTube videos don't change often, so a longer TTL is appropriate
const YouTubeSearchCacheTTL = 48 * time.Hour

// YouTubeNotFoundCacheTTL is the TTL for caching "not found" results (6 hours)
// Shorter TTL to allow retries if a video becomes available
const YouTubeNotFoundCacheTTL = 6 * time.Hour

func NewPlaylistConverterService(db *bun.DB, spotifyClient *spotify.Client, youtubeClient *youtube.Client, wsHub *ws.Hub, taskClient AnalyticsTaskClient, redisCache *cache.RedisCache) *PlaylistConverterService {
	return &PlaylistConverterService{
		db:            db,
		spotifyClient: spotifyClient,
		youtubeClient: youtubeClient,
		wsHub:         wsHub,
		taskClient:    taskClient,
		cache:         redisCache,
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
	GoogleAccountEmail  string // Google account email for quota tracking
}

// RetryJob represents a job to retry failed tracks
type RetryJob struct {
	ConversionID       string
	UserID             string
	ClerkUserID        string
	YouTubePlaylistID  string
	YouTubeAccessToken string
	GoogleAccountEmail string
	FailedTracks       []models.TrackConversionLog
	UseLyricVideos     bool
}

type TrackMatchResult struct {
	Index       int // Original position in Spotify playlist
	Track       *spotify.PlaylistTrack
	YouTubeURL  string
	VideoID     string
	Error       error
	MatchMethod string
	SearchCount int // Number of YouTube search API calls made for this track
}

// isYouTubeAPIError checks if an error is a YouTube API error that shouldn't count against quota
// Now uses typed errors instead of string matching
func isYouTubeAPIError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's a typed YouTube API error
	if errors.IsYouTubeAPIError(err) {
		return true
	}

	// Fallback to string matching for legacy errors
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "409") ||
		strings.Contains(errStr, "quota") ||
		strings.Contains(errStr, "quotaexceeded") ||
		strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "forbidden")
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

	// Save cover image (use the first/largest image if available)
	if len(playlist.Images) > 0 {
		conversion.SpotifyCoverImage = playlist.Images[0].URL
	}

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
		// Check if this is a YouTube API error (quota, 401, 403, 409)
		countsAgainstQuota := !isYouTubeAPIError(err)

		// Update conversion status with CountsAgainstQuota flag
		s.updateConversionStatusWithQuotaFlag(ctx, conversion.ID, "failed", fmt.Sprintf("Failed to create YouTube playlist: %v", err), countsAgainstQuota)
		s.publishProgress(job.ClerkUserID, conversion.ID.String(), "failed", fmt.Sprintf("Failed to create YouTube playlist: %v", err), nil)

		return nil, fmt.Errorf("failed to create YouTube playlist: %w", err)
	}

	conversion.YouTubePlaylistID = youtubePlaylistID
	conversion.YouTubePlaylistURL = fmt.Sprintf("https://www.youtube.com/playlist?list=%s", youtubePlaylistID)
	conversion.GoogleAccountEmail = job.GoogleAccountEmail

	// Send progress: YouTube playlist created
	s.publishProgress(job.ClerkUserID, conversion.ID.String(), "progress", "YouTube playlist created", ws.ProgressData{
		TotalTracks:        len(tracks),
		ProcessedTracks:    0,
		YouTubePlaylistID:  youtubePlaylistID,
		YouTubePlaylistURL: conversion.YouTubePlaylistURL,
	})

	// Match tracks concurrently using worker pool (uses user's YouTube quota via OAuth token)
	matchResults := s.matchTracksWithWorkerPool(ctx, tracks, job.UseLyricVideos, job.ClerkUserID, conversion.ID.String(), job.YouTubeAccessToken)

	// Add matched videos to YouTube playlist
	var conversionLogs []models.TrackConversionLog
	var videoIDs []string
	totalSearches := 0 // Track total YouTube search API calls

	for _, result := range matchResults {
		totalSearches += result.SearchCount // Sum up searches from all tracks

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

	// Add videos to YouTube playlist in batches with rate limiting
	playlistInserts := 0 // Track successful playlist insert API calls
	if len(videoIDs) > 0 {
		addErrors := s.youtubeClient.AddVideosToPlaylistBatch(ctx, job.YouTubeAccessToken, youtubePlaylistID, videoIDs)

		// Calculate successful inserts (each video added = 1 playlist insert API call)
		playlistInserts = len(videoIDs) - len(addErrors)

		// Update log entries for any errors adding videos
		for videoID, err := range addErrors {
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
		Column("playlist_name", "track_count", "success_count", "failure_count", "you_tube_playlist_id", "you_tube_playlist_url", "spotify_cover_image", "conversion_log", "status", "updated_at", "completed_at", "google_account_email").
		Where("id = ?", conversion.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("ConversionService: Failed to update conversion record: %v", err)
		return nil, fmt.Errorf("failed to save conversion results: %w", err)
	}

	// Enqueue analytics update task (async) - includes YouTube quota tracking
	isSuccess := conversion.Status == "completed"
	if err := s.taskClient.EnqueueAnalyticsUpdate(job.UserID, job.SpotifyType, isSuccess, conversion.TrackCount, conversion.SuccessCount, conversion.FailureCount, conversion.CountsAgainstQuota, totalSearches, playlistInserts, job.GoogleAccountEmail); err != nil {
		log.Printf("ConversionService: WARNING - Failed to enqueue analytics update task: %v", err)
		// Don't fail the conversion if analytics enqueue fails
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
func (s *PlaylistConverterService) matchTracksWithWorkerPool(ctx context.Context, tracks []*spotify.PlaylistTrack, useLyricVideos bool, userID string, conversionID string, accessToken string) []TrackMatchResult {
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
		go s.trackMatchWorker(ctx, i, jobsChan, resultsChan, useLyricVideos, accessToken, &wg)
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
func (s *PlaylistConverterService) trackMatchWorker(ctx context.Context, workerID int, jobs <-chan JobWithIndex, results chan<- TrackMatchResult, useLyricVideos bool, accessToken string, wg *sync.WaitGroup) {
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
			result := s.matchTrackToYouTube(ctx, job.Track, useLyricVideos, accessToken)
			result.Index = job.Index // Preserve the original index
			results <- result
		}
	}
}

// matchTrackToYouTube matches a single Spotify track to a YouTube video
// Uses user's OAuth token for searches (uses user's YouTube quota, not server's)
// Caches successful searches to avoid redundant API calls
func (s *PlaylistConverterService) matchTrackToYouTube(ctx context.Context, track *spotify.PlaylistTrack, useLyricVideos bool, accessToken string) TrackMatchResult {
	result := TrackMatchResult{
		Track:       track,
		SearchCount: 0,
	}

	artistsStr := strings.Join(track.Artists, " ")

	// Check cache first (if available)
	if s.cache != nil {
		cached, err := s.cache.GetYouTubeSearchResult(ctx, track.Name, artistsStr, useLyricVideos)
		if err == nil && cached != nil {
			// Cache hit!
			if cached.VideoID == "" {
				// Cached "not found" result
				result.Error = fmt.Errorf("no YouTube video found (cached)")
				return result
			}
			result.YouTubeURL = cached.VideoURL
			result.VideoID = cached.VideoID
			result.MatchMethod = cached.MatchMethod + "_cached"
			return result
		}
		// Cache miss - continue with API search
	}

	// Try different matching strategies
	var videoURL string
	var err error

	// Strategy 1: Official Music Video (using user's quota via OAuth token)
	if !useLyricVideos {
		result.SearchCount++ // Count this search
		videoURL, err = s.youtubeClient.SearchOfficialMVWithToken(ctx, accessToken, track.Name, artistsStr)
		if err == nil && videoURL != "" {
			result.YouTubeURL = videoURL
			result.VideoID = extractVideoID(videoURL)
			result.MatchMethod = "official_mv"

			// Cache the successful result
			s.cacheYouTubeResult(ctx, track.Name, artistsStr, useLyricVideos, result.VideoID, videoURL, result.MatchMethod)
			return result
		}
	}

	// Strategy 2: Lyric Video (using user's quota via OAuth token)
	result.SearchCount++ // Count this search
	videoURL, err = s.youtubeClient.SearchLyricVideoWithToken(ctx, accessToken, track.Name, artistsStr)
	if err == nil && videoURL != "" {
		result.YouTubeURL = videoURL
		result.VideoID = extractVideoID(videoURL)
		result.MatchMethod = "lyric_video"

		// Cache the successful result
		s.cacheYouTubeResult(ctx, track.Name, artistsStr, useLyricVideos, result.VideoID, videoURL, result.MatchMethod)
		return result
	}

	// No match found - cache the "not found" result to avoid repeated searches
	if s.cache != nil {
		_ = s.cache.CacheYouTubeNotFound(ctx, track.Name, artistsStr, useLyricVideos, YouTubeNotFoundCacheTTL)
	}

	result.Error = fmt.Errorf("no YouTube video found")
	return result
}

// cacheYouTubeResult caches a successful YouTube search result
func (s *PlaylistConverterService) cacheYouTubeResult(ctx context.Context, trackName, artists string, useLyricVideos bool, videoID, videoURL, matchMethod string) {
	if s.cache == nil {
		return
	}

	result := &cache.YouTubeSearchResult{
		VideoID:     videoID,
		VideoURL:    videoURL,
		MatchMethod: matchMethod,
	}

	if err := s.cache.SetYouTubeSearchResult(ctx, trackName, artists, useLyricVideos, result, YouTubeSearchCacheTTL); err != nil {
		log.Printf("Warning: Failed to cache YouTube search result: %v", err)
	}
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

// updateConversionStatusWithQuotaFlag updates conversion status with counts_against_quota flag
func (s *PlaylistConverterService) updateConversionStatusWithQuotaFlag(ctx context.Context, id uuid.UUID, status string, errorMsg string, countsAgainstQuota bool) {
	update := s.db.NewUpdate().
		Model((*models.PlaylistConversion)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", time.Now()).
		Set("counts_against_quota = ?", countsAgainstQuota).
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

// GetUserConversions fetches all conversions for a user (excludes soft-deleted)
func (s *PlaylistConverterService) GetUserConversions(ctx context.Context, userID uuid.UUID, limit int) ([]*models.PlaylistConversion, error) {
	var conversions []*models.PlaylistConversion

	query := s.db.NewSelect().
		Model(&conversions).
		Where("user_id = ?", userID).
		Where("deleted_at IS NULL"). // Exclude soft-deleted
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

// UpdateUserAnalytics updates or creates user analytics record (accepts string UUID)
func (s *PlaylistConverterService) UpdateUserAnalytics(ctx context.Context, userIDStr string, spotifyType string, isSuccess bool, trackCount, successCount, failureCount int, countsAgainstQuota bool, youtubeSearches, playlistInserts int, googleAccountEmail string) error {
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}
	return s.updateUserAnalytics(ctx, userID, spotifyType, isSuccess, trackCount, successCount, failureCount, countsAgainstQuota, youtubeSearches, playlistInserts, googleAccountEmail)
}

// updateUserAnalytics updates or creates user analytics record (internal method)
func (s *PlaylistConverterService) updateUserAnalytics(ctx context.Context, userID uuid.UUID, spotifyType string, isSuccess bool, trackCount, successCount, failureCount int, countsAgainstQuota bool, youtubeSearches, playlistInserts int, googleAccountEmail string) error {
	now := time.Now()

	// Update YouTube account quota (per Google account, not per user)
	if googleAccountEmail != "" && (youtubeSearches > 0 || playlistInserts > 0) {
		if err := s.updateYouTubeAccountQuota(ctx, googleAccountEmail, youtubeSearches, playlistInserts); err != nil {
			log.Printf("Warning: Failed to update YouTube account quota for %s: %v", googleAccountEmail, err)
			// Don't fail the analytics update if quota tracking fails
		}
	}

	// Try to get existing analytics record
	var analytics models.UserAnalytics
	err := s.db.NewSelect().
		Model(&analytics).
		Where("user_id = ?", userID).
		Scan(ctx)

	if err != nil {
		// Record doesn't exist, create it
		monthlyConversions := 0
		if countsAgainstQuota {
			monthlyConversions = 1
		}

		analytics = models.UserAnalytics{
			UserID:               userID,
			TotalConversions:     1,
			TotalTracksProcessed: trackCount,
			TotalTracksMatched:   successCount,
			TotalTracksFailed:    failureCount,
			MonthlyConversions:   monthlyConversions,
			CurrentMonth:         int(now.Month()),
			CurrentYear:          now.Year(),
			DailyYouTubeSearches: youtubeSearches,
			DailyPlaylistInserts: playlistInserts,
			LastQuotaResetDate:   &now,
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

	// Update YouTube quota tracking
	// Check if we need to reset daily quota counters (new day)
	if analytics.NeedsQuotaReset() {
		// New day, reset counters
		update = update.
			Set("daily_youtube_searches = ?", youtubeSearches).
			Set("daily_playlist_inserts = ?", playlistInserts).
			Set("last_quota_reset_date = ?", now)
	} else {
		// Same day, increment
		update = update.
			Set("daily_youtube_searches = daily_youtube_searches + ?", youtubeSearches).
			Set("daily_playlist_inserts = daily_playlist_inserts + ?", playlistInserts)
	}

	// Only update monthly counter if this conversion counts against quota
	if countsAgainstQuota {
		// Check if we need to reset monthly counter (new month)
		currentMonth := int(now.Month())
		currentYear := now.Year()
		if analytics.CurrentMonth != currentMonth || analytics.CurrentYear != currentYear {
			// New month, reset counter to 1
			update = update.Set("monthly_conversions = 1").
				Set("current_month = ?", currentMonth).
				Set("current_year = ?", currentYear)
		} else {
			// Same month, increment
			update = update.Set("monthly_conversions = monthly_conversions + 1")
		}
	}

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

// updateYouTubeAccountQuota updates or creates YouTube quota record per Google account
func (s *PlaylistConverterService) updateYouTubeAccountQuota(ctx context.Context, accountEmail string, searches, inserts int) error {
	now := time.Now()

	// Try to get existing quota record
	var quota models.YouTubeAccountQuota
	err := s.db.NewSelect().
		Model(&quota).
		Where("account_email = ?", accountEmail).
		Scan(ctx)

	if err != nil {
		// Record doesn't exist, create it
		quota = models.YouTubeAccountQuota{
			AccountEmail:         accountEmail,
			DailySearches:        searches,
			DailyPlaylistInserts: inserts,
			LastQuotaResetDate:   &now,
			CreatedAt:            now,
			UpdatedAt:            now,
		}

		_, err = s.db.NewInsert().Model(&quota).Exec(ctx)
		return err
	}

	// Record exists, update it
	update := s.db.NewUpdate().
		Model(&quota).
		Set("updated_at = ?", now)

	// Check if we need to reset daily quota counters (new day)
	if quota.NeedsQuotaReset() {
		// New day, reset counters
		update = update.
			Set("daily_searches = ?", searches).
			Set("daily_playlist_inserts = ?", inserts).
			Set("last_quota_reset_date = ?", now)
	} else {
		// Same day, increment
		update = update.
			Set("daily_searches = daily_searches + ?", searches).
			Set("daily_playlist_inserts = daily_playlist_inserts + ?", inserts)
	}

	_, err = update.Where("account_email = ?", accountEmail).Exec(ctx)
	return err
}

// RetryFailedTracks retries adding failed tracks to an existing YouTube playlist
func (s *PlaylistConverterService) RetryFailedTracks(ctx context.Context, job *RetryJob) error {
	conversionID, err := uuid.Parse(job.ConversionID)
	if err != nil {
		return fmt.Errorf("invalid conversion ID: %w", err)
	}

	// Get existing conversion record
	conversion := &models.PlaylistConversion{}
	err = s.db.NewSelect().
		Model(conversion).
		Where("id = ?", conversionID).
		Scan(ctx)

	if err != nil {
		return fmt.Errorf("failed to get conversion record: %w", err)
	}

	// Check quota before starting
	var accountQuota models.YouTubeAccountQuota
	quotaErr := s.db.NewSelect().
		Model(&accountQuota).
		Where("account_email = ?", job.GoogleAccountEmail).
		Scan(ctx)

	if quotaErr == nil && !accountQuota.NeedsQuotaReset() && !accountQuota.CanAffordSearch() {
		// Quota exceeded - fail immediately
		s.publishProgress(job.ClerkUserID, job.ConversionID, "retry_failed", "YouTube API quota exceeded", ws.ProgressData{
			TotalTracks:     len(job.FailedTracks),
			ProcessedTracks: 0,
			SuccessCount:    0,
			FailureCount:    len(job.FailedTracks),
			Error:           "quota_exceeded",
			ErrorMessage:    "YouTube API quota exceeded for today. Please try again after midnight Pacific Time.",
		})
		return fmt.Errorf("quota exceeded")
	}

	// Send started event
	s.publishProgress(job.ClerkUserID, job.ConversionID, "retry_started", "Retrying failed tracks", ws.ProgressData{
		TotalTracks:     len(job.FailedTracks),
		ProcessedTracks: 0,
	})

	// Match failed tracks to YouTube videos
	totalSearches := 0
	successfulRetries := 0
	var videoIDsToAdd []string
	updatedLogs := make([]models.TrackConversionLog, len(conversion.ConversionLog))
	copy(updatedLogs, conversion.ConversionLog)
	processedCount := 0
	quotaExhausted := false

	// Create a map for quick lookup of failed track indices
	failedTrackMap := make(map[string]int) // SpotifyTrackID -> index in job.FailedTracks
	for i, track := range job.FailedTracks {
		failedTrackMap[track.SpotifyTrackID] = i
	}

	for i, logEntry := range updatedLogs {
		if _, isFailedTrack := failedTrackMap[logEntry.SpotifyTrackID]; !isFailedTrack {
			continue // Skip tracks that weren't requested for retry
		}

		processedCount++

		// Create a PlaylistTrack from the log entry for matching
		track := &spotify.PlaylistTrack{
			ID:      logEntry.SpotifyTrackID,
			Name:    logEntry.SpotifyTrackName,
			Artists: strings.Split(logEntry.SpotifyArtists, ", "),
		}

		result := s.matchTrackToYouTube(ctx, track, job.UseLyricVideos, job.YouTubeAccessToken)
		totalSearches += result.SearchCount

		// Check if we hit a quota error (403 quotaExceeded)
		if result.Error != nil && isYouTubeAPIError(result.Error) {
			errStr := strings.ToLower(result.Error.Error())
			if strings.Contains(errStr, "quota") || strings.Contains(errStr, "403") {
				quotaExhausted = true
				// Send quota exceeded message
				s.publishProgress(job.ClerkUserID, job.ConversionID, "retry_failed", "YouTube API quota exceeded", ws.ProgressData{
					TotalTracks:     len(job.FailedTracks),
					ProcessedTracks: processedCount,
					SuccessCount:    successfulRetries,
					FailureCount:    len(job.FailedTracks) - successfulRetries,
					Error:           "quota_exceeded",
					ErrorMessage:    "YouTube API quota exceeded. Retry stopped. Successfully retried tracks have been saved.",
				})
				break
			}
		}

		if result.Error == nil && result.VideoID != "" {
			updatedLogs[i].Status = "success"
			updatedLogs[i].YouTubeVideoID = result.VideoID
			updatedLogs[i].YouTubeVideoURL = result.YouTubeURL
			updatedLogs[i].MatchMethod = result.MatchMethod
			updatedLogs[i].Error = ""
			videoIDsToAdd = append(videoIDsToAdd, result.VideoID)
			successfulRetries++
		}

		// Send progress update
		s.publishProgress(job.ClerkUserID, job.ConversionID, "retry_progress",
			fmt.Sprintf("Retrying: %d/%d", processedCount, len(job.FailedTracks)), ws.ProgressData{
				TotalTracks:     len(job.FailedTracks),
				ProcessedTracks: processedCount,
				CurrentTrack:    fmt.Sprintf("%s - %s", logEntry.SpotifyTrackName, logEntry.SpotifyArtists),
			})
	}

	// Add matched videos to YouTube playlist
	playlistInserts := 0
	if len(videoIDsToAdd) > 0 && !quotaExhausted {
		addErrors := s.youtubeClient.AddVideosToPlaylistBatch(ctx, job.YouTubeAccessToken, job.YouTubePlaylistID, videoIDsToAdd)
		playlistInserts = len(videoIDsToAdd) - len(addErrors)

		// Update log entries for any errors adding videos
		for videoID, addErr := range addErrors {
			// Check if this is a quota error
			if isYouTubeAPIError(addErr) {
				errStr := strings.ToLower(addErr.Error())
				if strings.Contains(errStr, "quota") || strings.Contains(errStr, "403") {
					quotaExhausted = true
				}
			}

			for j := range updatedLogs {
				if updatedLogs[j].YouTubeVideoID == videoID {
					updatedLogs[j].Status = "error"
					updatedLogs[j].Error = fmt.Sprintf("Failed to add to playlist: %v", addErr)
					successfulRetries--
				}
			}
		}
	}

	// Recalculate success/failure counts
	newSuccessCount := 0
	newFailureCount := 0
	for _, logItem := range updatedLogs {
		if logItem.Status == "success" {
			newSuccessCount++
		} else {
			newFailureCount++
		}
	}

	// Update conversion record
	conversion.ConversionLog = updatedLogs
	conversion.SuccessCount = newSuccessCount
	conversion.FailureCount = newFailureCount
	conversion.UpdatedAt = time.Now()

	_, err = s.db.NewUpdate().
		Model(conversion).
		Column("conversion_log", "success_count", "failure_count", "updated_at").
		Where("id = ?", conversion.ID).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to update conversion record: %w", err)
	}

	// Update analytics: only update YouTube quota usage (searches + inserts)
	// Retries don't count as new conversions, but they use quota
	if totalSearches > 0 || playlistInserts > 0 {
		if quotaUpdateErr := s.updateYouTubeAccountQuota(ctx, job.GoogleAccountEmail, totalSearches, playlistInserts); quotaUpdateErr != nil {
			log.Printf("Warning: Failed to update YouTube account quota for retry: %v", quotaUpdateErr)
		}
	}

	// Send completed or partial success event (if not already sent quota failure)
	if !quotaExhausted {
		eventType := "retry_completed"
		message := "Retry completed"
		if successfulRetries == 0 && len(job.FailedTracks) > 0 {
			message = "Retry completed - no tracks could be matched"
		} else if successfulRetries == len(job.FailedTracks) {
			message = "All tracks retried successfully!"
		} else {
			message = fmt.Sprintf("Retry completed - %d of %d tracks succeeded", successfulRetries, len(job.FailedTracks))
		}

		s.publishProgress(job.ClerkUserID, job.ConversionID, eventType, message, ws.ProgressData{
			TotalTracks:        len(job.FailedTracks),
			ProcessedTracks:    len(job.FailedTracks),
			SuccessCount:       successfulRetries,
			FailureCount:       len(job.FailedTracks) - successfulRetries,
			YouTubePlaylistID:  conversion.YouTubePlaylistID,
			YouTubePlaylistURL: conversion.YouTubePlaylistURL,
		})
	}

	return nil
}
