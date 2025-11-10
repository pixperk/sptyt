package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type playlistResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Tracks      struct {
		Total int `json:"total"`
	} `json:"tracks"`
	Owner struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	Images []PlaylistImage `json:"images"`
}

type playlistTracksResponse struct {
	Items []struct {
		Track struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
			ExternalIds struct {
				ISRC string `json:"isrc"`
			} `json:"external_ids"`
		} `json:"track"`
	} `json:"items"`
	Next   *string `json:"next"`
	Offset int     `json:"offset"`
	Limit  int     `json:"limit"`
	Total  int     `json:"total"`
}

// GetPlaylist fetches playlist metadata from Spotify (works with public playlists)
func (c *Client) GetPlaylist(ctx context.Context, playlistID string) (*Playlist, error) {
	if err := c.authenticate(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.spotify.com/v1/playlists/"+playlistID, nil)
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
		return nil, fmt.Errorf("spotify playlist api failed (status %d): %s", resp.StatusCode, body)
	}

	var playlistResp playlistResponse
	if err := json.NewDecoder(resp.Body).Decode(&playlistResp); err != nil {
		return nil, err
	}

	return &Playlist{
		ID:          playlistResp.ID,
		Name:        playlistResp.Name,
		Description: playlistResp.Description,
		TrackCount:  playlistResp.Tracks.Total,
		Owner:       playlistResp.Owner.DisplayName,
		Images:      playlistResp.Images,
	}, nil
}

// GetPlaylistTracks fetches all tracks from a Spotify playlist with pagination
// Works with public playlists using client credentials
func (c *Client) GetPlaylistTracks(ctx context.Context, playlistID string) ([]*PlaylistTrack, error) {
	if err := c.authenticate(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	var allTracks []*PlaylistTrack
	offset := 0
	limit := 100 // Spotify max limit per request

	for {
		query := url.Values{}
		query.Set("offset", fmt.Sprintf("%d", offset))
		query.Set("limit", fmt.Sprintf("%d", limit))
		query.Set("fields", "items(track(id,name,artists(name),external_ids(isrc))),next,offset,limit,total")

		apiURL := fmt.Sprintf("https://api.spotify.com/v1/playlists/%s/tracks?%s", playlistID, query.Encode())
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("spotify playlist tracks api failed (status %d): %s", resp.StatusCode, body)
		}

		var tracksResp playlistTracksResponse
		if err := json.NewDecoder(resp.Body).Decode(&tracksResp); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		// Process tracks from this page
		for i, item := range tracksResp.Items {
			if item.Track.ID == "" {
				continue // Skip deleted/unavailable tracks
			}

			artists := make([]string, len(item.Track.Artists))
			for j, artist := range item.Track.Artists {
				artists[j] = artist.Name
			}

			allTracks = append(allTracks, &PlaylistTrack{
				ID:       item.Track.ID,
				Name:     item.Track.Name,
				Artists:  artists,
				ISRC:     item.Track.ExternalIds.ISRC,
				Position: offset + i,
			})
		}

		// Check if there are more pages
		if tracksResp.Next == nil {
			break
		}

		offset += limit
	}

	return allTracks, nil
}

// GetAlbum fetches album metadata from Spotify
func (c *Client) GetAlbum(ctx context.Context, albumID string) (*Playlist, error) {
	if err := c.authenticate(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.spotify.com/v1/albums/"+albumID, nil)
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
		return nil, fmt.Errorf("spotify album api failed (status %d): %s", resp.StatusCode, body)
	}

	var albumResp struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Tracks struct {
			Total int `json:"total"`
		} `json:"tracks"`
		Artists []struct {
			Name string `json:"name"`
		} `json:"artists"`
		Images []PlaylistImage `json:"images"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&albumResp); err != nil {
		return nil, err
	}

	artistNames := make([]string, len(albumResp.Artists))
	for i, artist := range albumResp.Artists {
		artistNames[i] = artist.Name
	}

	return &Playlist{
		ID:          albumResp.ID,
		Name:        albumResp.Name,
		Description: fmt.Sprintf("Album by %s", fmt.Sprintf("%v", artistNames)),
		TrackCount:  albumResp.Tracks.Total,
		Owner:       fmt.Sprintf("%v", artistNames),
		Images:      albumResp.Images,
	}, nil
}

// GetAlbumTracks fetches all tracks from a Spotify album
func (c *Client) GetAlbumTracks(ctx context.Context, albumID string) ([]*PlaylistTrack, error) {
	if err := c.authenticate(ctx); err != nil {
		return nil, err
	}

	c.mu.RLock()
	token := c.accessToken
	c.mu.RUnlock()

	var allTracks []*PlaylistTrack
	offset := 0
	limit := 50 // Spotify max limit for album tracks

	for {
		query := url.Values{}
		query.Set("offset", fmt.Sprintf("%d", offset))
		query.Set("limit", fmt.Sprintf("%d", limit))

		apiURL := fmt.Sprintf("https://api.spotify.com/v1/albums/%s/tracks?%s", albumID, query.Encode())
		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("spotify album tracks api failed (status %d): %s", resp.StatusCode, body)
		}

		var tracksResp struct {
			Items []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				ExternalIds struct {
					ISRC string `json:"isrc"`
				} `json:"external_ids"`
			} `json:"items"`
			Next   *string `json:"next"`
			Offset int     `json:"offset"`
			Limit  int     `json:"limit"`
			Total  int     `json:"total"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&tracksResp); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		// Process tracks from this page
		for i, item := range tracksResp.Items {
			if item.ID == "" {
				continue
			}

			artists := make([]string, len(item.Artists))
			for j, artist := range item.Artists {
				artists[j] = artist.Name
			}

			allTracks = append(allTracks, &PlaylistTrack{
				ID:       item.ID,
				Name:     item.Name,
				Artists:  artists,
				ISRC:     item.ExternalIds.ISRC,
				Position: offset + i,
			})
		}

		// Check if there are more pages
		if tracksResp.Next == nil {
			break
		}

		offset += limit
	}

	return allTracks, nil
}
