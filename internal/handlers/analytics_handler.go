package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/pixperk/sptyt/internal/database"

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

	ctx, cancel := database.NewQueryContext()
	defer cancel()

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
			"total_conversions":      0,
			"successful_conversions": 0,
			"failed_conversions":     0,
			"playlists_converted":    0,
			"albums_converted":       0,
			"total_tracks_processed": 0,
			"total_tracks_matched":   0,
			"total_tracks_failed":    0,
			"total_custom_links":     0,
			"monthly_conversions":    0,
			"success_rate":           0,
			"track_match_rate":       0,
			"first_conversion_at":    nil,
			"last_conversion_at":     nil,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"total_conversions":      analytics.TotalConversions,
		"successful_conversions": analytics.SuccessfulConversions,
		"monthly_conversions":    analytics.MonthlyConversions,
		"failed_conversions":     analytics.FailedConversions,
		"playlists_converted":    analytics.PlaylistsConverted,
		"albums_converted":       analytics.AlbumsConverted,
		"total_tracks_processed": analytics.TotalTracksProcessed,
		"total_tracks_matched":   analytics.TotalTracksMatched,
		"total_tracks_failed":    analytics.TotalTracksFailed,
		"total_custom_links":     analytics.TotalCustomLinks,
		"success_rate":           analytics.GetSuccessRate(),
		"track_match_rate":       analytics.GetTrackMatchRate(),
		"first_conversion_at":    analytics.FirstConversionAt,
		"last_conversion_at":     analytics.LastConversionAt,
	})
}

// GetUserDashboard returns comprehensive dashboard data including analytics and recent conversions
func (h *AnalyticsHandler) GetUserDashboard(c echo.Context) error {
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
			"total_conversions":      0,
			"successful_conversions": 0,
			"failed_conversions":     0,
			"playlists_converted":    0,
			"albums_converted":       0,
			"total_tracks_processed": 0,
			"total_tracks_matched":   0,
			"total_tracks_failed":    0,
			"total_custom_links":     0,
			"success_rate":           0.0,
			"track_match_rate":       0.0,
			"first_conversion_at":    nil,
			"last_conversion_at":     nil,
		},
		"recent_conversions": recentConversions,
	}

	if hasAnalytics {
		response["analytics"] = map[string]interface{}{
			"total_conversions":      analytics.TotalConversions,
			"successful_conversions": analytics.SuccessfulConversions,
			"failed_conversions":     analytics.FailedConversions,
			"playlists_converted":    analytics.PlaylistsConverted,
			"albums_converted":       analytics.AlbumsConverted,
			"total_tracks_processed": analytics.TotalTracksProcessed,
			"total_tracks_matched":   analytics.TotalTracksMatched,
			"total_tracks_failed":    analytics.TotalTracksFailed,
			"total_custom_links":     analytics.TotalCustomLinks,
			"success_rate":           analytics.GetSuccessRate(),
			"track_match_rate":       analytics.GetTrackMatchRate(),
			"first_conversion_at":    analytics.FirstConversionAt,
			"last_conversion_at":     analytics.LastConversionAt,
		}
	}

	return c.JSON(http.StatusOK, response)
}

// GetMonthlyStats returns playlist statistics for the current month
func (h *AnalyticsHandler) GetMonthlyStats(c echo.Context) error {
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
		log.Printf("GetMonthlyStats: Failed to get user: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Get user analytics
	var analytics models.UserAnalytics
	err = h.db.NewSelect().
		Model(&analytics).
		Where("user_id = ?", user.ID).
		Scan(ctx)

	now := time.Now()
	currentMonth := int(now.Month())
	currentYear := now.Year()

	// Determine user limits
	var maxPlaylists, maxSongs int
	if user.IsPremium() {
		maxPlaylists = 20
		maxSongs = 100
	} else {
		maxPlaylists = 1
		maxSongs = 10
	}

	// Check if analytics exist and are for current month
	var monthlyConversions int
	if err == nil && analytics.CurrentMonth == currentMonth && analytics.CurrentYear == currentYear {
		monthlyConversions = analytics.MonthlyConversions
	}

	// Get this month's conversions for detailed stats
	startOfMonth := time.Date(currentYear, time.Month(currentMonth), 1, 0, 0, 0, 0, time.UTC)

	var monthlyDetails struct {
		TotalTracks   int
		MatchedTracks int
		FailedTracks  int
	}

	err = h.db.NewSelect().
		Model((*models.PlaylistConversion)(nil)).
		ColumnExpr("COALESCE(SUM(track_count), 0) as total_tracks").
		ColumnExpr("COALESCE(SUM(success_count), 0) as matched_tracks").
		ColumnExpr("COALESCE(SUM(failure_count), 0) as failed_tracks").
		Where("user_id = ?", user.ID).
		Where("created_at >= ?", startOfMonth).
		Scan(ctx, &monthlyDetails)

	if err != nil {
		log.Printf("GetMonthlyStats: Failed to get monthly details: %v", err)
	}

	// Build YouTube quota info
	// Check if we need to reset daily quota counters
	var dailySearches, dailyInserts, quotaUsed, quotaRemaining int
	var quotaPercentage float64
	quotaResetNeeded := true

	if err == nil {
		quotaResetNeeded = analytics.NeedsQuotaReset()
		if !quotaResetNeeded {
			dailySearches = analytics.DailyYouTubeSearches
			dailyInserts = analytics.DailyPlaylistInserts
			quotaUsed = analytics.GetDailyQuotaUsed()
			quotaRemaining = analytics.GetDailyQuotaRemaining()
			quotaPercentage = analytics.GetDailyQuotaPercentage()
		} else {
			// Quota was reset, show fresh values
			quotaRemaining = models.YouTubeQuotaDailyLimit
		}
	} else {
		quotaRemaining = models.YouTubeQuotaDailyLimit
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"month":             now.Format("January 2006"),
		"month_num":         currentMonth,
		"year":              currentYear,
		"subscription_tier": user.SubscriptionTier,
		"is_premium":        user.IsPremium(),
		"limits": map[string]interface{}{
			"max_playlists_per_month": maxPlaylists,
			"max_songs_per_playlist":  maxSongs,
		},
		"usage": map[string]interface{}{
			"playlists_converted": monthlyConversions,
			"playlists_remaining": maxPlaylists - monthlyConversions,
			"usage_percentage":    float64(monthlyConversions) / float64(maxPlaylists) * 100,
			"total_tracks":        monthlyDetails.TotalTracks,
			"matched_tracks":      monthlyDetails.MatchedTracks,
			"failed_tracks":       monthlyDetails.FailedTracks,
		},
		"youtube_quota": map[string]interface{}{
			"daily_searches":     dailySearches,
			"daily_inserts":      dailyInserts,
			"quota_used":         quotaUsed,
			"quota_remaining":    quotaRemaining,
			"quota_limit":        models.YouTubeQuotaDailyLimit,
			"usage_percentage":   quotaPercentage,
			"is_user_quota":      true, // Uses user's YouTube OAuth token quota
			"resets_at":          "midnight Pacific Time",
			"cost_per_search":    models.YouTubeQuotaCostSearch,
			"cost_per_insert":    models.YouTubeQuotaCostPlaylistInsert,
		},
		"can_convert": monthlyConversions < maxPlaylists,
	})
}
