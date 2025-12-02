package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/pixperk/sptyt/internal/config"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
}

type CachedTrack struct {
	Name    string   `json:"name"`
	Artists []string `json:"artists"`
}

type CachedRedirect struct {
	URL string `json:"url"`
}

// NewRedisCache creates a new Redis cache from config
func NewRedisCache(cfg *config.RedisConfig) (*RedisCache, error) {
	opts := cfg.NewRedisClient()
	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &RedisCache{client: client}, nil
}

func (r *RedisCache) GetTrack(ctx context.Context, trackID string) (*CachedTrack, error) {
	key := "track:" + trackID
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var track CachedTrack
	if err := json.Unmarshal([]byte(val), &track); err != nil {
		return nil, err
	}

	return &track, nil
}

func (r *RedisCache) SetTrack(ctx context.Context, trackID string, track *CachedTrack, ttl time.Duration) error {
	key := "track:" + trackID
	data, err := json.Marshal(track)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, data, ttl).Err()
}

func (r *RedisCache) GetYouTubeMVURL(ctx context.Context, trackID string) (string, error) {
	key := "youtube:mv:" + trackID
	return r.client.Get(ctx, key).Result()
}

func (r *RedisCache) SetYouTubeMVURL(ctx context.Context, trackID string, url string, ttl time.Duration) error {
	key := "youtube:mv:" + trackID
	return r.client.Set(ctx, key, url, ttl).Err()
}

func (r *RedisCache) GetYouTubeLyricsURL(ctx context.Context, trackID string) (string, error) {
	key := "youtube:lyrics:" + trackID
	return r.client.Get(ctx, key).Result()
}

func (r *RedisCache) SetYouTubeLyricsURL(ctx context.Context, trackID string, url string, ttl time.Duration) error {
	key := "youtube:lyrics:" + trackID
	return r.client.Set(ctx, key, url, ttl).Err()
}

func (r *RedisCache) GetGeniusURL(ctx context.Context, trackID string) (string, error) {
	key := "genius:" + trackID
	return r.client.Get(ctx, key).Result()
}

func (r *RedisCache) SetGeniusURL(ctx context.Context, trackID string, url string, ttl time.Duration) error {
	key := "genius:" + trackID
	return r.client.Set(ctx, key, url, ttl).Err()
}

func (r *RedisCache) GetClient() *redis.Client {
	return r.client
}

func (r *RedisCache) Close() error {
	return r.client.Close()
}

// Generic cache methods for string values
func (r *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisCache) Get(ctx context.Context, key string) (string, error) {
	return r.client.Get(ctx, key).Result()
}

func (r *RedisCache) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

// YouTubeSearchResult represents a cached YouTube search result
type YouTubeSearchResult struct {
	VideoID     string `json:"video_id"`
	VideoURL    string `json:"video_url"`
	MatchMethod string `json:"match_method"` // official_mv or lyric_video
}

// normalizeSearchKey creates a consistent cache key from track name and artists
// This allows cache hits across different Spotify track IDs for the same song
func normalizeSearchKey(trackName string, artists string) string {
	// Lowercase and trim spaces for consistency
	key := strings.ToLower(strings.TrimSpace(trackName) + ":" + strings.TrimSpace(artists))
	// Remove common variations that don't change the song
	key = strings.ReplaceAll(key, " - ", " ")
	key = strings.ReplaceAll(key, "  ", " ")
	return key
}

// GetYouTubeSearchResult gets a cached YouTube search result by track name and artists
func (r *RedisCache) GetYouTubeSearchResult(ctx context.Context, trackName, artists string, useLyricVideos bool) (*YouTubeSearchResult, error) {
	searchType := "mv"
	if useLyricVideos {
		searchType = "lyric"
	}
	key := "yt:search:" + searchType + ":" + normalizeSearchKey(trackName, artists)

	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var result YouTubeSearchResult
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// SetYouTubeSearchResult caches a YouTube search result
func (r *RedisCache) SetYouTubeSearchResult(ctx context.Context, trackName, artists string, useLyricVideos bool, result *YouTubeSearchResult, ttl time.Duration) error {
	searchType := "mv"
	if useLyricVideos {
		searchType = "lyric"
	}
	key := "yt:search:" + searchType + ":" + normalizeSearchKey(trackName, artists)

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, data, ttl).Err()
}

// CacheYouTubeNotFound caches that no YouTube video was found for a track
// This prevents repeated searches for tracks that don't have YouTube matches
func (r *RedisCache) CacheYouTubeNotFound(ctx context.Context, trackName, artists string, useLyricVideos bool, ttl time.Duration) error {
	searchType := "mv"
	if useLyricVideos {
		searchType = "lyric"
	}
	key := "yt:search:" + searchType + ":" + normalizeSearchKey(trackName, artists)

	// Store empty result to indicate "not found"
	return r.client.Set(ctx, key, "{\"video_id\":\"\"}", ttl).Err()
}

// SpotifySearchResult represents cached Spotify search results
type SpotifySearchResult struct {
	Query   string                 `json:"query"`
	Results []SpotifySearchTrack   `json:"results"`
}

type SpotifySearchTrack struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Artists    []string `json:"artists"`
	Album      string   `json:"album"`
	CoverImage string   `json:"cover_image"`
	Duration   int      `json:"duration_ms"`
	SpotifyURL string   `json:"spotify_url"`
}

// normalizeSpotifySearchKey creates a consistent cache key for Spotify searches
func normalizeSpotifySearchKey(query string, limit int) string {
	normalized := strings.ToLower(strings.TrimSpace(query))
	normalized = strings.ReplaceAll(normalized, "  ", " ")
	return fmt.Sprintf("spotify:search:%d:%s", limit, normalized)
}

// GetSpotifySearchResults gets cached Spotify search results
func (r *RedisCache) GetSpotifySearchResults(ctx context.Context, query string, limit int) ([]SpotifySearchTrack, error) {
	key := normalizeSpotifySearchKey(query, limit)

	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var result SpotifySearchResult
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// SetSpotifySearchResults caches Spotify search results
func (r *RedisCache) SetSpotifySearchResults(ctx context.Context, query string, limit int, results []SpotifySearchTrack, ttl time.Duration) error {
	key := normalizeSpotifySearchKey(query, limit)

	data, err := json.Marshal(SpotifySearchResult{
		Query:   query,
		Results: results,
	})
	if err != nil {
		return err
	}

	return r.client.Set(ctx, key, data, ttl).Err()
}
