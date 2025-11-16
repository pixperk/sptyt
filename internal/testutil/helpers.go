package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

// TestDB holds a test database connection
type TestDB struct {
	DB *bun.DB
	T  *testing.T
}

// SetupTestDB creates a test database connection
// Uses environment variables or defaults:
// - TEST_DB_HOST (default: localhost)
// - TEST_DB_PORT (default: 5432)
// - TEST_DB_USER (default: postgres)
// - TEST_DB_PASSWORD (default: postgres)
// - TEST_DB_NAME (default: sptyt_test)
func SetupTestDB(t *testing.T) *TestDB {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		getEnv("TEST_DB_USER", "postgres"),
		getEnv("TEST_DB_PASSWORD", "postgres"),
		getEnv("TEST_DB_HOST", "localhost"),
		getEnv("TEST_DB_PORT", "5432"),
		getEnv("TEST_DB_NAME", "sptyt_test"),
	)

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))

	// Configure connection pool for tests
	sqldb.SetMaxOpenConns(10)
	sqldb.SetMaxIdleConns(5)
	sqldb.SetConnMaxLifetime(5 * time.Minute)

	db := bun.NewDB(sqldb, pgdialect.New())

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("Failed to connect to test database: %v\nDSN: %s", err, dsn)
	}

	log.Printf("Test database connected: %s", getEnv("TEST_DB_NAME", "sptyt_test"))

	return &TestDB{
		DB: db,
		T:  t,
	}
}

// Close closes the test database connection
func (tdb *TestDB) Close() {
	if err := tdb.DB.Close(); err != nil {
		tdb.T.Logf("Failed to close test database: %v", err)
	}
}

// CreateTables creates all necessary tables for testing
func (tdb *TestDB) CreateTables(ctx context.Context) {
	// Create users table
	_, err := tdb.DB.NewCreateTable().
		Model((*models.User)(nil)).
		IfNotExists().
		Exec(ctx)
	require.NoError(tdb.T, err, "Failed to create users table")

	// Create playlist_conversions table
	_, err = tdb.DB.NewCreateTable().
		Model((*models.PlaylistConversion)(nil)).
		IfNotExists().
		Exec(ctx)
	require.NoError(tdb.T, err, "Failed to create playlist_conversions table")

	// Create user_analytics table
	_, err = tdb.DB.NewCreateTable().
		Model((*models.UserAnalytics)(nil)).
		IfNotExists().
		Exec(ctx)
	require.NoError(tdb.T, err, "Failed to create user_analytics table")

	// Create oauth_tokens table
	_, err = tdb.DB.NewCreateTable().
		Model((*models.UserOAuthToken)(nil)).
		IfNotExists().
		Exec(ctx)
	require.NoError(tdb.T, err, "Failed to create oauth_tokens table")

	// Create custom_links table
	_, err = tdb.DB.NewCreateTable().
		Model((*models.CustomLink)(nil)).
		IfNotExists().
		Exec(ctx)
	require.NoError(tdb.T, err, "Failed to create custom_links table")

	// Create link_elements table
	_, err = tdb.DB.NewCreateTable().
		Model((*models.LinkElement)(nil)).
		IfNotExists().
		Exec(ctx)
	require.NoError(tdb.T, err, "Failed to create link_elements table")

	log.Println("Test tables created successfully")
}

// DropTables drops all test tables
func (tdb *TestDB) DropTables(ctx context.Context) {
	tables := []string{
		"link_elements",
		"custom_links",
		"oauth_tokens",
		"user_analytics",
		"playlist_conversions",
		"users",
	}

	for _, table := range tables {
		_, err := tdb.DB.NewDropTable().
			Table(table).
			IfExists().
			Cascade().
			Exec(ctx)
		if err != nil {
			tdb.T.Logf("Warning: Failed to drop table %s: %v", table, err)
		}
	}

	log.Println("Test tables dropped")
}

// TruncateTables truncates all test tables (faster than drop/create)
func (tdb *TestDB) TruncateTables(ctx context.Context) {
	tables := []string{
		"link_elements",
		"custom_links",
		"oauth_tokens",
		"user_analytics",
		"playlist_conversions",
		"users",
	}

	for _, table := range tables {
		_, err := tdb.DB.NewRaw(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)).Exec(ctx)
		if err != nil {
			tdb.T.Logf("Warning: Failed to truncate table %s: %v", table, err)
		}
	}
}

