package spotify

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	clientID     string
	clientSecret string
	accessToken  string
	tokenExpiry  time.Time
	mu           sync.RWMutex
	httpClient   *http.Client
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type Track struct {
	Name    string
	Artists []string
}

type trackResponse struct {
	Name    string `json:"name"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
}

func NewClient(clientID, clientSecret string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) authenticate(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Now().Before(c.tokenExpiry) {
		return nil
	}

	auth := base64.StdEncoding.EncodeToString([]byte(c.clientID + ":" + c.clientSecret))

	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, "POST", "https://accounts.spotify.com/api/token", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Basic "+auth)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("spotify auth failed: %s", body)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}

	c.accessToken = tokenResp.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return nil
}

func (c *Client) GetTrack(ctx context.Context, trackID string) (*Track, error) {
	if err := c.authenticate(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.spotify.com/v1/tracks/"+trackID, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("spotify api failed: %s", body)
	}

	var trackResp trackResponse
	if err := json.NewDecoder(resp.Body).Decode(&trackResp); err != nil {
		return nil, err
	}

	artists := make([]string, len(trackResp.Artists))
	for i, artist := range trackResp.Artists {
		artists[i] = artist.Name
	}

	return &Track{
		Name:    trackResp.Name,
		Artists: artists,
	}, nil
}
