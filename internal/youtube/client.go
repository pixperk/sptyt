package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pixperk/sptyt/pkg/httputil"
)

// YouTubeAPIError represents a structured YouTube API error
type YouTubeAPIError struct {
	StatusCode int
	Message    string
	Reason     string // e.g., "quotaExceeded", "forbidden", etc.
	IsQuota    bool
}

func (e *YouTubeAPIError) Error() string {
	return e.Message
}

// parseYouTubeError parses YouTube API error response and returns a user-friendly error
func parseYouTubeError(statusCode int, body []byte) error {
	var apiErr struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Errors  []struct {
				Reason  string `json:"reason"`
				Domain  string `json:"domain"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &apiErr); err != nil {
		// Can't parse, return generic error
		return fmt.Errorf("YouTube API error (status %d): %s", statusCode, string(body))
	}

	// Check for quota exceeded
	for _, e := range apiErr.Error.Errors {
		if e.Reason == "quotaExceeded" || strings.Contains(e.Domain, "quota") {
			return &YouTubeAPIError{
				StatusCode: statusCode,
				Message:    "YouTube API quota exceeded for today. Please try again tomorrow or connect a different YouTube account.",
				Reason:     "quotaExceeded",
				IsQuota:    true,
			}
		}
		if e.Reason == "forbidden" || e.Reason == "accessNotConfigured" {
			return &YouTubeAPIError{
				StatusCode: statusCode,
				Message:    "Access denied. Please reconnect your YouTube account.",
				Reason:     e.Reason,
				IsQuota:    false,
			}
		}
	}

	// Return the original message if no special handling
	return &YouTubeAPIError{
		StatusCode: statusCode,
		Message:    apiErr.Error.Message,
		Reason:     "",
		IsQuota:    false,
	}
}

// IsQuotaError checks if an error is a YouTube quota exceeded error
func IsQuotaError(err error) bool {
	if ytErr, ok := err.(*YouTubeAPIError); ok {
		return ytErr.IsQuota
	}
	return strings.Contains(err.Error(), "quotaExceeded") || strings.Contains(err.Error(), "quota")
}

type Client struct {
	apiKey       string
	httpClient   *http.Client
	quotaTracker *QuotaTracker
}

type searchResponse struct {
	Items []struct {
		ID struct {
			VideoID string `json:"videoId"`
		} `json:"id"`
	} `json:"items"`
}

type videoDetailsResponse struct {
	Items []struct {
		RecordingDetails struct {
			RecordingDate string `json:"recordingDate"`
		} `json:"recordingDetails"`
		Snippet struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"snippet"`
		TopicDetails struct {
			TopicCategories []string `json:"topicCategories"`
		} `json:"topicDetails"`
	} `json:"items"`
}

type videoDetailsWithISRC struct {
	Items []struct {
		Snippet struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"snippet"`
		ContentDetails struct {
			ContentRating struct {
				ISRC string `json:"isrc"`
			} `json:"contentRating"`
		} `json:"contentDetails"`
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
		quotaTracker: nil, // Will be set via SetQuotaTracker if needed
	}
}

// SetQuotaTracker sets the quota tracker for this client
func (c *Client) SetQuotaTracker(tracker *QuotaTracker) {
	c.quotaTracker = tracker
}

func (c *Client) SearchOfficialMV(ctx context.Context, trackName string, artist string) (string, error) {
	// Check and consume quota before making API call
	if c.quotaTracker != nil {
		if err := c.quotaTracker.ConsumeQuota(ctx, QuotaCostSearch); err != nil {
			return "", err
		}
	}

	query := fmt.Sprintf("%s %s official music video", trackName, artist)

	params := url.Values{}
	params.Set("part", "id")
	params.Set("q", query)
	params.Set("type", "video")
	params.Set("videoCategoryId", "10")
	params.Set("maxResults", "1")
	params.Set("key", c.apiKey)

	apiURL := "https://www.googleapis.com/youtube/v3/search?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := httputil.DoWithRetry(ctx, c.httpClient, req, 2)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", parseYouTubeError(resp.StatusCode, body)
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

func (c *Client) SearchLyricVideo(ctx context.Context, trackName string, artist string) (string, error) {
	// Check and consume quota before making API call
	if c.quotaTracker != nil {
		if err := c.quotaTracker.ConsumeQuota(ctx, QuotaCostSearch); err != nil {
			return "", err
		}
	}

	query := fmt.Sprintf("%s %s lyrics", trackName, artist)

	params := url.Values{}
	params.Set("part", "id")
	params.Set("q", query)
	params.Set("type", "video")
	params.Set("videoCategoryId", "10")
	params.Set("maxResults", "1")
	params.Set("key", c.apiKey)

	apiURL := "https://www.googleapis.com/youtube/v3/search?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := httputil.DoWithRetry(ctx, c.httpClient, req, 2)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", parseYouTubeError(resp.StatusCode, body)
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

// SearchOfficialMVWithToken searches using user's OAuth token (uses user's quota)
func (c *Client) SearchOfficialMVWithToken(ctx context.Context, accessToken, trackName, artist string) (string, error) {
	query := fmt.Sprintf("%s %s official music video", trackName, artist)
	return c.searchWithToken(ctx, accessToken, query)
}

// SearchLyricVideoWithToken searches using user's OAuth token (uses user's quota)
func (c *Client) SearchLyricVideoWithToken(ctx context.Context, accessToken, trackName, artist string) (string, error) {
	query := fmt.Sprintf("%s %s lyrics", trackName, artist)
	return c.searchWithToken(ctx, accessToken, query)
}

// searchWithToken performs search using user's OAuth token instead of API key
func (c *Client) searchWithToken(ctx context.Context, accessToken, query string) (string, error) {
	params := url.Values{}
	params.Set("part", "id")
	params.Set("q", query)
	params.Set("type", "video")
	params.Set("videoCategoryId", "10")
	params.Set("maxResults", "1")

	apiURL := "https://www.googleapis.com/youtube/v3/search?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httputil.DoWithRetry(ctx, c.httpClient, req, 2)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", parseYouTubeError(resp.StatusCode, body)
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

func (c *Client) GetVideoMetadata(ctx context.Context, videoID string) (string, string, string, error) {
	params := url.Values{}
	params.Set("part", "snippet,contentDetails")
	params.Set("id", videoID)
	params.Set("key", c.apiKey)

	apiURL := "https://www.googleapis.com/youtube/v3/videos?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", "", "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", "", parseYouTubeError(resp.StatusCode, body)
	}

	var videoResp videoDetailsWithISRC
	if err := json.NewDecoder(resp.Body).Decode(&videoResp); err != nil {
		return "", "", "", err
	}

	if len(videoResp.Items) == 0 {
		return "", "", "", fmt.Errorf("video not found")
	}

	title := videoResp.Items[0].Snippet.Title
	description := videoResp.Items[0].Snippet.Description
	isrc := videoResp.Items[0].ContentDetails.ContentRating.ISRC

	return title, description, isrc, nil
}
