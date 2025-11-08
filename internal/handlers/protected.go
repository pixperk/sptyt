package handlers

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/auth"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/uptrace/bun"
)

type ProtectedHandler struct {
	handler *Handler
	db      *bun.DB
}

func NewProtectedHandler(h *Handler, db *bun.DB) *ProtectedHandler {
	return &ProtectedHandler{
		handler: h,
		db:      db,
	}
}

// GetOrCreateUser gets or creates a user from Clerk ID
func (ph *ProtectedHandler) GetOrCreateUser(c echo.Context) (*models.User, error) {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "Unable to get user ID")
	}

	ctx := context.Background()
	var user models.User

	// Try to find existing user
	err := ph.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err == nil {
		return &user, nil
	}

	// User not found - return error
	return nil, echo.NewHTTPError(http.StatusNotFound, "User profile not found. Please complete onboarding.")
}

// Me returns the authenticated user's profile information
func (ph *ProtectedHandler) Me(c echo.Context) error {
	user, err := ph.GetOrCreateUser(c)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"user": user,
		"subscription": map[string]interface{}{
			"tier":       user.SubscriptionTier,
			"status":     user.SubscriptionStatus,
			"expires_at": user.SubscriptionEndsAt,
			"is_premium": user.IsPremium(),
		},
	})
}
