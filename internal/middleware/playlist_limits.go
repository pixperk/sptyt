package middleware

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/auth"
	"github.com/pixperk/sptyt/internal/cache"
	"github.com/pixperk/sptyt/internal/database"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/pixperk/sptyt/pkg/errors"
	"github.com/uptrace/bun"
)

type PlaylistLimits struct {
	MaxPlaylistsPerMonth int
	MaxSongsPerPlaylist  int
}

// DefaultLimits - generous limits for all users
var DefaultLimits = PlaylistLimits{
	MaxPlaylistsPerMonth: 50,
	MaxSongsPerPlaylist:  100,
}

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
				return errors.ToHTTPError(errors.Unauthorized("User not authenticated"))
			}

			ctx, cancel := database.NewQueryContext()
			defer cancel()

			// Get user from database
			var user models.User
			err := pl.db.NewSelect().
				Model(&user).
				Where("clerk_id = ?", clerkUserID).
				Scan(ctx)

			if err != nil {
				log.Printf("PlaylistLimiter: Failed to get user: %v", err)
				return errors.ToHTTPError(errors.Database(err).WithDetails("Failed to get user"))
			}

			// Check monthly conversion count
			count, err := pl.getMonthlyConversionCount(ctx, user.ID)
			if err != nil {
				log.Printf("PlaylistLimiter: Failed to get conversion count: %v", err)
				return errors.ToHTTPError(errors.Database(err).WithDetails("Failed to check limits"))
			}

			if count >= DefaultLimits.MaxPlaylistsPerMonth {
				return errors.ToHTTPError(
					errors.QuotaExceeded("Monthly conversion limit reached").
						WithMeta("limit", DefaultLimits.MaxPlaylistsPerMonth).
						WithMeta("current_count", count).
						WithMeta("max_songs_per_playlist", DefaultLimits.MaxSongsPerPlaylist),
				)
			}

			// Store limits in context for handler to use
			c.Set("playlist_limits", DefaultLimits)
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

	// Get directly from analytics table (fast query with user_id unique index)
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
	if analytics.CurrentMonth == currentMonth && analytics.CurrentYear == currentYear {
		return analytics.MonthlyConversions, nil
	}

	// Old month data, effectively 0 for this month
	return 0, nil
}

// ValidatePlaylistSize validates that the playlist doesn't exceed track limit
func ValidatePlaylistSize(c echo.Context, trackCount int) error {
	limits, ok := c.Get("playlist_limits").(PlaylistLimits)
	if !ok {
		limits = DefaultLimits
	}

	if trackCount > limits.MaxSongsPerPlaylist {
		return errors.ToHTTPError(
			errors.New(errors.ErrCodeExceedsLimit, "Playlist exceeds maximum track limit").
				WithMeta("track_count", trackCount).
				WithMeta("max_tracks_allowed", limits.MaxSongsPerPlaylist),
		)
	}

	return nil
}

// GetUserLimitsInfo returns information about user's limits and usage
func (pl *PlaylistLimiter) GetUserLimitsInfo(c echo.Context) error {
	// Get authenticated user
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return errors.ToHTTPError(errors.Unauthorized("User not authenticated"))
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Get user from database
	var user models.User
	err := pl.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		return errors.ToHTTPError(errors.Database(err).WithDetails("Failed to get user"))
	}

	// Get current usage
	count, err := pl.getMonthlyConversionCount(ctx, user.ID)
	if err != nil {
		log.Printf("GetUserLimitsInfo: Failed to get conversion count: %v", err)
		count = 0
	}

	remaining := DefaultLimits.MaxPlaylistsPerMonth - count
	if remaining < 0 {
		remaining = 0
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"max_playlists_per_month":   DefaultLimits.MaxPlaylistsPerMonth,
		"max_songs_per_playlist":    DefaultLimits.MaxSongsPerPlaylist,
		"playlists_used_this_month": count,
		"playlists_remaining":       remaining,
	})
}
