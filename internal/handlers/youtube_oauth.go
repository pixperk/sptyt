package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/pixperk/sptyt/internal/auth"
	"github.com/pixperk/sptyt/internal/cache"
	"github.com/pixperk/sptyt/internal/models"
	"github.com/uptrace/bun"
)

const youtubeScopes = "https://www.googleapis.com/auth/youtube openid profile email"

type YouTubeOAuthHandler struct {
	db           *bun.DB
	cache        *cache.RedisCache
	clientID     string
	clientSecret string
	redirectURI  string
}

func NewYouTubeOAuthHandler(db *bun.DB, redisCache *cache.RedisCache) *YouTubeOAuthHandler {
	return &YouTubeOAuthHandler{
		db:           db,
		cache:        redisCache,
		clientID:     os.Getenv("YOUTUBE_OAUTH_CLIENT_ID"),
		clientSecret: os.Getenv("YOUTUBE_OAUTH_CLIENT_SECRET"),
		redirectURI:  os.Getenv("YOUTUBE_OAUTH_REDIRECT_URI"),
	}
}

// generateState generates a random state token for OAuth security
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// Authorize initiates the YouTube OAuth flow
func (h *YouTubeOAuthHandler) Authorize(c echo.Context) error {
	// Get authenticated user
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	// Generate state token
	state, err := generateState()
	if err != nil {
		log.Printf("Failed to generate state: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate state")
	}

	// Store state in Redis with 10 minute expiry
	cacheKey := fmt.Sprintf("oauth_state:%s", state)
	ctx := context.Background()
	if err := h.cache.Set(ctx, cacheKey, clerkUserID, 10*time.Minute); err != nil {
		log.Printf("Failed to store state in cache: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to store state")
	}

	// Build OAuth authorization URL
	authURL := "https://accounts.google.com/o/oauth2/v2/auth?" + url.Values{
		"client_id":     {h.clientID},
		"redirect_uri":  {h.redirectURI},
		"response_type": {"code"},
		"scope":         {youtubeScopes},
		"access_type":   {"offline"}, // Get refresh token
		"state":         {state},
		"prompt":        {"consent"}, // Force consent to get refresh token
	}.Encode()

	log.Printf("YouTubeOAuth: Redirecting user %s to authorization URL", clerkUserID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"authorization_url": authURL,
	})
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// Callback handles the OAuth callback from YouTube
func (h *YouTubeOAuthHandler) Callback(c echo.Context) error {
	code := c.QueryParam("code")
	state := c.QueryParam("state")
	errorParam := c.QueryParam("error")

	// Get frontend URL for redirects
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	if errorParam != "" {
		log.Printf("YouTubeOAuth: Authorization error: %s", errorParam)
		redirectURL := fmt.Sprintf("%s/dashboard?youtube_auth=error&error=%s", frontendURL, url.QueryEscape(errorParam))
		return c.Redirect(http.StatusFound, redirectURL)
	}

	if code == "" {
		redirectURL := fmt.Sprintf("%s/dashboard?youtube_auth=error&error=missing_code", frontendURL)
		return c.Redirect(http.StatusFound, redirectURL)
	}

	// Verify state and get user ID from Redis
	ctx := context.Background()
	cacheKey := fmt.Sprintf("oauth_state:%s", state)
	clerkUserID, err := h.cache.Get(ctx, cacheKey)
	if err != nil {
		log.Printf("YouTubeOAuth: Invalid or expired state: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid or expired state")
	}

	// Delete state from cache (one-time use)
	h.cache.Delete(ctx, cacheKey)

	log.Printf("YouTubeOAuth: Processing callback for user: %s", clerkUserID)

	// Exchange authorization code for tokens
	tokenResp, err := h.exchangeCodeForToken(ctx, code)
	if err != nil {
		log.Printf("YouTubeOAuth: Failed to exchange code: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to exchange authorization code")
	}

	if tokenResp.RefreshToken == "" {
		log.Printf("YouTubeOAuth: Warning - no refresh token received")
	}

	// Fetch Google user info
	userInfo, err := h.fetchGoogleUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		log.Printf("YouTubeOAuth: Warning - failed to fetch user info: %v", err)
		// Continue anyway, user info is optional - set to empty struct
		userInfo = &googleUserInfo{}
	}

	// Get user from database using the clerk ID from state
	var user models.User
	err = h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		log.Printf("YouTubeOAuth: Failed to get user: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Save or update OAuth token in database
	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	oauthToken := &models.UserOAuthToken{
		ID:             uuid.New(),
		UserID:         user.ID,
		Provider:       "youtube",
		AccessToken:    tokenResp.AccessToken,
		RefreshToken:   tokenResp.RefreshToken,
		ExpiresAt:      expiresAt,
		Scope:          youtubeScopes,
		AccountEmail:   userInfo.Email,
		AccountName:    userInfo.Name,
		AccountPicture: userInfo.Picture,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Upsert token (insert or update if exists)
	_, err = h.db.NewInsert().
		Model(oauthToken).
		On("CONFLICT (user_id, provider) DO UPDATE").
		Set("access_token = EXCLUDED.access_token").
		Set("refresh_token = EXCLUDED.refresh_token").
		Set("expires_at = EXCLUDED.expires_at").
		Set("scope = EXCLUDED.scope").
		Set("account_email = EXCLUDED.account_email").
		Set("account_name = EXCLUDED.account_name").
		Set("account_picture = EXCLUDED.account_picture").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)

	if err != nil {
		log.Printf("YouTubeOAuth: Failed to save token: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to save authorization")
	}

	log.Printf("YouTubeOAuth: Successfully saved OAuth token for user %s", user.ID)

	// Redirect to frontend with success indicator
	redirectURL := fmt.Sprintf("%s/dashboard?youtube_auth=success", frontendURL)
	return c.Redirect(http.StatusFound, redirectURL)
}

