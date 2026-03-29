package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/auth"
	"github.com/pixperk/sptyt/internal/cache"
	"github.com/pixperk/sptyt/internal/crypto"
	"github.com/pixperk/sptyt/internal/database"
	custommw "github.com/pixperk/sptyt/internal/middleware"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/pixperk/sptyt/internal/services"
	"github.com/pixperk/sptyt/internal/tasks"
	"github.com/pixperk/sptyt/pkg/utils"
	"github.com/uptrace/bun"
)

type PlaylistHandler struct {
	db               *bun.DB
	converterService *services.PlaylistConverterService
	taskClient       *tasks.Client
	cache            *cache.RedisCache
}

func NewPlaylistHandler(db *bun.DB, converterService *services.PlaylistConverterService, taskClient *tasks.Client, redisCache *cache.RedisCache) *PlaylistHandler {
	return &PlaylistHandler{
		db:               db,
		converterService: converterService,
		taskClient:       taskClient,
		cache:            redisCache,
	}
}

// ConvertPlaylistRequest is the request body for playlist conversion
type ConvertPlaylistRequest struct {
	SpotifyPlaylistURL  string `json:"spotify_playlist_url" validate:"required"`
	YouTubePlaylistName string `json:"youtube_playlist_name"`
	UseLyricVideos      bool   `json:"use_lyric_videos"`
}

// ConvertPlaylist converts a Spotify playlist to YouTube
func (h *PlaylistHandler) ConvertPlaylist(c echo.Context) error {
	// Get authenticated user
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Get user from database
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Check if user has YouTube OAuth token
	var youtubeToken models.UserOAuthToken
	err = h.db.NewSelect().
		Model(&youtubeToken).
		Where("user_id = ? AND provider = ?", user.ID, "youtube").
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "YouTube not authorized. Please connect your YouTube account first.")
	}

	if err := youtubeToken.DecryptTokens(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to read YouTube token")
	}

	// Refresh token if expired or about to expire (within 5 minutes)
	if time.Until(youtubeToken.ExpiresAt) < 5*time.Minute {
		refreshedToken, err := h.refreshYouTubeToken(ctx, &youtubeToken)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "YouTube token expired. Please reconnect your YouTube account.")
		}
		youtubeToken = *refreshedToken
	}

	// Parse request body
	var req ConvertPlaylistRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if req.SpotifyPlaylistURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "spotify_playlist_url is required")
	}

	// Extract Spotify playlist/album ID and type from URL
	spotifyID, spotifyType, err := utils.ExtractSpotifyPlaylistID(req.SpotifyPlaylistURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid Spotify playlist/album URL")
	}

	// Fetch playlist/album to check track count BEFORE starting conversion
	spotifyPlaylist, err := h.converterService.FetchPlaylistInfo(ctx, spotifyID, spotifyType)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Failed to fetch Spotify playlist. Make sure it's a valid public playlist.")
	}

	// Validate playlist size against user's limits
	if err := custommw.ValidatePlaylistSize(c, spotifyPlaylist.TrackCount); err != nil {
		return err
	}

	// Create conversion record in database
	conversion := &models.PlaylistConversion{
		ID:                 uuid.New(),
		UserID:             user.ID,
		SpotifyPlaylistID:  spotifyID,
		SpotifyPlaylistURL: req.SpotifyPlaylistURL,
		PlaylistName:       spotifyPlaylist.Name,
		TrackCount:         spotifyPlaylist.TrackCount,
		Status:             "pending",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	// Save initial conversion record
	_, err = h.db.NewInsert().Model(conversion).Exec(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create conversion record")
	}

	// Create task payload
	payload := tasks.PlaylistConversionPayload{
		ConversionID:        conversion.ID.String(),
		UserID:              user.ID.String(),
		ClerkUserID:         clerkUserID,
		SpotifyPlaylistID:   spotifyID,
		SpotifyType:         spotifyType,
		SpotifyPlaylistURL:  req.SpotifyPlaylistURL,
		YouTubeAccessToken:  youtubeToken.AccessToken,
		YouTubePlaylistName: req.YouTubePlaylistName,
		UseLyricVideos:      req.UseLyricVideos,
		GoogleAccountEmail:  youtubeToken.AccountEmail,
	}

	// Enqueue task with Asynq
	err = h.taskClient.EnqueuePlaylistConversion(payload)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to start conversion")
	}

	return c.JSON(http.StatusAccepted, map[string]interface{}{
		"message":       "Playlist conversion started",
		"conversion_id": conversion.ID.String(),
		"status":        "pending",
	})
}

