package handlers

import (
	"context"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/auth"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/uptrace/bun"
)

type AnalyticsHandler struct {
	db *bun.DB
}

func NewAnalyticsHandler(db *bun.DB) *AnalyticsHandler {
	return &AnalyticsHandler{
		db: db,
	}
}

// GetUserAnalytics returns analytics stats for the authenticated user
func (h *AnalyticsHandler) GetUserAnalytics(c echo.Context) error {
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
		log.Printf("GetUserAnalytics: Failed to get user: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Get user analytics
	var analytics models.UserAnalytics
	err = h.db.NewSelect().
		Model(&analytics).
		Where("user_id = ?", user.ID).
		Scan(ctx)

	if err != nil {
		// No analytics record yet, return zeros
		return c.JSON(http.StatusOK, map[string]interface{}{
			"total_conversions":       0,
			"successful_conversions":  0,
			"failed_conversions":      0,
			"playlists_converted":     0,
			"albums_converted":        0,
			"total_tracks_processed":  0,
			"total_tracks_matched":    0,
			"total_tracks_failed":     0,
			"total_custom_links":      0,
			"success_rate":            0,
			"track_match_rate":        0,
			"first_conversion_at":     nil,
			"last_conversion_at":      nil,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"total_conversions":       analytics.TotalConversions,
		"successful_conversions":  analytics.SuccessfulConversions,
		"failed_conversions":      analytics.FailedConversions,
		"playlists_converted":     analytics.PlaylistsConverted,
		"albums_converted":        analytics.AlbumsConverted,
		"total_tracks_processed":  analytics.TotalTracksProcessed,
		"total_tracks_matched":    analytics.TotalTracksMatched,
		"total_tracks_failed":     analytics.TotalTracksFailed,
		"total_custom_links":      analytics.TotalCustomLinks,
		"success_rate":            analytics.GetSuccessRate(),
		"track_match_rate":        analytics.GetTrackMatchRate(),
		"first_conversion_at":     analytics.FirstConversionAt,
		"last_conversion_at":      analytics.LastConversionAt,
	})
}

// GetUserDashboard returns comprehensive dashboard data including analytics and recent conversions
func (h *AnalyticsHandler) GetUserDashboard(c echo.Context) error {
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
		log.Printf("GetUserDashboard: Failed to get user: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Get user analytics
	var analytics models.UserAnalytics
	err = h.db.NewSelect().
		Model(&analytics).
		Where("user_id = ?", user.ID).
		Scan(ctx)

	hasAnalytics := err == nil

	// Get recent conversions
	var recentConversions []models.PlaylistConversion
	err = h.db.NewSelect().
		Model(&recentConversions).
		Where("user_id = ?", user.ID).
		Order("created_at DESC").
		Limit(10).
		Scan(ctx)

	if err != nil {
		log.Printf("GetUserDashboard: Failed to get recent conversions: %v", err)
		recentConversions = []models.PlaylistConversion{}
	}

	// Build response
	response := map[string]interface{}{
		"analytics": map[string]interface{}{
			"total_conversions":       0,
			"successful_conversions":  0,
			"failed_conversions":      0,
			"playlists_converted":     0,
			"albums_converted":        0,
			"total_tracks_processed":  0,
			"total_tracks_matched":    0,
			"total_tracks_failed":     0,
			"total_custom_links":      0,
			"success_rate":            0.0,
			"track_match_rate":        0.0,
			"first_conversion_at":     nil,
			"last_conversion_at":      nil,
		},
		"recent_conversions": recentConversions,
	}

	if hasAnalytics {
		response["analytics"] = map[string]interface{}{
			"total_conversions":       analytics.TotalConversions,
			"successful_conversions":  analytics.SuccessfulConversions,
			"failed_conversions":      analytics.FailedConversions,
			"playlists_converted":     analytics.PlaylistsConverted,
			"albums_converted":        analytics.AlbumsConverted,
			"total_tracks_processed":  analytics.TotalTracksProcessed,
			"total_tracks_matched":    analytics.TotalTracksMatched,
			"total_tracks_failed":     analytics.TotalTracksFailed,
			"total_custom_links":      analytics.TotalCustomLinks,
			"success_rate":            analytics.GetSuccessRate(),
			"track_match_rate":        analytics.GetTrackMatchRate(),
			"first_conversion_at":     analytics.FirstConversionAt,
			"last_conversion_at":      analytics.LastConversionAt,
		}
	}

	return c.JSON(http.StatusOK, response)
}
