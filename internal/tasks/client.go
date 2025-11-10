package tasks

import (
	"crypto/tls"

	"github.com/hibiken/asynq"
)

// Client wraps Asynq client for enqueuing tasks
type Client struct {
	client *asynq.Client
}

// NewClient creates a new task client
func NewClient(redisAddr, redisPassword string) *Client {
	redisOpt := asynq.RedisClientOpt{
		Addr: redisAddr,
	}
	if redisPassword != "" {
		redisOpt.Username = "default" // Upstash requires username
		redisOpt.Password = redisPassword
		// Enable TLS for Upstash (or any production Redis with TLS)
		redisOpt.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

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

// Close closes the client connection
func (c *Client) Close() error {
	return c.client.Close()
}