// CreateTestUser creates a test user in the database
func (tdb *TestDB) CreateTestUser(ctx context.Context, opts ...UserOption) *models.User {
	user := &models.User{
		ID:                 uuid.New(),
		ClerkID:            fmt.Sprintf("clerk_test_%s", uuid.New().String()[:8]),
		Email:              fmt.Sprintf("test_%s@example.com", uuid.New().String()[:8]),
		FirstName:          "Test",
		LastName:           "User",
		SubscriptionTier:   "free",
		SubscriptionStatus: "inactive",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	// Apply options
	for _, opt := range opts {
		opt(user)
	}

	_, err := tdb.DB.NewInsert().Model(user).Exec(ctx)
	require.NoError(tdb.T, err, "Failed to create test user")

	return user
}

// UserOption is a functional option for creating test users
type UserOption func(*models.User)

// WithPremiumSubscription sets the user as a premium subscriber
func WithPremiumSubscription(endsAt time.Time) UserOption {
	return func(u *models.User) {
		u.SubscriptionTier = "premium"
		u.SubscriptionStatus = "active"
		u.SubscriptionID = fmt.Sprintf("sub_test_%s", uuid.New().String()[:8])
		u.SubscriptionEndsAt = &endsAt
	}
}

// WithEmail sets a custom email
func WithEmail(email string) UserOption {
	return func(u *models.User) {
		u.Email = email
	}
}

// WithClerkID sets a custom Clerk ID
func WithClerkID(clerkID string) UserOption {
	return func(u *models.User) {
		u.ClerkID = clerkID
	}
}

// WithSubscriptionID sets a subscription ID
func WithSubscriptionID(subID string) UserOption {
	return func(u *models.User) {
		u.SubscriptionID = subID
	}
}

// CreateTestYouTubeToken creates a test YouTube OAuth token
func (tdb *TestDB) CreateTestYouTubeToken(ctx context.Context, userID uuid.UUID, accessToken string) *models.UserOAuthToken {
	token := &models.UserOAuthToken{
		ID:           uuid.New(),
		UserID:       userID,
		Provider:     "youtube",
		AccessToken:  accessToken,
		RefreshToken: "refresh_token_test",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	_, err := tdb.DB.NewInsert().Model(token).Exec(ctx)
	require.NoError(tdb.T, err, "Failed to create test YouTube token")

	return token
}

// CreateTestConversion creates a test playlist conversion
func (tdb *TestDB) CreateTestConversion(ctx context.Context, userID uuid.UUID, opts ...ConversionOption) *models.PlaylistConversion {
	conversion := &models.PlaylistConversion{
		ID:                 uuid.New(),
		UserID:             userID,
		SpotifyPlaylistID:  fmt.Sprintf("spotify_pl_%s", uuid.New().String()[:8]),
		SpotifyPlaylistURL: "https://open.spotify.com/playlist/test123",
		PlaylistName:       "Test Playlist",
		TrackCount:         5,
		Status:             "pending",
		CountsAgainstQuota: true,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	// Apply options
	for _, opt := range opts {
		opt(conversion)
	}

	_, err := tdb.DB.NewInsert().Model(conversion).Exec(ctx)
	require.NoError(tdb.T, err, "Failed to create test conversion")

	return conversion
}

// ConversionOption is a functional option for creating test conversions
type ConversionOption func(*models.PlaylistConversion)

// WithStatus sets the conversion status
func WithStatus(status string) ConversionOption {
	return func(c *models.PlaylistConversion) {
		c.Status = status
	}
}

// WithTrackCount sets the track count
func WithTrackCount(count int) ConversionOption {
	return func(c *models.PlaylistConversion) {
		c.TrackCount = count
	}
}

// WithYouTubePlaylist sets YouTube playlist info
func WithYouTubePlaylist(playlistID, playlistURL string) ConversionOption {
	return func(c *models.PlaylistConversion) {
		c.YouTubePlaylistID = playlistID
		c.YouTubePlaylistURL = playlistURL
	}
}

// WithConversionLog sets the conversion log
func WithConversionLog(logs []models.TrackConversionLog) ConversionOption {
	return func(c *models.PlaylistConversion) {
		c.ConversionLog = logs
	}
}

// CreateTestAnalytics creates test user analytics
func (tdb *TestDB) CreateTestAnalytics(ctx context.Context, userID uuid.UUID, monthlyConversions int) *models.UserAnalytics {
	now := time.Now()
	analytics := &models.UserAnalytics{
		UserID:             userID,
		MonthlyConversions: monthlyConversions,
		CurrentMonth:       int(now.Month()),
		CurrentYear:        now.Year(),
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	_, err := tdb.DB.NewInsert().Model(analytics).Exec(ctx)
	require.NoError(tdb.T, err, "Failed to create test analytics")

	return analytics
}

// GenerateMockClerkJWT generates a mock Clerk JWT token for testing
// Note: This does NOT create a real signed JWT - use only for mock verification
func GenerateMockClerkJWT(userID string) string {
	// In real tests, you'd use the Clerk SDK or a JWT library to create a proper token
	// For testing purposes, this returns a placeholder that can be used with mocked auth
	return fmt.Sprintf("mock_jwt_token_%s", userID)
}

// CreateAuthenticatedContext creates an Echo context with authenticated user
func CreateAuthenticatedContext(clerkUserID string, req *http.Request, rec *httptest.ResponseRecorder) echo.Context {
	e := echo.New()
	c := e.NewContext(req, rec)
	c.Set("clerk_user_id", clerkUserID)
	c.Set("clerk_session_id", "test_session_id")
	return c
}

// CreateUnauthenticatedContext creates an Echo context without authentication
func CreateUnauthenticatedContext(req *http.Request, rec *httptest.ResponseRecorder) echo.Context {
	e := echo.New()
	return e.NewContext(req, rec)
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// WaitForCondition waits for a condition to be true with timeout
func WaitForCondition(t *testing.T, timeout time.Duration, check func() bool, message string) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		if check() {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("Timeout waiting for: %s", message)
		}

		<-ticker.C
	}
}

// AssertEventuallyTrue retries a condition multiple times before failing
func AssertEventuallyTrue(t *testing.T, condition func() bool, retries int, waitBetween time.Duration, message string) {
	for i := 0; i < retries; i++ {
		if condition() {
			return
		}
		time.Sleep(waitBetween)
	}
	t.Fatalf("Condition never became true: %s", message)
}
