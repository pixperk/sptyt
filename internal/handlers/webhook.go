package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/models"
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
	// Verify webhook signature
	signature := c.Request().Header.Get("X-DodoPay-Signature")
	if signature == "" {
		return echo.NewHTTPError(http.StatusUnauthorized, "Missing signature")
	}

	// Read raw body
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Failed to read body")
	}

	// Verify signature
	webhookSecret := os.Getenv("DODOPAY_WEBHOOK_SECRET")
	if !verifySignature(body, signature, webhookSecret) {
		log.Printf("Invalid webhook signature")
		return echo.NewHTTPError(http.StatusUnauthorized, "Invalid signature")
	}

	// Parse webhook event
	var event struct {
		Type string `json:"type"`
		Data struct {
			SubscriptionID string `json:"subscription_id"`
			CustomerEmail  string `json:"customer_email"`
			Status         string `json:"status"`
			PlanID         string `json:"plan_id"`
			CurrentPeriodEnd int64 `json:"current_period_end"`
		} `json:"data"`
	}

	if err := c.Bind(&event); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid JSON")
	}

	ctx := context.Background()

	// Handle different event types
	switch event.Type {
	// Subscription events
	case "subscription.active":
		return wh.handleSubscriptionActive(ctx, event.Data.CustomerEmail, event.Data.SubscriptionID, event.Data.CurrentPeriodEnd)

	case "subscription.cancelled":
		return wh.handleSubscriptionCancelled(ctx, event.Data.SubscriptionID)

	case "subscription.expired":
		return wh.handleSubscriptionExpired(ctx, event.Data.SubscriptionID)

	case "subscription.failed":
		return wh.handleSubscriptionFailed(ctx, event.Data.SubscriptionID)

	case "subscription.on_hold":
		return wh.handleSubscriptionOnHold(ctx, event.Data.SubscriptionID)

	case "subscription.plan_changed":
		return wh.handleSubscriptionPlanChanged(ctx, event.Data.SubscriptionID, event.Data.CurrentPeriodEnd)

	case "subscription.renewed":
		return wh.handleSubscriptionRenewed(ctx, event.Data.SubscriptionID, event.Data.CurrentPeriodEnd)

	// Payment events
	case "payment.cancelled":
		return wh.handlePaymentCancelled(ctx, event.Data.SubscriptionID)

	case "payment.failed":
		return wh.handlePaymentFailed(ctx, event.Data.SubscriptionID)

	case "payment.processing":
		return wh.handlePaymentProcessing(ctx, event.Data.SubscriptionID)

	case "payment.succeeded":
		return wh.handlePaymentSucceeded(ctx, event.Data.SubscriptionID, event.Data.CurrentPeriodEnd)

	default:
		log.Printf("Unknown webhook event type: %s", event.Type)
		return c.JSON(http.StatusOK, map[string]string{"status": "ignored"})
	}
}

func (wh *WebhookHandler) handleSubscriptionActive(ctx context.Context, email, subscriptionID string, periodEnd int64) error {
	log.Printf("Subscription active: %s for %s", subscriptionID, email)

	// Find user by email
	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("email = ?", email).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for email %s: %v", email, err)
		return nil // Don't fail webhook
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
		return nil
	}

	log.Printf("User %s upgraded to premium", user.Email)
	return nil
}

func (wh *WebhookHandler) handleSubscriptionCancelled(ctx context.Context, subscriptionID string) error {
	log.Printf("Subscription cancelled: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return nil
	}

	_, err = wh.db.NewUpdate().
		Model(&user).
		Set("subscription_status = ?", "cancelled").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to cancel subscription: %v", err)
	}

	return nil
}

func (wh *WebhookHandler) handleSubscriptionExpired(ctx context.Context, subscriptionID string) error {
	log.Printf("Subscription expired: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return nil
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
	}

	log.Printf("User %s downgraded to free tier", user.Email)
	return nil
}

func (wh *WebhookHandler) handlePaymentSucceeded(ctx context.Context, subscriptionID string, periodEnd int64) error {
	log.Printf("Payment succeeded for subscription: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return nil
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
	}

	return nil
}

func (wh *WebhookHandler) handlePaymentFailed(ctx context.Context, subscriptionID string) error {
	log.Printf("Payment failed for subscription: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return nil
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
	}

	return nil
}

func (wh *WebhookHandler) handleSubscriptionFailed(ctx context.Context, subscriptionID string) error {
	log.Printf("Subscription failed: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return nil
	}

	_, err = wh.db.NewUpdate().
		Model(&user).
		Set("subscription_status = ?", "payment_failed").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to update subscription status: %v", err)
	}

	return nil
}

func (wh *WebhookHandler) handleSubscriptionOnHold(ctx context.Context, subscriptionID string) error {
	log.Printf("Subscription on hold: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return nil
	}

	_, err = wh.db.NewUpdate().
		Model(&user).
		Set("subscription_status = ?", "on_hold").
		Set("updated_at = ?", time.Now()).
		Where("id = ?", user.ID).
		Exec(ctx)

	if err != nil {
		log.Printf("Failed to update subscription status: %v", err)
	}

	return nil
}

func (wh *WebhookHandler) handleSubscriptionPlanChanged(ctx context.Context, subscriptionID string, periodEnd int64) error {
	log.Printf("Subscription plan changed: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return nil
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
	}

	return nil
}

func (wh *WebhookHandler) handleSubscriptionRenewed(ctx context.Context, subscriptionID string, periodEnd int64) error {
	log.Printf("Subscription renewed: %s", subscriptionID)

	var user models.User
	err := wh.db.NewSelect().
		Model(&user).
		Where("subscription_id = ?", subscriptionID).
		Scan(ctx)

	if err != nil {
		log.Printf("User not found for subscription %s: %v", subscriptionID, err)
		return nil
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
	}

	log.Printf("User subscription %s renewed until %v", subscriptionID, expiresAt)
	return nil
}

func (wh *WebhookHandler) handlePaymentCancelled(ctx context.Context, subscriptionID string) error {
	log.Printf("Payment cancelled for subscription: %s", subscriptionID)

	// Payment was cancelled during processing
	// No immediate action needed - user can retry
	return nil
}

func (wh *WebhookHandler) handlePaymentProcessing(ctx context.Context, subscriptionID string) error {
	log.Printf("Payment processing for subscription: %s", subscriptionID)

	// Payment is being processed
	// No immediate action needed - wait for success or failure
	return nil
}

// verifySignature verifies the webhook signature from DodoPay
func verifySignature(payload []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}
