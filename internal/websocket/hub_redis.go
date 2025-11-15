package websocket

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisHub wraps Hub with Redis pub/sub for multi-server support
type RedisHub struct {
	*Hub
	redisClient *redis.Client
	pubsub      *redis.PubSub
	stopChan    chan struct{}
}

const (
	redisChannelPrefix = "websocket:broadcast:"
)

// NewRedisHub creates a new hub with Redis pub/sub support
func NewRedisHub(redisClient *redis.Client) *RedisHub {
	hub := NewHub()

	if redisClient == nil {
		log.Println("WebSocket: Redis not available, running in single-server mode")
		return &RedisHub{
			Hub:         hub,
			redisClient: nil,
		}
	}

	rh := &RedisHub{
		Hub:         hub,
		redisClient: redisClient,
		stopChan:    make(chan struct{}),
	}

	// Subscribe to Redis pub/sub channel
	rh.pubsub = redisClient.Subscribe(context.Background(), redisChannelPrefix+"*")

	// Start Redis message listener
	go rh.listenRedis()

	log.Println("WebSocket: Redis pub/sub enabled for multi-server support")

	return rh
}

// BroadcastToUser publishes message to Redis so all servers receive it
func (rh *RedisHub) BroadcastToUser(userID string, event ProgressEvent) {
	messageBytes, err := json.Marshal(event)
	if err != nil {
		log.Printf("WebSocket: Failed to marshal event: %v", err)
		return
	}

	// If Redis is available, publish to Redis
	if rh.redisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		channel := redisChannelPrefix + userID
		err := rh.redisClient.Publish(ctx, channel, messageBytes).Err()
		if err != nil {
			log.Printf("WebSocket: Failed to publish to Redis (falling back to local): %v", err)
			// Fallback to local broadcast
			rh.Hub.BroadcastToUser(userID, event)
		} else {
			log.Printf("WebSocket: Published event type=%s to Redis channel=%s", event.Type, channel)
		}
	} else {
		// No Redis, use local hub
		rh.Hub.BroadcastToUser(userID, event)
	}
}

// listenRedis listens for messages from Redis and broadcasts to local clients
func (rh *RedisHub) listenRedis() {
	ch := rh.pubsub.Channel()

	for {
		select {
		case msg := <-ch:
			// Extract userID from channel name
			userID := msg.Channel[len(redisChannelPrefix):]

			log.Printf("WebSocket: Received Redis message for user %s", userID)

			// Broadcast to local clients for this user
			rh.Hub.broadcast <- &BroadcastMessage{
				UserID:  userID,
				Message: []byte(msg.Payload),
			}

		case <-rh.stopChan:
			log.Println("WebSocket: Stopping Redis listener")
			return
		}
	}
}

// Close cleans up Redis pub/sub resources
func (rh *RedisHub) Close() error {
	close(rh.stopChan)

	if rh.pubsub != nil {
		return rh.pubsub.Close()
	}

	return nil
}
