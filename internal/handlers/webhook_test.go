package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/pixperk/sptyt/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDodoPayWebhook_SignatureVerification tests webhook signature verification
func TestDodoPayWebhook_SignatureVerification(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	webhookSecret := "test_webhook_secret_key_12345"
	os.Setenv("DODOPAY_WEBHOOK_SECRET", webhookSecret)
	defer os.Unsetenv("DODOPAY_WEBHOOK_SECRET")

	handler := NewWebhookHandler(testDB.DB)
	e := echo.New()

	t.Run("valid signature - accepts webhook", func(t *testing.T) {
		helper := testutil.NewWebhookTestHelper(t, webhookSecret)

		// Create test user
		user := testDB.CreateTestUser(ctx,
			testutil.WithEmail("test@example.com"),
			testutil.WithSubscriptionID("sub_123"),
		)

		// Create valid webhook request
		eventData := testutil.SubscriptionActiveEvent(
			user.Email,
			user.SubscriptionID,
			time.Now().Add(30*24*time.Hour),
		)
		req := helper.CreateDodoPayWebhookRequest("subscription.active", eventData)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute
		err := handler.DodoPayWebhook(c)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]string
		json.Unmarshal(rec.Body.Bytes(), &response)
		assert.Equal(t, "success", response["status"])
	})

	t.Run("invalid signature - rejects webhook", func(t *testing.T) {
		helper := testutil.NewWebhookTestHelper(t, webhookSecret)

		eventData := testutil.SubscriptionActiveEvent(
			"test@example.com",
			"sub_123",
			time.Now().Add(30*24*time.Hour),
		)
		req := helper.CreateInvalidSignatureRequest("subscription.active", eventData)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute
		err := handler.DodoPayWebhook(c)

		// Assert
		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
		assert.Contains(t, httpErr.Message, "Invalid signature")
	})

	t.Run("missing headers - rejects webhook", func(t *testing.T) {
		helper := testutil.NewWebhookTestHelper(t, webhookSecret)

		eventData := testutil.SubscriptionActiveEvent(
			"test@example.com",
			"sub_123",
			time.Now().Add(30*24*time.Hour),
		)
		req := helper.CreateMissingHeadersRequest("subscription.active", eventData)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute
		err := handler.DodoPayWebhook(c)

		// Assert
		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
		assert.Contains(t, httpErr.Message, "Missing webhook headers")
	})

	t.Run("no webhook secret configured - returns error", func(t *testing.T) {
		os.Unsetenv("DODOPAY_WEBHOOK_SECRET")
		defer os.Setenv("DODOPAY_WEBHOOK_SECRET", webhookSecret)

		helper := testutil.NewWebhookTestHelper(t, "dummy")
		eventData := testutil.SubscriptionActiveEvent(
			"test@example.com",
			"sub_123",
			time.Now().Add(30*24*time.Hour),
		)
		req := helper.CreateDodoPayWebhookRequest("subscription.active", eventData)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute
		err := handler.DodoPayWebhook(c)

		// Assert
		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusInternalServerError, httpErr.Code)
	})
}

// TestDodoPayWebhook_SubscriptionActive tests subscription.active event handling
func TestDodoPayWebhook_SubscriptionActive(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	webhookSecret := "test_webhook_secret"
	os.Setenv("DODOPAY_WEBHOOK_SECRET", webhookSecret)
	defer os.Unsetenv("DODOPAY_WEBHOOK_SECRET")

	handler := NewWebhookHandler(testDB.DB)
	helper := testutil.NewWebhookTestHelper(t, webhookSecret)
	e := echo.New()

	t.Run("upgrades user to premium", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		// Create free tier user
		user := testDB.CreateTestUser(ctx,
			testutil.WithEmail("premium@example.com"),
		)

		assert.Equal(t, "free", user.SubscriptionTier)
		assert.Equal(t, "inactive", user.SubscriptionStatus)

		// Create subscription.active webhook
		nextBilling := time.Now().Add(30 * 24 * time.Hour)
		eventData := testutil.SubscriptionActiveEvent(
			user.Email,
			"sub_new_premium",
			nextBilling,
		)
		req := helper.CreateDodoPayWebhookRequest("subscription.active", eventData)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute
		err := handler.DodoPayWebhook(c)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify user was upgraded
		var updatedUser models.User
		err = testDB.DB.NewSelect().
			Model(&updatedUser).
			Where("id = ?", user.ID).
			Scan(ctx)
		require.NoError(t, err)

		assert.Equal(t, "premium", updatedUser.SubscriptionTier)
		assert.Equal(t, "active", updatedUser.SubscriptionStatus)
		assert.Equal(t, "sub_new_premium", updatedUser.SubscriptionID)
		require.NotNil(t, updatedUser.SubscriptionEndsAt)
		assert.WithinDuration(t, nextBilling, *updatedUser.SubscriptionEndsAt, 1*time.Second)
	})

	t.Run("user not found - returns success with user_not_found", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		eventData := testutil.SubscriptionActiveEvent(
			"nonexistent@example.com",
			"sub_123",
			time.Now().Add(30*24*time.Hour),
		)
		req := helper.CreateDodoPayWebhookRequest("subscription.active", eventData)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute
		err := handler.DodoPayWebhook(c)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]string
		json.Unmarshal(rec.Body.Bytes(), &response)
		assert.Equal(t, "user_not_found", response["status"])
	})
}

