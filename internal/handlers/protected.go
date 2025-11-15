package handlers

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/pixperk/sptyt/internal/database"

	"github.com/clerk/clerk-sdk-go/v2/user"
	dodopayments "github.com/dodopayments/dodopayments-go"
	"github.com/dodopayments/dodopayments-go/option"
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

	ctx, cancel := database.NewQueryContext()
	defer cancel()
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

	// Store user in context for other middlewares
	c.Set("current_user", user)

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

// CreateCheckoutSession creates a DodoPay checkout session for subscription
func (ph *ProtectedHandler) CreateCheckoutSession(c echo.Context) error {
	log.Println("CreateCheckoutSession: Starting checkout session creation")

	user, err := ph.GetOrCreateUser(c)
	if err != nil {
		log.Printf("CreateCheckoutSession: Failed to get/create user: %v", err)
		return err
	}
	log.Printf("CreateCheckoutSession: User retrieved: %s (email: %s)", user.ID, user.Email)

	// Store user in context
	c.Set("current_user", user)

	// Parse payment method from request
	var requestBody struct {
		PaymentMethod string `json:"payment_method"` // "card" or "upi"
	}
	if err := c.Bind(&requestBody); err != nil {
		log.Printf("CreateCheckoutSession: Failed to bind request body, using default: %v", err)
		requestBody.PaymentMethod = "card" // Default to card
	}
	log.Printf("CreateCheckoutSession: Payment method: %s", requestBody.PaymentMethod)

	// Get DodoPay configuration
	dodopayAPIKey := os.Getenv("DODOPAY_API_KEY")
	dodopayAPIHost := os.Getenv("DODOPAY_API_HOST")
	productID := os.Getenv("DODOPAY_PRODUCT_ID")
	returnURL := os.Getenv("DODOPAY_RETURN_URL")

	log.Printf("CreateCheckoutSession: Config check - API Key exists: %v, API Host: %s, Product ID exists: %v, Return URL exists: %v",
		dodopayAPIKey != "", dodopayAPIHost, productID != "", returnURL != "")

	if dodopayAPIKey == "" || productID == "" {
		log.Println("CreateCheckoutSession: Missing DodoPay configuration")
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Payment system not configured")
	}

	if returnURL == "" {
		returnURL = "https://sptyt.xyz/payment/return"
		log.Printf("CreateCheckoutSession: Using default return URL: %s", returnURL)
	}

	// Initialize DodoPay client
	log.Println("CreateCheckoutSession: Initializing DodoPay client")

	var clientOptions []option.RequestOption
	clientOptions = append(clientOptions, option.WithBearerToken(dodopayAPIKey))

	// Use custom API host if provided
	if dodopayAPIHost != "" {
		log.Printf("CreateCheckoutSession: Using custom API host: %s", dodopayAPIHost)
		clientOptions = append(clientOptions, option.WithBaseURL(dodopayAPIHost))
	}

	client := dodopayments.NewClient(clientOptions...)

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Determine allowed payment methods based on request
	allowedPaymentMethods := []dodopayments.PaymentMethodTypes{
		dodopayments.PaymentMethodTypesCredit,
		dodopayments.PaymentMethodTypesDebit,
	}

	// Add UPI payment methods if requested
	if requestBody.PaymentMethod == "upi" {
		allowedPaymentMethods = append(allowedPaymentMethods,
			dodopayments.PaymentMethodTypesUpiCollect,
			dodopayments.PaymentMethodTypesUpiIntent,
		)
	}
	log.Printf("CreateCheckoutSession: Allowed payment methods: %v", allowedPaymentMethods)

	// Create customer name
	customerName := user.FirstName + " " + user.LastName
	if customerName == " " {
		customerName = user.Email
	}
	log.Printf("CreateCheckoutSession: Customer name: %s", customerName)

	// Create checkout session with product
	log.Printf("CreateCheckoutSession: Creating checkout session with product ID: %s", productID)
	session, err := client.CheckoutSessions.New(ctx, dodopayments.CheckoutSessionNewParams{
		CheckoutSessionRequest: dodopayments.CheckoutSessionRequestParam{
			ProductCart: dodopayments.F([]dodopayments.CheckoutSessionRequestProductCartParam{{
				ProductID: dodopayments.F(productID),
				Quantity:  dodopayments.F(int64(1)),
			}}),
			ReturnURL: dodopayments.F(returnURL),
			Customer: dodopayments.F[dodopayments.CustomerRequestUnionParam](dodopayments.CustomerRequestParam{
				Email: dodopayments.F(user.Email),
				Name:  dodopayments.F(customerName),
			}),
			AllowedPaymentMethodTypes: dodopayments.F(allowedPaymentMethods),
		},
	})

	if err != nil {
		log.Printf("CreateCheckoutSession: Failed to create checkout session: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create checkout session")
	}

	log.Printf("CreateCheckoutSession: Success! Session ID: %s, Checkout URL: %s", session.SessionID, session.CheckoutURL)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"checkout_url":   session.CheckoutURL,
		"session_id":     session.SessionID,
		"payment_method": requestBody.PaymentMethod,
		"message":        "Checkout session created. Supports both Card and UPI payments.",
	})
}