// exchangeCodeForToken exchanges authorization code for access and refresh tokens
func (h *YouTubeOAuthHandler) exchangeCodeForToken(ctx context.Context, code string) (*tokenResponse, error) {
	data := url.Values{
		"code":          {code},
		"client_id":     {h.clientID},
		"client_secret": {h.clientSecret},
		"redirect_uri":  {h.redirectURI},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token", nil)
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = data.Encode()
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange failed (status %d): %s", resp.StatusCode, body)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

// GetYouTubeAuthStatus checks if user has authorized YouTube and returns account details
func (h *YouTubeOAuthHandler) GetYouTubeAuthStatus(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx := context.Background()

	// Get user from database
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Check if OAuth token exists
	var token models.UserOAuthToken
	err = h.db.NewSelect().
		Model(&token).
		Where("user_id = ? AND provider = ?", user.ID, "youtube").
		Scan(ctx)

	if err != nil {
		// No token found
		return c.JSON(http.StatusOK, map[string]interface{}{
			"authorized":       false,
			"connected_at":     nil,
			"expires_at":       nil,
			"is_expired":       false,
			"needs_reconnect":  false,
			"has_refresh_token": false,
		})
	}

	// Check if token is expired or expiring soon (within 5 minutes)
	isExpired := token.IsExpired()
	expiresWithin5Min := time.Until(token.ExpiresAt) < 5*time.Minute
	needsReconnect := isExpired || expiresWithin5Min || token.RefreshToken == ""

	return c.JSON(http.StatusOK, map[string]interface{}{
		"authorized":         true,
		"provider":           "youtube",
		"connected_at":       token.CreatedAt,
		"last_updated":       token.UpdatedAt,
		"expires_at":         token.ExpiresAt,
		"is_expired":         isExpired,
		"expires_soon":       expiresWithin5Min,
		"needs_reconnect":    needsReconnect,
		"has_refresh_token":  token.RefreshToken != "",
		"scope":              token.Scope,
		"time_until_expiry":  time.Until(token.ExpiresAt).String(),
		"account_email":      token.AccountEmail,
		"account_name":       token.AccountName,
		"account_picture":    token.AccountPicture,
	})
}

// DisconnectYouTube removes the YouTube OAuth connection for the user
func (h *YouTubeOAuthHandler) DisconnectYouTube(c echo.Context) error {
	clerkUserID, ok := auth.GetClerkUserID(c)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not authenticated")
	}

	ctx := context.Background()

	// Get user from database
	var user models.User
	err := h.db.NewSelect().
		Model(&user).
		Where("clerk_id = ?", clerkUserID).
		Scan(ctx)

	if err != nil {
		log.Printf("DisconnectYouTube: Failed to get user: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	// Delete the OAuth token
	result, err := h.db.NewDelete().
		Model((*models.UserOAuthToken)(nil)).
		Where("user_id = ? AND provider = ?", user.ID, "youtube").
		Exec(ctx)

	if err != nil {
		log.Printf("DisconnectYouTube: Failed to delete token: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to disconnect YouTube")
	}

	rowsAffected, _ := result.RowsAffected()

	if rowsAffected == 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success": true,
			"message": "No YouTube connection found",
		})
	}

	log.Printf("DisconnectYouTube: Successfully disconnected YouTube for user %s", user.ID)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "YouTube account disconnected successfully",
	})
}

// ReconnectYouTube is an alias for Authorize - it's the same flow
func (h *YouTubeOAuthHandler) ReconnectYouTube(c echo.Context) error {
	return h.Authorize(c)
}

// googleUserInfo represents the user info from Google's userinfo endpoint
type googleUserInfo struct {
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// fetchGoogleUserInfo fetches user information from Google's userinfo endpoint
func (h *YouTubeOAuthHandler) fetchGoogleUserInfo(ctx context.Context, accessToken string) (*googleUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch user info (status %d): %s", resp.StatusCode, body)
	}

	var userInfo googleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}
