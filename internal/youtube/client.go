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
	}
}

func (c *Client) SearchOfficialMV(ctx context.Context, trackName string, artist string) (string, error) {
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

func (c *Client) SearchLyricVideo(ctx context.Context, trackName string, artist string) (string, error) {
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
		return "", "", "", fmt.Errorf("youtube api failed: %s", body)
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
