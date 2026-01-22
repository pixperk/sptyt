package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/auth"
	"github.com/pixperk/sptyt/internal/cache"
	"github.com/pixperk/sptyt/internal/database"
	"github.com/pixperk/sptyt/internal/genius"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/pixperk/sptyt/internal/services"
	"github.com/pixperk/sptyt/internal/spotify"
	"github.com/pixperk/sptyt/internal/youtube"
	"github.com/pixperk/sptyt/pkg/errors"
	"github.com/uptrace/bun"
)

// SpotifySearchCacheTTL is the TTL for cached Spotify search results (1 hour)
const SpotifySearchCacheTTL = 1 * time.Hour

type CustomLinkHandler struct {
	service       *services.CustomLinkService
	db            *bun.DB
	frontendURL   string
	spotifyClient *spotify.Client
	youtubeClient *youtube.Client
	geniusClient  *genius.Client
	cache         *cache.RedisCache
}

func NewCustomLinkHandler(service *services.CustomLinkService, db *bun.DB, spotifyClient *spotify.Client, youtubeClient *youtube.Client, geniusClient *genius.Client, redisCache *cache.RedisCache) *CustomLinkHandler {
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	return &CustomLinkHandler{
		service:       service,
		db:            db,
		frontendURL:   frontendURL,
		spotifyClient: spotifyClient,
		youtubeClient: youtubeClient,
		geniusClient:  geniusClient,
		cache:         redisCache,
	}
}

// CreateCustomLink creates a new custom link
func (h *CustomLinkHandler) CreateCustomLink(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Get user
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	var req services.CreateLinkRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	link, err := h.service.CreateCustomLink(ctx, user.ID, req)
	if err != nil {
		log.Printf("CreateCustomLink: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"link":       link,
		"public_url": fmt.Sprintf("%s/l/%s", h.frontendURL, link.Slug),
	})
}

