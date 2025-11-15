package handlers

import (
	"github.com/pixperk/sptyt/internal/database"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/models"
	svix "github.com/svix/svix-webhooks/go"
	"github.com/uptrace/bun"
)

type WebhookHandler struct {
	db *bun.DB
}

func NewWebhookHandler(db *bun.DB) *WebhookHandler {
	return &WebhookHandler{db: db}
}

// DodoPayWebhook handles webhook events from DodoPay
func (wh *WebhookHandler) DodoPayWebhook(c echo.Context) error {
	log.Println("DodoPayWebhook: Received webhook")

	// Get Standard Webhooks headers (DodoPay uses Standard Webhooks spec)
	webhookID := c.Request().Header.Get("webhook-id")
	webhookTimestamp := c.Request().Header.Get("webhook-timestamp")
	webhookSignature := c.Request().Header.Get("webhook-signature")

	log.Printf("DodoPayWebhook: Headers - webhook-id: %s, webhook-timestamp: %s, webhook-signature exists: %v",
		webhookID, webhookTimestamp, webhookSignature != "")

	if webhookID == "" || webhookTimestamp == "" || webhookSignature == "" {
		log.Println("DodoPayWebhook: Missing webhook headers")
		return echo.NewHTTPError(http.StatusUnauthorized, "Missing webhook headers")
	}

	// Read raw body
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		log.Printf("DodoPayWebhook: Failed to read body: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Failed to read body")
	}

	log.Printf("DodoPayWebhook: Received payload: %s", string(body))

	// Verify signature using Standard Webhooks (svix library implements this spec)
	webhookSecret := os.Getenv("DODOPAY_WEBHOOK_SECRET")
	if webhookSecret == "" {
		log.Println("DodoPayWebhook: WARNING - No webhook secret configured")
		return echo.NewHTTPError(http.StatusInternalServerError, "Webhook secret not configured")
	}

	wh_verify, err := svix.NewWebhook(webhookSecret)
	if err != nil {
		log.Printf("DodoPayWebhook: Failed to create webhook verifier: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Webhook verification setup failed")
	}

	// Standard Webhooks uses webhook-* headers
	headers := http.Header{}
	headers.Set("webhook-id", webhookID)
	headers.Set("webhook-timestamp", webhookTimestamp)
	headers.Set("webhook-signature", webhookSignature)

	err = wh_verify.Verify(body, headers)
	if err != nil {
		log.Printf("DodoPayWebhook: Signature verification failed: %v", err)
		return echo.NewHTTPError(http.StatusUnauthorized, "Invalid signature")
	}

	log.Println("DodoPayWebhook: Signature verified successfully")

	// Parse webhook event - DodoPay structure
	var event struct {
		Type string `json:"type"`
		Data struct {
			SubscriptionID string `json:"subscription_id"`
			Customer       struct {
				Email string `json:"email"`
			} `json:"customer"`
			Status           string `json:"status"`
			ProductID        string `json:"product_id"`
			NextBillingDate  string `json:"next_billing_date"`
			ExpiresAt        string `json:"expires_at"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &event); err != nil {
		log.Printf("DodoPayWebhook: Failed to parse JSON: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid JSON")
	}

	log.Printf("DodoPayWebhook: Event type: %s, Subscription ID: %s, Customer: %s",
		event.Type, event.Data.SubscriptionID, event.Data.Customer.Email)

	// Parse next_billing_date to Unix timestamp
	var nextBillingTimestamp int64
	if event.Data.NextBillingDate != "" {
		t, err := time.Parse(time.RFC3339, event.Data.NextBillingDate)
		if err == nil {
			nextBillingTimestamp = t.Unix()
		}
	}

	// Fallback to expires_at if next_billing_date is not available
	if nextBillingTimestamp == 0 && event.Data.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, event.Data.ExpiresAt)
		if err == nil {
			nextBillingTimestamp = t.Unix()
		}
	}

	ctx, cancel := database.NewQueryContext()
	defer cancel()

	// Handle different event types
	switch event.Type {
	// Subscription events
	case "subscription.active":
		return wh.handleSubscriptionActive(ctx, c, event.Data.Customer.Email, event.Data.SubscriptionID, nextBillingTimestamp)

	case "subscription.cancelled":
		return wh.handleSubscriptionCancelled(ctx, c, event.Data.SubscriptionID)

	case "subscription.expired":
		return wh.handleSubscriptionExpired(ctx, c, event.Data.SubscriptionID)

	case "subscription.failed":
		return wh.handleSubscriptionFailed(ctx, c, event.Data.SubscriptionID)

	case "subscription.on_hold":
		return wh.handleSubscriptionOnHold(ctx, c, event.Data.SubscriptionID)

	case "subscription.plan_changed":
		return wh.handleSubscriptionPlanChanged(ctx, c, event.Data.SubscriptionID, nextBillingTimestamp)

	case "subscription.renewed":
		return wh.handleSubscriptionRenewed(ctx, c, event.Data.SubscriptionID, nextBillingTimestamp)

	// Payment events
	case "payment.cancelled":
		return wh.handlePaymentCancelled(ctx, c, event.Data.SubscriptionID)

	case "payment.failed":
		return wh.handlePaymentFailed(ctx, c, event.Data.SubscriptionID)

	case "payment.processing":
		return wh.handlePaymentProcessing(ctx, c, event.Data.SubscriptionID)

	case "payment.succeeded":
		return wh.handlePaymentSucceeded(ctx, c, event.Data.SubscriptionID, nextBillingTimestamp)

	default:
		log.Printf("Unknown webhook event type: %s", event.Type)
		return c.JSON(http.StatusOK, map[string]string{"status": "ignored"})
	}
}

func (wh *WebhookHandler) handleSubscriptionActive(ctx context.Context, c echo.Context, email, subscriptionID string, periodEnd int64) error {
	log.Printf("Subscription active: %s for %s", subscriptionID, email)

	// Find user by email
	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("email = ?", email).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for email %s: %v", email, err)
		return c.JSON(http.StatusOK, map[string]string{"status": "user_not_found"})
	}

	// Update user subscription
	expiresAt := time.Unix(periodEnd, 0)
	_, err = wh.db.NewUpdate().
		Model(&user).
		Set("subscription_tier = ?", "premium").
		Set("subscription_status = ?", "active").
		Set("subscription_id = ?", subscriptionID).
		Set("subscription_ends_at = ?", expiresAt).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to update user subscription: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_update_failed"})
	}

	log.Printf("User %s upgraded to premium", user.Email)
	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

func (wh *WebhookHandler) handleSubscriptionCancelled(ctx context.Context, c echo.Context, subscriptionID string) error {
	log.Printf("Subscription cancelled: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return c.JSON(http.StatusOK, map[string]string{"status": "user_not_found"})
	}

	_, err = wh.db.NewUpdate().
		Model(&user).
		Set("subscription_status = ?", "cancelled").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to cancel subscription: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_update_failed"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

func (wh *WebhookHandler) handleSubscriptionExpired(ctx context.Context, c echo.Context, subscriptionID string) error {
	log.Printf("Subscription expired: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return c.JSON(http.StatusOK, map[string]string{"status": "user_not_found"})
	}

	_, err = wh.db.NewUpdate().
		Model(&user).
		Set("subscription_tier = ?", "free").
		Set("subscription_status = ?", "inactive").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to downgrade user: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_update_failed"})
	}

	log.Printf("User %s downgraded to free tier", user.Email)
	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

func (wh *WebhookHandler) handlePaymentSucceeded(ctx context.Context, c echo.Context, subscriptionID string, periodEnd int64) error {
	log.Printf("Payment succeeded for subscription: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return c.JSON(http.StatusOK, map[string]string{"status": "user_not_found"})
	}

	expiresAt := time.Unix(periodEnd, 0)
	_, err = wh.db.NewUpdate().
		Model(&user).
		Set("subscription_status = ?", "active").
		Set("subscription_ends_at = ?", expiresAt).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to update subscription: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_update_failed"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

func (wh *WebhookHandler) handlePaymentFailed(ctx context.Context, c echo.Context, subscriptionID string) error {
	log.Printf("Payment failed for subscription: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return c.JSON(http.StatusOK, map[string]string{"status": "user_not_found"})
	}

	// Mark as payment_failed but don't downgrade immediately
	// Give user grace period to update payment method
	_, err = wh.db.NewUpdate().
		Model(&user).
		Set("subscription_status = ?", "payment_failed").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to update subscription status: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_update_failed"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

func (wh *WebhookHandler) handleSubscriptionFailed(ctx context.Context, c echo.Context, subscriptionID string) error {
	log.Printf("Subscription failed: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return c.JSON(http.StatusOK, map[string]string{"status": "user_not_found"})
	}

	_, err = wh.db.NewUpdate().
		Model(&user).
		Set("subscription_status = ?", "payment_failed").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to update subscription status: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_update_failed"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

func (wh *WebhookHandler) handleSubscriptionOnHold(ctx context.Context, c echo.Context, subscriptionID string) error {
	log.Printf("Subscription on hold: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return c.JSON(http.StatusOK, map[string]string{"status": "user_not_found"})
	}

	_, err = wh.db.NewUpdate().
		Model(&user).
		Set("subscription_status = ?", "on_hold").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to update subscription status: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_update_failed"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

func (wh *WebhookHandler) handleSubscriptionPlanChanged(ctx context.Context, c echo.Context, subscriptionID string, periodEnd int64) error {
	log.Printf("Subscription plan changed: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return c.JSON(http.StatusOK, map[string]string{"status": "user_not_found"})
	}

	expiresAt := time.Unix(periodEnd, 0)
	_, err = wh.db.NewUpdate().
		Model(&user).
		Set("subscription_ends_at = ?", expiresAt).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to update subscription: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_update_failed"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

func (wh *WebhookHandler) handleSubscriptionRenewed(ctx context.Context, c echo.Context, subscriptionID string, periodEnd int64) error {
	log.Printf("Subscription renewed: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return c.JSON(http.StatusOK, map[string]string{"status": "user_not_found"})
	}

	expiresAt := time.Unix(periodEnd, 0)
	_, err = wh.db.NewUpdate().
		Model(&user).
		Set("subscription_status = ?", "active").
		Set("subscription_ends_at = ?", expiresAt).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to renew subscription: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "database_update_failed"})
	}

	log.Printf("User subscription %s renewed until %v", subscriptionID, expiresAt)
	return c.JSON(http.StatusOK, map[string]string{"status": "success"})
}

func (wh *WebhookHandler) handlePaymentCancelled(ctx context.Context, c echo.Context, subscriptionID string) error {
	log.Printf("Payment cancelled for subscription: %s", subscriptionID)

	// Payment was cancelled during processing
	// No immediate action needed - user can retry
	return c.JSON(http.StatusOK, map[string]string{"status": "no_action"})
}

func (wh *WebhookHandler) handlePaymentProcessing(ctx context.Context, c echo.Context, subscriptionID string) error {
	log.Printf("Payment processing for subscription: %s", subscriptionID)

	// Payment is being processed
	// No immediate action needed - wait for success or failure
	return c.JSON(http.StatusOK, map[string]string{"status": "no_action"})
}
