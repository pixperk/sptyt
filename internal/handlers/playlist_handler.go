package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/auth"
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
}

func NewPlaylistHandler(db *bun.DB, converterService *services.PlaylistConverterService, taskClient *tasks.Client) *PlaylistHandler {
	return &PlaylistHandler{
		db:               db,
		converterService: converterService,
		taskClient:       taskClient,
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

	ctx := context.Background()

	// Get user from database
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		log.Printf("ConvertPlaylist: Failed to get user: %v", err)
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

	// Refresh token if expired or about to expire (within 5 minutes)
	if time.Until(youtubeToken.ExpiresAt) < 5*time.Minute {
		log.Printf("ConvertPlaylist: Refreshing YouTube token for user %s (expires in %v)", user.ID, time.Until(youtubeToken.ExpiresAt))
		refreshedToken, err := h.refreshYouTubeToken(ctx, &youtubeToken)
		if err != nil {
			log.Printf("ConvertPlaylist: Failed to refresh YouTube token: %v", err)
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

	log.Printf("ConvertPlaylist: User %s converting %s %s", user.ID, spotifyType, spotifyID)

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
		log.Printf("ConvertPlaylist: Failed to create conversion record: %v", err)
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
	}

	// Enqueue task with Asynq
	err = h.taskClient.EnqueuePlaylistConversion(payload)
	if err != nil {
		log.Printf("ConvertPlaylist: Failed to enqueue task: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to start conversion")
	}

	log.Printf("ConvertPlaylist: Enqueued conversion task %s for user %s", conversion.ID, user.ID)

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

	ctx := context.Background()

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

	ctx := context.Background()

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
		log.Printf("GetUserConversions: Failed to get conversions: %v", err)
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

	ctx := context.Background()

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

	// Get total count
	totalCount, err := h.db.NewSelect().
		Model((*models.PlaylistConversion)(nil)).
		Where("user_id = ?", user.ID).
		Count(ctx)

	if err != nil {
		log.Printf("GetDetailedUserConversions: Failed to count conversions: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get conversions")
	}

	// Get conversions with pagination
	var conversions []models.PlaylistConversion
	err = h.db.NewSelect().
		Model(&conversions).
		Where("user_id = ?", user.ID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)

	if err != nil {
		log.Printf("GetDetailedUserConversions: Failed to get conversions: %v", err)
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

	resp, err := http.Post("https://oauth2.googleapis.com/token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
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

	// Update token in database
	token.AccessToken = tokenResp.AccessToken
	token.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	token.UpdatedAt = time.Now()

	_, err = h.db.NewUpdate().
		Model(token).
		Column("access_token", "expires_at", "updated_at").
		Where("id = ?", token.ID).
		Exec(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to update token in database: %w", err)
	}

	log.Printf("refreshYouTubeToken: Successfully refreshed token for user %s (new expiry: %v)", token.UserID, token.ExpiresAt)
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
