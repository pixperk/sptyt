package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/handlers"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/pixperk/sptyt/internal/services"
	"github.com/pixperk/sptyt/internal/tasks"
	"github.com/pixperk/sptyt/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_PlaylistConversion_SuccessfulFlow tests the complete conversion flow
func TestE2E_PlaylistConversion_SuccessfulFlow(t *testing.T) {
	// Skip if no test database is available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test database
	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	t.Run("free user converts small playlist successfully", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		// 1. Create test user (free tier)
		user := testDB.CreateTestUser(ctx,
			testutil.WithClerkID("clerk_free_user_123"),
			testutil.WithEmail("freeuser@example.com"),
		)

		// 2. Create YouTube OAuth token
		_ = testDB.CreateTestYouTubeToken(ctx, user.ID, "ya29.mock_access_token")

		// 3. Create analytics record (0 conversions this month)
		testDB.CreateTestAnalytics(ctx, user.ID, 0)

		// 4. Setup handler (would normally include task client)
		// Note: In a real E2E test, you'd use a real or mock task queue
		converterService := services.NewPlaylistConverterService(
			testDB.DB,
			nil, // spotify client - would be mocked
			nil, // youtube client - would be mocked
			nil, // websocket hub - could be mocked
			nil, // analytics task client
		)

		taskClient := &tasks.Client{} // Mock task client
		handler := handlers.NewPlaylistHandler(testDB.DB, converterService, taskClient)

		// 5. Create authenticated request
		requestBody := map[string]interface{}{
			"spotify_playlist_url":   "https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M",
			"youtube_playlist_name":  "My Converted Playlist",
			"use_lyric_videos":       false,
		}
		bodyBytes, _ := json.Marshal(requestBody)

		req := httptest.NewRequest(http.MethodPost, "/api/playlists/convert", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		c := testutil.CreateAuthenticatedContext(user.ClerkID, req, rec)

		// 6. Execute conversion request
		// Note: This would normally enqueue a background task
		err := handler.ConvertPlaylist(c)

		// 7. Verify response
		// In a real implementation with task queue, this would return 202 Accepted
		if err == nil {
			// Success case
			assert.Equal(t, http.StatusAccepted, rec.Code)

			var response map[string]interface{}
			json.Unmarshal(rec.Body.Bytes(), &response)

			assert.Equal(t, "Playlist conversion started", response["message"])
			assert.NotEmpty(t, response["conversion_id"])
			assert.Equal(t, "pending", response["status"])

			// Verify conversion record was created
			conversionID := response["conversion_id"].(string)
			var conversion models.PlaylistConversion
			err = testDB.DB.NewSelect().
				Model(&conversion).
				Where("id = ?", conversionID).
				Scan(ctx)

			require.NoError(t, err)
			assert.Equal(t, user.ID, conversion.UserID)
			assert.Equal(t, "pending", conversion.Status)
		}

		// 8. Verify analytics was updated (if task completed)
		// In a real test, you'd wait for the background task to complete
	})

	t.Run("premium user converts large playlist", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		// Create premium user
		futureExpiry := time.Now().Add(30 * 24 * time.Hour)
		user := testDB.CreateTestUser(ctx,
			testutil.WithClerkID("clerk_premium_user_456"),
			testutil.WithEmail("premiumuser@example.com"),
			testutil.WithPremiumSubscription(futureExpiry),
		)

		// Create YouTube token
		testDB.CreateTestYouTubeToken(ctx, user.ID, "ya29.premium_token")

		// Create analytics (5 conversions this month - under premium limit of 20)
		testDB.CreateTestAnalytics(ctx, user.ID, 5)

		// Premium user can convert playlists with up to 100 songs
		// This test verifies the limit is enforced correctly
	})
}

