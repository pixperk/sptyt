package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/models"
)

// RequirePremium is middleware that checks if user has active premium subscription
func RequirePremium() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get user from context (set by GetOrCreateUser)
			userInterface := c.Get("current_user")
			if userInterface == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "User not found")
			}

			user, ok := userInterface.(*models.User)
			if !ok {
				return echo.NewHTTPError(http.StatusInternalServerError, "Invalid user data")
			}

			// Check if user has premium subscription
			if !user.IsPremium() {
				return c.JSON(http.StatusForbidden, map[string]interface{}{
					"error":   "Premium subscription required",
					"message": "This feature is only available for premium users. Upgrade to access playlist conversion, batch operations, and more!",
					"upgrade_url": "/upgrade",
				})
			}

			return next(c)
		}
	}
}

// CheckSubscription middleware adds subscription info to context
func CheckSubscription() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get user from context
			userInterface := c.Get("current_user")
			if userInterface != nil {
				user, ok := userInterface.(*models.User)
				if ok {
					// Add subscription info to context
					c.Set("is_premium", user.IsPremium())
					c.Set("subscription_tier", user.SubscriptionTier)
					c.Set("subscription_status", user.SubscriptionStatus)
				}
			}

			return next(c)
		}
	}
}
