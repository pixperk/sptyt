package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pixperk/sptyt/internal/crypto"
	"github.com/uptrace/bun"
)

// oauthToken represents the OAuth token stored in the database
// Using a local type to avoid import cycle with models package
type oauthToken struct {
	bun.BaseModel `bun:"table:oauth_tokens"`

	ID           uuid.UUID `bun:"id,pk,type:uuid"`
	UserID       uuid.UUID `bun:"user_id,type:uuid"`
	Provider     string    `bun:"provider"`
	AccessToken  string    `bun:"access_token"`
	RefreshToken string    `bun:"refresh_token"`
	ExpiresAt    time.Time `bun:"expires_at"`
	Scope        string    `bun:"scope"`
	AccountEmail string    `bun:"account_email"`
	CreatedAt    time.Time `bun:"created_at"`
	UpdatedAt    time.Time `bun:"updated_at"`
}

// TokenManager handles YouTube OAuth token retrieval and auto-refresh
type TokenManager struct {
	db     *bun.DB
	userID uuid.UUID
	mu     sync.Mutex

	// Cached token data
	accessToken string
	expiresAt   time.Time
	tokenID     uuid.UUID
}

// NewTokenManager creates a new token manager for a user
func NewTokenManager(db *bun.DB, userID uuid.UUID) *TokenManager {
	return &TokenManager{
		db:     db,
		userID: userID,
	}
}

// GetAccessToken returns a valid access token, refreshing if necessary
// This is safe to call concurrently
func (tm *TokenManager) GetAccessToken(ctx context.Context) (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Check if we have a cached token that's still valid (with 5 min buffer)
	if tm.accessToken != "" && time.Until(tm.expiresAt) > 5*time.Minute {
		return tm.accessToken, nil
	}

	// Need to fetch/refresh token from database
	var token oauthToken
	err := tm.db.NewSelect().
		Model(&token).
		Where("user_id = ?", tm.userID).
		Where("provider = ?", "youtube").
		Scan(ctx)

	if err != nil {
		return "", fmt.Errorf("failed to get YouTube token: %w", err)
	}

	tm.tokenID = token.ID

	// Decrypt tokens read from DB
	if access, err := crypto.Decrypt(token.AccessToken); err == nil {
		token.AccessToken = access
	}
	if refresh, err := crypto.Decrypt(token.RefreshToken); err == nil {
		token.RefreshToken = refresh
	}

	// Check if token is expired or expiring soon
	if time.Until(token.ExpiresAt) < 5*time.Minute {
		// Need to refresh
		refreshedToken, err := tm.refreshToken(ctx, &token)
		if err != nil {
			return "", fmt.Errorf("failed to refresh token: %w", err)
		}
		tm.accessToken = refreshedToken.AccessToken
		tm.expiresAt = refreshedToken.ExpiresAt
	} else {
		// Token is still valid
		tm.accessToken = token.AccessToken
		tm.expiresAt = token.ExpiresAt
	}

	return tm.accessToken, nil
}

// refreshToken refreshes the YouTube OAuth token
func (tm *TokenManager) refreshToken(ctx context.Context, token *oauthToken) (*oauthToken, error) {
	clientID := os.Getenv("YOUTUBE_OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("YOUTUBE_OAUTH_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("youtube oauth credentials not configured")
	}

	if token.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available - user needs to reconnect YouTube")
	}

	// Prepare token refresh request
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("refresh_token", token.RefreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token refresh failed (status %d): %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	// Keep plaintext for in-memory use
	token.AccessToken = tokenResp.AccessToken
	token.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	token.UpdatedAt = time.Now()

	// Encrypt before persisting to DB
	encAccessToken, err := crypto.Encrypt(tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt token: %w", err)
	}

	_, err = tm.db.NewUpdate().
		TableExpr("oauth_tokens").
		Set("access_token = ?", encAccessToken).
		Set("expires_at = ?", token.ExpiresAt).
		Set("updated_at = ?", token.UpdatedAt).
		Where("id = ?", token.ID).
		Exec(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to update token in database: %w", err)
	}

	return token, nil
}