// TestDodoPayWebhook_SubscriptionCancelled tests subscription.cancelled event
func TestDodoPayWebhook_SubscriptionCancelled(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	webhookSecret := "test_webhook_secret"
	os.Setenv("DODOPAY_WEBHOOK_SECRET", webhookSecret)
	defer os.Unsetenv("DODOPAY_WEBHOOK_SECRET")

	handler := NewWebhookHandler(testDB.DB)
	helper := testutil.NewWebhookTestHelper(t, webhookSecret)
	e := echo.New()

	t.Run("marks subscription as cancelled", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		// Create premium user
		futureDate := time.Now().Add(30 * 24 * time.Hour)
		user := testDB.CreateTestUser(ctx,
			testutil.WithEmail("cancel@example.com"),
			testutil.WithSubscriptionID("sub_cancel_123"),
			testutil.WithPremiumSubscription(futureDate),
		)

		assert.Equal(t, "active", user.SubscriptionStatus)

		// Send cancellation webhook
		eventData := testutil.SubscriptionCancelledEvent(user.SubscriptionID)
		req := helper.CreateDodoPayWebhookRequest("subscription.cancelled", eventData)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute
		err := handler.DodoPayWebhook(c)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify subscription was cancelled
		var updatedUser models.User
		err = testDB.DB.NewSelect().
			Model(&updatedUser).
			Where("id = ?", user.ID).
			Scan(ctx)
		require.NoError(t, err)

		assert.Equal(t, "cancelled", updatedUser.SubscriptionStatus)
		// Note: Tier remains premium until expiry date
		assert.Equal(t, "premium", updatedUser.SubscriptionTier)
	})
}

// TestDodoPayWebhook_SubscriptionExpired tests subscription.expired event
func TestDodoPayWebhook_SubscriptionExpired(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	webhookSecret := "test_webhook_secret"
	os.Setenv("DODOPAY_WEBHOOK_SECRET", webhookSecret)
	defer os.Unsetenv("DODOPAY_WEBHOOK_SECRET")

	handler := NewWebhookHandler(testDB.DB)
	helper := testutil.NewWebhookTestHelper(t, webhookSecret)
	e := echo.New()

	t.Run("downgrades user to free tier", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		// Create premium user with past expiry
		pastDate := time.Now().Add(-1 * time.Hour)
		user := testDB.CreateTestUser(ctx,
			testutil.WithEmail("expired@example.com"),
			testutil.WithSubscriptionID("sub_expired_123"),
			testutil.WithPremiumSubscription(pastDate),
		)

		// Send expiration webhook
		eventData := testutil.SubscriptionExpiredEvent(user.SubscriptionID)
		req := helper.CreateDodoPayWebhookRequest("subscription.expired", eventData)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute
		err := handler.DodoPayWebhook(c)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify user was downgraded
		var updatedUser models.User
		err = testDB.DB.NewSelect().
			Model(&updatedUser).
			Where("id = ?", user.ID).
			Scan(ctx)
		require.NoError(t, err)

		assert.Equal(t, "free", updatedUser.SubscriptionTier)
		assert.Equal(t, "inactive", updatedUser.SubscriptionStatus)
	})
}