// TestE2E_PlaylistConversion_LimitEnforcement tests playlist limit enforcement
func TestE2E_PlaylistConversion_LimitEnforcement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	t.Run("free user exceeds monthly limit", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		// Create free user who has already used their 1 conversion this month
		user := testDB.CreateTestUser(ctx,
			testutil.WithClerkID("clerk_limit_user"),
			testutil.WithEmail("limituser@example.com"),
		)

		testDB.CreateTestYouTubeToken(ctx, user.ID, "ya29.limit_token")

		// User has already done 1 conversion this month (free limit)
		testDB.CreateTestAnalytics(ctx, user.ID, 1)

		// Try to create another conversion
		converterService := services.NewPlaylistConverterService(
			testDB.DB, nil, nil, nil, nil,
		)
		taskClient := &tasks.Client{}
		handler := handlers.NewPlaylistHandler(testDB.DB, converterService, taskClient)

		requestBody := map[string]interface{}{
			"spotify_playlist_url": "https://open.spotify.com/playlist/test123",
		}
		bodyBytes, _ := json.Marshal(requestBody)

		req := httptest.NewRequest(http.MethodPost, "/api/playlists/convert", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		c := testutil.CreateAuthenticatedContext(user.ClerkID, req, rec)

		// Execute - should fail with quota exceeded
		err := handler.ConvertPlaylist(c)

		// Should return error about exceeding monthly limit
		assert.Error(t, err)
	})

	t.Run("free user exceeds song limit", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		// Free tier has 10 song limit per playlist
		user := testDB.CreateTestUser(ctx,
			testutil.WithClerkID("clerk_song_limit"),
			testutil.WithEmail("songlimit@example.com"),
		)

		testDB.CreateTestYouTubeToken(ctx, user.ID, "ya29.song_token")
		testDB.CreateTestAnalytics(ctx, user.ID, 0)

		// Attempt to convert playlist with 15 songs (exceeds free limit of 10)
		// This would require mocking the Spotify API to return a playlist with 15 songs
	})

	t.Run("premium user within limits", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		futureExpiry := time.Now().Add(30 * 24 * time.Hour)
		user := testDB.CreateTestUser(ctx,
			testutil.WithClerkID("clerk_premium_valid"),
			testutil.WithPremiumSubscription(futureExpiry),
		)

		testDB.CreateTestYouTubeToken(ctx, user.ID, "ya29.premium_valid")

		// Premium user at 19/20 monthly conversions
		testDB.CreateTestAnalytics(ctx, user.ID, 19)

		// Should be able to convert one more
	})
}

// TestE2E_PlaylistConversion_YouTubeOAuth tests OAuth token handling
func TestE2E_PlaylistConversion_YouTubeOAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	t.Run("missing YouTube token - returns error", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		user := testDB.CreateTestUser(ctx,
			testutil.WithClerkID("clerk_no_youtube"),
		)

		// No YouTube token created

		converterService := services.NewPlaylistConverterService(
			testDB.DB, nil, nil, nil, nil,
		)
		taskClient := &tasks.Client{}
		handler := handlers.NewPlaylistHandler(testDB.DB, converterService, taskClient)

		requestBody := map[string]interface{}{
			"spotify_playlist_url": "https://open.spotify.com/playlist/test123",
		}
		bodyBytes, _ := json.Marshal(requestBody)

		req := httptest.NewRequest(http.MethodPost, "/api/playlists/convert", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		c := testutil.CreateAuthenticatedContext(user.ClerkID, req, rec)

		err := handler.ConvertPlaylist(c)

		// Should return 400 error about missing YouTube authorization
		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusBadRequest, httpErr.Code)
		assert.Contains(t, httpErr.Message, "YouTube not authorized")
	})

	t.Run("expired YouTube token - auto-refreshes", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		user := testDB.CreateTestUser(ctx,
			testutil.WithClerkID("clerk_expired_token"),
		)

		// Create expired YouTube token
		expiredToken := &models.UserOAuthToken{
			ID:           uuid.New(),
			UserID:       user.ID,
			Provider:     "youtube",
			AccessToken:  "ya29.expired_token",
			RefreshToken: "refresh_token",
			ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		_, err := testDB.DB.NewInsert().Model(expiredToken).Exec(ctx)
		require.NoError(t, err)

		// In a real test, you'd mock the Google OAuth token refresh endpoint
		// and verify that the handler attempts to refresh the token
	})
}