// GetConversionStatus gets the status of a playlist conversion
func (h *PlaylistHandler) GetConversionStatus(c echo.Context) error {
	conversionID := c.Param("id")
	if conversionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Conversion ID required")
	}

	// Get authenticated user
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Get user from database
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Parse UUID
	var conversion models.PlaylistConversion
	err = h.db.NewSelect().
		Model(&conversion).
		Where("id = ? AND user_id = ?", conversionID, user.ID).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Conversion not found")
	}

	return c.JSON(http.StatusOK, conversion)
}

// GetUserConversions gets all conversions for the authenticated user
func (h *PlaylistHandler) GetUserConversions(c echo.Context) error {
	// Get authenticated user
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Get user from database
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Get conversions
	conversions, err := h.converterService.GetUserConversions(ctx, user.ID, 20) // Limit to 20
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get conversions")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"conversions": conversions,
	})
}

// GetDetailedUserConversions returns all user conversions with complete song details and covers (with pagination)
func (h *PlaylistHandler) GetDetailedUserConversions(c echo.Context) error {
	// Get authenticated user
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Get user from database
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Parse pagination parameters
	var limit, offset int
	limitParam := c.QueryParam("limit")
	offsetParam := c.QueryParam("offset")

	// Default limit: 10, max: 100
	if limitParam == "" {
		limit = 10
	} else {
		fmt.Sscanf(limitParam, "%d", &limit)
		if limit <= 0 || limit > 100 {
			limit = 10
		}
	}

	// Default offset: 0
	if offsetParam == "" {
		offset = 0
	} else {
		fmt.Sscanf(offsetParam, "%d", &offset)
		if offset < 0 {
			offset = 0
		}
	}

	// Get total count (excluding soft-deleted)
	totalCount, err := h.db.NewSelect().
		Model((*models.PlaylistConversion)(nil)).
		Where("user_id = ?", user.ID).
		Where("deleted_at IS NULL").
		Count(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get conversions")
	}

	// Get conversions with pagination (excluding soft-deleted)
	var conversions []models.PlaylistConversion
	err = h.db.NewSelect().
		Model(&conversions).
		Where("user_id = ?", user.ID).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get conversions")
	}

	// Build detailed response
	detailedConversions := make([]map[string]interface{}, len(conversions))
	for i, conv := range conversions {
		// Count successful and failed tracks from conversion log
		var successfulTracks, failedTracks []models.TrackConversionLog
		for _, track := range conv.ConversionLog {
			if track.Status == "success" {
				successfulTracks = append(successfulTracks, track)
			} else {
				failedTracks = append(failedTracks, track)
			}
		}

		detailedConversions[i] = map[string]interface{}{
			"id":                    conv.ID,
			"playlist_name":         conv.PlaylistName,
			"spotify_playlist_id":   conv.SpotifyPlaylistID,
			"spotify_playlist_url":  conv.SpotifyPlaylistURL,
			"spotify_cover_image":   conv.SpotifyCoverImage,
			"youtube_playlist_id":   conv.YouTubePlaylistID,
			"youtube_playlist_url":  conv.YouTubePlaylistURL,
			"status":                conv.Status,
			"track_count":           conv.TrackCount,
			"success_count":         conv.SuccessCount,
			"failure_count":         conv.FailureCount,
			"progress_percentage":   conv.GetProgress(),
			"created_at":            conv.CreatedAt,
			"updated_at":            conv.UpdatedAt,
			"completed_at":          conv.CompletedAt,
			"is_complete":           conv.IsComplete(),
			"successful_tracks":     successfulTracks,
			"failed_tracks":         failedTracks,
			"all_tracks":            conv.ConversionLog,
		}
	}

	// Calculate pagination metadata
	hasMore := offset+len(conversions) < totalCount
	nextOffset := offset + len(conversions)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"total":       totalCount,
		"limit":       limit,
		"offset":      offset,
		"count":       len(conversions),
		"has_more":    hasMore,
		"next_offset": nextOffset,
		"conversions": detailedConversions,
	})
}

// DeleteConversion soft deletes a playlist conversion (keeps record for analytics)
func (h *PlaylistHandler) DeleteConversion(c echo.Context) error {
	conversionID := c.Param("id")
	if conversionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Conversion ID required")
	}

	// Get authenticated user
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Get user from database
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Find the conversion and verify ownership
	var conversion models.PlaylistConversion
	err = h.db.NewSelect().
		Model(&conversion).
		Where("id = ? AND user_id = ?", conversionID, user.ID).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Conversion not found")
	}

	// Check if already deleted
	if conversion.IsDeleted() {
		return echo.NewHTTPError(http.StatusBadRequest, "Conversion already deleted")
	}

	// Soft delete - set deleted_at timestamp
	now := time.Now()
	_, err = h.db.NewUpdate().
		Model(&conversion).
		Set("deleted_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", conversion.ID).
		Exec(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to delete conversion")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Playlist removed from your list",
	})
}