// TestDodoPayWebhook_PaymentSucceeded tests payment.succeeded event
func TestDodoPayWebhook_PaymentSucceeded(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	webhookSecret := "test_webhook_secret"
	os.Setenv("DODOPAY_WEBHOOK_SECRET", webhookSecret)
	defer os.Unsetenv("DODOPAY_WEBHOOK_SECRET")

	handler := NewWebhookHandler(testDB.DB)
	helper := testutil.NewWebhookTestHelper(t, webhookSecret)
	e := echo.New()

	t.Run("activates subscription and extends expiry", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		// Create user with payment_failed status
		oldExpiry := time.Now().Add(5 * 24 * time.Hour)
		user := testDB.CreateTestUser(ctx,
			testutil.WithEmail("payment@example.com"),
			testutil.WithSubscriptionID("sub_payment_123"),
			testutil.WithPremiumSubscription(oldExpiry),
		)

		// Manually set to payment_failed status
		_, err := testDB.DB.NewUpdate().
			Model(&user).
			Set("subscription_status = ?", "payment_failed").
			Where("id = ?", user.ID).
			Exec(ctx)
		require.NoError(t, err)

		// Send payment succeeded webhook with new expiry
		newExpiry := time.Now().Add(35 * 24 * time.Hour)
		eventData := testutil.PaymentSucceededEvent(user.SubscriptionID, newExpiry)
		req := helper.CreateDodoPayWebhookRequest("payment.succeeded", eventData)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute
		err = handler.DodoPayWebhook(c)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify subscription was activated
		var updatedUser models.User
		err = testDB.DB.NewSelect().
			Model(&updatedUser).
			Where("id = ?", user.ID).
			Scan(ctx)
		require.NoError(t, err)

		assert.Equal(t, "active", updatedUser.SubscriptionStatus)
		require.NotNil(t, updatedUser.SubscriptionEndsAt)
		assert.WithinDuration(t, newExpiry, *updatedUser.SubscriptionEndsAt, 1*time.Second)
	})
}

// TestDodoPayWebhook_PaymentFailed tests payment.failed event
func TestDodoPayWebhook_PaymentFailed(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	webhookSecret := "test_webhook_secret"
	os.Setenv("DODOPAY_WEBHOOK_SECRET", webhookSecret)
	defer os.Unsetenv("DODOPAY_WEBHOOK_SECRET")

	handler := NewWebhookHandler(testDB.DB)
	helper := testutil.NewWebhookTestHelper(t, webhookSecret)
	e := echo.New()

	t.Run("marks as payment_failed without downgrading", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		// Create active premium user
		futureDate := time.Now().Add(30 * 24 * time.Hour)
		user := testDB.CreateTestUser(ctx,
			testutil.WithEmail("failed@example.com"),
			testutil.WithSubscriptionID("sub_failed_123"),
			testutil.WithPremiumSubscription(futureDate),
		)

		// Send payment failed webhook
		eventData := testutil.PaymentFailedEvent(user.SubscriptionID)
		req := helper.CreateDodoPayWebhookRequest("payment.failed", eventData)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute
		err := handler.DodoPayWebhook(c)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify status changed but tier remains
		var updatedUser models.User
		err = testDB.DB.NewSelect().
			Model(&updatedUser).
			Where("id = ?", user.ID).
			Scan(ctx)
		require.NoError(t, err)

		assert.Equal(t, "payment_failed", updatedUser.SubscriptionStatus)
		assert.Equal(t, "premium", updatedUser.SubscriptionTier) // Grace period
	})
}

// TestDodoPayWebhook_SubscriptionRenewed tests subscription.renewed event
func TestDodoPayWebhook_SubscriptionRenewed(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	webhookSecret := "test_webhook_secret"
	os.Setenv("DODOPAY_WEBHOOK_SECRET", webhookSecret)
	defer os.Unsetenv("DODOPAY_WEBHOOK_SECRET")

	handler := NewWebhookHandler(testDB.DB)
	helper := testutil.NewWebhookTestHelper(t, webhookSecret)
	e := echo.New()

	t.Run("extends subscription expiry", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		// Create premium user
		oldExpiry := time.Now().Add(5 * 24 * time.Hour)
		user := testDB.CreateTestUser(ctx,
			testutil.WithEmail("renew@example.com"),
			testutil.WithSubscriptionID("sub_renew_123"),
			testutil.WithPremiumSubscription(oldExpiry),
		)

		// Send renewal webhook
		newExpiry := time.Now().Add(35 * 24 * time.Hour)
		eventData := testutil.SubscriptionRenewedEvent(user.SubscriptionID, newExpiry)
		req := helper.CreateDodoPayWebhookRequest("subscription.renewed", eventData)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		// Execute
		err := handler.DodoPayWebhook(c)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify expiry was extended
		var updatedUser models.User
		err = testDB.DB.NewSelect().
			Model(&updatedUser).
			Where("id = ?", user.ID).
			Scan(ctx)
		require.NoError(t, err)

		assert.Equal(t, "active", updatedUser.SubscriptionStatus)
		require.NotNil(t, updatedUser.SubscriptionEndsAt)
		assert.WithinDuration(t, newExpiry, *updatedUser.SubscriptionEndsAt, 1*time.Second)
		assert.True(t, updatedUser.SubscriptionEndsAt.After(oldExpiry))
	})
}

