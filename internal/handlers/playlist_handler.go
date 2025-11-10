package handlers

import (
	"context"
	"log"
	"net/http"
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