// RetryFailedTracksRequest is the request body for retrying failed tracks
type RetryFailedTracksRequest struct {
	TrackIDs       []string `json:"track_ids"`        // Optional: specific track IDs to retry. If empty, retries all failed tracks
	UseLyricVideos bool     `json:"use_lyric_videos"` // Whether to search for lyric videos
}

// RetryFailedTracks retries adding failed tracks to an existing YouTube playlist
func (h *PlaylistHandler) RetryFailedTracks(c echo.Context) error {
	conversionID := c.Param("id")
	if conversionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Conversion ID required")
	}

	// Get authenticated user
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Get user from database
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Get the conversion
	var conversion models.PlaylistConversion
	err = h.db.NewSelect().
		Model(&conversion).
		Where("id = ? AND user_id = ?", conversionID, user.ID).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Conversion not found")
	}

	// Check if conversion has a YouTube playlist
	if conversion.YouTubePlaylistID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "no_youtube_playlist",
			"message": "This conversion doesn't have a YouTube playlist yet. The original conversion may have failed before creating the playlist.",
		})
	}

	// Get user's YouTube token
	var youtubeToken models.UserOAuthToken
	err = h.db.NewSelect().
		Model(&youtubeToken).
		Where("user_id = ? AND provider = ?", user.ID, "youtube").
		Scan(ctx)

	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "youtube_not_connected",
			"message": "Please connect your YouTube account to retry failed tracks.",
		})
	}

	if err := youtubeToken.DecryptTokens(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "token_error",
			"message": "Failed to read YouTube token",
		})
	}

	// Verify same YouTube account is connected (if we know the original account)
	if conversion.GoogleAccountEmail != "" && youtubeToken.AccountEmail != conversion.GoogleAccountEmail {
		return c.JSON(http.StatusForbidden, map[string]interface{}{
			"success":          false,
			"error":            "wrong_youtube_account",
			"message":          fmt.Sprintf("Please connect the YouTube account '%s' that was used to create this playlist.", conversion.GoogleAccountEmail),
			"required_account": conversion.GoogleAccountEmail,
			"current_account":  youtubeToken.AccountEmail,
		})
	}

	// Check YouTube quota before retry
	var accountQuota models.YouTubeAccountQuota
	quotaErr := h.db.NewSelect().
		Model(&accountQuota).
		Where("account_email = ?", youtubeToken.AccountEmail).
		Scan(ctx)

	if quotaErr == nil && !accountQuota.NeedsQuotaReset() {
		// Check if there's enough quota for at least one search
		if !accountQuota.CanAffordSearch() {
			return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
				"success":         false,
				"error":           "quota_exceeded",
				"message":         "YouTube API limit reached for today. Please try again tomorrow.",
				"quota_used":      accountQuota.GetDailyQuotaUsed(),
				"quota_limit":     models.YouTubeQuotaDailyLimit,
				"quota_remaining": 0,
				"resets_at":       "midnight Pacific Time",
			})
		}
	}

	// Refresh token if needed
	if time.Until(youtubeToken.ExpiresAt) < 5*time.Minute {
		refreshedToken, err := h.refreshYouTubeToken(ctx, &youtubeToken)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]interface{}{
				"success": false,
				"error":   "youtube_token_expired",
				"message": "Your YouTube session has expired. Please reconnect your YouTube account.",
			})
		}
		youtubeToken = *refreshedToken
	}

	// Parse request body
	var req RetryFailedTracksRequest
	if err := c.Bind(&req); err != nil {
		// Empty body is OK - means retry all failed tracks
		req.TrackIDs = nil
	}

	// Get failed tracks from conversion log
	var failedTracks []models.TrackConversionLog
	trackIDSet := make(map[string]bool)
	for _, id := range req.TrackIDs {
		trackIDSet[id] = true
	}

	for _, track := range conversion.ConversionLog {
		if track.Status != "success" {
			// If specific track IDs provided, only include those
			if len(req.TrackIDs) > 0 {
				if trackIDSet[track.SpotifyTrackID] {
					failedTracks = append(failedTracks, track)
				}
			} else {
				failedTracks = append(failedTracks, track)
			}
		}
	}

	if len(failedTracks) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "no_failed_tracks",
			"message": "All tracks have already been successfully added to the playlist!",
		})
	}

	// Create retry task payload
	payload := tasks.RetryFailedTracksPayload{
		ConversionID:       conversion.ID.String(),
		UserID:             user.ID.String(),
		ClerkUserID:        clerkUserID,
		YouTubePlaylistID:  conversion.YouTubePlaylistID,
		YouTubeAccessToken: youtubeToken.AccessToken,
		GoogleAccountEmail: youtubeToken.AccountEmail,
		FailedTracks:       failedTracks,
		UseLyricVideos:     req.UseLyricVideos,
	}

	// Enqueue retry task
	err = h.taskClient.EnqueueRetryFailedTracks(payload)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "retry_enqueue_failed",
			"message": "Failed to start retry. Please try again.",
		})
	}

	// Build track names for response
	trackNames := make([]string, 0, len(failedTracks))
	for _, track := range failedTracks {
		trackNames = append(trackNames, fmt.Sprintf("%s - %s", track.SpotifyTrackName, track.SpotifyArtists))
	}

	return c.JSON(http.StatusAccepted, map[string]interface{}{
		"success":         true,
		"message":         fmt.Sprintf("Retrying %d failed track(s). You'll be notified when complete.", len(failedTracks)),
		"conversion_id":   conversion.ID.String(),
		"tracks_to_retry": len(failedTracks),
		"track_names":     trackNames,
	})
}