// GetUserLinks returns all custom links for the authenticated user
func (h *CustomLinkHandler) GetUserLinks(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Parse pagination
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	offset, _ := strconv.Atoi(c.QueryParam("offset"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	links, total, err := h.service.GetUserLinks(ctx, user.ID, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get links")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"links":  links,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetCustomLink returns a specific custom link by ID (for owner)
func (h *CustomLinkHandler) GetCustomLink(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	link, err := h.service.GetLinkByID(ctx, linkID, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Link not found")
	}

	return c.JSON(http.StatusOK, link)
}

// UpdateCustomLink updates a custom link
func (h *CustomLinkHandler) UpdateCustomLink(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return errors.ToHTTPError(errors.Unauthorized("User not authenticated"))
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return errors.ToHTTPError(errors.Validation("Invalid link ID"))
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return errors.ToHTTPError(errors.Database(err).WithDetails("Failed to get user"))
	}

	var req services.UpdateLinkRequest
	if err := c.Bind(&req); err != nil {
		return errors.ToHTTPError(errors.Validation("Invalid request body"))
	}

	err = h.service.UpdateCustomLink(ctx, linkID, user.ID, req)
	if err != nil {
		return errors.ToHTTPError(err)
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Link updated successfully"})
}

// DeleteCustomLink deletes a custom link
func (h *CustomLinkHandler) DeleteCustomLink(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	err = h.service.DeleteCustomLink(ctx, linkID, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Link deleted successfully"})
}

// AddElement adds an element to a custom link
func (h *CustomLinkHandler) AddElement(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	var req services.AddElementRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	element, err := h.service.AddElement(ctx, linkID, user.ID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusCreated, element)
}

// UpdateElement updates an existing element in a custom link
func (h *CustomLinkHandler) UpdateElement(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	elementID, err := uuid.Parse(c.Param("element_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid element ID")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	var req services.UpdateElementRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	err = h.service.UpdateElement(ctx, linkID, user.ID, elementID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Element updated successfully"})
}

// BatchUpdateElements updates multiple elements at once
func (h *CustomLinkHandler) BatchUpdateElements(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	var req services.BatchUpdateElementRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	if len(req.Updates) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "No updates provided")
	}

	err = h.service.BatchUpdateElements(ctx, linkID, user.ID, req.Updates)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": fmt.Sprintf("%d elements updated successfully", len(req.Updates)),
	})
}

// ReorderElements updates element display order
func (h *CustomLinkHandler) ReorderElements(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	var order []services.ElementOrder
	if err := c.Bind(&order); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	err = h.service.ReorderElements(ctx, linkID, user.ID, order)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Elements reordered successfully"})
}

// DeleteElement removes an element from a custom link
func (h *CustomLinkHandler) DeleteElement(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	elementID, err := uuid.Parse(c.Param("element_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid element ID")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	err = h.service.DeleteElement(ctx, linkID, user.ID, elementID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Element deleted successfully"})
}

// GetLinkBySlugPublic returns a custom link by slug (public access, for API calls)
func (h *CustomLinkHandler) GetLinkBySlugPublic(c echo.Context) error {
	slug := c.Param("slug")
	ctx, cancel := database.NewQueryContext()
	defer cancel()

	link, err := h.service.GetLinkBySlug(ctx, slug)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Link not found")
	}

	// Check if link is expired
	if link.IsExpired() {
		return echo.NewHTTPError(http.StatusGone, "Link has expired")
	}

	// Check if link is public
	if !link.IsPublic {
		return echo.NewHTTPError(http.StatusNotFound, "Link not found")
	}

	// Track page view (async in production, but sync for now)
	ipAddress := c.RealIP()
	userAgent := c.Request().UserAgent()
	referrer := c.Request().Referer()

	go func() {
		bgCtx := context.Background()
		h.service.IncrementViewCount(bgCtx, link.ID)
		h.service.TrackPageView(bgCtx, link.ID, ipAddress, userAgent, referrer)
	}()

	return c.JSON(http.StatusOK, link)
}

// TrackElementClick tracks an element click and returns the target URL
func (h *CustomLinkHandler) TrackElementClick(c echo.Context) error {
	linkID, err := uuid.Parse(c.Param("link_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	elementID, err := uuid.Parse(c.Param("element_id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid element ID")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Get the element to retrieve the target URL
	var element models.LinkElement
	err = h.db.NewSelect().
		Model(&element).
		Where("id = ? AND custom_link_id = ?", elementID, linkID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Element not found")
	}

	// Track the click (async)
	ipAddress := c.RealIP()
	userAgent := c.Request().UserAgent()
	referrer := c.Request().Referer()

	go func() {
		bgCtx := context.Background()
		h.service.TrackElementClick(bgCtx, linkID, elementID, ipAddress, userAgent, referrer)
	}()

	// Determine target URL based on element type
	var targetURL string
	switch element.ElementType {
	case "spotify_track":
		targetURL = element.ElementData.SpotifyURL
	case "youtube_video":
		targetURL = element.ElementData.YouTubeURL
	case "genius_lyrics":
		targetURL = element.ElementData.GeniusURL
	default:
		return echo.NewHTTPError(http.StatusBadRequest, "Element has no clickable URL")
	}

	if targetURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Element has no target URL")
	}

	// Redirect to the target URL
	return c.Redirect(http.StatusFound, targetURL)
}

// GetLinkAnalytics returns analytics data for a custom link (owner only)
func (h *CustomLinkHandler) GetLinkAnalytics(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	linkID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid link ID")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	analytics, err := h.service.GetLinkAnalytics(ctx, linkID, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.JSON(http.StatusOK, analytics)
}

// VerifyLinkPassword verifies a password for a password-protected link
func (h *CustomLinkHandler) VerifyLinkPassword(c echo.Context) error {
	slug := c.Param("slug")

	var req struct {
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	isValid, err := h.service.VerifyPassword(ctx, slug, req.Password)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Link not found")
	}

	if !isValid {
		return echo.NewHTTPError(http.StatusUnauthorized, "Incorrect password")
	}

	return c.JSON(http.StatusOK, map[string]bool{"valid": true})
}

// GetSongElementData fetches element data for a song from Spotify and derives platform links
// Caches complete track details and derived URLs to reduce API calls
func (h *CustomLinkHandler) GetSongElementData(c echo.Context) error {
	_, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	spotifyURL := c.QueryParam("spotify_url")
	if spotifyURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "spotify_url is required")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Extract Spotify track ID from URL
	trackID := extractSpotifyTrackID(spotifyURL)
	if trackID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid Spotify URL")
	}

	var trackName string
	var trackArtists []string
	var trackCoverImage string
	var trackDuration int
	var trackSpotifyURL string
	cacheHit := false

	// Check cache for complete track details first
	if h.cache != nil {
		if cached, err := h.cache.GetTrackDetails(ctx, trackID); err == nil {
			trackName = cached.Name
			trackArtists = cached.Artists
			trackCoverImage = cached.CoverImage
			trackDuration = cached.Duration
			trackSpotifyURL = cached.SpotifyURL
			cacheHit = true
		}
	}

	// Fetch track details from Spotify only if not cached
	if !cacheHit {
		trackDetails, err := h.spotifyClient.GetTrackDetails(ctx, trackID)
		if err != nil {
			log.Printf("Failed to fetch track details: %v", err)
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch track details")
		}
		trackName = trackDetails.Name
		trackArtists = trackDetails.Artists
		trackCoverImage = trackDetails.CoverImage
		trackDuration = trackDetails.Duration
		trackSpotifyURL = trackDetails.SpotifyURL

		// Cache complete track details (24 hours)
		if h.cache != nil {
			_ = h.cache.SetTrackDetails(ctx, trackID, &cache.CachedTrackDetails{
				ID:         trackID,
				Name:       trackName,
				Artists:    trackArtists,
				CoverImage: trackCoverImage,
				Duration:   trackDuration,
				SpotifyURL: trackSpotifyURL,
			}, 24*time.Hour)
		}
	}

	// Get primary artist (first one)
	primaryArtist := ""
	if len(trackArtists) > 0 {
		primaryArtist = trackArtists[0]
	}

	// Check cache for YouTube MV URL
	var youtubeURL string
	if h.cache != nil {
		if cached, err := h.cache.GetYouTubeMVURL(ctx, trackID); err == nil {
			youtubeURL = cached
		}
	}
	if youtubeURL == "" {
		youtubeURL, _ = h.youtubeClient.SearchOfficialMV(ctx, trackName, primaryArtist)
		if h.cache != nil && youtubeURL != "" {
			_ = h.cache.SetYouTubeMVURL(ctx, trackID, youtubeURL, 24*time.Hour)
		}
	}

	// Check cache for YouTube Lyric URL
	var youtubeLyricURL string
	if h.cache != nil {
		if cached, err := h.cache.GetYouTubeLyricsURL(ctx, trackID); err == nil {
			youtubeLyricURL = cached
		}
	}
	if youtubeLyricURL == "" {
		youtubeLyricURL, _ = h.youtubeClient.SearchLyricVideo(ctx, trackName, primaryArtist)
		if h.cache != nil && youtubeLyricURL != "" {
			_ = h.cache.SetYouTubeLyricsURL(ctx, trackID, youtubeLyricURL, 24*time.Hour)
		}
	}

	// Check cache for Genius URL
	var geniusURL string
	if h.cache != nil {
		if cached, err := h.cache.GetGeniusURL(ctx, trackID); err == nil {
			geniusURL = cached
		}
	}
	if geniusURL == "" {
		geniusURL, _ = h.geniusClient.SearchLyrics(ctx, trackName, primaryArtist)
		if h.cache != nil && geniusURL != "" {
			_ = h.cache.SetGeniusURL(ctx, trackID, geniusURL, 24*time.Hour)
		}
	}

	// Format duration
	durationStr := formatDuration(trackDuration)

	elementData := models.ElementData{
		Title:            trackName,
		Artists:          strings.Join(trackArtists, ", "),
		CoverImage:       trackCoverImage,
		Duration:         durationStr,
		SpotifyURL:       trackSpotifyURL,
		YouTubeURL:       youtubeURL,
		YouTubeLyricURL:  youtubeLyricURL,
		GeniusURL:        geniusURL,
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"element_type": "song",
		"element_data": elementData,
	})
}

// Helper function to extract Spotify track ID from URL
func extractSpotifyTrackID(spotifyURL string) string {
	// Handle different Spotify URL formats
	// https://open.spotify.com/track/3n3Ppam7vgaVa1iaRUc9Lp
	// spotify:track:3n3Ppam7vgaVa1iaRUc9Lp
	if strings.Contains(spotifyURL, "open.spotify.com/track/") {
		parts := strings.Split(spotifyURL, "/track/")
		if len(parts) > 1 {
			trackID := strings.Split(parts[1], "?")[0]
			return trackID
		}
	} else if strings.HasPrefix(spotifyURL, "spotify:track:") {
		return strings.TrimPrefix(spotifyURL, "spotify:track:")
	}
	// If it's already just an ID
	if !strings.Contains(spotifyURL, "/") && !strings.Contains(spotifyURL, ":") {
		return spotifyURL
	}
	return ""
}

// Helper function to format duration from milliseconds to MM:SS
func formatDuration(ms int) string {
	totalSeconds := ms / 1000
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// GetConversionSongs returns all songs from a specific playlist conversion
func (h *CustomLinkHandler) GetConversionSongs(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	conversionIDStr := c.Param("conversion_id")
	conversionID, err := uuid.Parse(conversionIDStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid conversion ID")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Get user
	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Get conversion and verify ownership
	var conversion models.PlaylistConversion
	err = h.db.NewSelect().
		Model(&conversion).
		Where("id = ? AND user_id = ?", conversionID, user.ID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Conversion not found")
	}

	// Return conversion with all songs from the log
	return c.JSON(http.StatusOK, map[string]interface{}{
		"conversion_id":        conversion.ID,
		"playlist_name":        conversion.PlaylistName,
		"cover_image":          conversion.SpotifyCoverImage,
		"track_count":          conversion.TrackCount,
		"spotify_playlist_url": conversion.SpotifyPlaylistURL,
		"youtube_playlist_url": conversion.YouTubePlaylistURL,
		"songs":                conversion.ConversionLog,
	})
}

// GetConversionSongsPublic returns songs from a conversion (public endpoint)
// Used when viewing a public custom link that contains a playlist element
func (h *CustomLinkHandler) GetConversionSongsPublic(c echo.Context) error {
	conversionIDStr := c.Param("conversion_id")
	conversionID, err := uuid.Parse(conversionIDStr)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid conversion ID")
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Get conversion
	var conversion models.PlaylistConversion
	err = h.db.NewSelect().
		Model(&conversion).
		Where("id = ?", conversionID).
		Scan(ctx)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "Conversion not found")
	}

	// Check if this conversion is linked to any public custom links
	var customLink models.CustomLink
	err = h.db.NewSelect().
		Model(&customLink).
		Where("conversion_id = ?", conversionID).
		WhereOr("id IN (SELECT custom_link_id FROM link_elements WHERE element_data->>'conversion_id' = ?)", conversionID.String()).
		Scan(ctx)

	// If we can't find a custom link, check if there's a playlist element referencing this conversion
	if err != nil {
		// Check in link_elements for any element that references this conversion
		var element models.LinkElement
		err = h.db.NewSelect().
			Model(&element).
			Where("element_data->>'conversion_id' = ?", conversionID.String()).
			Relation("CustomLink").
			Scan(ctx)

		if err != nil || element.CustomLink == nil {
			return echo.NewHTTPError(http.StatusNotFound, "Conversion not found or not public")
		}

		// Check if the link is public
		if !element.CustomLink.IsPublic {
			return echo.NewHTTPError(http.StatusNotFound, "Conversion not found or not public")
		}
	} else {
		// Found a custom link directly, check if it's public
		if !customLink.IsPublic {
			return echo.NewHTTPError(http.StatusNotFound, "Conversion not found or not public")
		}
	}

	// Return conversion data
	return c.JSON(http.StatusOK, map[string]interface{}{
		"conversion_id":        conversion.ID,
		"playlist_name":        conversion.PlaylistName,
		"cover_image":          conversion.SpotifyCoverImage,
		"track_count":          conversion.TrackCount,
		"spotify_playlist_url": conversion.SpotifyPlaylistURL,
		"youtube_playlist_url": conversion.YouTubePlaylistURL,
		"songs":                conversion.ConversionLog,
	})
}

// SearchSpotifyTracks searches for tracks on Spotify with caching
func (h *CustomLinkHandler) SearchSpotifyTracks(c echo.Context) error {
	query := c.QueryParam("q")
	if query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Query parameter 'q' is required")
	}

	// Limit results (default 10, max 20)
	limit := 10
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 20 {
			limit = l
		}
	}

	ctx := c.Request().Context()

	// Check cache first
	if h.cache != nil {
		cached, err := h.cache.GetSpotifySearchResults(ctx, query, limit)
		if err == nil && cached != nil {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"results": cached,
				"cached":  true,
			})
		}
	}

	// Cache miss - search Spotify
	results, err := h.spotifyClient.SearchTracks(ctx, query, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to search Spotify")
	}

	// Cache the results
	if h.cache != nil && len(results) > 0 {
		// Convert to cache format
		cacheResults := make([]cache.SpotifySearchTrack, len(results))
		for i, r := range results {
			cacheResults[i] = cache.SpotifySearchTrack{
				ID:         r.ID,
				Name:       r.Name,
				Artists:    r.Artists,
				Album:      r.Album,
				CoverImage: r.CoverImage,
				Duration:   r.Duration,
				SpotifyURL: r.SpotifyURL,
			}
		}
		_ = h.cache.SetSpotifySearchResults(ctx, query, limit, cacheResults, SpotifySearchCacheTTL)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"results": results,
		"cached":  false,
	})
}
