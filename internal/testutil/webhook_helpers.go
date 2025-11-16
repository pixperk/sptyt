package testutil

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// WebhookTestHelper helps with testing webhook endpoints
type WebhookTestHelper struct {
	T             *testing.T
	WebhookSecret string
}

// NewWebhookTestHelper creates a new webhook test helper
func NewWebhookTestHelper(t *testing.T, secret string) *WebhookTestHelper {
	return &WebhookTestHelper{
		T:             t,
		WebhookSecret: secret,
	}
}

// DodoPayEvent represents a DodoPay webhook event
type DodoPayEvent struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// CreateDodoPayWebhookRequest creates a signed DodoPay webhook request
// This follows the Standard Webhooks specification used by DodoPay
func (w *WebhookTestHelper) CreateDodoPayWebhookRequest(eventType string, data map[string]interface{}) *http.Request {
	event := DodoPayEvent{
		Type: eventType,
		Data: data,
	}

	payload, err := json.Marshal(event)
	require.NoError(w.T, err, "Failed to marshal webhook payload")

	// Generate webhook headers following Standard Webhooks spec
	webhookID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	// Create signature using Standard Webhooks format: "{id}.{timestamp}.{payload}"
	signature := w.signWebhook(webhookID, timestamp, payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/dodopay", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("webhook-id", webhookID)
	req.Header.Set("webhook-timestamp", timestamp)
	req.Header.Set("webhook-signature", signature)

	return req
}

// signWebhook signs a webhook payload using Standard Webhooks format
func (w *WebhookTestHelper) signWebhook(webhookID, timestamp string, payload []byte) string {
	// Standard Webhooks signature format
	signedContent := fmt.Sprintf("%s.%s.%s", webhookID, timestamp, payload)

	mac := hmac.New(sha256.New, []byte(w.WebhookSecret))
	mac.Write([]byte(signedContent))
	signature := mac.Sum(nil)

	// Standard Webhooks uses base64 encoding with versioning
	return fmt.Sprintf("v1,%s", base64.StdEncoding.EncodeToString(signature))
}

// CreateInvalidSignatureRequest creates a webhook request with invalid signature
func (w *WebhookTestHelper) CreateInvalidSignatureRequest(eventType string, data map[string]interface{}) *http.Request {
	event := DodoPayEvent{
		Type: eventType,
		Data: data,
	}

	payload, err := json.Marshal(event)
	require.NoError(w.T, err, "Failed to marshal webhook payload")

	webhookID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/dodopay", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("webhook-id", webhookID)
	req.Header.Set("webhook-timestamp", timestamp)
	req.Header.Set("webhook-signature", "v1,invalid_signature_here")

	return req
}

// CreateMissingHeadersRequest creates a webhook request with missing headers
func (w *WebhookTestHelper) CreateMissingHeadersRequest(eventType string, data map[string]interface{}) *http.Request {
	event := DodoPayEvent{
		Type: eventType,
		Data: data,
	}

	payload, err := json.Marshal(event)
	require.NoError(w.T, err, "Failed to marshal webhook payload")

	req := httptest.NewRequest(http.MethodPost, "/webhooks/dodopay", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	// Deliberately missing webhook-id, webhook-timestamp, webhook-signature

	return req
}

// SubscriptionActiveEvent creates a subscription.active event payload
func SubscriptionActiveEvent(email, subscriptionID string, nextBillingDate time.Time) map[string]interface{} {
	return map[string]interface{}{
		"subscription_id": subscriptionID,
		"customer": map[string]interface{}{
			"email": email,
		},
		"status":            "active",
		"next_billing_date": nextBillingDate.Format(time.RFC3339),
	}
}

// SubscriptionCancelledEvent creates a subscription.cancelled event payload
func SubscriptionCancelledEvent(subscriptionID string) map[string]interface{} {
	return map[string]interface{}{
		"subscription_id": subscriptionID,
		"status":          "cancelled",
	}
}

// SubscriptionExpiredEvent creates a subscription.expired event payload
func SubscriptionExpiredEvent(subscriptionID string) map[string]interface{} {
	return map[string]interface{}{
		"subscription_id": subscriptionID,
		"status":          "expired",
	}
}

// SubscriptionFailedEvent creates a subscription.failed event payload
func SubscriptionFailedEvent(subscriptionID string) map[string]interface{} {
	return map[string]interface{}{
		"subscription_id": subscriptionID,
		"status":          "failed",
	}
}

// SubscriptionOnHoldEvent creates a subscription.on_hold event payload
func SubscriptionOnHoldEvent(subscriptionID string) map[string]interface{} {
	return map[string]interface{}{
		"subscription_id": subscriptionID,
		"status":          "on_hold",
	}
}

// SubscriptionPlanChangedEvent creates a subscription.plan_changed event payload
func SubscriptionPlanChangedEvent(subscriptionID string, nextBillingDate time.Time) map[string]interface{} {
	return map[string]interface{}{
		"subscription_id":   subscriptionID,
		"status":            "active",
		"next_billing_date": nextBillingDate.Format(time.RFC3339),
	}
}

// SubscriptionRenewedEvent creates a subscription.renewed event payload
func SubscriptionRenewedEvent(subscriptionID string, nextBillingDate time.Time) map[string]interface{} {
	return map[string]interface{}{
		"subscription_id":   subscriptionID,
		"status":            "active",
		"next_billing_date": nextBillingDate.Format(time.RFC3339),
	}
}

// PaymentSucceededEvent creates a payment.succeeded event payload
func PaymentSucceededEvent(subscriptionID string, nextBillingDate time.Time) map[string]interface{} {
	return map[string]interface{}{
		"subscription_id":   subscriptionID,
		"status":            "active",
		"next_billing_date": nextBillingDate.Format(time.RFC3339),
	}
}

// PaymentFailedEvent creates a payment.failed event payload
func PaymentFailedEvent(subscriptionID string) map[string]interface{} {
	return map[string]interface{}{
		"subscription_id": subscriptionID,
		"status":          "failed",
	}
}

// PaymentCancelledEvent creates a payment.cancelled event payload
func PaymentCancelledEvent(subscriptionID string) map[string]interface{} {
	return map[string]interface{}{
		"subscription_id": subscriptionID,
		"status":          "cancelled",
	}
}

// PaymentProcessingEvent creates a payment.processing event payload
func PaymentProcessingEvent(subscriptionID string) map[string]interface{} {
	return map[string]interface{}{
		"subscription_id": subscriptionID,
		"status":          "processing",
	}
}