// CancelRetry cancels an in-progress retry operation
func (h *PlaylistHandler) CancelRetry(c echo.Context) error {
	conversionID := c.Param("id")
	if conversionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   "missing_conversion_id",
			"message": "Conversion ID is required.",
		})
	}

	// Get authenticated user
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Get user from database
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Verify ownership of the conversion
	var conversion models.PlaylistConversion
	err = h.db.NewSelect().
		Model(&conversion).
		Where("id = ? AND user_id = ?", conversionID, user.ID).
		Scan(ctx)

	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]interface{}{
			"success": false,
			"error":   "conversion_not_found",
			"message": "Conversion not found.",
		})
	}

	// Check if cache is available
	if h.cache == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]interface{}{
			"success": false,
			"error":   "service_unavailable",
			"message": "Cancel feature is temporarily unavailable.",
		})
	}

	// Set cancel flag in Redis
	err = h.cache.SetConversionCancel(ctx, conversionID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]interface{}{
			"success": false,
			"error":   "cancel_failed",
			"message": "Failed to cancel. Please try again.",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":       true,
		"message":       "Cancel request sent. The operation will stop after the current track.",
		"conversion_id": conversionID,
	})
}

// refreshYouTubeToken refreshes an expired YouTube OAuth token
func (h *PlaylistHandler) refreshYouTubeToken(ctx context.Context, token *models.UserOAuthToken) (*models.UserOAuthToken, error) {
	clientID := os.Getenv("YOUTUBE_OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("YOUTUBE_OAUTH_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("youtube oauth credentials not configured")
	}

	// Prepare token refresh request
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("refresh_token", token.RefreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed (status %d): %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	// Keep plaintext for in-memory use
	token.AccessToken = tokenResp.AccessToken
	token.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	token.UpdatedAt = time.Now()

	// Encrypt before persisting to DB
	encAccessToken, encErr := crypto.Encrypt(tokenResp.AccessToken)
	if encErr != nil {
		return nil, fmt.Errorf("failed to encrypt token: %w", encErr)
	}

	_, err = h.db.NewUpdate().
		Model((*models.UserOAuthToken)(nil)).
		Set("access_token = ?", encAccessToken).
		Set("expires_at = ?", token.ExpiresAt).
		Set("updated_at = ?", token.UpdatedAt).
		Where("id = ?", token.ID).
		Exec(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to update token in database: %w", err)
	}

	return token, nil
}

// incrementMonthlyCounter immediately increments the monthly conversion counter for a user
func (h *PlaylistHandler) incrementMonthlyCounter(ctx context.Context, userID uuid.UUID) error {
	now := time.Now()
	currentMonth := int(now.Month())
	currentYear := now.Year()

	// Try to get existing analytics record
	var analytics models.UserAnalytics
	err := h.db.NewSelect().
		Model(&analytics).
		Where("user_id = ?", userID).
		Scan(ctx)

	if err != nil {
		// Record doesn't exist, create it with counter = 1
		analytics = models.UserAnalytics{
			UserID:             userID,
			MonthlyConversions: 1,
			CurrentMonth:       currentMonth,
			CurrentYear:        currentYear,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		_, err = h.db.NewInsert().Model(&analytics).Exec(ctx)
		return err
	}

	// Record exists, update it
	update := h.db.NewUpdate().
		Model(&analytics).
		Set("updated_at = ?", now)

	// Check if we need to reset monthly counter (new month)
	if analytics.CurrentMonth != currentMonth || analytics.CurrentYear != currentYear {
		// New month, reset counter to 1
		update = update.Set("monthly_conversions = 1").
			Set("current_month = ?", currentMonth).
			Set("current_year = ?", currentYear)
	} else {
		// Same month, increment
		update = update.Set("monthly_conversions = monthly_conversions + 1")
	}

	_, err = update.Where("user_id = ?", userID).Exec(ctx)
	return err
}
