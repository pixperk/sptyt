package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type playlistInsertRequest struct {
	Snippet struct {
		Title       string `json:"title"`
		Description string `json:"description,omitempty"`
	} `json:"snippet"`
	Status struct {
		PrivacyStatus string `json:"privacyStatus"`
	} `json:"status"`
}

type playlistInsertResponse struct {
	ID      string `json:"id"`
	Snippet struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	} `json:"snippet"`
}

type playlistItemInsertRequest struct {
	Snippet struct {
		PlaylistID string `json:"playlistId"`
		ResourceID struct {
			Kind    string `json:"kind"`
			VideoID string `json:"videoId"`
		} `json:"resourceId"`
	} `json:"snippet"`
}

type playlistItemInsertResponse struct {
	ID string `json:"id"`
}

// CreatePlaylist creates a new YouTube playlist for the authenticated user
func (c *Client) CreatePlaylist(ctx context.Context, accessToken, title, description string) (string, error) {
	reqBody := playlistInsertRequest{}
	reqBody.Snippet.Title = title
	reqBody.Snippet.Description = description
	reqBody.Status.PrivacyStatus = "public" // Can be "public", "private", or "unlisted"

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		"https://www.googleapis.com/youtube/v3/playlists?part=snippet,status",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", parseYouTubeError(resp.StatusCode, body)
	}

	var playlistResp playlistInsertResponse
	if err := json.NewDecoder(resp.Body).Decode(&playlistResp); err != nil {
		return "", err
	}

	return playlistResp.ID, nil
}

// AddVideoToPlaylist adds a video to an existing YouTube playlist
func (c *Client) AddVideoToPlaylist(ctx context.Context, accessToken, playlistID, videoID string) error {
	reqBody := playlistItemInsertRequest{}
	reqBody.Snippet.PlaylistID = playlistID
	reqBody.Snippet.ResourceID.Kind = "youtube#video"
	reqBody.Snippet.ResourceID.VideoID = videoID

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		"https://www.googleapis.com/youtube/v3/playlistItems?part=snippet",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return parseYouTubeError(resp.StatusCode, body)
	}

	return nil
}

// AddVideosToPlaylistBatch adds multiple videos to a playlist with rate limiting
// Returns a map of videoID -> error for failed additions
func (c *Client) AddVideosToPlaylistBatch(ctx context.Context, accessToken, playlistID string, videoIDs []string) map[string]error {
	errors := make(map[string]error)

	// Rate limit: max 1 request per 100ms to avoid quota issues
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for _, videoID := range videoIDs {
		select {
		case <-ctx.Done():
			errors[videoID] = ctx.Err()
			continue
		case <-ticker.C:
			if err := c.AddVideoToPlaylist(ctx, accessToken, playlistID, videoID); err != nil {
				errors[videoID] = err
			}
		}
	}

	return errors
}
