package handlers

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/clerk/clerk-sdk-go/v2/user"
	"github.com/google/uuid"
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
	var dbUser models.User

	// Try to find existing user
	err := ph.db.NewSelect().
		Model(&dbUser).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err == nil {
		return &dbUser, nil
	}

	// User not found - fetch from Clerk and create
	log.Printf("User %s not found in database, fetching from Clerk...", clerkUserID)

	clerkUser, err := user.Get(ctx, clerkUserID)
	if err != nil {
		log.Printf("Failed to fetch user from Clerk: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch user data")
	}

	// Extract email from Clerk user
	var email string
	if len(clerkUser.EmailAddresses) > 0 {
		for _, emailAddr := range clerkUser.EmailAddresses {
			if clerkUser.PrimaryEmailAddressID != nil && emailAddr.ID == *clerkUser.PrimaryEmailAddressID {
				email = emailAddr.EmailAddress
				break
			}
		}
		// Fallback to first email if primary not found
		if email == "" {
			email = clerkUser.EmailAddresses[0].EmailAddress
		}
	}

	// Extract username
	username := ""
	if clerkUser.Username != nil {
		username = *clerkUser.Username
	}

	// Extract profile image
	profileImageURL := ""
	if clerkUser.ImageURL != nil {
		profileImageURL = *clerkUser.ImageURL
	}

	// Extract names
	firstName := ""
	if clerkUser.FirstName != nil {
		firstName = *clerkUser.FirstName
	}

	lastName := ""
	if clerkUser.LastName != nil {
		lastName = *clerkUser.LastName
	}

	// Create new user in database
	newUser := &models.User{
		ID:                 uuid.New(),
		ClerkID:            clerkUserID,
		Email:              email,
		Username:           username,
		FirstName:          firstName,
		LastName:           lastName,
		ProfileImageURL:    profileImageURL,
		SubscriptionTier:   "free",
		SubscriptionStatus: "inactive",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	_, err = ph.db.NewInsert().Model(newUser).Exec(ctx)
	if err != nil {
		log.Printf("Failed to create user in database: %v", err)
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "Failed to create user profile")
	}

	log.Printf("Successfully created user %s in database", clerkUserID)
	return newUser, nil
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
