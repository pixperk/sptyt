package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/auth"
	"github.com/pixperk/sptyt/internal/cache"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/uptrace/bun"
)

type PlaylistLimits struct {
	MaxPlaylistsPerMonth int
	MaxSongsPerPlaylist  int
}

var (
	FreeTierLimits = PlaylistLimits{
		MaxPlaylistsPerMonth: 1,
		MaxSongsPerPlaylist:  10,
	}

	PremiumTierLimits = PlaylistLimits{
		MaxPlaylistsPerMonth: 20,
		MaxSongsPerPlaylist:  100,
	}
)

type PlaylistLimiter struct {
	db    *bun.DB
	cache *cache.RedisCache
}

func NewPlaylistLimiter(db *bun.DB, redisCache *cache.RedisCache) *PlaylistLimiter {
	return &PlaylistLimiter{
		db:    db,
		cache: redisCache,
	}
}

// CheckPlaylistConversionLimits middleware checks if user has reached their conversion limits
func (pl *PlaylistLimiter) CheckPlaylistConversionLimits() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get authenticated user
			clerkUserID, ok := auth.GetClerkUserID(c)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
			}

			ctx := context.Background()

			// Get user from database
			var user models.User
			err := pl.db.NewSelect().
				Model(&user).
				Where("clerk_id = ?", clerkUserID).
				Scan(ctx)

			if err != nil {
				log.Printf("PlaylistLimiter: Failed to get user: %v", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
			}

			// Determine limits based on subscription tier
			var limits PlaylistLimits
			if user.IsPremium() {
				limits = PremiumTierLimits
			} else {
				limits = FreeTierLimits
			}

			// Check monthly conversion count
			count, err := pl.getMonthlyConversionCount(ctx, user.ID)
			if err != nil {
				log.Printf("PlaylistLimiter: Failed to get conversion count: %v", err)
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check limits")
			}

			if count >= limits.MaxPlaylistsPerMonth {
				return echo.NewHTTPError(http.StatusTooManyRequests, map[string]interface{}{
					"error":                     "Monthly conversion limit reached",
					"limit":                     limits.MaxPlaylistsPerMonth,
					"current_count":             count,
					"max_songs_per_playlist":    limits.MaxSongsPerPlaylist,
					"upgrade_required":          !user.IsPremium(),
					"subscription_tier":         user.SubscriptionTier,
				})
			}

			// Store limits in context for handler to use
			c.Set("playlist_limits", limits)
			c.Set("current_user", &user)
			c.Set("monthly_conversions_used", count)

			return next(c)
		}
	}
}

// getMonthlyConversionCount gets the number of conversions this month for a user from analytics
func (pl *PlaylistLimiter) getMonthlyConversionCount(ctx context.Context, userID interface{}) (int, error) {
	now := time.Now()
	currentMonth := int(now.Month())
	currentYear := now.Year()

	// Try to get from cache first
	cacheKey := fmt.Sprintf("monthly_conversions:%v:%d-%d", userID, currentYear, currentMonth)
	if cached, err := pl.cache.Get(ctx, cacheKey); err == nil {
		var count int
		fmt.Sscanf(cached, "%d", &count)
		return count, nil
	}

	// Get from analytics table
	var analytics models.UserAnalytics
	err := pl.db.NewSelect().
		Model(&analytics).
		Where("user_id = ?", userID).
		Scan(ctx)

	if err != nil {
		// No analytics record yet, return 0
		return 0, nil
	}

	// Check if the analytics record is for the current month
	var count int
	if analytics.CurrentMonth == currentMonth && analytics.CurrentYear == currentYear {
		count = analytics.MonthlyConversions
	} else {
		// Old month data, effectively 0 for this month
		count = 0
	}

	// Cache for 5 minutes (short cache since this changes frequently)
	pl.cache.Set(ctx, cacheKey, fmt.Sprintf("%d", count), 5*time.Minute)

	return count, nil
}

// IncrementMonthlyConversionCount invalidates the cache for monthly conversion count
func (pl *PlaylistLimiter) IncrementMonthlyConversionCount(ctx context.Context, userID interface{}) error {
	now := time.Now()
	currentMonth := int(now.Month())
	currentYear := now.Year()
	cacheKey := fmt.Sprintf("monthly_conversions:%v:%d-%d", userID, currentYear, currentMonth)

	// Invalidate cache
	return pl.cache.Delete(ctx, cacheKey)
}

// ValidatePlaylistSize validates that the playlist doesn't exceed user's limits
func ValidatePlaylistSize(c echo.Context, trackCount int) error {
	limits, ok := c.Get("playlist_limits").(PlaylistLimits)
	if !ok {
		// Default to free tier if limits not set
		limits = FreeTierLimits
	}

	if trackCount > limits.MaxSongsPerPlaylist {
		user, _ := c.Get("current_user").(*models.User)
		isPremium := user != nil && user.IsPremium()

		return echo.NewHTTPError(http.StatusBadRequest, map[string]interface{}{
			"error":                  "Playlist exceeds maximum track limit",
			"track_count":            trackCount,
			"max_tracks_allowed":     limits.MaxSongsPerPlaylist,
			"upgrade_required":       !isPremium,
			"premium_max_tracks":     PremiumTierLimits.MaxSongsPerPlaylist,
			"subscription_tier":      user.SubscriptionTier,
		})
	}

	return nil
}

// GetUserLimitsInfo returns information about user's limits and usage
func (pl *PlaylistLimiter) GetUserLimitsInfo(c echo.Context) error {
	// Get authenticated user
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx := context.Background()

	// Get user from database
	var user models.User
	err := pl.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Determine limits
	var limits PlaylistLimits
	if user.IsPremium() {
		limits = PremiumTierLimits
	} else {
		limits = FreeTierLimits
	}

	// Get current usage
	count, err := pl.getMonthlyConversionCount(ctx, user.ID)
	if err != nil {
		log.Printf("GetUserLimitsInfo: Failed to get conversion count: %v", err)
		count = 0
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"subscription_tier":         user.SubscriptionTier,
		"is_premium":                user.IsPremium(),
		"max_playlists_per_month":   limits.MaxPlaylistsPerMonth,
		"max_songs_per_playlist":    limits.MaxSongsPerPlaylist,
		"playlists_used_this_month": count,
		"playlists_remaining":       limits.MaxPlaylistsPerMonth - count,
		"free_tier_limits": map[string]int{
			"max_playlists_per_month": FreeTierLimits.MaxPlaylistsPerMonth,
			"max_songs_per_playlist":  FreeTierLimits.MaxSongsPerPlaylist,
		},
		"premium_tier_limits": map[string]int{
			"max_playlists_per_month": PremiumTierLimits.MaxPlaylistsPerMonth,
			"max_songs_per_playlist":  PremiumTierLimits.MaxSongsPerPlaylist,
		},
	})
}
