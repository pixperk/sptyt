package middleware

import (
	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/pixperk/sptyt/pkg/errors"
)

// RequirePremium is middleware that checks if user has active premium subscription
func RequirePremium() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get user from context (set by GetOrCreateUser)
			userInterface := c.Get("current_user")
			if userInterface == nil {
				return errors.ToHTTPError(errors.Unauthorized("User not found"))
			}

			user, ok := userInterface.(*models.User)
			if !ok {
				return errors.ToHTTPError(errors.Internal("Invalid user data"))
			}

			// Check if user has premium subscription
			if !user.IsPremium() {
				return errors.ToHTTPError(
					errors.PremiumRequired("This feature").
						WithDetails("This feature is only available for premium users. Upgrade to access playlist conversion, batch operations, and more!").
						WithMeta("upgrade_url", "/upgrade"),
				)
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
