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
	ISRC    string
}

type trackResponse struct {
	Name        string `json:"name"`
	Artists     []struct {
		Name string `json:"name"`
	} `json:"artists"`
	ExternalIds struct {
		ISRC string `json:"isrc"`
	} `json:"external_ids"`
}

func NewClient(clientID, clientSecret string) *Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	}

	client := &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}

	go client.refreshTokenLoop()

	return client
}

func (c *Client) refreshTokenLoop() {
	ctx := context.Background()
	c.authenticate(ctx)

	ticker := time.NewTicker(55 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.authenticate(ctx)
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
		ISRC:    trackResp.ExternalIds.ISRC,
	}, nil
}

type searchResponse struct {
	Tracks struct {
		Items []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Artists     []struct {
				Name string `json:"name"`
			} `json:"artists"`
			ExternalIds struct {
				ISRC string `json:"isrc"`
			} `json:"external_ids"`
			ExternalUrls struct {
				Spotify string `json:"spotify"`
			} `json:"external_urls"`
		} `json:"items"`
	} `json:"tracks"`
}

func (c *Client) SearchByISRC(ctx context.Context, isrc string) (string, error) {
	if err := c.authenticate(ctx); err != nil {
		return "", err
	}

	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	query := url.Values{}
	query.Set("q", "isrc:"+isrc)
	query.Set("type", "track")
	query.Set("limit", "1")

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.spotify.com/v1/search?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("spotify search failed: %s", body)
	}

	var searchResp searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return "", err
	}

	if len(searchResp.Tracks.Items) == 0 {
		return "", fmt.Errorf("no track found for ISRC: %s", isrc)
	}

	return searchResp.Tracks.Items[0].ExternalUrls.Spotify, nil
}

func (c *Client) SearchByTitle(ctx context.Context, query string) (string, error) {
	if err := c.authenticate(ctx); err != nil {
		return "", err
	}

	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	params := url.Values{}
	params.Set("q", query)
	params.Set("type", "track")
	params.Set("limit", "1")

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.spotify.com/v1/search?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("spotify search failed: %s", body)
	}

	var searchResp searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return "", err
	}

	if len(searchResp.Tracks.Items) == 0 {
		return "", fmt.Errorf("no track found for query: %s", query)
	}

	return searchResp.Tracks.Items[0].ExternalUrls.Spotify, nil
}

func (c *Client) GetTrackByISRC(ctx context.Context, isrc string) (*Track, error) {
	if err := c.authenticate(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	query := url.Values{}
	query.Set("q", "isrc:"+isrc)
	query.Set("type", "track")
	query.Set("limit", "1")

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.spotify.com/v1/search?"+query.Encode(), nil)
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
		return nil, fmt.Errorf("spotify search failed: %s", body)
	}

	var searchResp searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	if len(searchResp.Tracks.Items) == 0 {
		return nil, fmt.Errorf("no track found for ISRC: %s", isrc)
	}

	item := searchResp.Tracks.Items[0]
	artists := make([]string, len(item.Artists))
	for i, artist := range item.Artists {
		artists[i] = artist.Name
	}

	return &Track{
		Name:    item.Name,
		Artists: artists,
		ISRC:    item.ExternalIds.ISRC,
	}, nil
}