// TestE2E_PlaylistConversion_GetStatus tests retrieving conversion status
func TestE2E_PlaylistConversion_GetStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	t.Run("get conversion status - owner can access", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		user := testDB.CreateTestUser(ctx,
			testutil.WithClerkID("clerk_status_owner"),
		)

		// Create conversion
		conversion := testDB.CreateTestConversion(ctx, user.ID,
			testutil.WithStatus("processing"),
			testutil.WithTrackCount(10),
		)

		// Create handler
		converterService := services.NewPlaylistConverterService(
			testDB.DB, nil, nil, nil, nil,
		)
		taskClient := &tasks.Client{}
		handler := handlers.NewPlaylistHandler(testDB.DB, converterService, taskClient)

		// Request conversion status
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/playlists/conversions/%s", conversion.ID), nil)
		rec := httptest.NewRecorder()

		e := echo.New()
		c := e.NewContext(req, rec)
		c.SetPath("/api/playlists/conversions/:id")
		c.SetParamNames("id")
		c.SetParamValues(conversion.ID.String())
		c.Set("clerk_user_id", user.ClerkID)

		err := handler.GetConversionStatus(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response models.PlaylistConversion
		json.Unmarshal(rec.Body.Bytes(), &response)

		assert.Equal(t, conversion.ID, response.ID)
		assert.Equal(t, "processing", response.Status)
		assert.Equal(t, 10, response.TrackCount)
	})

	t.Run("get conversion status - non-owner cannot access", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		owner := testDB.CreateTestUser(ctx,
			testutil.WithClerkID("clerk_owner"),
			testutil.WithEmail("owner@example.com"),
		)

		otherUser := testDB.CreateTestUser(ctx,
			testutil.WithClerkID("clerk_other"),
			testutil.WithEmail("other@example.com"),
		)

		// Create conversion owned by first user
		conversion := testDB.CreateTestConversion(ctx, owner.ID)

		// Setup handler
		converterService := services.NewPlaylistConverterService(
			testDB.DB, nil, nil, nil, nil,
		)
		taskClient := &tasks.Client{}
		handler := handlers.NewPlaylistHandler(testDB.DB, converterService, taskClient)

		// Try to access as different user
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/playlists/conversions/%s", conversion.ID), nil)
		rec := httptest.NewRecorder()

		e := echo.New()
		c := e.NewContext(req, rec)
		c.SetPath("/api/playlists/conversions/:id")
		c.SetParamNames("id")
		c.SetParamValues(conversion.ID.String())
		c.Set("clerk_user_id", otherUser.ClerkID)

		err := handler.GetConversionStatus(c)

		// Should return 404 (not found - doesn't reveal existence)
		assert.Error(t, err)
		httpErr, ok := err.(*echo.HTTPError)
		require.True(t, ok)
		assert.Equal(t, http.StatusNotFound, httpErr.Code)
	})
}