// CancelSubscription cancels the user's subscription
func (ph *ProtectedHandler) CancelSubscription(c echo.Context) error {
	user, err := ph.GetOrCreateUser(c)
	if err != nil {
		return err
	}

	if user.SubscriptionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "No active subscription")
	}

	// TODO: Call DodoPay API to cancel subscription
	// For now, just mark as cancelled in database

	ctx, cancel := database.NewQueryContext()
	defer cancel()
	_, err = ph.db.NewUpdate().
		Model(user).
		Set("subscription_status = ?", "cancelled").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to cancel subscription: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to cancel subscription")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":      "Subscription cancelled successfully",
		"access_until": user.SubscriptionEndsAt,
	})
}

// PaymentReturn handles the redirect after payment completion
func (ph *ProtectedHandler) PaymentReturn(c echo.Context) error {
	// Get query parameters from the redirect
	sessionID := c.QueryParam("session_id")
	status := c.QueryParam("status")                  // From DodoPay redirect (e.g., "active")
	subscriptionID := c.QueryParam("subscription_id") // From DodoPay redirect

	log.Printf("PaymentReturn: session_id=%s, status=%s, subscription_id=%s",
		sessionID, status, subscriptionID)

	// Get the authenticated user
	user, err := ph.GetOrCreateUser(c)
	if err != nil {
		return err
	}

	// Fetch the latest user data from database to check if webhook updated it
	ctx, cancel := database.NewQueryContext()
	defer cancel()
	var freshUser models.User
	err = ph.db.NewSelect().
		Model(&freshUser).
		Where("id = ?", user.ID).
		Scan(ctx)

	if err != nil {
		log.Printf("PaymentReturn: Failed to fetch user: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch user data")
	}

	// Determine payment status based on query params and database state
	paymentStatus := "pending"
	message := "Payment is being processed. Your subscription will be activated shortly."

	if status == "active" || (freshUser.SubscriptionStatus == "active" && freshUser.SubscriptionTier == "premium") {
		paymentStatus = "success"
		message = "Payment successful! Your premium subscription is now active."
	} else if status == "cancelled" || status == "failed" {
		paymentStatus = "failed"
		message = "Payment was not completed. Please try again."
	}

	log.Printf("PaymentReturn: user_id=%s, payment_status=%s, db_subscription_status=%s, subscription_tier=%s",
		freshUser.ID, paymentStatus, freshUser.SubscriptionStatus, freshUser.SubscriptionTier)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":          paymentStatus,
		"message":         message,
		"session_id":      sessionID,
		"subscription_id": subscriptionID,
		"user": map[string]interface{}{
			"email":                freshUser.Email,
			"subscription_tier":    freshUser.SubscriptionTier,
			"subscription_status":  freshUser.SubscriptionStatus,
			"subscription_ends_at": freshUser.SubscriptionEndsAt,
			"is_premium":           freshUser.IsPremium(),
		},
	})
}
