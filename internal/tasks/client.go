package tasks

import (
	"github.com/hibiken/asynq"
	"github.com/pixperk/sptyt/internal/config"
)

// Client wraps Asynq client for enqueuing tasks
type Client struct {
	client *asynq.Client
}

// NewClient creates a new task client from Redis config
func NewClient(cfg *config.RedisConfig) *Client {
	redisOpt := cfg.NewAsynqRedisOpt()
	client := asynq.NewClient(redisOpt)
	return &Client{
		client: client,
	}
}

// EnqueuePlaylistConversion enqueues a playlist conversion task
func (c *Client) EnqueuePlaylistConversion(payload PlaylistConversionPayload) error {
	task, err := NewPlaylistConversionTask(payload)
	if err != nil {
		return err
	}

	// Enqueue with default options (processed immediately)
	_, err = c.client.Enqueue(task)
	return err
}

// EnqueueAnalyticsUpdate enqueues an analytics update task (implements services.AnalyticsTaskClient)
func (c *Client) EnqueueAnalyticsUpdate(userID, spotifyType string, isSuccess bool, trackCount, successCount, failureCount int, countsAgainstQuota bool, youtubeSearches, playlistInserts int, googleAccountEmail string) error {
	payload := AnalyticsUpdatePayload{
		UserID:             userID,
		SpotifyType:        spotifyType,
		IsSuccess:          isSuccess,
		TrackCount:         trackCount,
		SuccessCount:       successCount,
		FailureCount:       failureCount,
		CountsAgainstQuota: countsAgainstQuota,
		YouTubeSearches:    youtubeSearches,
		PlaylistInserts:    playlistInserts,
		GoogleAccountEmail: googleAccountEmail,
	}

	task, err := NewAnalyticsUpdateTask(payload)
	if err != nil {
		return err
	}

	// Enqueue with default options (processed immediately)
	_, err = c.client.Enqueue(task)
	return err
}

// Close closes the client connection
func (c *Client) Close() error {
	return c.client.Close()
}
