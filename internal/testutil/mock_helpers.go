package testutil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// MockHTTPServer provides a mock HTTP server for testing external APIs
type MockHTTPServer struct {
	Server   *httptest.Server
	T        *testing.T
	Handlers map[string]http.HandlerFunc
}

// NewMockHTTPServer creates a new mock HTTP server
func NewMockHTTPServer(t *testing.T) *MockHTTPServer {
	mock := &MockHTTPServer{
		T:        t,
		Handlers: make(map[string]http.HandlerFunc),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if handler, ok := mock.Handlers[path]; ok {
			handler(w, r)
		} else {
			http.NotFound(w, r)
		}
	})

	mock.Server = httptest.NewServer(mux)
	return mock
}

// Close shuts down the mock server
func (m *MockHTTPServer) Close() {
	m.Server.Close()
}

// RegisterHandler registers a handler for a specific path
func (m *MockHTTPServer) RegisterHandler(path string, handler http.HandlerFunc) {
	m.Handlers[path] = handler
}

// SpotifyMockServer provides mock Spotify API responses
type SpotifyMockServer struct {
	*MockHTTPServer
}

// NewSpotifyMockServer creates a mock Spotify API server
func NewSpotifyMockServer(t *testing.T) *SpotifyMockServer {
	mock := NewMockHTTPServer(t)
	spotifyMock := &SpotifyMockServer{MockHTTPServer: mock}

	// Mock token endpoint
	mock.RegisterHandler("/api/token", spotifyMock.TokenHandler)

	return spotifyMock
}

// TokenHandler handles Spotify token requests
func (s *SpotifyMockServer) TokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string]interface{}{
		"access_token": "mock_spotify_access_token",
		"token_type":   "Bearer",
		"expires_in":   3600,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// MockPlaylistResponse mocks a Spotify playlist response
func (s *SpotifyMockServer) MockPlaylistResponse(playlistID string, name string, trackCount int) {
	path := fmt.Sprintf("/v1/playlists/%s", playlistID)
	s.RegisterHandler(path, func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"id":   playlistID,
			"name": name,
			"tracks": map[string]interface{}{
				"total": trackCount,
			},
			"images": []map[string]interface{}{
				{
					"url":    "https://example.com/cover.jpg",
					"height": 640,
					"width":  640,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
}

// MockPlaylistTracksResponse mocks Spotify playlist tracks response
func (s *SpotifyMockServer) MockPlaylistTracksResponse(playlistID string, tracks []SpotifyTrack) {
	path := fmt.Sprintf("/v1/playlists/%s/tracks", playlistID)
	s.RegisterHandler(path, func(w http.ResponseWriter, r *http.Request) {
		items := make([]map[string]interface{}, len(tracks))
		for i, track := range tracks {
			items[i] = map[string]interface{}{
				"track": map[string]interface{}{
					"id":   track.ID,
					"name": track.Name,
					"artists": []map[string]interface{}{
						{"name": track.Artist},
					},
					"external_ids": map[string]interface{}{
						"isrc": track.ISRC,
					},
				},
			}
		}

		response := map[string]interface{}{
			"items": items,
			"next":  nil,
			"total": len(tracks),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
}

// SpotifyTrack represents a mock Spotify track
type SpotifyTrack struct {
	ID     string
	Name   string
	Artist string
	ISRC   string
}

// YouTubeMockServer provides mock YouTube API responses
type YouTubeMockServer struct {
	*MockHTTPServer
}

// NewYouTubeMockServer creates a mock YouTube API server
func NewYouTubeMockServer(t *testing.T) *YouTubeMockServer {
	mock := NewMockHTTPServer(t)
	return &YouTubeMockServer{MockHTTPServer: mock}
}

// MockCreatePlaylist mocks YouTube playlist creation
func (y *YouTubeMockServer) MockCreatePlaylist(expectedName string, playlistID string) {
	y.RegisterHandler("/youtube/v3/playlists", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := map[string]interface{}{
			"id": playlistID,
			"snippet": map[string]interface{}{
				"title": expectedName,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})
}

// MockSearchVideo mocks YouTube video search
func (y *YouTubeMockServer) MockSearchVideo(query string, videoID string) {
	y.RegisterHandler("/youtube/v3/search", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"id": map[string]interface{}{
						"videoId": videoID,
					},
					"snippet": map[string]interface{}{
						"title":       query,
						"channelTitle": "Mock Channel",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
}

// MockAddVideoToPlaylist mocks adding a video to a playlist
func (y *YouTubeMockServer) MockAddVideoToPlaylist() {
	y.RegisterHandler("/youtube/v3/playlistItems", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := map[string]interface{}{
			"id": "playlistItem_mock_id",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})
}

// MockQuotaExceeded mocks YouTube quota exceeded error
func (y *YouTubeMockServer) MockQuotaExceeded() {
	y.RegisterHandler("/youtube/v3/search", func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"code":    403,
				"message": "The request cannot be completed because you have exceeded your quota.",
				"errors": []map[string]interface{}{
					{
						"domain": "youtube.quota",
						"reason": "quotaExceeded",
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(response)
	})
}

// WebSocketMock provides a mock WebSocket connection for testing
type WebSocketMock struct {
	Messages []interface{}
	T        *testing.T
}

// NewWebSocketMock creates a new WebSocket mock
func NewWebSocketMock(t *testing.T) *WebSocketMock {
	return &WebSocketMock{
		Messages: make([]interface{}, 0),
		T:        t,
	}
}

// BroadcastToUser simulates broadcasting a message to a user
func (w *WebSocketMock) BroadcastToUser(userID string, message interface{}) {
	w.Messages = append(w.Messages, message)
}

// GetLastMessage returns the last message sent
func (w *WebSocketMock) GetLastMessage() interface{} {
	if len(w.Messages) == 0 {
		return nil
	}
	return w.Messages[len(w.Messages)-1]
}

// GetMessageCount returns the number of messages sent
func (w *WebSocketMock) GetMessageCount() int {
	return len(w.Messages)
}

// ClearMessages clears all messages
func (w *WebSocketMock) ClearMessages() {
	w.Messages = make([]interface{}, 0)
}