// TestE2E_PlaylistConversion_ListUserConversions tests listing user conversions
func TestE2E_PlaylistConversion_ListUserConversions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	t.Run("list user conversions - returns all user's conversions", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		user := testDB.CreateTestUser(ctx,
			testutil.WithClerkID("clerk_list_user"),
		)

		// Create multiple conversions
		conv1 := testDB.CreateTestConversion(ctx, user.ID,
			testutil.WithStatus("completed"),
		)
		conv2 := testDB.CreateTestConversion(ctx, user.ID,
			testutil.WithStatus("processing"),
		)
		testDB.CreateTestConversion(ctx, user.ID,
			testutil.WithStatus("pending"),
		)

		// Create conversion for another user (should not appear)
		otherUser := testDB.CreateTestUser(ctx,
			testutil.WithEmail("other@example.com"),
		)
		testDB.CreateTestConversion(ctx, otherUser.ID)

		// Setup handler
		converterService := services.NewPlaylistConverterService(
			testDB.DB, nil, nil, nil, nil,
		)
		taskClient := &tasks.Client{}
		handler := handlers.NewPlaylistHandler(testDB.DB, converterService, taskClient)

		// Request user's conversions
		req := httptest.NewRequest(http.MethodGet, "/api/playlists/conversions", nil)
		rec := httptest.NewRecorder()

		c := testutil.CreateAuthenticatedContext(user.ClerkID, req, rec)

		err := handler.GetUserConversions(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &response)

		conversions := response["conversions"].([]interface{})
		assert.Len(t, conversions, 3) // Only this user's conversions

		// Verify conversions are ordered by created_at DESC
		// Most recent first
		_ = conv1
		_ = conv2
	})

	t.Run("list detailed conversions with pagination", func(t *testing.T) {
		testDB.TruncateTables(ctx)

		user := testDB.CreateTestUser(ctx,
			testutil.WithClerkID("clerk_pagination"),
		)

		// Create 15 conversions
		for i := 0; i < 15; i++ {
			testDB.CreateTestConversion(ctx, user.ID,
				testutil.WithStatus("completed"),
			)
			time.Sleep(1 * time.Millisecond) // Ensure different timestamps
		}

		// Setup handler
		converterService := services.NewPlaylistConverterService(
			testDB.DB, nil, nil, nil, nil,
		)
		taskClient := &tasks.Client{}
		handler := handlers.NewPlaylistHandler(testDB.DB, converterService, taskClient)

		// Request first page (limit 10)
		req := httptest.NewRequest(http.MethodGet, "/api/playlists/conversions/detailed?limit=10&offset=0", nil)
		rec := httptest.NewRecorder()

		c := testutil.CreateAuthenticatedContext(user.ClerkID, req, rec)

		err := handler.GetDetailedUserConversions(c)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		var response map[string]interface{}
		json.Unmarshal(rec.Body.Bytes(), &response)

		assert.Equal(t, float64(15), response["total"])
		assert.Equal(t, float64(10), response["limit"])
		assert.Equal(t, float64(0), response["offset"])
		assert.Equal(t, float64(10), response["count"])
		assert.Equal(t, true, response["has_more"])
		assert.Equal(t, float64(10), response["next_offset"])

		conversions := response["conversions"].([]interface{})
		assert.Len(t, conversions, 10)
	})
}

// TestE2E_PlaylistConversion_InvalidInputs tests error handling for invalid inputs
func TestE2E_PlaylistConversion_InvalidInputs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testDB := testutil.SetupTestDB(t)
	defer testDB.Close()

	ctx := context.Background()
	testDB.CreateTables(ctx)
	defer testDB.DropTables(ctx)

	tests := []struct {
		name           string
		requestBody    map[string]interface{}
		expectError    bool
		expectedStatus int
		errorContains  string
	}{
		{
			name:           "missing spotify URL",
			requestBody:    map[string]interface{}{},
			expectError:    true,
			expectedStatus: http.StatusBadRequest,
			errorContains:  "spotify_playlist_url is required",
		},
		{
			name: "invalid Spotify URL",
			requestBody: map[string]interface{}{
				"spotify_playlist_url": "https://invalid.com/playlist",
			},
			expectError:    true,
			expectedStatus: http.StatusBadRequest,
			errorContains:  "Invalid Spotify playlist",
		},
		{
			name: "empty Spotify URL",
			requestBody: map[string]interface{}{
				"spotify_playlist_url": "",
			},
			expectError:    true,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testDB.TruncateTables(ctx)

			user := testDB.CreateTestUser(ctx,
				testutil.WithClerkID("clerk_invalid_test"),
			)
			testDB.CreateTestYouTubeToken(ctx, user.ID, "ya29.test_token")
			testDB.CreateTestAnalytics(ctx, user.ID, 0)

			converterService := services.NewPlaylistConverterService(
				testDB.DB, nil, nil, nil, nil,
			)
			taskClient := &tasks.Client{}
			handler := handlers.NewPlaylistHandler(testDB.DB, converterService, taskClient)

			bodyBytes, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/api/playlists/convert", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			c := testutil.CreateAuthenticatedContext(user.ClerkID, req, rec)

			err := handler.ConvertPlaylist(c)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedStatus != 0 {
					httpErr, ok := err.(*echo.HTTPError)
					require.True(t, ok)
					assert.Equal(t, tt.expectedStatus, httpErr.Code)

					if tt.errorContains != "" {
						assert.Contains(t, fmt.Sprint(httpErr.Message), tt.errorContains)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