// TestDodoPayWebhook_AllEventTypes tests all webhook event types
func TestDodoPayWebhook_AllEventTypes(t *testing.T) {
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	webhookSecret := "test_webhook_secret"
	os.Setenv("DODOPAY_WEBHOOK_SECRET", webhookSecret)
	defer os.Unsetenv("DODOPAY_WEBHOOK_SECRET")

	handler := NewWebhookHandler(testDB.DB)
	helper := testutil.NewWebhookTestHelper(t, webhookSecret)
	e := echo.New()

	tests := []struct {
		name          string
		eventType     string
		setupUser     func() *models.User
		createEvent   func(user *models.User) map[string]interface{}
		validateUser  func(t *testing.T, user *models.User)
		expectStatus  int
		expectSuccess bool
	}{
		{
			name:      "subscription.on_hold",
			eventType: "subscription.on_hold",
			setupUser: func() *models.User {
				return testDB.CreateTestUser(ctx,
					testutil.WithSubscriptionID("sub_hold"),
					testutil.WithPremiumSubscription(time.Now().Add(30*24*time.Hour)),
				)
			},
			createEvent: func(user *models.User) map[string]interface{} {
				return testutil.SubscriptionOnHoldEvent(user.SubscriptionID)
			},
			validateUser: func(t *testing.T, user *models.User) {
				var updated models.User
				testDB.DB.NewSelect().Model(&updated).Where("id = ?", user.ID).Scan(ctx)
				assert.Equal(t, "on_hold", updated.SubscriptionStatus)
			},
			expectStatus:  http.StatusOK,
			expectSuccess: true,
		},
		{
			name:      "payment.cancelled - no action",
			eventType: "payment.cancelled",
			setupUser: func() *models.User {
				return testDB.CreateTestUser(ctx,
					testutil.WithSubscriptionID("sub_cancel_payment"),
				)
			},
			createEvent: func(user *models.User) map[string]interface{} {
				return testutil.PaymentCancelledEvent(user.SubscriptionID)
			},
			validateUser: func(t *testing.T, user *models.User) {
				// Should not change user status
			},
			expectStatus:  http.StatusOK,
			expectSuccess: true,
		},
		{
			name:      "payment.processing - no action",
			eventType: "payment.processing",
			setupUser: func() *models.User {
				return testDB.CreateTestUser(ctx,
					testutil.WithSubscriptionID("sub_processing"),
				)
			},
			createEvent: func(user *models.User) map[string]interface{} {
				return testutil.PaymentProcessingEvent(user.SubscriptionID)
			},
			validateUser: func(t *testing.T, user *models.User) {
				// Should not change user status
			},
			expectStatus:  http.StatusOK,
			expectSuccess: true,
		},
		{
			name:      "unknown event type - ignored",
			eventType: "unknown.event.type",
			setupUser: func() *models.User {
				return testDB.CreateTestUser(ctx,
					testutil.WithSubscriptionID("sub_unknown"),
				)
			},
			createEvent: func(user *models.User) map[string]interface{} {
				return map[string]interface{}{
					"subscription_id": user.SubscriptionID,
				}
			},
			validateUser: func(t *testing.T, user *models.User) {
				// Should not change user status
			},
			expectStatus:  http.StatusOK,
			expectSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDB.TruncateTables(ctx)

			user := tt.setupUser()
			eventData := tt.createEvent(user)

			req := helper.CreateDodoPayWebhookRequest(tt.eventType, eventData)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.DodoPayWebhook(c)

			if tt.expectSuccess {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.expectStatus, rec.Code)

			tt.validateUser(t, user)
		})
	}
}
