package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
}

type searchResponse struct {
	Items []struct {
		ID struct {
			VideoID string `json:"videoId"`
		} `json:"id"`
	} `json:"items"`
}

func NewClient(apiKey string) *Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	}

	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: transport,
		},
	}
}

func (c *Client) SearchLyricVideo(ctx context.Context, trackName string, artist string) (string, error) {
	query := fmt.Sprintf("%s %s lyrics", trackName, artist)

	params := url.Values{}
	params.Set("part", "id")
	params.Set("q", query)
	params.Set("type", "video")
	params.Set("maxResults", "1")
	params.Set("key", c.apiKey)

	apiURL := "https://www.googleapis.com/youtube/v3/search?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("youtube api failed: %s", body)
	}

	var searchResp searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return "", err
	}

	if len(searchResp.Items) == 0 {
		return "", fmt.Errorf("no youtube video found")
	}

	videoID := searchResp.Items[0].ID.VideoID
	return "https://www.youtube.com/watch?v=" + videoID, nil
}
