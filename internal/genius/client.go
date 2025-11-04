package genius

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
	accessToken string
	httpClient  *http.Client
}

type searchResponse struct {
	Response struct {
		Hits []struct {
			Result struct {
				URL string `json:"url"`
			} `json:"result"`
		} `json:"hits"`
	} `json:"response"`
}

func NewClient(accessToken string) *Client {
	return &Client{
		accessToken: accessToken,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) SearchLyrics(ctx context.Context, trackName string, artist string) (string, error) {
	query := fmt.Sprintf("%s %s", trackName, artist)

	params := url.Values{}
	params.Set("q", query)

	apiURL := "https://api.genius.com/search?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("genius api failed: %s", body)
	}

	var searchResp searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return "", err
	}

	if len(searchResp.Response.Hits) == 0 {
		return "", fmt.Errorf("no lyrics found on genius")
	}

	return searchResp.Response.Hits[0].Result.URL, nil
}
